package nbt

import (
	"bytes"
	"encoding"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"
)

// An InvalidUnmarshalError describes an invalid argument passed to Unmarshal.
// The argument must be a non-nil pointer.
type InvalidUnmarshalError struct {
	Type reflect.Type
}

func (e *InvalidUnmarshalError) Error() string {
	if e.Type == nil {
		return "nbt: Unmarshal(nil)"
	}
	if e.Type.Kind() != reflect.Pointer {
		return "nbt: Unmarshal(non-pointer " + e.Type.String() + ")"
	}
	return "nbt: Unmarshal(nil " + e.Type.String() + ")"
}

// An UnmarshalTypeError describes an NBT tag that could not be stored in a Go
// value of a particular type.
type UnmarshalTypeError struct {
	Tag   TagType      // the NBT type that was found
	Type  reflect.Type // the Go type it could not be assigned to
	Path  string       // dotted path of the offending value, e.g. "blocks[3].pos"
	Value string       // optional detail, e.g. the overflowing number
}

func (e *UnmarshalTypeError) Error() string {
	var b strings.Builder
	b.WriteString("nbt: cannot unmarshal ")
	b.WriteString(e.Tag.String())
	if e.Value != "" {
		b.WriteString(" " + e.Value)
	}
	b.WriteString(" into Go value of type " + e.Type.String())
	if e.Path != "" {
		b.WriteString(" at " + e.Path)
	}
	return b.String()
}

// An UnknownFieldError is returned when [Decoder.DisallowUnknownFields] is set
// and a compound has a key with no matching struct field.
type UnknownFieldError struct {
	Name string
	Type reflect.Type
	Path string
}

func (e *UnknownFieldError) Error() string {
	s := fmt.Sprintf("nbt: unknown field %q in %s", e.Name, e.Type)
	if e.Path != "" {
		s += " at " + e.Path
	}
	return s
}

// Unmarshal parses binary NBT data and stores the result in the value pointed
// to by v. Gzip and zlib compressed data is detected and decompressed
// automatically. The root tag's name is ignored; use a [Decoder] to read it.
//
// v may be a pointer to a struct or map (for a compound root), to a [Tag] or
// empty interface (to get the raw tree), or to any other type compatible with
// the root tag. See the package documentation for the conversion rules.
func Unmarshal(data []byte, v any) error {
	err := NewDecoder(bytes.NewReader(data)).Decode(v)
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return err
}

// FromTag stores t in the value pointed to by v, using the same rules as
// [Unmarshal]. It is the inverse of [ToTag].
func FromTag(t Tag, v any) error {
	return (&decodeState{}).root(t, v)
}

type decodeState struct {
	disallowUnknown bool
	path            []string
}

func (d *decodeState) root(t Tag, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return &InvalidUnmarshalError{Type: reflect.TypeOf(v)}
	}
	return d.value(t, rv.Elem())
}

func (d *decodeState) pathString() string {
	return strings.TrimPrefix(strings.Join(d.path, ""), ".")
}

func (d *decodeState) typeError(t Tag, v reflect.Value, detail string) error {
	return &UnmarshalTypeError{Tag: t.Type(), Type: v.Type(), Path: d.pathString(), Value: detail}
}

// indirect walks down pointers, allocating as needed, and stops early at a
// type implementing Unmarshaler or TextUnmarshaler (returned as u).
func indirect(v reflect.Value) (u any, rv reflect.Value) {
	for {
		if v.Kind() != reflect.Pointer && v.CanAddr() {
			p := v.Addr()
			if p.Type().Implements(unmarshalerType) || p.Type().Implements(textUnmarshaler) {
				return p.Interface(), v
			}
		}
		if v.Kind() != reflect.Pointer {
			return nil, v
		}
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		if v.Type().NumMethod() > 0 && v.CanInterface() {
			if u, ok := v.Interface().(Unmarshaler); ok {
				return u, v
			}
			if u, ok := v.Interface().(encoding.TextUnmarshaler); ok {
				return u, v
			}
		}
		v = v.Elem()
	}
}

func (d *decodeState) value(t Tag, v reflect.Value) error {
	if t == nil {
		return nil
	}
	u, v := indirect(v)
	switch u := u.(type) {
	case Unmarshaler:
		return u.UnmarshalNBT(t)
	case encoding.TextUnmarshaler:
		s, ok := t.(String)
		if !ok {
			return d.typeError(t, v, "")
		}
		return u.UnmarshalText([]byte(s))
	}

	// Concrete tag types: assign directly when the types match exactly.
	if reflect.TypeOf(t) == v.Type() {
		v.Set(reflect.ValueOf(t))
		return nil
	}

	switch v.Kind() {
	case reflect.Interface:
		if v.NumMethod() == 0 || v.Type() == tagType {
			v.Set(reflect.ValueOf(t))
			return nil
		}
		return d.typeError(t, v, "")
	case reflect.Bool:
		n, ok := tagInt(t)
		if !ok {
			return d.typeError(t, v, "")
		}
		v.SetBool(n != 0)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, ok := tagInt(t)
		if !ok {
			return d.typeError(t, v, "")
		}
		if v.OverflowInt(n) {
			return d.typeError(t, v, fmt.Sprint(n))
		}
		v.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		n, ok := tagUint(t)
		if !ok {
			return d.typeError(t, v, "")
		}
		if v.OverflowUint(n) {
			return d.typeError(t, v, fmt.Sprint(n))
		}
		v.SetUint(n)
	case reflect.Float32, reflect.Float64:
		var f float64
		switch t := t.(type) {
		case Float:
			f = float64(t)
		case Double:
			f = float64(t)
		default:
			n, ok := tagInt(t)
			if !ok {
				return d.typeError(t, v, "")
			}
			f = float64(n)
		}
		if v.Kind() == reflect.Float32 && !math.IsInf(f, 0) && v.OverflowFloat(f) {
			return d.typeError(t, v, fmt.Sprint(f))
		}
		v.SetFloat(f)
	case reflect.String:
		s, ok := t.(String)
		if !ok {
			return d.typeError(t, v, "")
		}
		v.SetString(string(s))
	case reflect.Slice, reflect.Array:
		return d.sequence(t, v)
	case reflect.Map:
		return d.mapValue(t, v)
	case reflect.Struct:
		return d.structValue(t, v)
	default:
		return d.typeError(t, v, "")
	}
	return nil
}

// tagInt returns the value of an integer tag, sign-extended.
func tagInt(t Tag) (int64, bool) {
	switch t := t.(type) {
	case Byte:
		return int64(t), true
	case Short:
		return int64(t), true
	case Int:
		return int64(t), true
	case Long:
		return int64(t), true
	}
	return 0, false
}

// tagUint reinterprets an integer tag as unsigned at its own width, so a
// TAG_Byte of -1 decodes into a uint8 as 255.
func tagUint(t Tag) (uint64, bool) {
	switch t := t.(type) {
	case Byte:
		return uint64(uint8(t)), true
	case Short:
		return uint64(uint16(t)), true
	case Int:
		return uint64(uint32(t)), true
	case Long:
		return uint64(t), true
	}
	return 0, false
}

func (d *decodeState) sequence(t Tag, v reflect.Value) error {
	var (
		n   int
		get func(i int) Tag
	)
	switch t := t.(type) {
	case List:
		n, get = len(t), func(i int) Tag { return t[i] }
	case ByteArray:
		if v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8 && !v.Type().Elem().Implements(tagType) {
			v.SetBytes(append([]byte(nil), t...))
			return nil
		}
		n, get = len(t), func(i int) Tag { return Byte(t[i]) }
	case IntArray:
		n, get = len(t), func(i int) Tag { return Int(t[i]) }
	case LongArray:
		n, get = len(t), func(i int) Tag { return Long(t[i]) }
	default:
		return d.typeError(t, v, "")
	}

	if v.Kind() == reflect.Slice {
		if v.IsNil() || v.Cap() < n {
			v.Set(reflect.MakeSlice(v.Type(), n, n))
		} else {
			v.SetLen(n)
		}
	}
	for i := 0; i < n; i++ {
		if i >= v.Len() {
			break // extra elements are dropped, as in encoding/json
		}
		d.path = append(d.path, fmt.Sprintf("[%d]", i))
		err := d.value(get(i), v.Index(i))
		d.path = d.path[:len(d.path)-1]
		if err != nil {
			return err
		}
	}
	if v.Kind() == reflect.Array {
		zero := reflect.Zero(v.Type().Elem())
		for i := n; i < v.Len(); i++ {
			v.Index(i).Set(zero)
		}
	}
	return nil
}

func (d *decodeState) mapValue(t Tag, v reflect.Value) error {
	c, ok := t.(Compound)
	if !ok {
		return d.typeError(t, v, "")
	}
	typ := v.Type()
	if typ.Key().Kind() != reflect.String {
		return d.typeError(t, v, "")
	}
	if v.IsNil() {
		v.Set(reflect.MakeMapWithSize(typ, len(c)))
	}
	elemType := typ.Elem()
	for k, e := range c {
		ev := reflect.New(elemType).Elem()
		d.path = append(d.path, "."+k)
		err := d.value(e, ev)
		d.path = d.path[:len(d.path)-1]
		if err != nil {
			return err
		}
		v.SetMapIndex(reflect.ValueOf(k).Convert(typ.Key()), ev)
	}
	return nil
}

func (d *decodeState) structValue(t Tag, v reflect.Value) error {
	c, ok := t.(Compound)
	if !ok {
		return d.typeError(t, v, "")
	}
	fields := cachedFields(v.Type())
	for k, e := range c {
		i, ok := fields.byName[k]
		if !ok {
			i, ok = fields.byFold[strings.ToLower(k)]
		}
		if !ok {
			if d.disallowUnknown {
				return &UnknownFieldError{Name: k, Type: v.Type(), Path: d.pathString()}
			}
			continue
		}
		f := fields.list[i]
		fv, ok := fieldByIndex(v, f.index, true)
		if !ok {
			continue
		}
		d.path = append(d.path, "."+k)
		err := d.value(e, fv)
		d.path = d.path[:len(d.path)-1]
		if err != nil {
			return err
		}
	}
	return nil
}

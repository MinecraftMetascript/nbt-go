package nbt

import (
	"bytes"
	"encoding"
	"fmt"
	"math"
	"reflect"
)

// Marshaler is implemented by types that can convert themselves to a Tag.
type Marshaler interface {
	MarshalNBT() (Tag, error)
}

// Unmarshaler is implemented by types that can set themselves from a Tag.
type Unmarshaler interface {
	UnmarshalNBT(Tag) error
}

// An UnsupportedTypeError is returned by Marshal when asked to encode a value
// of a type that has no NBT representation, such as a channel or func.
type UnsupportedTypeError struct {
	Type reflect.Type
}

func (e *UnsupportedTypeError) Error() string {
	return "nbt: unsupported type: " + e.Type.String()
}

// A MarshalerError wraps an error returned by a MarshalNBT method.
type MarshalerError struct {
	Type reflect.Type
	Err  error
}

func (e *MarshalerError) Error() string {
	return "nbt: error calling MarshalNBT for type " + e.Type.String() + ": " + e.Err.Error()
}

func (e *MarshalerError) Unwrap() error { return e.Err }

// Marshal returns the uncompressed binary NBT encoding of v, written as a root
// tag with an empty name (as Java Edition does for structure and level
// files). Use an [Encoder] to set the root name or compress the output.
//
// v is normally a struct or map, giving a TAG_Compound root. See the package
// documentation for how Go values map to NBT.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := NewEncoder(&buf).Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ToTag converts v to an NBT tree using the same rules as [Marshal]. It is
// useful for building a [Compound] from Go values, for example to embed in
// another structure or format as SNBT.
func ToTag(v any) (Tag, error) {
	t, ok, err := encodeValue(reflect.ValueOf(v), false)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("nbt: cannot encode nil %T", v)
	}
	return t, nil
}

var (
	tagType         = reflect.TypeFor[Tag]()
	marshalerType   = reflect.TypeFor[Marshaler]()
	unmarshalerType = reflect.TypeFor[Unmarshaler]()
	textMarshaler   = reflect.TypeFor[encoding.TextMarshaler]()
	textUnmarshaler = reflect.TypeFor[encoding.TextUnmarshaler]()
)

// encodeValue converts v to a Tag. ok is false if v is nil-like and should be
// omitted, since NBT has no null.
func encodeValue(v reflect.Value, asList bool) (t Tag, ok bool, err error) {
	if !v.IsValid() {
		return nil, false, nil
	}
	typ := v.Type()

	if typ.Implements(marshalerType) {
		if (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) && v.IsNil() {
			return nil, false, nil
		}
		return callMarshaler(v.Interface().(Marshaler), typ)
	}
	if v.Kind() != reflect.Pointer && v.CanAddr() && reflect.PointerTo(typ).Implements(marshalerType) {
		return callMarshaler(v.Addr().Interface().(Marshaler), typ)
	}
	if typ.Implements(tagType) && v.Kind() != reflect.Interface {
		return v.Interface().(Tag), true, nil
	}
	if v.Kind() != reflect.Interface {
		var tm encoding.TextMarshaler
		if typ.Implements(textMarshaler) {
			if v.Kind() == reflect.Pointer && v.IsNil() {
				return nil, false, nil
			}
			tm = v.Interface().(encoding.TextMarshaler)
		} else if v.Kind() != reflect.Pointer && v.CanAddr() && reflect.PointerTo(typ).Implements(textMarshaler) {
			tm = v.Addr().Interface().(encoding.TextMarshaler)
		}
		if tm != nil {
			b, err := tm.MarshalText()
			if err != nil {
				return nil, false, &MarshalerError{Type: typ, Err: err}
			}
			return String(b), true, nil
		}
	}

	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return nil, false, nil
		}
		return encodeValue(v.Elem(), asList)
	case reflect.Bool:
		if v.Bool() {
			return Byte(1), true, nil
		}
		return Byte(0), true, nil
	case reflect.Int8:
		return Byte(v.Int()), true, nil
	case reflect.Int16:
		return Short(v.Int()), true, nil
	case reflect.Int32:
		return Int(v.Int()), true, nil
	case reflect.Int64:
		return Long(v.Int()), true, nil
	case reflect.Int:
		n := v.Int()
		if n < math.MinInt32 || n > math.MaxInt32 {
			return nil, false, fmt.Errorf("nbt: int value %d overflows TAG_Int; use int64 for a TAG_Long", n)
		}
		return Int(n), true, nil
	case reflect.Uint8:
		return Byte(int8(v.Uint())), true, nil
	case reflect.Uint16:
		return Short(int16(v.Uint())), true, nil
	case reflect.Uint32:
		return Int(int32(v.Uint())), true, nil
	case reflect.Uint64:
		return Long(int64(v.Uint())), true, nil
	case reflect.Uint, reflect.Uintptr:
		n := v.Uint()
		if n > math.MaxUint32 {
			return nil, false, fmt.Errorf("nbt: uint value %d overflows TAG_Int; use uint64 for a TAG_Long", n)
		}
		return Int(int32(uint32(n))), true, nil
	case reflect.Float32:
		return Float(v.Float()), true, nil
	case reflect.Float64:
		return Double(v.Float()), true, nil
	case reflect.String:
		return String(v.String()), true, nil
	case reflect.Slice, reflect.Array:
		return encodeSequence(v, asList)
	case reflect.Map:
		return encodeMap(v)
	case reflect.Struct:
		return encodeStruct(v)
	}
	return nil, false, &UnsupportedTypeError{Type: typ}
}

func callMarshaler(m Marshaler, typ reflect.Type) (Tag, bool, error) {
	t, err := m.MarshalNBT()
	if err != nil {
		return nil, false, &MarshalerError{Type: typ, Err: err}
	}
	return t, t != nil, nil
}

// arrayKind reports whether a sequence of t encodes as a typed array
// (ByteArray, IntArray or LongArray) by default, or TagEnd if it encodes as a List.
func arrayKind(t reflect.Type) TagType {
	if t.Implements(tagType) ||
		t.Implements(marshalerType) || reflect.PointerTo(t).Implements(marshalerType) ||
		t.Implements(textMarshaler) || reflect.PointerTo(t).Implements(textMarshaler) {
		return TagEnd
	}
	switch t.Kind() {
	case reflect.Int8, reflect.Uint8:
		return TagByteArray
	case reflect.Int32, reflect.Uint32:
		return TagIntArray
	case reflect.Int64, reflect.Uint64:
		return TagLongArray
	}
	return TagEnd
}

func encodeSequence(v reflect.Value, asList bool) (Tag, bool, error) {
	n := v.Len()
	if !asList {
		switch arrayKind(v.Type().Elem()) {
		case TagByteArray:
			b := make(ByteArray, n)
			if v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8 {
				copy(b, v.Bytes())
			} else {
				for i := range b {
					b[i] = byte(intOf(v.Index(i)))
				}
			}
			return b, true, nil
		case TagIntArray:
			a := make(IntArray, n)
			for i := range a {
				a[i] = int32(intOf(v.Index(i)))
			}
			return a, true, nil
		case TagLongArray:
			a := make(LongArray, n)
			for i := range a {
				a[i] = intOf(v.Index(i))
			}
			return a, true, nil
		}
	}
	l := make(List, 0, n)
	for i := 0; i < n; i++ {
		t, ok, err := encodeValue(v.Index(i), false)
		if err != nil {
			return nil, false, err
		}
		if !ok {
			return nil, false, fmt.Errorf("nbt: cannot encode nil element at index %d of %s", i, v.Type())
		}
		if i > 0 && t.Type() != l[0].Type() {
			return nil, false, fmt.Errorf("nbt: list elements must share a type, got %v and %v in %s", l[0].Type(), t.Type(), v.Type())
		}
		l = append(l, t)
	}
	return l, true, nil
}

func intOf(v reflect.Value) int64 {
	if v.CanInt() {
		return v.Int()
	}
	return int64(v.Uint())
}

func encodeMap(v reflect.Value) (Tag, bool, error) {
	if v.Type().Key().Kind() != reflect.String {
		return nil, false, &UnsupportedTypeError{Type: v.Type()}
	}
	if v.IsNil() {
		return nil, false, nil
	}
	c := make(Compound, v.Len())
	iter := v.MapRange()
	for iter.Next() {
		t, ok, err := encodeValue(iter.Value(), false)
		if err != nil {
			return nil, false, err
		}
		if ok {
			c[iter.Key().String()] = t
		}
	}
	return c, true, nil
}

func encodeStruct(v reflect.Value) (Tag, bool, error) {
	fields := cachedFields(v.Type())
	c := make(Compound, len(fields.list))
	for _, f := range fields.list {
		fv, ok := fieldByIndex(v, f.index, false)
		if !ok {
			continue
		}
		if f.omitEmpty && isEmptyValue(fv) {
			continue
		}
		t, ok, err := encodeValue(fv, f.asList)
		if err != nil {
			return nil, false, fmt.Errorf("%w (field %s)", err, f.name)
		}
		if ok {
			c[f.name] = t
		}
	}
	return c, true, nil
}

// fieldByIndex follows index from v. When alloc is false a nil embedded
// pointer stops the walk and ok is false; when true it is allocated.
func fieldByIndex(v reflect.Value, index []int, alloc bool) (reflect.Value, bool) {
	for i, x := range index {
		if i > 0 && v.Kind() == reflect.Pointer {
			if v.IsNil() {
				if !alloc || !v.CanSet() {
					return reflect.Value{}, false
				}
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.Field(x)
	}
	return v, true
}

func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.Interface, reflect.Pointer:
		return v.IsZero()
	}
	return false
}

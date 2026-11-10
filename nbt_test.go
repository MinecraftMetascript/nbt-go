package nbt

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"
)

// helloWorld is the classic test file from the original NBT specification.
var helloWorld = []byte{
	0x0a, 0x00, 0x0b, 'h', 'e', 'l', 'l', 'o', ' ', 'w', 'o', 'r', 'l', 'd',
	0x08, 0x00, 0x04, 'n', 'a', 'm', 'e',
	0x00, 0x09, 'B', 'a', 'n', 'a', 'n', 'r', 'a', 'm', 'a',
	0x00,
}

func TestDecodeHelloWorld(t *testing.T) {
	var v struct {
		Name string `nbt:"name"`
	}
	dec := NewDecoder(bytes.NewReader(helloWorld))
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	if v.Name != "Bananrama" {
		t.Errorf("name = %q", v.Name)
	}
	if dec.RootName() != "hello world" {
		t.Errorf("root name = %q", dec.RootName())
	}
	if dec.Compression() != None {
		t.Errorf("compression = %v", dec.Compression())
	}
}

func TestEncodeHelloWorld(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	enc.SetRootName("hello world")
	if err := enc.Encode(map[string]string{"name": "Bananrama"}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf.Bytes(), helloWorld) {
		t.Errorf("got  % x\nwant % x", buf.Bytes(), helloWorld)
	}
}

func allTags() Compound {
	return Compound{
		"byte":      Byte(-12),
		"short":     Short(-1234),
		"int":       Int(-123456),
		"long":      Long(-1234567890123),
		"float":     Float(1.5),
		"double":    Double(-2.25),
		"bytes":     ByteArray{0, 1, 0xff},
		"string":    String("héllo\x00wörld 🧱"),
		"list":      List{Int(1), Int(2), Int(3)},
		"emptyList": List{},
		"nested":    List{Compound{"a": Byte(1)}, Compound{"b": List{String("x")}}},
		"compound":  Compound{"inner": Compound{}},
		"ints":      IntArray{1, -1, math.MaxInt32},
		"longs":     LongArray{1, -1, math.MinInt64},
	}
}

func TestTagRoundTrip(t *testing.T) {
	want := allTags()
	data, err := Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got Tag
	if err := Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, Tag(want)) {
		t.Errorf("round trip mismatch:\n got %v\nwant %v", got, want)
	}

	// Decoding into a concrete Compound and into any gives the same tree.
	var c Compound
	if err := Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c, want) {
		t.Errorf("Compound mismatch")
	}
	var a any
	if err := Unmarshal(data, &a); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, any(want)) {
		t.Errorf("any mismatch")
	}
}

func TestModifiedUTF8(t *testing.T) {
	// NUL is C0 80 and U+1F9F1 is a surrogate pair, each half three bytes.
	got := encodeMUTF8("a\x00🧱")
	want := []byte{'a', 0xC0, 0x80, 0xED, 0xA0, 0xBE, 0xED, 0xB7, 0xB1}
	if !bytes.Equal(got, want) {
		t.Errorf("encode: got % x want % x", got, want)
	}
	if s := decodeMUTF8(want); s != "a\x00🧱" {
		t.Errorf("decode: got %q", s)
	}
	if s := decodeMUTF8([]byte("plain ascii")); s != "plain ascii" {
		t.Errorf("decode plain: got %q", s)
	}
}

type Inner struct {
	ID    string `nbt:"id"`
	Count int8   `nbt:"count"`
}

type Base struct {
	DataVersion int32
	Shadowed    string `nbt:"shadowed"`
}

type Everything struct {
	Base
	Shadowed string    `nbt:"shadowed"`
	Bool     bool      `nbt:"bool"`
	I8       int8      `nbt:"i8"`
	U8       uint8     `nbt:"u8"`
	I16      int16     `nbt:"i16"`
	U16      uint16    `nbt:"u16"`
	I        int       `nbt:"i"`
	I32      int32     `nbt:"i32"`
	U32      uint32    `nbt:"u32"`
	I64      int64     `nbt:"i64"`
	U64      uint64    `nbt:"u64"`
	F32      float32   `nbt:"f32"`
	F64      float64   `nbt:"f64"`
	Str      string    `nbt:"str"`
	Bytes    []byte    `nbt:"bytes"`
	Ints     []int32   `nbt:"ints"`
	Longs    []int64   `nbt:"longs"`
	Pos      [3]int32  `nbt:"pos,list"`
	Motion   []float64 `nbt:"Motion"`
	Items    []Inner   `nbt:"Items"`
	Ptr      *Inner    `nbt:"ptr"`
	NilPtr   *Inner    `nbt:"nilptr"`
	Props    map[string]string
	Extra    Compound `nbt:"extra,omitempty"`
	Skip     string   `nbt:"-"`
	Empty    string   `nbt:"empty,omitempty"`
	Any      any      `nbt:"any"`
	unexp    int      //nolint:unused // unexported fields must be skipped
}

func sampleEverything() Everything {
	return Everything{
		Base:     Base{DataVersion: 3955, Shadowed: "base"},
		Shadowed: "outer",
		Bool:     true,
		I8:       -8, U8: 250,
		I16: -16, U16: 65000,
		I: -100000, I32: 32, U32: 4000000000,
		I64: -64, U64: math.MaxUint64,
		F32: 0.5, F64: 0.25,
		Str:    "stone",
		Bytes:  []byte{1, 2, 3},
		Ints:   []int32{4, 5},
		Longs:  []int64{6},
		Pos:    [3]int32{1, 2, 3},
		Motion: []float64{0, -0.5, 0},
		Items:  []Inner{{"minecraft:diamond", 3}, {"minecraft:stick", 1}},
		Ptr:    &Inner{"minecraft:apple", 1},
		Props:  map[string]string{"facing": "north"},
		Any:    Compound{"x": Int(1)},
	}
}

func TestStructRoundTrip(t *testing.T) {
	in := sampleEverything()
	data, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}

	// Check the wire shape.
	var tree Compound
	if err := Unmarshal(data, &tree); err != nil {
		t.Fatal(err)
	}
	wantTypes := map[string]TagType{
		"DataVersion": TagInt, "shadowed": TagString, "bool": TagByte,
		"i8": TagByte, "u8": TagByte, "i16": TagShort, "u16": TagShort,
		"i": TagInt, "i32": TagInt, "u32": TagInt, "i64": TagLong, "u64": TagLong,
		"f32": TagFloat, "f64": TagDouble, "str": TagString,
		"bytes": TagByteArray, "ints": TagIntArray, "longs": TagLongArray,
		"pos": TagList, "Motion": TagList, "Items": TagList, "ptr": TagCompound,
		"Props": TagCompound, "any": TagCompound,
	}
	for k, typ := range wantTypes {
		if tree[k] == nil {
			t.Errorf("missing key %q", k)
		} else if tree[k].Type() != typ {
			t.Errorf("%s: type %v, want %v", k, tree[k].Type(), typ)
		}
	}
	if len(tree) != len(wantTypes) {
		t.Errorf("got %d keys, want %d: %v", len(tree), len(wantTypes), tree)
	}
	if tree["shadowed"] != String("outer") {
		t.Errorf("embedded field was not shadowed: %v", tree["shadowed"])
	}
	if l := tree["pos"].(List); l.ElemType() != TagInt {
		t.Errorf("pos elem type %v", l.ElemType())
	}

	var out Everything
	if err := Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	in.Base.Shadowed = "" // hidden by the outer field, so never encoded
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", out, in)
	}
}

func TestOmitNil(t *testing.T) {
	type T struct {
		P *int
		I any
		M map[string]int
	}
	data, err := Marshal(T{})
	if err != nil {
		t.Fatal(err)
	}
	var c Compound
	if err := Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	if len(c) != 0 {
		t.Errorf("nil values should be omitted, got %v", c)
	}
}

func TestLenientNumericDecode(t *testing.T) {
	var v struct {
		A int64   `nbt:"a"`
		B bool    `nbt:"b"`
		C float64 `nbt:"c"`
		D uint8   `nbt:"d"`
		E []int   `nbt:"e"`
	}
	src := Compound{"a": Byte(5), "b": Byte(1), "c": Int(7), "d": Byte(-1), "e": IntArray{1, 2}}
	if err := FromTag(src, &v); err != nil {
		t.Fatal(err)
	}
	if v.A != 5 || !v.B || v.C != 7 || v.D != 255 || !reflect.DeepEqual(v.E, []int{1, 2}) {
		t.Errorf("got %+v", v)
	}
}

func TestCaseInsensitiveFallback(t *testing.T) {
	var v struct{ Name string }
	if err := FromTag(Compound{"name": String("x")}, &v); err != nil {
		t.Fatal(err)
	}
	if v.Name != "x" {
		t.Errorf("got %q", v.Name)
	}
}

func TestUnmarshalTypeError(t *testing.T) {
	var v struct {
		Blocks []struct {
			Pos [3]int8 `nbt:"pos"`
		} `nbt:"blocks"`
	}
	src := Compound{"blocks": List{Compound{"pos": List{Int(1), Int(1000), Int(0)}}}}
	err := FromTag(src, &v)
	var te *UnmarshalTypeError
	if !errors.As(err, &te) {
		t.Fatalf("got %v, want UnmarshalTypeError", err)
	}
	if te.Path != "blocks[0].pos[1]" || te.Tag != TagInt {
		t.Errorf("got path %q tag %v (%v)", te.Path, te.Tag, err)
	}

	var s string
	if err := FromTag(Int(1), &s); !errors.As(err, &te) {
		t.Errorf("int into string: got %v", err)
	}
}

func TestDisallowUnknownFields(t *testing.T) {
	data, _ := Marshal(map[string]int32{"known": 1, "other": 2})
	var v struct {
		Known int32 `nbt:"known"`
	}
	dec := NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var ue *UnknownFieldError
	if err := dec.Decode(&v); !errors.As(err, &ue) || ue.Name != "other" {
		t.Errorf("got %v", err)
	}
}

func TestInvalidUnmarshal(t *testing.T) {
	var v struct{}
	for _, target := range []any{nil, v, (*struct{})(nil)} {
		var ie *InvalidUnmarshalError
		if err := Unmarshal(helloWorld, target); !errors.As(err, &ie) {
			t.Errorf("Unmarshal(%T): got %v", target, err)
		}
	}
}

func TestMarshalErrors(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want string
	}{
		{"chan", map[string]any{"c": make(chan int)}, "unsupported type"},
		{"mixed list", map[string]any{"l": []any{1, "a"}}, "share a type"},
		{"nil elem", map[string]any{"l": []*int{nil}}, "nil element"},
		{"int overflow", map[string]int{"i": math.MaxInt32 + 1}, "overflows"},
		{"int map key", map[int]int{1: 1}, "unsupported type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Marshal(tt.v)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("got %v, want error containing %q", err, tt.want)
			}
		})
	}
}

type color struct{ R, G, B uint8 }

func (c color) MarshalNBT() (Tag, error) {
	return Int(int32(c.R)<<16 | int32(c.G)<<8 | int32(c.B)), nil
}

func (c *color) UnmarshalNBT(t Tag) error {
	n, ok := t.(Int)
	if !ok {
		return errors.New("color must be an int")
	}
	c.R, c.G, c.B = uint8(n>>16), uint8(n>>8), uint8(n)
	return nil
}

func TestMarshaler(t *testing.T) {
	type display struct {
		Color  color   `nbt:"color"`
		Colors []color `nbt:"colors"`
	}
	in := display{Color: color{1, 2, 3}, Colors: []color{{4, 5, 6}}}
	tag, err := ToTag(in)
	if err != nil {
		t.Fatal(err)
	}
	c := tag.(Compound)
	if c["color"] != Int(0x010203) {
		t.Errorf("color = %v", c["color"])
	}
	if _, ok := c["colors"].(List); !ok {
		t.Errorf("slice of Marshalers should be a List, got %v", c["colors"].Type())
	}
	var out display
	if err := FromTag(tag, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("got %+v", out)
	}
}

func TestCompression(t *testing.T) {
	for _, c := range []Compression{None, Gzip, Zlib} {
		t.Run(c.String(), func(t *testing.T) {
			var buf bytes.Buffer
			enc := NewEncoder(&buf)
			enc.SetCompression(c)
			for i := int32(0); i < 3; i++ {
				if err := enc.Encode(map[string]int32{"i": i}); err != nil {
					t.Fatal(err)
				}
			}
			dec := NewDecoder(&buf)
			for i := int32(0); i < 3; i++ {
				var v struct{ I int32 }
				if err := dec.Decode(&v); err != nil {
					t.Fatalf("value %d: %v", i, err)
				}
				if v.I != i {
					t.Errorf("value %d: got %d", i, v.I)
				}
			}
			if dec.Compression() != c {
				t.Errorf("detected %v", dec.Compression())
			}
			if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
				t.Errorf("at end: got %v, want EOF", err)
			}
		})
	}
}

func TestUnmarshalGzip(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(helloWorld); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	var v struct {
		Name string `nbt:"name"`
	}
	if err := Unmarshal(buf.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.Name != "Bananrama" {
		t.Errorf("got %q", v.Name)
	}
}

func TestMalformed(t *testing.T) {
	deep := bytes.Repeat([]byte{byte(TagList), 0, 0}, 0)
	deep = append(deep, byte(TagCompound), 0, 0)
	for i := 0; i < MaxDepth+1; i++ {
		deep = append(deep, byte(TagCompound), 0, 1, 'a')
	}
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"truncated", helloWorld[:len(helloWorld)-3]},
		{"bad type", []byte{0x0d, 0, 0}},
		{"end root", []byte{0, 0, 0}},
		{"negative length", []byte{byte(TagByteArray), 0, 0, 0xff, 0xff, 0xff, 0xff}},
		{"huge length", []byte{byte(TagLongArray), 0, 0, 0x7f, 0xff, 0xff, 0xff, 1, 2}},
		{"end list with length", []byte{byte(TagList), 0, 0, 0, 0, 0, 0, 1}},
		{"too deep", deep},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v any
			if err := Unmarshal(tt.data, &v); err == nil {
				t.Errorf("expected error, got %v", v)
			}
		})
	}
}

func TestDeterministicOutput(t *testing.T) {
	a, _ := Marshal(allTags())
	for i := 0; i < 10; i++ {
		b, _ := Marshal(allTags())
		if !bytes.Equal(a, b) {
			t.Fatal("Marshal output is not deterministic")
		}
	}
}

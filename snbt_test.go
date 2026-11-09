package nbt

import (
	"reflect"
	"testing"
)

func TestSNBTFormat(t *testing.T) {
	tag := Compound{
		"b":       Byte(1),
		"s":       Short(2),
		"i":       Int(3),
		"l":       Long(4),
		"f":       Float(1.5),
		"d":       Double(2),
		"str":     String(`say "hi"`),
		"plain":   String("minecraft:stone"),
		"ba":      ByteArray{1, 0xff},
		"ia":      IntArray{1, 2},
		"la":      LongArray{3},
		"list":    List{String("a"), String("b")},
		"odd key": Compound{},
	}
	want := `{b:1b,ba:[B;1b,-1b],d:2d,f:1.5f,i:3,ia:[I;1,2],l:4L,la:[L;3L],list:["a","b"],"odd key":{},plain:"minecraft:stone",s:2s,str:'say "hi"'}`
	if got := tag.String(); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestSNBTParse(t *testing.T) {
	tests := []struct {
		in   string
		want Tag
	}{
		{"1b", Byte(1)},
		{"-3s", Short(-3)},
		{"42", Int(42)},
		{"42L", Long(42)},
		{"1.5f", Float(1.5)},
		{"1.5", Double(1.5)},
		{"2d", Double(2)},
		{"1e3d", Double(1000)},
		{".5", Double(0.5)},
		{"true", Byte(1)},
		{"False", Byte(0)},
		{"minecraft:stone", nil}, // ':' is not an unquoted char, so this is a parse error
		{"stone", String("stone")},
		{"300b", String("300b")}, // out of range numbers become strings
		{"99999999999", String("99999999999")},
		{`"a\"b"`, String(`a"b`)},
		{`'it\'s'`, String("it's")},
		{`"é\n"`, String("é\n")},
		{"[]", List{}},
		{"[1, 2 ,3]", List{Int(1), Int(2), Int(3)}},
		{"[B; 1b, -2b]", ByteArray{1, 0xfe}},
		{"[I;]", IntArray{}},
		{"[L;1L,2]", LongArray{1, 2}},
		{" { a : 1 , 'b c':{x:[{}]} } ", Compound{"a": Int(1), "b c": Compound{"x": List{Compound{}}}}},
		{"[1,2b]", nil},
		{"{a:1", nil},
		{"{a:1}}", nil},
		{`"unterminated`, nil},
		{"[B;300]", nil},
		{"", nil},
	}
	for _, tt := range tests {
		got, err := ParseSNBT(tt.in)
		if tt.want == nil {
			if err == nil {
				t.Errorf("ParseSNBT(%q) = %v, want error", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseSNBT(%q): %v", tt.in, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ParseSNBT(%q) = %#v, want %#v", tt.in, got, tt.want)
		}
	}
}

func TestSNBTRoundTrip(t *testing.T) {
	want := allTags()
	got, err := ParseSNBT(want.String())
	if err != nil {
		t.Fatalf("%v\n%s", err, want.String())
	}
	if !reflect.DeepEqual(got, Tag(want)) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}

func TestUnmarshalSNBT(t *testing.T) {
	type item struct {
		Slot  int8   `nbt:"Slot"`
		ID    string `nbt:"id"`
		Count int32  `nbt:"count"`
	}
	var chest struct {
		Items []item `nbt:"Items"`
	}
	err := UnmarshalSNBT(`{Items:[{Slot:0b,id:"minecraft:diamond",count:3},{Slot:1b,id:"minecraft:torch",count:64}]}`, &chest)
	if err != nil {
		t.Fatal(err)
	}
	want := []item{{0, "minecraft:diamond", 3}, {1, "minecraft:torch", 64}}
	if !reflect.DeepEqual(chest.Items, want) {
		t.Errorf("got %+v", chest.Items)
	}
	s, err := MarshalSNBT(chest)
	if err != nil {
		t.Fatal(err)
	}
	if s != `{Items:[{Slot:0b,count:3,id:"minecraft:diamond"},{Slot:1b,count:64,id:"minecraft:torch"}]}` {
		t.Errorf("MarshalSNBT = %s", s)
	}
}

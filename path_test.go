package nbt

import (
	"errors"
	"testing"
)

func mustSNBT(t *testing.T, s string) Tag {
	t.Helper()
	v, err := ParseSNBT(s)
	if err != nil {
		t.Fatalf("%s: %v", s, err)
	}
	return v
}

func joined(ts []Tag) string {
	return List(ts).String()
}

func TestParsePath(t *testing.T) {
	for _, s := range []string{
		"a", "a.b.c", `"a b".c`, `'it\'s'.x`, "a[0]", "a[-1]", "a[]", `a[{id:"x"}]`, `a{b:1}`,
		`{a:1}`, `{a:1}.b`, `a[0][1].b`, "minecraft:custom_data", `components."minecraft:custom_data".x`,
	} {
		p, err := ParsePath(s)
		if err != nil {
			t.Errorf("ParsePath(%q): %v", s, err)
			continue
		}
		if p.String() != s {
			t.Errorf("ParsePath(%q).String() = %q", s, p.String())
		}
	}
	for _, s := range []string{"", ".a", "a.", "a..b", "a[", "a[x]", "a[0", "a[{]", `"a`, "a b", "{a:1}b", "a[0]b", "a]"} {
		_, err := ParsePath(s)
		var pe *PathError
		if !errors.As(err, &pe) {
			t.Errorf("ParsePath(%q) = %v, want a PathError", s, err)
		}
	}
}

func TestPathGet(t *testing.T) {
	root := mustSNBT(t, `{a:{b:[{id:"x",n:1},{id:"y",n:2},{id:"x",n:3}]},l:[1,2,3],arr:[I;4,5],"odd key":{"1":"one"},"minecraft:c":7,e:[]}`)
	for path, want := range map[string]string{
		"a.b[0].n":          "[1]",
		"a.b[-1].n":         "[3]",
		"a.b[].n":           "[1,2,3]",
		`a.b[{id:"x"}].n`:   "[1,3]",
		`a{b:[{id:"y"}]}.b`: `[[{id:"x",n:1},{id:"y",n:2},{id:"x",n:3}]]`,
		`a{b:[{id:"z"}]}`:   "[]",
		"l[1]":              "[2]",
		"l[3]":              "[]",
		"l[-4]":             "[]",
		"arr[1]":            "[5]",
		"arr[]":             "[4,5]",
		`"odd key"."1"`:     `["one"]`,
		"minecraft:c":       "[7]",
		"nothing":           "[]",
		"l.x":               "[]",
		"a.b.x":             "[]",
		`{l:[2]}.l[0]`:      "[1]",
		`{l:[9]}.l[0]`:      "[]",
		"e{}":               "[]",
		`e[]`:               "[]",
	} {
		if got := joined(MustParsePath(path).Get(root)); got != want {
			t.Errorf("%s: got %s, want %s", path, got, want)
		}
	}
}

func TestPathSet(t *testing.T) {
	tests := []struct {
		root, path, value, want string
		changed                 int
	}{
		{`{}`, "a", "1", "{a:1}", 1},
		{`{a:1}`, "a", "1", "{a:1}", 0},
		{`{a:1}`, "a", "1b", "{a:1b}", 1},
		{`{}`, "a.b.c", `"x"`, `{a:{b:{c:"x"}}}`, 1},
		{`{}`, "a[0]", "1", "{a:[]}", 0},
		{`{}`, "a[]", "1", "{a:[1]}", 1},
		{`{}`, `a[{id:"x"}].n`, "1", `{a:[{id:"x",n:1}]}`, 1},
		{`{a:[{id:"y"}]}`, `a[{id:"x"}].n`, "1", `{a:[{id:"y"},{id:"x",n:1}]}`, 1},
		{`{a:[{id:"x"},{id:"y"},{id:"x"}]}`, `a[{id:"x"}].n`, "1", `{a:[{id:"x",n:1},{id:"y"},{id:"x",n:1}]}`, 2},
		{`{l:[1,2,3]}`, "l[1]", "20", "{l:[1,20,3]}", 1},
		{`{l:[1,2,3]}`, "l[-1]", "30", "{l:[1,2,30]}", 1},
		{`{l:[1,2,3]}`, "l[]", "0", "{l:[0,0,0]}", 3},
		{`{l:[1,2,3]}`, "l[5]", "0", "{l:[1,2,3]}", 0},
		{`{arr:[I;1,2]}`, "arr[0]", "9", "{arr:[I;9,2]}", 1},
		{`{}`, `c{k:1}.x`, "2", "{c:{k:1,x:2}}", 1},
		{`{c:{k:2}}`, `c{k:1}.x`, "2", "{c:{k:2}}", 0},
		{`{s:"str"}`, "s.x", "1", `{s:"str"}`, 0},
		{`{a:[[1],[2]]}`, "a[1][0]", "5", "{a:[[1],[5]]}", 1},
		{`{a:1}`, `{a:1}.b`, "2", "{a:1,b:2}", 1},
		{`{a:1}`, `{a:2}.b`, "2", "{a:1}", 0},
	}
	for _, tt := range tests {
		root := mustSNBT(t, tt.root)
		n, err := MustParsePath(tt.path).Set(root, mustSNBT(t, tt.value))
		if err != nil {
			t.Errorf("%s set %s on %s: %v", tt.path, tt.value, tt.root, err)
			continue
		}
		if got := root.String(); got != tt.want || n != tt.changed {
			t.Errorf("%s set %s on %s: %s (%d changed), want %s (%d)", tt.path, tt.value, tt.root, got, n, tt.want, tt.changed)
		}
	}
	for _, tt := range []struct{ root, path, value string }{
		{`{l:[1,2]}`, "l[0]", `"s"`},
		{`{arr:[I;1]}`, "arr[0]", "1b"},
		{`{a:1}`, `{a:1}`, "2"},
		{`{l:[1]}`, `l[{a:1}]`, "{}"},
	} {
		root := mustSNBT(t, tt.root)
		if _, err := MustParsePath(tt.path).Set(root, mustSNBT(t, tt.value)); err == nil {
			t.Errorf("%s set %s on %s: no error, now %s", tt.path, tt.value, tt.root, root)
		}
	}
}

// Set stores a copy: changing the value afterwards leaves root alone.
func TestPathSetCopies(t *testing.T) {
	root := Compound{}
	v := Compound{"x": List{Int(1)}}
	if _, err := MustParsePath("a").Set(root, v); err != nil {
		t.Fatal(err)
	}
	v["x"] = List{Int(2)}
	if got := root.String(); got != "{a:{x:[1]}}" {
		t.Errorf("root = %s", got)
	}
}

func TestPathModify(t *testing.T) {
	root := mustSNBT(t, `{}`)
	appendOne := func(v Tag) (Tag, error) { return append(v.(List), Int(1)), nil }
	newList := func() Tag { return List{} }
	p := MustParsePath("a.l")
	for i := 0; i < 2; i++ {
		if n, err := p.Modify(root, newList, appendOne); err != nil || n != 1 {
			t.Fatalf("modify: %d %v", n, err)
		}
	}
	if got := root.String(); got != "{a:{l:[1,1]}}" {
		t.Errorf("root = %s", got)
	}
	if n, _ := MustParsePath("b").Modify(root, nil, appendOne); n != 0 || root.String() != "{a:{l:[1,1]}}" {
		t.Errorf("modify without create made %d, root %s", n, root)
	}
	wantErr := errors.New("nope")
	if _, err := p.Modify(root, newList, func(Tag) (Tag, error) { return nil, wantErr }); !errors.Is(err, wantErr) {
		t.Errorf("err = %v", err)
	}
}

func TestPathRemove(t *testing.T) {
	tests := []struct {
		root, path, want string
		removed          int
	}{
		{`{a:1,b:2}`, "a", "{b:2}", 1},
		{`{a:1}`, "x", "{a:1}", 0},
		{`{a:{b:1,c:2}}`, "a.b", "{a:{c:2}}", 1},
		{`{l:[1,2,3]}`, "l[0]", "{l:[2,3]}", 1},
		{`{l:[1,2,3]}`, "l[-1]", "{l:[1,2]}", 1},
		{`{l:[1,2,3]}`, "l[]", "{l:[]}", 3},
		{`{l:[{k:1},{k:2},{k:1}]}`, "l[{k:1}]", "{l:[{k:2}]}", 2},
		{`{a:[{l:[1,2]},{l:[3]}]}`, "a[].l[0]", "{a:[{l:[2]},{l:[]}]}", 2},
		{`{arr:[L;1L,2L]}`, "arr[1]", "{arr:[L;1L]}", 1},
		{`{c:{k:1}}`, "c{k:2}", "{c:{k:1}}", 0},
		{`{c:{k:1}}`, "c{k:1}", "{}", 1},
		{`{a:1}`, "{a:1}", "{a:1}", 0},
	}
	for _, tt := range tests {
		root := mustSNBT(t, tt.root)
		n := MustParsePath(tt.path).Remove(root)
		if got := root.String(); got != tt.want || n != tt.removed {
			t.Errorf("remove %s from %s: %s (%d), want %s (%d)", tt.path, tt.root, got, n, tt.want, tt.removed)
		}
	}
}

func TestMatchesAndEqual(t *testing.T) {
	for _, tt := range []struct {
		pattern, tag string
		want         bool
	}{
		{"{}", "{a:1}", true},
		{"{a:1}", "{a:1,b:2}", true},
		{"{a:1}", "{a:1b}", false},
		{"{a:{b:1}}", "{a:{b:1,c:2}}", true},
		{"{l:[2]}", "{l:[1,2,3]}", true},
		{"{l:[4]}", "{l:[1,2,3]}", false},
		{"{l:[]}", "{l:[1]}", false},
		{"{l:[]}", "{l:[]}", true},
		{"{l:[{id:x}]}", "{l:[{id:x,n:1}]}", true},
		{"1", "1", true},
		{"1", "1L", false},
	} {
		if got := Matches(mustSNBT(t, tt.pattern), mustSNBT(t, tt.tag)); got != tt.want {
			t.Errorf("Matches(%s, %s) = %v", tt.pattern, tt.tag, got)
		}
	}
	for _, tt := range []struct {
		a, b Tag
		want bool
	}{
		{Compound{"a": Int(1)}, Compound{"a": Int(1)}, true},
		{Compound{"a": Int(1), "n": nil}, Compound{"a": Int(1)}, true},
		{Compound{"a": Int(1)}, Compound{"a": Int(1), "b": Int(2)}, false},
		{List{}, List(nil), true},
		{List{Int(1)}, List{Int(2)}, false},
		{ByteArray{1}, ByteArray{1}, true},
		{IntArray{}, IntArray(nil), true},
		{LongArray{1}, LongArray{2}, false},
		{Int(1), Long(1), false},
		{Double(0.5), Double(0.5), true},
		{nil, nil, true},
		{nil, Int(0), false},
	} {
		if got := Equal(tt.a, tt.b); got != tt.want {
			t.Errorf("Equal(%v, %v) = %v", tt.a, tt.b, got)
		}
	}
}

func TestClone(t *testing.T) {
	orig := mustSNBT(t, `{a:[{b:1}],arr:[I;1],ba:[B;1b],la:[L;1L]}`).(Compound)
	c := Clone(orig).(Compound)
	c["a"].(List)[0].(Compound)["b"] = Int(2)
	c["arr"].(IntArray)[0] = 2
	c["ba"].(ByteArray)[0] = 2
	c["la"].(LongArray)[0] = 2
	if got := orig.String(); got != "{a:[{b:1}],arr:[I;1],ba:[B;1b],la:[L;1L]}" {
		t.Errorf("original changed: %s", got)
	}
}

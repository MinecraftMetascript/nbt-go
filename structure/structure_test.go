package structure

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/MinecraftMetascript/nbt-go"
)

func TestBuildWriteRead(t *testing.T) {
	s := New(3955)
	stone := MustParseBlockState("stone_bricks")
	if err := s.Fill(0, 0, 0, 2, 0, 2, stone); err != nil {
		t.Fatal(err)
	}
	chest := MustParseBlockState("minecraft:chest[facing=north, waterlogged=false]")
	loot := nbt.Compound{"LootTable": nbt.String("minecraft:chests/simple_dungeon")}
	if err := s.SetBlock(1, 1, 1, chest, loot); err != nil {
		t.Fatal(err)
	}
	// Replacing a block keeps one entry per position.
	if err := s.SetBlock(0, 0, 0, MustParseBlockState("mossy_stone_bricks"), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.AddEntity([3]float64{1.5, 1, 0.5}, nbt.Compound{"id": nbt.String("minecraft:armor_stand")}); err != nil {
		t.Fatal(err)
	}

	if s.Size != [3]int32{3, 2, 3} {
		t.Errorf("size = %v", s.Size)
	}
	if len(s.Blocks) != 10 {
		t.Errorf("blocks = %d, want 10", len(s.Blocks))
	}
	if len(s.Palette) != 3 {
		t.Errorf("palette = %v", s.Palette)
	}

	var buf bytes.Buffer
	if err := s.Write(&buf); err != nil {
		t.Fatal(err)
	}
	if b := buf.Bytes(); b[0] != 0x1f || b[1] != 0x8b {
		t.Fatal("structure file is not gzip compressed")
	}

	// Check the raw shape against what Minecraft expects.
	var raw nbt.Compound
	dec := nbt.NewDecoder(bytes.NewReader(buf.Bytes()))
	if err := dec.Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if dec.RootName() != "" {
		t.Errorf("root name = %q", dec.RootName())
	}
	checkList := func(tag nbt.Tag, elem nbt.TagType, n int) {
		t.Helper()
		l, ok := tag.(nbt.List)
		if !ok || len(l) != n || (n > 0 && l.ElemType() != elem) {
			t.Errorf("got %v, want list of %d %v", tag, n, elem)
		}
	}
	if raw["DataVersion"] != nbt.Int(3955) {
		t.Errorf("DataVersion = %v", raw["DataVersion"])
	}
	checkList(raw["size"], nbt.TagInt, 3)
	checkList(raw["palette"], nbt.TagCompound, 3)
	checkList(raw["blocks"], nbt.TagCompound, 10)
	checkList(raw["entities"], nbt.TagCompound, 1)
	if _, ok := raw["palettes"]; ok {
		t.Error("empty palettes should be omitted")
	}
	b0 := raw["blocks"].(nbt.List)[0].(nbt.Compound)
	checkList(b0["pos"], nbt.TagInt, 3)
	if _, ok := b0["nbt"]; ok {
		t.Error("plain block should have no nbt key")
	}
	e0 := raw["entities"].(nbt.List)[0].(nbt.Compound)
	checkList(e0["pos"], nbt.TagDouble, 3)
	checkList(e0["blockPos"], nbt.TagInt, 3)

	got, err := Read(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	state, data, ok := got.Block(1, 1, 1)
	if !ok || state.String() != "minecraft:chest[facing=north,waterlogged=false]" || !reflect.DeepEqual(data, loot) {
		t.Errorf("Block(1,1,1) = %v %v %v", state, data, ok)
	}
	if state, _, _ := got.Block(0, 0, 0); state.Name != "minecraft:mossy_stone_bricks" {
		t.Errorf("Block(0,0,0) = %v", state)
	}
	if _, _, ok := got.Block(2, 1, 2); ok {
		t.Error("expected no block at (2,1,2)")
	}
	s.blockIndex, s.paletteIndex, s.indexedLen = nil, nil, 0
	got.blockIndex, got.paletteIndex, got.indexedLen = nil, nil, 0
	if !reflect.DeepEqual(s, got) {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", got, s)
	}

	// Editing a structure read from a file reuses its palette.
	if err := got.SetBlock(2, 1, 2, stone, nil); err != nil {
		t.Fatal(err)
	}
	if len(got.Palette) != 3 {
		t.Errorf("palette grew to %d", len(got.Palette))
	}
}

func TestPaletteKeysFollowDataVersion(t *testing.T) {
	for _, tt := range []struct {
		version          int32
		nameKey, propKey string
	}{
		{BlockStateIDVersion - 1, "Name", "Properties"},
		{BlockStateIDVersion, "id", "properties"},
	} {
		s := New(tt.version)
		if err := s.SetBlock(0, 0, 0, MustParseBlockState("oak_log[axis=y]"), nil); err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := s.Write(&buf); err != nil {
			t.Fatal(err)
		}
		var raw nbt.Compound
		if err := nbt.Unmarshal(buf.Bytes(), &raw); err != nil {
			t.Fatal(err)
		}
		entry := raw["palette"].(nbt.List)[0].(nbt.Compound)
		want := nbt.Compound{tt.nameKey: nbt.String("minecraft:oak_log"), tt.propKey: nbt.Compound{"axis": nbt.String("y")}}
		if !reflect.DeepEqual(entry, want) {
			t.Errorf("DataVersion %d: palette entry %v, want %v", tt.version, entry, want)
		}
		got, err := Read(&buf)
		if err != nil {
			t.Fatal(err)
		}
		if got.Palette[0].String() != "minecraft:oak_log[axis=y]" {
			t.Errorf("DataVersion %d: read back %v", tt.version, got.Palette[0])
		}
	}
}

func TestRemoveBlock(t *testing.T) {
	s := New(1)
	stone := MustParseBlockState("stone")
	if err := s.Fill(0, 0, 0, 1, 1, 1, stone); err != nil {
		t.Fatal(err)
	}
	if !s.RemoveBlock(0, 0, 0) {
		t.Fatal("RemoveBlock returned false")
	}
	if s.RemoveBlock(0, 0, 0) {
		t.Fatal("second RemoveBlock returned true")
	}
	if len(s.Blocks) != 7 {
		t.Errorf("blocks = %d", len(s.Blocks))
	}
	for _, p := range [][3]int{{1, 1, 1}, {0, 1, 0}, {1, 0, 0}} {
		if _, _, ok := s.Block(p[0], p[1], p[2]); !ok {
			t.Errorf("lost block at %v", p)
		}
	}
}

func TestIndexSurvivesDirectEdits(t *testing.T) {
	s := New(1)
	for x, b := range []string{"stone", "dirt"} {
		if err := s.SetBlock(x, 0, 0, MustParseBlockState(b), nil); err != nil {
			t.Fatal(err)
		}
	}
	// Reorder the slice behind the helpers' back.
	s.Blocks[0], s.Blocks[1] = s.Blocks[1], s.Blocks[0]
	if st, _, _ := s.Block(0, 0, 0); st.Name != "minecraft:stone" {
		t.Errorf("got %v", st)
	}
	if err := s.SetBlock(0, 0, 0, MustParseBlockState("glass"), nil); err != nil {
		t.Fatal(err)
	}
	if len(s.Blocks) != 2 {
		t.Errorf("blocks = %d", len(s.Blocks))
	}
}

func TestValidate(t *testing.T) {
	s := New(1)
	if err := s.SetBlock(0, 0, 0, MustParseBlockState("stone"), nil); err != nil {
		t.Fatal(err)
	}
	s.Blocks[0].State = 5
	if err := s.Write(new(bytes.Buffer)); err == nil {
		t.Error("expected error for bad palette index")
	}
	s.Blocks[0].State = 0
	s.DataVersion = 0
	if err := s.Validate(); err == nil {
		t.Error("expected error for missing DataVersion")
	}
	s.DataVersion = 1
	s.Size = [3]int32{0, 0, 0}
	if err := s.Validate(); err == nil {
		t.Error("expected error for block outside size")
	}
	if err := s.SetBlock(-1, 0, 0, MustParseBlockState("stone"), nil); err == nil {
		t.Error("expected error for negative position")
	}
}

func TestParseBlockState(t *testing.T) {
	tests := []struct {
		in, want string
		err      bool
	}{
		{"stone", "minecraft:stone", false},
		{"mymod:thing", "mymod:thing", false},
		{"oak_stairs[half=top,facing=east]", "minecraft:oak_stairs[facing=east,half=top]", false},
		{"lever[]", "minecraft:lever", false},
		{"", "", true},
		{"stone[facing=north", "", true},
		{"stone[facing]", "", true},
	}
	for _, tt := range tests {
		bs, err := ParseBlockState(tt.in)
		if tt.err {
			if err == nil {
				t.Errorf("ParseBlockState(%q) = %v, want error", tt.in, bs)
			}
			continue
		}
		if err != nil || bs.String() != tt.want {
			t.Errorf("ParseBlockState(%q) = %v, %v; want %s", tt.in, bs, err, tt.want)
		}
	}
}

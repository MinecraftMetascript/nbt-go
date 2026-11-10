package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MinecraftMetascript/nbt-go"
	"github.com/MinecraftMetascript/nbt-go/structure"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpSession = *mcp.ClientSession

func wantContains(t *testing.T, what, got string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("%s missing %q:\n%s", what, w, got)
		}
	}
}

func TestPlaceJigsaw(t *testing.T) {
	cs := connect(t, t.TempDir())
	call(t, cs, "create_structure", map[string]any{"name": "floor"}, false)
	call(t, cs, "fill", map[string]any{"name": "floor", "from": []int{0, 0, 0}, "to": []int{4, 0, 4}, "block": "stone_bricks"}, false)
	out := call(t, cs, "place_jigsaw", map[string]any{
		"name": "floor", "pos": []int{2, 0, 2}, "facing": "up", "jigsaw_name": "start", "target": "mypack:top", "pool": "mypack:tops",
	}, false)
	wantContains(t, "place_jigsaw", out, "orientation up_north", "final_state minecraft:stone_bricks (the block it replaced)",
		"name minecraft:start", "target mypack:top", "joint rollable", "(2, 1, 2)")
	// A jigsaw it just placed correctly gets no complaints about its own data.
	for _, bad := range []string{"no block entity data", "orientation property"} {
		if strings.Contains(out, bad) {
			t.Errorf("place_jigsaw warned %q about the jigsaw it just placed:\n%s", bad, out)
		}
	}

	// A jigsaw in empty space copies the block below it.
	out = call(t, cs, "place_jigsaw", map[string]any{"name": "floor", "pos": []int{0, 1, 0}, "facing": "east", "target": "a", "pool": "b"}, false)
	wantContains(t, "place_jigsaw", out, "orientation east_up", "final_state minecraft:stone_bricks (the block below it)", "joint aligned")

	call(t, cs, "place_jigsaw", map[string]any{"name": "floor", "pos": []int{0, 0, 0}, "facing": "east", "top": "north"}, true)
	call(t, cs, "place_jigsaw", map[string]any{"name": "floor", "pos": []int{0, 0, 0}, "facing": "sideways"}, true)

	// An explicit air final_state in a solid floor is flagged.
	out = call(t, cs, "place_jigsaw", map[string]any{"name": "floor", "pos": []int{1, 0, 1}, "facing": "up", "final_state": "air", "target": "a", "pool": "b"}, false)
	wantContains(t, "place_jigsaw", out, "leave a hole")

	desc := call(t, cs, "describe_structure", map[string]any{"name": "floor"}, false)
	wantContains(t, "describe_structure", desc, "Jigsaws (3):", "(2, 0, 2) up_north: name minecraft:start", "probably leave a hole")
}

// plinth builds a 3x5x3 plinth: two layers of stone, air above, and a jigsaw
// in the middle of the top stone layer that faces up into the air.
func plinth(t *testing.T, cs mcpSession, airHeight int) {
	call(t, cs, "create_structure", map[string]any{"name": "plinth"}, false)
	call(t, cs, "fill", map[string]any{"name": "plinth", "from": []int{0, 0, 0}, "to": []int{2, 1, 2}, "block": "stone"}, false)
	call(t, cs, "fill", map[string]any{"name": "plinth", "from": []int{0, 2, 0}, "to": []int{2, 1 + airHeight, 2}, "block": "air"}, false)
	call(t, cs, "place_jigsaw", map[string]any{"name": "plinth", "pos": []int{1, 1, 1}, "facing": "up", "target": "test:statue_bottom", "pool": "test:statues"}, false)

	call(t, cs, "create_structure", map[string]any{"name": "statue"}, false)
	call(t, cs, "fill", map[string]any{"name": "statue", "from": []int{0, 0, 0}, "to": []int{0, 3, 0}, "block": "gold_block"}, false)
	call(t, cs, "place_jigsaw", map[string]any{"name": "statue", "pos": []int{0, 0, 0}, "facing": "down", "jigsaw_name": "test:statue_bottom"}, false)
}

var statuePools = map[string]any{
	"test:statues": map[string]any{"fallback": "minecraft:empty", "elements": []any{
		map[string]any{"weight": 1, "element": map[string]any{"element_type": "minecraft:single_pool_element", "location": "statue", "projection": "rigid", "processors": "minecraft:empty"}},
	}},
}

func TestJigsawFitTooSmall(t *testing.T) {
	cs := connect(t, t.TempDir())
	plinth(t, cs, 3) // room for 3 blocks, the statue is 4 tall

	desc := call(t, cs, "describe_structure", map[string]any{"name": "plinth"}, false)
	wantContains(t, "describe_structure", desc, "connects inside this piece")

	out := call(t, cs, "check_jigsaw_fit", map[string]any{"parent": "plinth", "child": "statue"}, false)
	wantContains(t, "check_jigsaw_fit", out, "connects inside the parent", "DOES NOT FIT, it sticks out 1 too far up (+y)")
	if strings.Contains(out, ": fits.") {
		t.Errorf("no rotation should fit:\n%s", out)
	}

	out = call(t, cs, "assemble_jigsaw", map[string]any{"into": "layout", "start": "plinth", "pools": statuePools, "seed": 1}, false)
	wantContains(t, "assemble_jigsaw", out, "Assembled 1 pieces", "nothing placed; every option was blocked", "doesn't fit inside the parent", "1 too far up")
}

func TestJigsawAssembleInside(t *testing.T) {
	cs := connect(t, t.TempDir())
	plinth(t, cs, 4)
	out := call(t, cs, "check_jigsaw_fit", map[string]any{"parent": "plinth", "child": "statue"}, false)
	wantContains(t, "check_jigsaw_fit", out, ": fits.")

	out = call(t, cs, "assemble_jigsaw", map[string]any{"into": "layout", "start": "plinth", "pools": statuePools, "seed": 1}, false)
	wantContains(t, "assemble_jigsaw", out, "Assembled 2 pieces", "size 3 x 6 x 3", "#1 minecraft:statue", "attached to #0 by the jigsaw at (1, 1, 1)")
	desc := call(t, cs, "describe_structure", map[string]any{"name": "layout"}, false)
	// The plinth's jigsaw became stone, the statue's jigsaw became gold.
	wantContains(t, "describe_structure", desc, "18  minecraft:stone", "4  minecraft:gold_block")
	if strings.Contains(desc, "jigsaw") {
		t.Errorf("jigsaws should be replaced:\n%s", desc)
	}
}

// hallway builds a start room with a jigsaw on its east wall and a hall
// piece with a matching jigsaw on its west end.
func hallway(t *testing.T, cs mcpSession) {
	call(t, cs, "create_structure", map[string]any{"name": "room"}, false)
	call(t, cs, "fill", map[string]any{"name": "room", "from": []int{0, 0, 0}, "to": []int{2, 2, 2}, "block": "oak_planks", "mode": "hollow"}, false)
	call(t, cs, "set_blocks", map[string]any{"name": "room", "blocks": []map[string]any{{"x": 0, "y": 1, "z": 1, "block": "oak_stairs[facing=east,half=bottom]"}}}, false)
	call(t, cs, "place_jigsaw", map[string]any{"name": "room", "pos": []int{2, 1, 1}, "facing": "east", "target": "test:hall", "pool": "test:halls", "final_state": "air"}, false)

	call(t, cs, "create_structure", map[string]any{"name": "hall"}, false)
	call(t, cs, "fill", map[string]any{"name": "hall", "from": []int{0, 0, 0}, "to": []int{4, 2, 2}, "block": "cobblestone", "mode": "hollow"}, false)
	call(t, cs, "place_jigsaw", map[string]any{"name": "hall", "pos": []int{0, 1, 1}, "facing": "west", "jigsaw_name": "test:hall", "final_state": "air"}, false)
	call(t, cs, "place_jigsaw", map[string]any{"name": "hall", "pos": []int{4, 1, 1}, "facing": "east", "target": "test:hall", "pool": "test:halls", "final_state": "air"}, false)
}

var hallPools = map[string]any{
	"test:halls": map[string]any{"fallback": "test:ends", "elements": []any{
		map[string]any{"weight": 1, "element": map[string]any{"element_type": "minecraft:single_pool_element", "location": "hall", "projection": "rigid"}},
	}},
	"test:ends": map[string]any{"elements": []any{}},
}

func TestJigsawAssembleChain(t *testing.T) {
	cs := connect(t, t.TempDir())
	hallway(t, cs)
	out := call(t, cs, "assemble_jigsaw", map[string]any{"into": "layout", "start": "room", "pools": hallPools, "max_depth": 2, "seed": 7}, false)
	wantContains(t, "assemble_jigsaw", out, "Assembled 3 pieces", "size 13 x 3 x 3", "#2 minecraft:hall, rotation none, depth 2",
		"max_depth reached, so only the fallback pool test:ends was tried")
	desc := call(t, cs, "describe_structure", map[string]any{"name": "layout"}, false)
	if strings.Contains(desc, "jigsaw") {
		t.Errorf("jigsaws should be replaced:\n%s", desc)
	}

	// Rotated, the chain runs south and block states turn with it.
	out = call(t, cs, "assemble_jigsaw", map[string]any{"into": "rotated", "start": "room", "pools": hallPools, "max_depth": 1, "start_rotation": "clockwise_90", "seed": 7}, false)
	wantContains(t, "assemble_jigsaw", out, "Assembled 2 pieces", "size 3 x 3 x 8", "#1 minecraft:hall, rotation clockwise_90")
	desc = call(t, cs, "describe_structure", map[string]any{"name": "rotated"}, false)
	wantContains(t, "describe_structure", desc, "minecraft:oak_stairs[facing=south,half=bottom]")
}

func TestJigsawAssembleErrors(t *testing.T) {
	cs := connect(t, t.TempDir())
	hallway(t, cs)
	out := call(t, cs, "assemble_jigsaw", map[string]any{"into": "x", "start": "room", "seed": 1}, false)
	wantContains(t, "assemble_jigsaw", out, "template pool test:halls not found")
	call(t, cs, "assemble_jigsaw", map[string]any{"into": "x", "start": "nope"}, true)
	call(t, cs, "assemble_jigsaw", map[string]any{"into": "x"}, true)
	// A hall whose jigsaw faces the wrong way can't attach.
	call(t, cs, "place_jigsaw", map[string]any{"name": "hall", "pos": []int{0, 1, 1}, "facing": "up", "jigsaw_name": "test:hall"}, false)
	out = call(t, cs, "check_jigsaw_fit", map[string]any{"parent": "room", "child": "hall"}, false)
	wantContains(t, "check_jigsaw_fit", out, "it faces up, but must face west")
}

func TestRotateState(t *testing.T) {
	cases := []struct{ in, want string }{
		{"oak_stairs[facing=north,half=top]", "minecraft:oak_stairs[facing=east,half=top]"},
		{"oak_log[axis=x]", "minecraft:oak_log[axis=z]"},
		{"oak_sign[rotation=14]", "minecraft:oak_sign[rotation=2]"},
		{"oak_fence[east=true,north=false,south=false,west=true]", "minecraft:oak_fence[east=false,north=true,south=true,west=false]"},
		{"rail[shape=north_east]", "minecraft:rail[shape=south_east]"},
		{"rail[shape=ascending_west]", "minecraft:rail[shape=ascending_north]"},
		{"jigsaw[orientation=up_west]", "minecraft:jigsaw[orientation=up_north]"},
	}
	for _, c := range cases {
		got := rotateState(structure.MustParseBlockState(c.in), 1).String()
		if got != c.want {
			t.Errorf("rotate %s = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestJigsawDefaults(t *testing.T) {
	j := parseJigsaw(vec{}, structure.MustParseBlockState("jigsaw[orientation=north_up]"), nbt.Compound{"target": nbt.String("a")})
	if j.joint != "aligned" || j.pool != emptyID || j.target != "minecraft:a" || j.finalState != "minecraft:air" {
		t.Errorf("defaults: %+v", j)
	}
}

func TestJigsawAssembleDatapack(t *testing.T) {
	dir := t.TempDir()
	cs := connect(t, dir)
	plinth(t, cs, 4)
	call(t, cs, "save_structure", map[string]any{"name": "plinth", "path": "pack/data/test/structure/plinth.nbt"}, false)
	call(t, cs, "save_structure", map[string]any{"name": "statue", "path": "pack/data/test/structure/monuments/statue.nbt"}, false)
	pools := filepath.Join(dir, "pack", "data", "test", "worldgen", "template_pool")
	if err := os.MkdirAll(pools, 0o755); err != nil {
		t.Fatal(err)
	}
	pool := `{"fallback": "minecraft:empty", "elements": [{"weight": 3, "element": {"element_type": "minecraft:single_pool_element", "location": "test:monuments/statue", "projection": "rigid", "processors": "minecraft:empty"}}]}`
	if err := os.WriteFile(filepath.Join(pools, "statues.json"), []byte(pool), 0o644); err != nil {
		t.Fatal(err)
	}
	start := `{"elements": [{"weight": 1, "element": {"element_type": "minecraft:single_pool_element", "location": "test:plinth", "projection": "rigid"}}]}`
	if err := os.WriteFile(filepath.Join(pools, "start.json"), []byte(start), 0o644); err != nil {
		t.Fatal(err)
	}
	// Close the in-memory copies so the files are used.
	call(t, cs, "create_structure", map[string]any{"name": "statue"}, false)
	out := call(t, cs, "assemble_jigsaw", map[string]any{"into": "layout", "start_pool": "test:start", "datapack": "pack", "seed": 3}, false)
	wantContains(t, "assemble_jigsaw", out, "Assembled 2 pieces", "#0 test:plinth", "#1 test:monuments/statue")
}

package main

import (
	"strings"
	"testing"
)

func TestLint(t *testing.T) {
	cs := connect(t, t.TempDir())
	// 1.21.4, so that iron_chain (1.21.9) is flagged as renamed.
	call(t, cs, "create_structure", map[string]any{"name": "s", "data_version": 4189}, false)
	call(t, cs, "fill", map[string]any{"name": "s", "from": []int{0, 0, 0}, "to": []int{4, 0, 4}, "block": "stone"}, false)
	call(t, cs, "set_blocks", map[string]any{"name": "s", "blocks": []map[string]any{
		{"x": 0, "y": 1, "z": 0, "block": "cobblestone_wall[east=tall,north=none,south=none,west=none,up=true]"},
		{"x": 1, "y": 1, "z": 0, "block": "air"},
		{"x": 2, "y": 1, "z": 0, "block": "cobblestone_wall[east=none,north=none,south=none,west=none,up=true]"},
		{"x": 3, "y": 1, "z": 0, "block": "cobblestone_wall[east=none,north=none,south=none,west=low,up=true]"},
		{"x": 4, "y": 2, "z": 4, "block": "lantern[hanging=true]"},
		{"x": 4, "y": 3, "z": 4, "block": "air"},
		{"x": 4, "y": 1, "z": 4, "block": "air"},
		{"x": 2, "y": 1, "z": 2, "block": "wall_torch[facing=north]"},
		{"x": 1, "y": 1, "z": 4, "block": "chest[facing=north]"},
		{"x": 0, "y": 1, "z": 2, "block": "chest[facing=north]", "nbt": `{LootTable:"minecraft:chests/simple_dungeon"}`},
		{"x": 4, "y": 1, "z": 2, "block": "gold_block"},
		{"x": 5, "y": 1, "z": 2, "block": "gold_block"},
		{"x": 0, "y": 4, "z": 4, "block": "stone"},
		{"x": 3, "y": 1, "z": 3, "block": "iron_chain"},
		{"x": 1, "y": 1, "z": 1, "block": "structure_void"},
	}}, false)
	call(t, cs, "add_entity", map[string]any{"name": "s", "pos": []float64{2.5, 1, 3.5}, "id": "armor_stand", "nbt": "{UUID:[I;1,2,3,4]}"}, false)
	call(t, cs, "place_jigsaw", map[string]any{"name": "s", "pos": []int{0, 0, 4}, "facing": "west", "pool": "x:y"}, false)

	out := call(t, cs, "lint_structure", map[string]any{"name": "s"}, false)
	wantContains(t, "lint", out,
		"format (2)", "structure_void", "fixed UUID",
		"renamed (1)", "iron_chain: this id only exists from 1.21.9",
		"connections (2)", "east=tall but to the east is air", "east=none but to the east is minecraft:cobblestone_wall",
		"support (2)", "lantern[hanging=true] needs a block to the up at (4, 3, 4), but there's only air",
		"wall_torch[facing=north] needs a block to the south at (2, 1, 3), but the structure has no block there",
		"unsupported (1)", "(5, 1, 2) 1 block(s)",
		"floating (2)", "(0, 4, 4) a group of 1 block(s)",
		"containers (1)", "(1, 1, 4) minecraft:chest has no LootTable",
		"valuables (1)", "2 × minecraft:gold_block",
		"jigsaw (1)", "no target",
	)

	// The third wall connects west to the second, which doesn't connect back.
	if strings.Contains(out, "(3, 1, 0) minecraft:cobblestone_wall") {
		t.Errorf("wall at (3,1,0) is consistent with its neighbours:\n%s", out)
	}

	out = call(t, cs, "lint_structure", map[string]any{"name": "s", "checks": []string{"valuables"}, "limit": 1}, false)
	wantContains(t, "lint valuables", out, "1 findings")
	call(t, cs, "lint_structure", map[string]any{"name": "s", "checks": []string{"nope"}}, true)
}

func TestLintClean(t *testing.T) {
	cs := connect(t, t.TempDir())
	call(t, cs, "create_structure", map[string]any{"name": "hut"}, false)
	call(t, cs, "fill", map[string]any{"name": "hut", "from": []int{0, 0, 0}, "to": []int{4, 3, 4}, "block": "oak_planks", "mode": "hollow"}, false)
	call(t, cs, "set_blocks", map[string]any{"name": "hut", "blocks": []map[string]any{
		{"x": 2, "y": 1, "z": 0, "block": "oak_door[facing=south,half=lower]"},
		{"x": 2, "y": 2, "z": 0, "block": "oak_door[facing=south,half=upper]"},
		{"x": 2, "y": 2, "z": 3, "block": "wall_torch[facing=north]"},
		{"x": 1, "y": 1, "z": 3, "block": "barrel[facing=up]", "nbt": `{LootTable:"minecraft:chests/village/village_plains_house"}`},
		{"x": 3, "y": 1, "z": 1, "block": "oak_fence[north=true,south=false,east=true,west=false]"},
		{"x": 2, "y": 1, "z": 2, "block": "lantern[hanging=false]"},
	}}, false)
	out := call(t, cs, "lint_structure", map[string]any{"name": "hut"}, false)
	if !strings.Contains(out, ": 0 findings") {
		t.Errorf("clean hut has findings:\n%s", out)
	}
}

func TestLintGravityHalvesFire(t *testing.T) {
	cs := connect(t, t.TempDir())
	call(t, cs, "create_structure", map[string]any{"name": "s"}, false)
	call(t, cs, "fill", map[string]any{"name": "s", "from": []int{0, 0, 0}, "to": []int{6, 0, 6}, "block": "stone"}, false)
	call(t, cs, "set_blocks", map[string]any{"name": "s", "blocks": []map[string]any{
		// Sand over air falls; gravel on stone and sand on the bottom layer are fine.
		{"x": 0, "y": 2, "z": 0, "block": "sand"},
		{"x": 0, "y": 1, "z": 0, "block": "air"},
		{"x": 1, "y": 1, "z": 0, "block": "gravel"},
		{"x": 2, "y": 2, "z": 0, "block": "red_concrete_powder"},
		{"x": 2, "y": 1, "z": 0, "block": "short_grass"},
		// A door with no top, a whole door, a bed with no head, a whole bed.
		{"x": 0, "y": 1, "z": 2, "block": "oak_door[facing=south,half=lower,hinge=left]"},
		{"x": 2, "y": 1, "z": 2, "block": "oak_door[facing=south,half=lower,hinge=left]"},
		{"x": 2, "y": 2, "z": 2, "block": "oak_door[facing=south,half=upper,hinge=left]"},
		{"x": 4, "y": 1, "z": 2, "block": "red_bed[facing=east,part=foot]"},
		{"x": 4, "y": 1, "z": 4, "block": "red_bed[facing=east,part=foot]"},
		{"x": 5, "y": 1, "z": 4, "block": "red_bed[facing=east,part=head]"},
		{"x": 6, "y": 2, "z": 0, "block": "sunflower[half=upper]"},
		// Fire beside planks; lava beside cobblestone is fine.
		{"x": 0, "y": 1, "z": 6, "block": "fire"},
		{"x": 1, "y": 1, "z": 6, "block": "oak_planks"},
		{"x": 4, "y": 1, "z": 6, "block": "lava"},
		{"x": 5, "y": 1, "z": 6, "block": "mossy_cobblestone"},
	}}, false)
	out := call(t, cs, "lint_structure", map[string]any{"name": "s", "checks": []string{"gravity", "halves", "fire"}}, false)
	wantContains(t, "lint", out,
		"gravity (2)", "(0, 2, 0) minecraft:sand is above air", "(2, 2, 0) minecraft:red_concrete_powder is above minecraft:short_grass",
		"halves (3)", "(0, 1, 2) minecraft:oak_door[facing=south,half=lower,hinge=left] needs its upper half at (0, 2, 2), but there is no block",
		"(4, 1, 2) minecraft:red_bed[facing=east,part=foot] needs its head at (5, 1, 2)",
		"(6, 2, 0) minecraft:sunflower[half=upper] needs its lower half at (6, 1, 0)",
		"fire (1)", "(0, 1, 6) minecraft:fire is within reach of 1 flammable block(s), such as minecraft:oak_planks",
	)
}

func TestLintNewerBlocks(t *testing.T) {
	cs := connect(t, t.TempDir())
	call(t, cs, "create_structure", map[string]any{"name": "old", "data_version": 4189}, false)
	call(t, cs, "fill", map[string]any{"name": "old", "from": []int{0, 0, 0}, "to": []int{2, 0, 0}, "block": "poplar_planks"}, false)
	call(t, cs, "set_blocks", map[string]any{"name": "old", "blocks": []map[string]any{
		{"x": 0, "y": 1, "z": 0, "block": "sulfur_bricks"},
		{"x": 1, "y": 1, "z": 0, "block": "stone"},
	}}, false)
	out := call(t, cs, "lint_structure", map[string]any{"name": "old", "checks": []string{"newer"}}, false)
	wantContains(t, "lint newer", out,
		"newer (2)",
		"3 × minecraft:poplar_planks: added in 26.3 (DataVersion 5023), but the file says DataVersion 4189 (1.21.4)",
		"1 × minecraft:sulfur_bricks: added in 26.2 (DataVersion 4903)",
	)

	call(t, cs, "create_structure", map[string]any{"name": "new", "data_version": 5023}, false)
	call(t, cs, "fill", map[string]any{"name": "new", "from": []int{0, 0, 0}, "to": []int{2, 0, 0}, "block": "poplar_planks"}, false)
	out = call(t, cs, "lint_structure", map[string]any{"name": "new", "checks": []string{"newer"}}, false)
	if strings.Contains(out, "poplar_planks") {
		t.Errorf("poplar_planks is fine at DataVersion 5023:\n%s", out)
	}
}

func TestAddedBlocksAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range addedBlocks {
		if i := indexOfVersion(a.version); i < 0 || knownVersions[i].dataVersion != a.dataVersion {
			t.Errorf("%s/%d isn't in knownVersions", a.version, a.dataVersion)
		}
		for _, id := range a.ids {
			if seen[id] {
				t.Errorf("%s listed twice", id)
			}
			seen[id] = true
		}
	}
}

func TestLintSupportJigsawAndFence(t *testing.T) {
	cs := connect(t, t.TempDir())
	call(t, cs, "create_structure", map[string]any{"name": "s"}, false)
	call(t, cs, "fill", map[string]any{"name": "s", "from": []int{0, 0, 0}, "to": []int{3, 0, 0}, "block": "stone_bricks"}, false)
	// A rail and a door stand on jigsaws that become stone bricks.
	call(t, cs, "place_jigsaw", map[string]any{"name": "s", "pos": []int{0, 0, 0}, "facing": "north", "target": "a", "pool": "b", "final_state": "stone_bricks"}, false)
	call(t, cs, "place_jigsaw", map[string]any{"name": "s", "pos": []int{1, 0, 0}, "facing": "north", "target": "a", "pool": "b", "final_state": "stone_bricks"}, false)
	// This one becomes air, so the rail on it really has nothing under it.
	call(t, cs, "place_jigsaw", map[string]any{"name": "s", "pos": []int{3, 0, 0}, "facing": "north", "target": "a", "pool": "b", "final_state": "air"}, false)
	call(t, cs, "set_blocks", map[string]any{"name": "s", "blocks": []map[string]any{
		{"x": 0, "y": 1, "z": 0, "block": "rail"},
		{"x": 1, "y": 1, "z": 0, "block": "oak_door[facing=north,half=lower]"},
		{"x": 1, "y": 2, "z": 0, "block": "oak_door[facing=north,half=upper]"},
		{"x": 2, "y": 1, "z": 0, "block": "oak_fence"},
		{"x": 2, "y": 2, "z": 0, "block": "oak_pressure_plate"},
		{"x": 3, "y": 1, "z": 0, "block": "rail"},
	}}, false)
	out := call(t, cs, "lint_structure", map[string]any{"name": "s", "checks": []string{"support"}}, false)
	wantContains(t, "lint_structure", out, "support (1)", "(3, 1, 0) minecraft:rail")
	for _, bad := range []string{"(0, 1, 0)", "(1, 1, 0)", "pressure_plate"} {
		if strings.Contains(out, bad) {
			t.Errorf("support lint flagged %s:\n%s", bad, out)
		}
	}
}

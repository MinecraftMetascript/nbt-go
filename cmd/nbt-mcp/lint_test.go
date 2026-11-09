package main

import (
	"strings"
	"testing"
)

func TestLint(t *testing.T) {
	cs := connect(t, t.TempDir())
	call(t, cs, "create_structure", map[string]any{"name": "s"}, false)
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

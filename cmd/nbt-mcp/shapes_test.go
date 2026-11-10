package main

import (
	"strings"
	"testing"
)

func TestSphere(t *testing.T) {
	cs := connect(t, t.TempDir())
	call(t, cs, "create_structure", map[string]any{"name": "s"}, false)
	out := call(t, cs, "fill_ellipsoid", map[string]any{"name": "s", "center": []float64{2, 2, 2}, "radius": 2, "block": "stone"}, false)
	wantContains(t, "fill_ellipsoid", out, "sphere", "with 33 blocks", "[5 5 5]")
	call(t, cs, "create_structure", map[string]any{"name": "dome"}, false)
	out = call(t, cs, "fill_ellipsoid", map[string]any{"name": "dome", "center": []float64{2, 0, 2}, "radius": 2, "block": "stone"}, false)
	wantContains(t, "fill_ellipsoid", out, "with 23 blocks", "[5 3 5]", "part below 0 was cut off")
	call(t, cs, "fill_ellipsoid", map[string]any{"name": "s", "center": []float64{-5, 1, 1}, "radius": 2, "block": "stone"}, true) // all below 0
	call(t, cs, "fill_ellipsoid", map[string]any{"name": "s", "center": []float64{3, 3, 3}, "block": "stone"}, true)               // no radius
}

func TestHollowShellsAreSealed(t *testing.T) {
	cs := connect(t, t.TempDir())
	call(t, cs, "create_structure", map[string]any{"name": "dome"}, false)
	out := call(t, cs, "fill_ellipsoid", map[string]any{"name": "dome", "center": []float64{7, 6, 7}, "radii": []float64{7, 6, 5.5}, "block": "glass", "hollow": true}, false)
	wantContains(t, "fill_ellipsoid", out, "ellipsoid", "shell 1 thick", "air inside")
	out = call(t, cs, "check_sealed", map[string]any{"name": "dome", "inside": []int{7, 6, 7}}, false)
	wantContains(t, "check_sealed", out, "Sealed")

	// A slab stops water but is a gap for anything else.
	call(t, cs, "set_blocks", map[string]any{"name": "dome", "blocks": []map[string]any{{"x": 7, "y": 12, "z": 7, "block": "oak_slab"}}}, false)
	out = call(t, cs, "check_sealed", map[string]any{"name": "dome", "inside": []int{7, 6, 7}}, false)
	wantContains(t, "check_sealed", out, "Sealed", "Checked against water")
	out = call(t, cs, "check_sealed", map[string]any{"name": "dome", "inside": []int{7, 6, 7}, "against": "gaps"}, false)
	wantContains(t, "check_sealed", out, "Not sealed", "likely gap is at (7, 12, 7)", "oak_slab", "Checked for any gap")

	// A torch lets water through, and so does a waterlogged slab.
	call(t, cs, "set_blocks", map[string]any{"name": "dome", "blocks": []map[string]any{{"x": 7, "y": 12, "z": 7, "block": "oak_slab[waterlogged=true]"}}}, false)
	out = call(t, cs, "check_sealed", map[string]any{"name": "dome", "inside": []int{7, 6, 7}}, false)
	wantContains(t, "check_sealed", out, "Not sealed", "likely gap is at (7, 12, 7)")
	call(t, cs, "set_blocks", map[string]any{"name": "dome", "blocks": []map[string]any{{"x": 7, "y": 12, "z": 7, "block": "torch"}}}, false)
	out = call(t, cs, "check_sealed", map[string]any{"name": "dome", "inside": []int{7, 6, 7}}, false)
	wantContains(t, "check_sealed", out, "Not sealed", "minecraft:torch")
	call(t, cs, "check_sealed", map[string]any{"name": "dome", "inside": []int{7, 6, 7}, "against": "lava"}, true)

	out = call(t, cs, "check_sealed", map[string]any{"name": "dome", "inside": []int{0, 0, 0}}, false)
	wantContains(t, "check_sealed", out, "Not sealed")
	call(t, cs, "check_sealed", map[string]any{"name": "dome", "inside": []int{7, 0, 7}}, true) // solid
}

func TestSealedDoors(t *testing.T) {
	cs := connect(t, t.TempDir())
	call(t, cs, "create_structure", map[string]any{"name": "room"}, false)
	call(t, cs, "fill", map[string]any{"name": "room", "from": []int{0, 0, 0}, "to": []int{4, 4, 4}, "block": "stone", "mode": "hollow"}, false)
	call(t, cs, "set_blocks", map[string]any{"name": "room", "blocks": []map[string]any{
		{"x": 2, "y": 1, "z": 0, "block": "oak_door[facing=south,half=lower,open=false]"},
		{"x": 2, "y": 2, "z": 0, "block": "oak_door[facing=south,half=upper,open=false]"},
		{"x": 2, "y": 4, "z": 2, "block": "oak_trapdoor[half=top,open=false]"},
	}}, false)
	for _, against := range []string{"water", "gaps"} {
		out := call(t, cs, "check_sealed", map[string]any{"name": "room", "inside": []int{2, 2, 2}, "against": against}, false)
		wantContains(t, "closed doors, against "+against, out, "Sealed")
	}
	// An open door is a gap, but still keeps water out.
	call(t, cs, "set_blocks", map[string]any{"name": "room", "blocks": []map[string]any{
		{"x": 2, "y": 1, "z": 0, "block": "oak_door[facing=south,half=lower,open=true]"},
	}}, false)
	out := call(t, cs, "check_sealed", map[string]any{"name": "room", "inside": []int{2, 2, 2}, "against": "gaps"}, false)
	wantContains(t, "open door", out, "Not sealed", "oak_door")
	out = call(t, cs, "check_sealed", map[string]any{"name": "room", "inside": []int{2, 2, 2}}, false)
	wantContains(t, "open door, water", out, "Sealed")
}

func TestCylinders(t *testing.T) {
	cs := connect(t, t.TempDir())
	call(t, cs, "create_structure", map[string]any{"name": "c"}, false)
	out := call(t, cs, "fill_cylinder", map[string]any{"name": "c", "base": []float64{3, 0, 3}, "radius": 3, "height": 1, "block": "stone"}, false)
	wantContains(t, "fill_cylinder", out, "disc", "y=0..0")
	out = call(t, cs, "fill_cylinder", map[string]any{"name": "c", "base": []float64{3, 1, 3}, "radius": 3, "height": 5, "block": "stone", "hollow": true, "open_ends": true}, false)
	wantContains(t, "fill_cylinder", out, "tube")
	// The disc closes the bottom, but the top is open.
	out = call(t, cs, "check_sealed", map[string]any{"name": "c", "inside": []int{3, 2, 3}}, false)
	wantContains(t, "check_sealed", out, "Not sealed", "leaving at (3, 5, 3)")

	call(t, cs, "create_structure", map[string]any{"name": "pipe"}, false)
	out = call(t, cs, "fill_cylinder", map[string]any{"name": "pipe", "base": []float64{0, 2, 2}, "axis": "x", "radii": []float64{2, 2}, "height": 6, "block": "copper_block"}, false)
	wantContains(t, "fill_cylinder", out, "x=0..5 y=0..4 z=0..4")
}

func TestTube(t *testing.T) {
	cs := connect(t, t.TempDir())
	call(t, cs, "create_structure", map[string]any{"name": "tree"}, false)
	out := call(t, cs, "fill_tube", map[string]any{"name": "tree", "points": [][]float64{{3, 2, 3}, {3, 8, 3}, {6, 11, 3}}, "radii": []float64{2, 1, 0.5}, "block": "oak_wood"}, false)
	wantContains(t, "fill_tube", out, "tube", "x=1..6 y=0..11")
	call(t, cs, "fill_tube", map[string]any{"name": "tree", "points": [][]float64{{3, 0, 3}}, "radii": []float64{1, 2}, "block": "oak_wood"}, true)
}

func TestPatternsAndSeeds(t *testing.T) {
	cs := connect(t, t.TempDir())
	mix := "60%stone_bricks,30%cracked_stone_bricks,10%mossy_stone_bricks"
	var descs []string
	for _, name := range []string{"a", "b"} {
		call(t, cs, "create_structure", map[string]any{"name": name}, false)
		out := call(t, cs, "fill", map[string]any{"name": name, "from": []int{0, 0, 0}, "to": []int{9, 0, 9}, "block": mix, "seed": 42}, false)
		wantContains(t, "fill", out, "seed 42", "60%minecraft:stone_bricks")
		descs = append(descs, call(t, cs, "describe_structure", map[string]any{"name": name}, false))
	}
	if descs[0] != descs[1] {
		t.Errorf("same seed gave different results:\n%s\n%s", descs[0], descs[1])
	}
	wantContains(t, "describe_structure", descs[0], "minecraft:cracked_stone_bricks", "minecraft:mossy_stone_bricks")

	p, err := parsePattern("oak_stairs[facing=east,half=top],2*stone")
	if err != nil || len(p.states) != 2 || p.states[0].Properties["half"] != "top" || p.weights[1] != 20 || p.weights[0] != 10 {
		t.Errorf("parsePattern = %+v, %v", p, err)
	}
	if _, err := parsePattern("x%stone"); err == nil {
		t.Error("bad weight accepted")
	}
}

func TestPlaceGrid(t *testing.T) {
	cs := connect(t, t.TempDir())
	call(t, cs, "create_structure", map[string]any{"name": "g"}, false)
	out := call(t, cs, "place_grid", map[string]any{"name": "g", "origin": []int{1, 0, 1},
		"legend": map[string]string{"#": "stone_bricks", "D": "oak_door[facing=south,half=lower]"},
		"layers": [][]string{
			{"###", "###"},
			{"#D#", "# .", "#"},
		}, "ragged": true}, false)
	wantContains(t, "place_grid", out, "Placed 12 blocks from 2 layers", "[4 2 4]")
	layer := call(t, cs, "render_layer", map[string]any{"name": "g", "y": 1}, false)
	wantContains(t, "render_layer", layer, "   1 .#A#", "   2 .# .", "   3 .#..")
	call(t, cs, "place_grid", map[string]any{"name": "g", "origin": []int{0, 0, 0}, "legend": map[string]string{}, "layers": [][]string{{"x"}}}, true)

	// Without ragged, one row a character too long is refused.
	out = call(t, cs, "place_grid", map[string]any{"name": "g", "origin": []int{0, 0, 0}, "legend": map[string]string{"#": "stone"},
		"layers": [][]string{{"#######", "########"}}}, true)
	wantContains(t, "place_grid", out, "layers[0][1] is 8 characters wide but layers[0][0] is 7")
	out = call(t, cs, "place_grid", map[string]any{"name": "g", "origin": []int{0, 0, 0}, "legend": map[string]string{"#": "stone"},
		"layers": [][]string{{"##", "##"}, {"##"}}}, true)
	wantContains(t, "place_grid", out, "layers[1] has 1 rows but layers[0] has 2")
	// Growing an existing structure is pointed out.
	out = call(t, cs, "place_grid", map[string]any{"name": "g", "origin": []int{3, 0, 0}, "legend": map[string]string{"#": "stone"},
		"layers": [][]string{{"##"}}}, false)
	wantContains(t, "place_grid", out, "Size is now [5 2 4]. Note: the structure grew from [4 2 4]")
}

func TestDecorateSurface(t *testing.T) {
	cs := connect(t, t.TempDir())
	call(t, cs, "create_structure", map[string]any{"name": "d"}, false)
	call(t, cs, "fill", map[string]any{"name": "d", "from": []int{0, 0, 0}, "to": []int{2, 0, 2}, "block": "stone"}, false)
	call(t, cs, "set_blocks", map[string]any{"name": "d", "blocks": []map[string]any{{"x": 1, "y": 1, "z": 1, "block": "dirt"}}}, false)
	out := call(t, cs, "decorate_surface", map[string]any{"name": "d", "block": "moss_carpet", "only": []string{"stone"}}, false)
	wantContains(t, "decorate_surface", out, "Decorated 8 of 8")
	out = call(t, cs, "decorate_surface", map[string]any{"name": "d", "block": "grass_block", "mode": "replace", "only": []string{"dirt"}}, false)
	wantContains(t, "decorate_surface", out, "Decorated 1 of 1")
	desc := call(t, cs, "describe_structure", map[string]any{"name": "d"}, false)
	wantContains(t, "describe_structure", desc, "8  minecraft:moss_carpet", "1  minecraft:grass_block")
	if strings.Contains(desc, "dirt") {
		t.Errorf("dirt should be replaced:\n%s", desc)
	}
}

func TestTransforms(t *testing.T) {
	cs := connect(t, t.TempDir())
	call(t, cs, "create_structure", map[string]any{"name": "s"}, false)
	call(t, cs, "fill", map[string]any{"name": "s", "from": []int{0, 0, 0}, "to": []int{3, 0, 1}, "block": "stone"}, false)
	call(t, cs, "set_blocks", map[string]any{"name": "s", "blocks": []map[string]any{
		{"x": 3, "y": 1, "z": 0, "block": "oak_stairs[facing=east,shape=inner_left]"},
	}}, false)
	call(t, cs, "add_entity", map[string]any{"name": "s", "pos": []float64{3.5, 1, 0.5}, "id": "armor_stand"}, false)

	out := call(t, cs, "transform_structure", map[string]any{"name": "s", "rotation": "clockwise_90"}, false)
	wantContains(t, "transform_structure", out, "[2 2 4]")
	layer := call(t, cs, "render_layer", map[string]any{"name": "s", "y": 1}, false)
	wantContains(t, "render_layer", layer, "   3 .#", "oak_stairs[facing=south,shape=inner_left]")
	desc := call(t, cs, "describe_structure", map[string]any{"name": "s"}, false)
	wantContains(t, "describe_structure", desc, `armor_stand" at [1.5 1 3.5]`)

	call(t, cs, "transform_structure", map[string]any{"name": "s", "rotation": "counterclockwise_90", "mirror": "x"}, false)
	layer = call(t, cs, "render_layer", map[string]any{"name": "s", "y": 1}, false)
	wantContains(t, "render_layer", layer, "   1 ...#", "oak_stairs[facing=east,shape=inner_right]")

	// Paste two copies side by side, then crop and resize.
	call(t, cs, "create_structure", map[string]any{"name": "row"}, false)
	call(t, cs, "paste_structure", map[string]any{"source": "s", "target": "row", "at": []int{0, 0, 0}}, false)
	out = call(t, cs, "paste_structure", map[string]any{"source": "s", "target": "row", "at": []int{4, 0, 0}, "mirror": "x"}, false)
	wantContains(t, "paste_structure", out, "Pasted 9 blocks and 1 entities", "[8 2 2]")
	out = call(t, cs, "crop_structure", map[string]any{"name": "row", "from": []int{2, 0, 0}, "to": []int{5, 1, 1}, "into": "middle"}, false)
	wantContains(t, "crop_structure", out, "10 blocks and 2 entities, size [4 2 2]")
	call(t, cs, "set_size", map[string]any{"name": "middle", "size": []int{2, 2, 2}}, true)
	out = call(t, cs, "set_size", map[string]any{"name": "middle", "size": []int{2, 2, 2}, "drop": true}, false)
	wantContains(t, "set_size", out, "Removed 5 blocks and 1 entities")
	out = call(t, cs, "set_size", map[string]any{"name": "middle", "size": []int{2, 5, 2}}, false)
	wantContains(t, "set_size", out, "[2 5 2]")
}

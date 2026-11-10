package main

import (
	"strings"
	"testing"
)

func TestCheckLighting(t *testing.T) {
	cs := connect(t, t.TempDir())
	// A 15 x 15 room inside, 3 tall, with a stone floor and roof.
	call(t, cs, "create_structure", map[string]any{"name": "hall"}, false)
	call(t, cs, "fill", map[string]any{"name": "hall", "from": []int{0, 0, 0}, "to": []int{16, 4, 16}, "block": "stone", "mode": "hollow"}, false)

	out := call(t, cs, "check_lighting", map[string]any{"name": "hall"}, false)
	wantContains(t, "check_lighting", out, "225 roofed floor positions", "0 light sources", "By layer: y=1: 225", "Layer y=1")

	// One torch in the middle lights everything within 13 steps.
	call(t, cs, "set_blocks", map[string]any{"name": "hall", "blocks": []map[string]any{{"x": 8, "y": 1, "z": 8, "block": "torch"}}}, false)
	out = call(t, cs, "check_lighting", map[string]any{"name": "hall"}, false)
	// Corners are 14 steps away (7 + 7), so they stay dark.
	wantContains(t, "check_lighting", out, "4 roofed floor positions", "1 light sources", "(1, 1, 1) on minecraft:stone, light 0")

	// A lantern in the corner fixes that one, and carpet makes floor unspawnable.
	call(t, cs, "set_blocks", map[string]any{"name": "hall", "blocks": []map[string]any{
		{"x": 1, "y": 3, "z": 1, "block": "lantern[hanging=true]"},
		{"x": 15, "y": 1, "z": 15, "block": "white_carpet"},
		{"x": 1, "y": 1, "z": 15, "block": "oak_slab[type=bottom]"},
	}}, false)
	out = call(t, cs, "check_lighting", map[string]any{"name": "hall"}, false)
	wantContains(t, "check_lighting", out, "1 roofed floor positions", "(15, 1, 1)")
	if strings.Contains(out, "(1, 1, 1)") {
		t.Errorf("the lantern should light the corner:\n%s", out)
	}

	// Older versions spawn up to light 7.
	out = call(t, cs, "check_lighting", map[string]any{"name": "hall", "max_light": 7}, false)
	if strings.Contains(out, " 1 roofed") || strings.Contains(out, "Every such") {
		t.Errorf("max_light 7 should find more:\n%s", out)
	}
	call(t, cs, "check_lighting", map[string]any{"name": "hall", "max_light": 15}, true)

	// Open ground only counts with include_open.
	call(t, cs, "create_structure", map[string]any{"name": "yard"}, false)
	call(t, cs, "fill", map[string]any{"name": "yard", "from": []int{0, 0, 0}, "to": []int{2, 0, 2}, "block": "grass_block"}, false)
	call(t, cs, "fill", map[string]any{"name": "yard", "from": []int{0, 1, 0}, "to": []int{2, 2, 2}, "block": "air"}, false)
	wantContains(t, "check_lighting", call(t, cs, "check_lighting", map[string]any{"name": "yard"}, false), "0 roofed floor positions", "Every such")
	wantContains(t, "check_lighting", call(t, cs, "check_lighting", map[string]any{"name": "yard", "include_open": true}, false), "9 floor positions")
}

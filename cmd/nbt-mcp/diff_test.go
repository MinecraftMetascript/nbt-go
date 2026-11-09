package main

import "testing"

func TestDiffAndAirStats(t *testing.T) {
	cs := connect(t, t.TempDir())
	for _, n := range []string{"a", "b"} {
		call(t, cs, "create_structure", map[string]any{"name": n}, false)
		call(t, cs, "fill", map[string]any{"name": n, "from": []int{0, 0, 0}, "to": []int{3, 3, 3}, "block": "stone", "mode": "hollow"}, false)
	}
	desc := call(t, cs, "describe_structure", map[string]any{"name": "a"}, false)
	wantContains(t, "describe_structure", desc, "Of 64 positions: 56 blocks (87.5%), 8 air (12.5%), 0 empty")

	out := call(t, cs, "diff_structures", map[string]any{"before": "a", "after": "b"}, false)
	wantContains(t, "diff", out, "No blocks differ.")

	call(t, cs, "fill", map[string]any{"name": "b", "from": []int{1, 0, 1}, "to": []int{2, 0, 2}, "block": "air"}, false)
	call(t, cs, "remove_blocks", map[string]any{"name": "b", "from": []int{0, 3, 0}, "to": []int{0, 3, 0}}, false)
	call(t, cs, "set_blocks", map[string]any{"name": "b", "blocks": []map[string]any{{"x": 4, "y": 0, "z": 0, "block": "chest[facing=north]"}}}, false)
	out = call(t, cs, "diff_structures", map[string]any{"before": "a", "after": "b", "limit": 2}, false)
	wantContains(t, "diff", out, "Size changed from [4 4 4] to [5 4 4]", "6 positions differ, within x=0..4 y=0..3 z=0..2",
		"4  minecraft:stone → minecraft:air", "1  (no block) → minecraft:chest", "... and 1 more kinds",
		"(4, 0, 0) (no block) → minecraft:chest[facing=north]", "... and 4 more")
	desc = call(t, cs, "describe_structure", map[string]any{"name": "b"}, false)
	wantContains(t, "describe_structure", desc, "Of 80 positions: 52 blocks (65.0%), 12 air (15.0%), 16 empty")
}

package main

import (
	"strings"
	"testing"
)

func TestEntitiesStuckMoveRemove(t *testing.T) {
	cs := connect(t, t.TempDir())
	call(t, cs, "create_structure", map[string]any{"name": "home"}, false)
	call(t, cs, "fill", map[string]any{"name": "home", "from": []int{0, 0, 0}, "to": []int{4, 0, 4}, "block": "oak_planks"}, false)
	call(t, cs, "fill", map[string]any{"name": "home", "from": []int{1, 1, 1}, "to": []int{3, 1, 3}, "block": "red_carpet"}, false)
	call(t, cs, "fill", map[string]any{"name": "home", "from": []int{0, 4, 0}, "to": []int{4, 4, 4}, "block": "oak_planks"}, false)

	// A villager at y = 1.0 in a carpet cell sinks into the carpet.
	out := call(t, cs, "add_entity", map[string]any{"name": "home", "pos": []float64{2.5, 1, 2.5}, "id": "villager"}, false)
	wantContains(t, "add_entity", out, "as entity #0", "starts inside the collision box of minecraft:red_carpet at (2, 1, 2)", "stand it on top, at y = 1.0625")
	// On the planks with nothing in the way, it's fine.
	out = call(t, cs, "add_entity", map[string]any{"name": "home", "pos": []float64{0.5, 1, 0.5}, "id": "villager"}, false)
	if strings.Contains(out, "Warning") {
		t.Errorf("villager on planks shouldn't warn:\n%s", out)
	}
	// An item frame doesn't collide.
	out = call(t, cs, "add_entity", map[string]any{"name": "home", "pos": []float64{4.5, 2.5, 0.5}, "id": "item_frame"}, false)
	if strings.Contains(out, "Warning") {
		t.Errorf("item frame shouldn't warn:\n%s", out)
	}

	lint := call(t, cs, "lint_structure", map[string]any{"name": "home", "checks": []string{"entities"}}, false)
	wantContains(t, "lint_structure", lint, "entities (1)", "entity #0 (minecraft:villager)")

	out = call(t, cs, "move_entity", map[string]any{"name": "home", "index": 0, "by": []float64{0, 0.0625, 0}}, false)
	wantContains(t, "move_entity", out, "Moved entity #0 (minecraft:villager) from [2.5 1 2.5] to [2.5 1.0625 2.5]")
	if strings.Contains(out, "Warning") {
		t.Errorf("villager on the carpet shouldn't warn:\n%s", out)
	}
	lint = call(t, cs, "lint_structure", map[string]any{"name": "home", "checks": []string{"entities", "format"}}, false)
	wantContains(t, "lint_structure", lint, "0 findings")

	// Moving it up into the ceiling is flagged: a villager is 1.95 tall.
	out = call(t, cs, "move_entity", map[string]any{"name": "home", "near": []float64{2.5, 1, 2.5}, "id": "villager", "to": []float64{2.5, 2.5, 2.5}}, false)
	wantContains(t, "move_entity", out, "minecraft:oak_planks at (2, 4, 2)")

	out = call(t, cs, "remove_entity", map[string]any{"name": "home", "near": []float64{0.5, 1, 0.5}}, false)
	wantContains(t, "remove_entity", out, "Removed entity #1 (minecraft:villager)", "renumbered")
	desc := call(t, cs, "describe_structure", map[string]any{"name": "home"}, false)
	wantContains(t, "describe_structure", desc, "#0 \"minecraft:villager\" at [2.5 2.5 2.5]", "#1 \"minecraft:item_frame\"")

	call(t, cs, "move_entity", map[string]any{"name": "home", "index": 5, "by": []float64{0, 1, 0}}, true)
	call(t, cs, "move_entity", map[string]any{"name": "home", "index": 0}, true)
	call(t, cs, "remove_entity", map[string]any{"name": "home", "near": []float64{20, 20, 20}}, true)
	call(t, cs, "remove_entity", map[string]any{"name": "home"}, true)
}

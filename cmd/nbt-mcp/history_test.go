package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestUndoRedo(t *testing.T) {
	cs := connect(t, t.TempDir())
	blocks := func() string { return call(t, cs, "list_structures", nil, false) }

	call(t, cs, "create_structure", map[string]any{"name": "a"}, false)
	call(t, cs, "fill", map[string]any{"name": "a", "from": []int{0, 0, 0}, "to": []int{1, 1, 1}, "block": "stone"}, false)
	call(t, cs, "fill", map[string]any{"name": "a", "from": []int{0, 2, 0}, "to": []int{1, 2, 1}, "block": "glass"}, false)
	if got := blocks(); !strings.Contains(got, "12 blocks") {
		t.Fatalf("before undo: %s", got)
	}

	if got := call(t, cs, "undo", nil, false); !strings.Contains(got, "Undid fill on a") || !strings.Contains(got, "8 blocks") {
		t.Errorf("undo: %s", got)
	}
	if got := blocks(); !strings.Contains(got, "size 2x2x2, 8 blocks") {
		t.Errorf("after undo: %s", got)
	}
	if got := call(t, cs, "redo", nil, false); !strings.Contains(got, "Redid fill on a") {
		t.Errorf("redo: %s", got)
	}
	if got := blocks(); !strings.Contains(got, "size 2x3x2, 12 blocks") {
		t.Errorf("after redo: %s", got)
	}

	// Several steps at once, back past the structure's creation.
	if got := call(t, cs, "undo", map[string]any{"steps": 5}, false); !strings.Contains(got, `"a" is no longer open`) {
		t.Errorf("undo all: %s", got)
	}
	if got := blocks(); !strings.Contains(got, "No structures are open") {
		t.Errorf("after undoing everything: %s", got)
	}
	call(t, cs, "undo", nil, true)
	call(t, cs, "redo", map[string]any{"steps": 2}, false)
	if got := blocks(); !strings.Contains(got, "8 blocks") {
		t.Errorf("after redoing two: %s", got)
	}

	// A new edit ends the chance to redo.
	call(t, cs, "set_blocks", map[string]any{"name": "a", "blocks": []map[string]any{{"x": 0, "y": 0, "z": 0, "block": "dirt"}}}, false)
	call(t, cs, "redo", nil, true)
}

func TestUndoCoversEveryEditTool(t *testing.T) {
	cs := connect(t, t.TempDir())
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		a := tool.Annotations
		if a.ReadOnlyHint || *a.DestructiveHint || tool.Name == "undo" || tool.Name == "redo" {
			continue
		}
		props := tool.InputSchema.(map[string]any)["properties"].(map[string]any)
		found := false
		for _, key := range targetArgs {
			if _, ok := props[key]; ok {
				found = true
			}
		}
		if !found {
			t.Errorf("edit tool %s names its structure with none of %v, so undo can't snapshot it", tool.Name, targetArgs)
		}
	}
}

func TestUndoOtherEdits(t *testing.T) {
	cs := connect(t, t.TempDir())
	call(t, cs, "create_structure", map[string]any{"name": "src"}, false)
	call(t, cs, "fill", map[string]any{"name": "src", "from": []int{0, 0, 0}, "to": []int{3, 0, 3}, "block": "stone"}, false)
	call(t, cs, "create_structure", map[string]any{"name": "dst"}, false)
	before := call(t, cs, "describe_structure", map[string]any{"name": "src"}, false)

	// Each of these changes or creates a structure in its own way.
	call(t, cs, "paste_structure", map[string]any{"source": "src", "target": "dst", "at": []int{0, 0, 0}}, false)
	call(t, cs, "crop_structure", map[string]any{"name": "src", "from": []int{0, 0, 0}, "to": []int{1, 0, 1}, "into": "small"}, false)
	call(t, cs, "crop_structure", map[string]any{"name": "src", "from": []int{0, 0, 0}, "to": []int{1, 0, 1}}, false)
	call(t, cs, "transform_structure", map[string]any{"name": "src", "rotation": "clockwise_90"}, false)
	call(t, cs, "create_structure", map[string]any{"name": "src"}, false) // replaces it
	call(t, cs, "undo", map[string]any{"steps": 5}, false)

	if got := call(t, cs, "describe_structure", map[string]any{"name": "src"}, false); got != before {
		t.Errorf("src after undo:\n%s\nwant:\n%s", got, before)
	}
	list := call(t, cs, "list_structures", nil, false)
	if strings.Contains(list, "small") || !strings.Contains(list, "dst: size 0x0x0, 0 blocks") {
		t.Errorf("after undo: %s", list)
	}
}

func TestFailedEditChangesNothing(t *testing.T) {
	cs := connect(t, t.TempDir())
	call(t, cs, "create_structure", map[string]any{"name": "a"}, false)
	// The second layer has a character missing from the legend.
	call(t, cs, "place_grid", map[string]any{
		"name": "a", "origin": []int{0, 0, 0}, "legend": map[string]string{"#": "stone"},
		"layers": [][]string{{"##"}, {"#?"}},
	}, true)
	if got := call(t, cs, "list_structures", nil, false); !strings.Contains(got, "0 blocks") {
		t.Errorf("a failed edit left changes behind: %s", got)
	}
	// And it isn't a step to undo: this undoes the creation.
	call(t, cs, "undo", nil, false)
	if got := call(t, cs, "list_structures", nil, false); !strings.Contains(got, "No structures") {
		t.Errorf("got %s", got)
	}
}

func TestUndoHistoryIsBounded(t *testing.T) {
	s := newState(t.TempDir(), defaultDataVersion)
	for range maxUndoSteps + 20 {
		s.record(s.snapshot("fill", map[string]any{"name": "a"}))
	}
	if len(s.undo) != maxUndoSteps {
		t.Errorf("history holds %d steps, want %d", len(s.undo), maxUndoSteps)
	}
}

// structured runs a tool and returns its structured content, checked
// against the tool's output schema.
func structured(t *testing.T, cs *mcp.ClientSession, tool string, args map[string]any) map[string]any {
	t.Helper()
	ctx := context.Background()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil || res.IsError {
		t.Fatalf("%s: %v %v", tool, err, res)
	}
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil || out == nil {
		t.Fatalf("%s: structured content is %s", tool, data)
	}
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range tools.Tools {
		if tl.Name != tool {
			continue
		}
		raw, _ := json.Marshal(tl.OutputSchema)
		var schema jsonschema.Schema
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		resolved, err := schema.Resolve(nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := resolved.Validate(out); err != nil {
			t.Errorf("%s: structured content doesn't match the output schema: %v\n%s", tool, err, data)
		}
		return out
	}
	t.Fatalf("no tool %s", tool)
	return nil
}

func TestStructuredOutput(t *testing.T) {
	cs := connect(t, t.TempDir())
	call(t, cs, "create_structure", map[string]any{"name": "a"}, false)
	call(t, cs, "fill", map[string]any{"name": "a", "from": []int{0, 0, 0}, "to": []int{2, 0, 2}, "block": "stone"}, false)
	call(t, cs, "set_blocks", map[string]any{"name": "a", "blocks": []map[string]any{
		{"x": 1, "y": 3, "z": 1, "block": "sand"},
		{"x": 1, "y": 1, "z": 1, "block": "chest"},
	}}, false)
	call(t, cs, "add_entity", map[string]any{"name": "a", "pos": []float64{1.5, 1, 0.5}, "id": "armor_stand"}, false)
	call(t, cs, "place_jigsaw", map[string]any{"name": "a", "pos": []int{0, 1, 0}, "facing": "north", "jigsaw_name": "x:door", "pool": "x:rooms", "target": "x:door"}, false)

	d := structured(t, cs, "describe_structure", map[string]any{"name": "a"})
	if d["blocks"] != 12.0 || d["game_version"] != "26.3" || len(d["block_counts"].([]any)) != 4 {
		t.Errorf("describe: %v", d)
	}
	if e := d["entities"].([]any)[0].(map[string]any); e["id"] != "minecraft:armor_stand" {
		t.Errorf("describe entities: %v", d["entities"])
	}
	if j := d["jigsaws"].([]any)[0].(map[string]any); j["pool"] != "x:rooms" || j["orientation"] != "north_up" {
		t.Errorf("describe jigsaws: %v", d["jigsaws"])
	}

	l := structured(t, cs, "lint_structure", map[string]any{"name": "a"})
	checks := map[string]map[string]any{}
	for _, c := range l["checks"].([]any) {
		checks[c.(map[string]any)["check"].(string)] = c.(map[string]any)
	}
	g := checks["gravity"]
	if g == nil || g["count"] != 1.0 {
		t.Fatalf("lint: %v", l)
	}
	if f := g["findings"].([]any)[0].(map[string]any); len(f["pos"].([]any)) != 3 || f["message"] == "" {
		t.Errorf("lint finding: %v", f)
	}
	if l["findings"].(float64) < 2 || len(l["clean"].([]any)) == 0 {
		t.Errorf("lint totals: %v", l)
	}

	c := structured(t, cs, "critique_structure", map[string]any{"name": "a", "style": "survival"})
	if c["style"] != "survival" || len(c["categories"].([]any)) != 9 {
		t.Errorf("critique: %v", c)
	}

	call(t, cs, "create_structure", map[string]any{"name": "b"}, false)
	call(t, cs, "paste_structure", map[string]any{"source": "a", "target": "b", "at": []int{0, 0, 0}}, false)
	call(t, cs, "set_blocks", map[string]any{"name": "b", "blocks": []map[string]any{{"x": 0, "y": 0, "z": 0, "block": "dirt"}}}, false)
	same := structured(t, cs, "diff_structures", map[string]any{"before": "a", "after": "a"})
	if same["differ"] != 0.0 || same["changes"] != nil {
		t.Errorf("diff of a structure with itself: %v", same)
	}
	df := structured(t, cs, "diff_structures", map[string]any{"before": "a", "after": "b"})
	ch := df["changes"].([]any)[0].(map[string]any)
	if df["differ"] != 1.0 || ch["before"] != "minecraft:stone" || ch["after"] != "minecraft:dirt" {
		t.Errorf("diff: %v", df)
	}

	u := structured(t, cs, "undo", nil)
	if u["can_redo"] != 1.0 || u["steps"].([]any)[0].(map[string]any)["tool"] != "set_blocks" {
		t.Errorf("undo: %v", u)
	}
}

// TestUndoByName is two agents sharing a server: each undoes only its own
// structure's edits.
func TestUndoByName(t *testing.T) {
	cs := connect(t, t.TempDir())
	blocks := func() string { return call(t, cs, "list_structures", nil, false) }
	call(t, cs, "create_structure", map[string]any{"name": "a"}, false)
	call(t, cs, "create_structure", map[string]any{"name": "b"}, false)
	call(t, cs, "fill", map[string]any{"name": "a", "from": []int{0, 0, 0}, "to": []int{1, 0, 0}, "block": "stone"}, false)
	call(t, cs, "fill", map[string]any{"name": "b", "from": []int{0, 0, 0}, "to": []int{2, 0, 0}, "block": "dirt"}, false)

	out := call(t, cs, "undo", map[string]any{"name": "a"}, false)
	wantContains(t, "undo", out, "Undid fill on a.", "1 more edits to \"a\" can be undone, 1 redone")
	wantContains(t, "list_structures", blocks(), "a: size 0x0x0, 0 blocks", "b: size 3x1x1, 3 blocks")

	// b's agent keeps editing; a's undone fill can still be redone.
	call(t, cs, "fill", map[string]any{"name": "b", "from": []int{0, 1, 0}, "to": []int{2, 1, 0}, "block": "dirt"}, false)
	call(t, cs, "redo", map[string]any{"name": "a"}, false)
	wantContains(t, "list_structures", blocks(), "a: size 2x1x1, 2 blocks", "b: size 3x2x1, 6 blocks")

	// Steps stop at the structure's own history.
	out = call(t, cs, "undo", map[string]any{"name": "b", "steps": 5}, false)
	wantContains(t, "undo", out, `"b" is no longer open`)
	wantContains(t, "list_structures", blocks(), "a: size 2x1x1, 2 blocks")
	call(t, cs, "undo", map[string]any{"name": "b"}, true)
	call(t, cs, "redo", map[string]any{"name": "nope"}, true)
}

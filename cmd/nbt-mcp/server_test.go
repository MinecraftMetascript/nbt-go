package main

import (
	"context"
	"github.com/google/jsonschema-go/jsonschema"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/MinecraftMetascript/nbt-go"
	"github.com/MinecraftMetascript/nbt-go/structure"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func connect(t *testing.T, dir string) *mcp.ClientSession {
	t.Helper()
	return connectState(t, newState(dir, defaultDataVersion))
}

func connectState(t *testing.T, s *state) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	srv := newServer(s)
	ct, st := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// call runs a tool and returns its text, failing the test on a tool error
// unless wantErr is set.
func call(t *testing.T, cs *mcp.ClientSession, tool string, args map[string]any, wantErr bool) string {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	if res.IsError != wantErr {
		t.Fatalf("%s: IsError=%v, want %v: %s", tool, res.IsError, wantErr, b.String())
	}
	return b.String()
}

func TestListTools(t *testing.T) {
	cs := connect(t, t.TempDir())
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Each tool's own tests cover its behavior; this checks they're all registered.
	want := []string{"add_entity", "move_entity", "remove_entity", "create_structure", "describe_structure", "fill", "list_structures",
		"load_structure", "remove_blocks", "render_layer", "save_structure", "set_blocks",
		"place_jigsaw", "check_jigsaw_fit", "assemble_jigsaw",
		"fill_ellipsoid", "fill_cylinder", "fill_tube", "decorate_surface", "place_grid", "check_sealed", "check_lighting",
		"transform_structure", "paste_structure", "crop_structure", "set_size",
		"render_view", "render_image", "lint_structure", "diff_structures",
		"critique_structure", "building_guide", "undo", "redo"}
	got := map[string]bool{}
	for _, tool := range res.Tools {
		got[tool.Name] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("tool %s is not registered", w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("%d tools registered, want %d", len(got), len(want))
	}
}

func TestBuildAndSave(t *testing.T) {
	dir := t.TempDir()
	cs := connect(t, dir)

	call(t, cs, "create_structure", map[string]any{"name": "hut"}, false)
	out := call(t, cs, "fill", map[string]any{
		"name": "hut", "from": []int{0, 0, 0}, "to": []int{4, 3, 4}, "block": "oak_planks", "mode": "hollow",
	}, false)
	if !strings.Contains(out, "Filled 100 positions") {
		t.Errorf("fill: %s", out)
	}
	call(t, cs, "set_blocks", map[string]any{"name": "hut", "blocks": []map[string]any{
		{"x": 2, "y": 1, "z": 0, "block": "oak_door[facing=south,half=lower]"},
		{"x": 2, "y": 2, "z": 0, "block": "oak_door[facing=south,half=upper]"},
		{"x": 1, "y": 1, "z": 3, "block": "chest[facing=north]", "nbt": `{LootTable:"minecraft:chests/village/village_plains_house"}`},
	}}, false)
	call(t, cs, "add_entity", map[string]any{"name": "hut", "pos": []float64{2.5, 1, 2.5}, "id": "armor_stand", "nbt": "{NoGravity:1b}"}, false)

	layer := call(t, cs, "render_layer", map[string]any{"name": "hut", "y": 1}, false)
	for _, want := range []string{"   0 ##A##", "   3 #B  #", "minecraft:oak_planks", "minecraft:chest[facing=north]"} {
		if !strings.Contains(layer, want) {
			t.Errorf("render_layer missing %q:\n%s", want, layer)
		}
	}

	desc := call(t, cs, "describe_structure", map[string]any{"name": "hut"}, false)
	if !strings.Contains(desc, "size 5 x 4 x 5") || !strings.Contains(desc, "minecraft:armor_stand") {
		t.Errorf("describe_structure:\n%s", desc)
	}

	out = call(t, cs, "save_structure", map[string]any{"name": "hut", "path": "out/hut"}, false)
	path := filepath.Join(dir, "out", "hut.nbt")
	if !strings.Contains(out, path) {
		t.Errorf("save_structure: %s", out)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, err := structure.Read(f)
	if err != nil {
		t.Fatal(err)
	}
	if st.DataVersion != defaultDataVersion || st.Size != [3]int32{5, 4, 5} {
		t.Errorf("saved structure: DataVersion %d size %v", st.DataVersion, st.Size)
	}
	bs, data, ok := st.Block(1, 1, 3)
	if !ok || bs.Name != "minecraft:chest" || data["LootTable"] != nbt.String("minecraft:chests/village/village_plains_house") {
		t.Errorf("chest = %v %v %v", bs, data, ok)
	}
	if bs, _, _ := st.Block(2, 2, 2); bs.Name != "minecraft:air" {
		t.Errorf("hollow interior = %v", bs)
	}
	if len(st.Entities) != 1 || st.Entities[0].NBT["NoGravity"] != nbt.Byte(1) {
		t.Errorf("entities = %+v", st.Entities)
	}

	// Load it back under another name.
	out = call(t, cs, "load_structure", map[string]any{"name": "copy", "path": path}, false)
	if !strings.Contains(out, "size 5 x 4 x 5") {
		t.Errorf("load_structure: %s", out)
	}
	list := call(t, cs, "list_structures", map[string]any{}, false)
	if !strings.Contains(list, "copy:") || !strings.Contains(list, "hut:") {
		t.Errorf("list_structures: %s", list)
	}
}

func TestOutlineAndRemove(t *testing.T) {
	cs := connect(t, t.TempDir())
	call(t, cs, "create_structure", map[string]any{"name": "s"}, false)
	call(t, cs, "fill", map[string]any{"name": "s", "from": []int{0, 0, 0}, "to": []int{2, 2, 2}, "block": "stone", "mode": "outline"}, false)
	desc := call(t, cs, "describe_structure", map[string]any{"name": "s"}, false)
	if !strings.Contains(desc, "26 blocks") {
		t.Errorf("outline should leave the center empty:\n%s", desc)
	}
	out := call(t, cs, "remove_blocks", map[string]any{"name": "s", "from": []int{0, 0, 0}, "to": []int{2, 0, 2}}, false)
	if !strings.Contains(out, "Removed 9 blocks") {
		t.Errorf("remove_blocks: %s", out)
	}
}

func TestErrors(t *testing.T) {
	cs := connect(t, t.TempDir())
	if out := call(t, cs, "fill", map[string]any{"name": "nope", "from": []int{0, 0, 0}, "to": []int{1, 1, 1}, "block": "stone"}, true); !strings.Contains(out, "create_structure") {
		t.Errorf("unknown structure: %s", out)
	}
	call(t, cs, "create_structure", map[string]any{"name": "s"}, false)
	call(t, cs, "fill", map[string]any{"name": "s", "from": []int{0, 0, 0}, "to": []int{1000, 1000, 1000}, "block": "stone"}, true)
	call(t, cs, "fill", map[string]any{"name": "s", "from": []int{0, 0, 0}, "to": []int{1, 1, 1}, "block": "stone", "mode": "sideways"}, true)
	call(t, cs, "set_blocks", map[string]any{"name": "s", "blocks": []map[string]any{{"x": -1, "y": 0, "z": 0, "block": "stone"}}}, true)
	// A bad entry rejects the whole batch.
	call(t, cs, "set_blocks", map[string]any{"name": "s", "blocks": []map[string]any{
		{"x": 0, "y": 0, "z": 0, "block": "stone"},
		{"x": 1, "y": 0, "z": 0, "block": "chest", "nbt": "{oops"},
	}}, true)
	if out := call(t, cs, "describe_structure", map[string]any{"name": "s"}, false); !strings.Contains(out, " 0 blocks") {
		t.Errorf("failed batch changed the structure:\n%s", out)
	}
	call(t, cs, "render_layer", map[string]any{"name": "s", "y": 0}, true) // empty structure has no layers
	call(t, cs, "add_entity", map[string]any{"name": "s", "pos": []float64{0, 0, 0}, "id": "pig", "nbt": "[1,2]"}, true)
	call(t, cs, "load_structure", map[string]any{"name": "x", "path": "missing.nbt"}, true)
}

// TestToolSchemas checks that what tools/list advertises stays within the
// plain subset of JSON Schema that clients other than Claude also accept,
// and is fully described.
func TestToolSchemas(t *testing.T) {
	cs := connect(t, t.TempDir())
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var check func(path string, s map[string]any)
	check = func(path string, s map[string]any) {
		if _, union := s["type"].([]any); union {
			t.Errorf("%s: type is a union %v; use a single type", path, s["type"])
		}
		for _, kw := range []string{"anyOf", "oneOf", "allOf", "$ref", "$defs"} {
			if _, ok := s[kw]; ok {
				t.Errorf("%s: uses %s, which some clients do not support", path, kw)
			}
		}
		if items, ok := s["items"].(map[string]any); ok {
			check(path+"[]", items)
		}
		props, _ := s["properties"].(map[string]any)
		for name, p := range props {
			check(path+"."+name, p.(map[string]any))
		}
	}
	for _, tool := range res.Tools {
		if tool.Title == "" || tool.Description == "" {
			t.Errorf("%s: missing title or description", tool.Name)
		}
		if a := tool.Annotations; a == nil || a.OpenWorldHint == nil || a.DestructiveHint == nil {
			t.Errorf("%s: missing annotations", tool.Name)
		}
		schema, ok := tool.InputSchema.(map[string]any)
		if !ok || schema["type"] != "object" {
			t.Fatalf("%s: input schema is %v", tool.Name, tool.InputSchema)
		}
		check(tool.Name, schema)
		if out, ok := tool.OutputSchema.(map[string]any); ok {
			check(tool.Name+" output", out)
		}
		props, _ := schema["properties"].(map[string]any)
		for name, p := range props {
			if d, _ := p.(map[string]any)["description"].(string); d == "" {
				t.Errorf("%s.%s has no description", tool.Name, name)
			}
		}
	}
}

// TestSchemaEnums checks that the enum tags list exactly the values the
// handlers know.
func TestSchemaEnums(t *testing.T) {
	enum := func(s *jsonschema.Schema, prop string) []string {
		p := s.Properties[prop]
		if p.Type == "array" {
			p = p.Items
		}
		out := make([]string, len(p.Enum))
		for i, v := range p.Enum {
			out[i] = v.(string)
		}
		sort.Strings(out)
		return out
	}
	same := func(what string, got, want []string) {
		t.Helper()
		want = slices.Sorted(slices.Values(want)) // a copy: some callers pass package tables
		if !slices.Equal(got, want) {
			t.Errorf("%s enum is %v, want %v", what, got, want)
		}
	}

	var checks, topics, views []string
	for _, c := range lintChecks {
		checks = append(checks, c.name)
	}
	for _, g := range guideTopics {
		topics = append(topics, g.name)
	}
	for v := range viewDescriptions {
		views = append(views, v)
	}
	for v := range viewPresets {
		views = append(views, v)
	}
	same("lint checks", enum(clientSchema[lintIn](), "checks"), checks)
	same("guide topics", enum(clientSchema[guideIn](), "topics"), topics)
	same("image views", enum(clientSchema[imageIn](), "views"), views)
	same("image view", enum(clientSchema[imageIn](), "view"), views)
	same("rotation", enum(clientSchema[transformIn](), "rotation"), rotationNames[:])
	same("facing", enum(clientSchema[jigsawIn](), "facing"), dirNames[:])
}

// TestLenientArguments checks that calls stay forgiving even though the
// advertised schema is strict: explicit nulls for optional fields, as some
// clients send, and the aliases handlers accept.
func TestLenientArguments(t *testing.T) {
	cs := connect(t, t.TempDir())
	call(t, cs, "create_structure", map[string]any{"name": "a", "data_version": nil}, false)
	call(t, cs, "fill", map[string]any{"name": "a", "from": []int{0, 0, 0}, "to": []int{2, 0, 1}, "block": "stone", "mode": nil, "seed": nil}, false)
	call(t, cs, "transform_structure", map[string]any{"name": "a", "rotation": "cw"}, false)
	call(t, cs, "render_image", map[string]any{"name": "a", "views": nil, "view": "front", "from": nil, "cutaway_y": nil}, false)

	// Mistakes are reported as tool errors the model can read.
	if msg := call(t, cs, "fill", map[string]any{"name": "a", "from": []int{0, 0, 0}, "block": "stone"}, true); !strings.Contains(msg, "to") {
		t.Errorf("missing field: got %q", msg)
	}
	if msg := call(t, cs, "fill", map[string]any{"name": "a", "from": []int{0, 0, 0}, "to": []int{1, 1, 1}, "block": "stone", "bogus": 1}, true); !strings.Contains(msg, "bogus") {
		t.Errorf("unknown field: got %q", msg)
	}
	if msg := call(t, cs, "fill", map[string]any{"name": "a", "from": []int{0, 0, 0}, "to": []int{1, 1, 1}, "block": "stone", "mode": "sideways"}, true); !strings.Contains(msg, "hollow") {
		t.Errorf("bad option: got %q", msg)
	}
	call(t, cs, "list_structures", nil, false)
}

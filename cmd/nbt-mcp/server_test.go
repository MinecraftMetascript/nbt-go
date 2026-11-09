package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MinecraftMetascript/nbt-go"
	"github.com/MinecraftMetascript/nbt-go/structure"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func connect(t *testing.T, dir string) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	srv := newServer(newState(dir, defaultDataVersion))
	ct, st := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
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
	want := []string{"add_entity", "create_structure", "describe_structure", "fill", "list_structures",
		"load_structure", "remove_blocks", "render_layer", "save_structure", "set_blocks",
		"place_jigsaw", "check_jigsaw_fit", "assemble_jigsaw",
		"fill_ellipsoid", "fill_cylinder", "fill_tube", "decorate_surface", "place_grid", "check_sealed",
		"transform_structure", "paste_structure", "crop_structure", "set_size",
		"render_view", "render_image"}
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

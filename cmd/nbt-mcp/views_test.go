package main

import (
	"bytes"
	"context"
	"image/png"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// tower is a 3x4x2 stone block with a gold block sticking out of its top on
// the east side and a glass block in front at the bottom on the south side.
func tower(t *testing.T, cs mcpSession) {
	call(t, cs, "create_structure", map[string]any{"name": "t"}, false)
	call(t, cs, "fill", map[string]any{"name": "t", "from": []int{0, 0, 0}, "to": []int{2, 2, 1}, "block": "stone"}, false)
	call(t, cs, "set_blocks", map[string]any{"name": "t", "blocks": []map[string]any{
		{"x": 2, "y": 3, "z": 0, "block": "gold_block"},
		{"x": 0, "y": 0, "z": 2, "block": "glass"},
		{"x": 1, "y": 3, "z": 0, "block": "air"},
	}}, false)
}

func TestRenderView(t *testing.T) {
	cs := connect(t, t.TempDir())
	tower(t, cs)
	out := call(t, cs, "render_view", map[string]any{"name": "t", "view": "south"}, false)
	wantContains(t, "south view", out, "     012\n", "   3 . #\n", "   2 AAA\n", "   0 BAA\n", "B  minecraft:glass")
	out = call(t, cs, "render_view", map[string]any{"name": "t", "view": "north"}, false)
	wantContains(t, "north view", out, "     210\n", "   3 # .\n", "   0 AAA\n")
	out = call(t, cs, "render_view", map[string]any{"name": "t", "view": "east"}, false)
	wantContains(t, "east view", out, "     210\n", "   3 ..#\n", "   0 BAA\n")
	out = call(t, cs, "render_view", map[string]any{"name": "t", "view": "top"}, false)
	wantContains(t, "top view", out, "   0 ##A\n", "   2 B..\n")
	out = call(t, cs, "render_view", map[string]any{"name": "t", "view": "top", "show": "depth"}, false)
	wantContains(t, "top depth", out, "   0 110\n", "   1 111\n", "   2 3..\n")
	call(t, cs, "render_view", map[string]any{"name": "t", "view": "diagonal"}, true)
}

func TestRenderImage(t *testing.T) {
	cs := connect(t, t.TempDir())
	tower(t, cs)
	for _, view := range []string{"", "isometric_nw", "south", "top"} {
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "render_image", Arguments: map[string]any{"name": "t", "view": view, "scale": 8}})
		if err != nil || res.IsError {
			t.Fatalf("render_image %s: %v %+v", view, err, res)
		}
		img, ok := res.Content[0].(*mcp.ImageContent)
		if !ok || img.MIMEType != "image/png" {
			t.Fatalf("render_image %s: content %T", view, res.Content[0])
		}
		decoded, err := png.Decode(bytes.NewReader(img.Data))
		if err != nil {
			t.Fatalf("render_image %s: %v", view, err)
		}
		if decoded.Bounds().Dx() < 16 {
			t.Errorf("render_image %s: image is %v", view, decoded.Bounds())
		}
	}
	if c := blockColor(mustState("light_blue_wool")); c != rgb(dyeColors["light_blue"]) {
		t.Errorf("light_blue_wool color = %v", c)
	}
}

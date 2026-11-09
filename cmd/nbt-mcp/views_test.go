package main

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"strings"
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

// images calls render_image and decodes the images it returns, with the
// caption before each.
func images(t *testing.T, cs mcpSession, args map[string]any, wantErr bool) ([]image.Image, []string) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "render_image", Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError != wantErr {
		t.Fatalf("render_image %v: IsError=%v: %+v", args, res.IsError, res.Content)
	}
	var imgs []image.Image
	var captions []string
	for _, c := range res.Content {
		switch c := c.(type) {
		case *mcp.ImageContent:
			if c.MIMEType != "image/png" {
				t.Fatalf("MIME type %s", c.MIMEType)
			}
			img, err := png.Decode(bytes.NewReader(c.Data))
			if err != nil {
				t.Fatal(err)
			}
			imgs = append(imgs, img)
		case *mcp.TextContent:
			captions = append(captions, c.Text)
		}
	}
	return imgs, captions
}

func TestRenderImage(t *testing.T) {
	cs := connect(t, t.TempDir())
	tower(t, cs)
	for _, view := range []string{"", "isometric", "isometric_nw", "iso_sw", "iso_ne", "south", "top", "bottom"} {
		imgs, caps := images(t, cs, map[string]any{"name": "t", "view": view, "scale": 8}, false)
		if len(imgs) != 1 || imgs[0].Bounds().Dx() < 16 {
			t.Errorf("view %q: %d images, captions %v", view, len(imgs), caps)
		}
	}
	if c := blockColor(mustState("light_blue_wool")); c != rgb(dyeColors["light_blue"]) {
		t.Errorf("light_blue_wool color = %v", c)
	}
}

func TestRenderImageViews(t *testing.T) {
	cs := connect(t, t.TempDir())
	tower(t, cs)
	cases := map[string]int{"corners": 4, "elevations": 4, "overview": 5, "all": 9}
	for preset, n := range cases {
		imgs, caps := images(t, cs, map[string]any{"name": "t", "views": []string{preset}}, false)
		if len(imgs) != n {
			t.Errorf("%s: %d images, want %d", preset, len(imgs), n)
		}
		if !strings.HasPrefix(caps[0], viewPresets[preset][0]+": ") {
			t.Errorf("%s: first caption %q", preset, caps[0])
		}
	}
	// Duplicates collapse; presets and single views mix.
	imgs, _ := images(t, cs, map[string]any{"name": "t", "views": []string{"top", "overview", "iso_se"}}, false)
	if len(imgs) != 5 {
		t.Errorf("mixed views: %d images, want 5", len(imgs))
	}
	images(t, cs, map[string]any{"name": "t", "views": []string{"all", "bottom"}}, true)
	images(t, cs, map[string]any{"name": "t", "views": []string{"sideways"}}, true)

	// The four corners see different sides of an asymmetric structure.
	imgs, _ = images(t, cs, map[string]any{"name": "t", "views": []string{"corners"}, "scale": 6}, false)
	for i := 0; i < len(imgs); i++ {
		for j := i + 1; j < len(imgs); j++ {
			if sameImage(imgs[i], imgs[j]) {
				t.Errorf("corner views %d and %d are identical", i, j)
			}
		}
	}
}

func sameImage(a, b image.Image) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			if a.At(x, y) != b.At(x, y) {
				return false
			}
		}
	}
	return true
}

func TestRenderImageCutaway(t *testing.T) {
	cs := connect(t, t.TempDir())
	call(t, cs, "create_structure", map[string]any{"name": "box"}, false)
	call(t, cs, "fill", map[string]any{"name": "box", "from": []int{0, 0, 0}, "to": []int{4, 4, 4}, "block": "stone", "mode": "hollow"}, false)
	call(t, cs, "set_blocks", map[string]any{"name": "box", "blocks": []map[string]any{{"x": 2, "y": 1, "z": 2, "block": "gold_block"}}}, false)
	// Gold is the only yellow block, whatever shade it's drawn in.
	hasGold := func(img image.Image) bool {
		for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
			for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				if r>>8 > 120 && g>>8 > 100 && b>>8 < 70 {
					return true
				}
			}
		}
		return false
	}
	imgs, _ := images(t, cs, map[string]any{"name": "box", "view": "top", "scale": 4}, false)
	if hasGold(imgs[0]) {
		t.Error("the roof should hide the gold block")
	}
	imgs, caps := images(t, cs, map[string]any{"name": "box", "view": "top", "scale": 4, "cutaway_y": 2}, false)
	if !hasGold(imgs[0]) {
		t.Errorf("a cutaway at y=2 should show the gold block: %v", caps)
	}
	imgs, _ = images(t, cs, map[string]any{"name": "box", "view": "iso_se", "scale": 8, "cutaway_y": 1}, false)
	if !hasGold(imgs[0]) {
		t.Error("an isometric cutaway should show the gold block's top")
	}
	images(t, cs, map[string]any{"name": "box", "view": "top", "cutaway_y": -1}, true)
}

func TestShapes(t *testing.T) {
	cases := map[string]int{
		"stone": 1, "oak_slab[type=bottom]": 1, "oak_stairs[facing=north,half=bottom]": 2,
		"oak_fence[north=true,east=true,south=false,west=false]": 3, "glass_pane[north=true,south=true]": 3,
		"cobblestone_wall[up=false,east=low,west=tall]": 2, "torch": 1, "structure_void": 0,
	}
	for s, n := range cases {
		if got := len(shapeOf(mustState(s))); got != n {
			t.Errorf("shapeOf(%s) has %d boxes, want %d", s, got, n)
		}
	}
	st := shapeOf(mustState("oak_stairs[facing=north,half=bottom]"))
	if st[1].max[2] != 0.5 || st[1].min[1] != 0.5 {
		t.Errorf("north-facing stairs should rise on the north side: %+v", st)
	}
}

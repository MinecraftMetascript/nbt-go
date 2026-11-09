package main

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func fillBox(t *testing.T, cs *mcp.ClientSession, name string, from, to [3]int, block string, mode string) {
	t.Helper()
	args := map[string]any{"name": name, "from": from, "to": to, "block": block, "seed": 1}
	if mode != "" {
		args["mode"] = mode
	}
	call(t, cs, "fill", args, false)
}

// boxHouse is the kind of build the critique should mark down: one block,
// flat walls, flat roof, no detail.
func boxHouse(t *testing.T, cs *mcp.ClientSession) {
	call(t, cs, "create_structure", map[string]any{"name": "box"}, false)
	fillBox(t, cs, "box", [3]int{0, 0, 0}, [3]int{12, 7, 10}, "oak_planks", "hollow")
}

// craftedHouse uses the guide's techniques: plinth, frame, set-back infill,
// mixed palette, windows with sills, overhanging pitched roof and a wing.
func craftedHouse(t *testing.T, cs *mcp.ClientSession) {
	n := "house"
	call(t, cs, "create_structure", map[string]any{"name": n}, false)
	// Plinth, one wider than the walls.
	fillBox(t, cs, n, [3]int{1, 0, 1}, [3]int{13, 0, 11}, "70%cobblestone,30%mossy_cobblestone", "")
	// Infill walls, set back from the frame.
	fillBox(t, cs, n, [3]int{3, 1, 3}, [3]int{11, 5, 9}, "75%calcite,25%white_terracotta", "hollow")
	// Frame posts at the corners and midpoints, a beam at the top.
	for _, x := range []int{2, 7, 12} {
		for _, z := range []int{2, 10} {
			fillBox(t, cs, n, [3]int{x, 1, z}, [3]int{x, 5, z}, "stripped_spruce_log", "")
		}
	}
	for _, z := range []int{2, 6, 10} {
		fillBox(t, cs, n, [3]int{2, 1, z}, [3]int{2, 5, z}, "stripped_spruce_log", "")
		fillBox(t, cs, n, [3]int{12, 1, z}, [3]int{12, 5, z}, "stripped_spruce_log", "")
	}
	fillBox(t, cs, n, [3]int{2, 5, 2}, [3]int{12, 5, 2}, "spruce_log[axis=x]", "")
	fillBox(t, cs, n, [3]int{2, 5, 10}, [3]int{12, 5, 10}, "spruce_log[axis=x]", "")
	// Windows with sills on each side and a door.
	for _, x := range []int{4, 9} {
		fillBox(t, cs, n, [3]int{x, 2, 3}, [3]int{x + 1, 3, 3}, "glass_pane", "")
		fillBox(t, cs, n, [3]int{x, 1, 2}, [3]int{x + 1, 1, 2}, "spruce_stairs[facing=south,half=top]", "")
		fillBox(t, cs, n, [3]int{x, 2, 9}, [3]int{x + 1, 3, 9}, "glass_pane", "")
		fillBox(t, cs, n, [3]int{x, 2, 2}, [3]int{x, 3, 2}, "spruce_trapdoor[facing=north,open=true]", "")
	}
	fillBox(t, cs, n, [3]int{3, 2, 5}, [3]int{3, 3, 7}, "glass_pane", "")
	fillBox(t, cs, n, [3]int{11, 2, 5}, [3]int{11, 3, 7}, "glass_pane", "")
	fillBox(t, cs, n, [3]int{7, 1, 9}, [3]int{7, 2, 9}, "air", "")
	// Pitched roof overhanging one block on every side.
	for i := 0; i <= 5; i++ {
		y := 6 + i
		fillBox(t, cs, n, [3]int{1, y, 1 + i}, [3]int{13, y, 1 + i}, "deepslate_tile_stairs[facing=south]", "")
		fillBox(t, cs, n, [3]int{1, y, 11 - i}, [3]int{13, y, 11 - i}, "deepslate_tile_stairs[facing=north]", "")
	}
	fillBox(t, cs, n, [3]int{2, 7, 3}, [3]int{2, 10, 9}, "spruce_planks", "")
	// A lower wing and a chimney break the outline and the symmetry.
	fillBox(t, cs, n, [3]int{14, 0, 4}, [3]int{18, 0, 9}, "cobblestone", "")
	fillBox(t, cs, n, [3]int{14, 1, 5}, [3]int{17, 3, 8}, "80%spruce_planks,20%stripped_spruce_wood", "hollow")
	fillBox(t, cs, n, [3]int{14, 4, 4}, [3]int{18, 4, 9}, "spruce_slab", "")
	fillBox(t, cs, n, [3]int{10, 6, 7}, [3]int{10, 13, 7}, "70%bricks,30%stone_bricks", "")
	call(t, cs, "set_blocks", map[string]any{"name": n, "blocks": []map[string]any{
		{"x": 10, "y": 14, "z": 7, "block": "campfire"},
		{"x": 6, "y": 4, "z": 1, "block": "lantern[hanging=true]"},
	}}, false)
}

var scoreRE = regexp.MustCompile(`: (\d+)/100`)

func score(t *testing.T, out string) int {
	t.Helper()
	m := scoreRE.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no score in %s", out)
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func TestCritique(t *testing.T) {
	cs := connect(t, t.TempDir())
	boxHouse(t, cs)
	craftedHouse(t, cs)
	for _, style := range []string{"creative", "survival"} {
		box := call(t, cs, "critique_structure", map[string]any{"name": "box", "style": style}, false)
		house := call(t, cs, "critique_structure", map[string]any{"name": "house", "style": style}, false)
		t.Logf("%s\n%s\n%s", style, box, house)
		if b, h := score(t, box), score(t, house); b >= 50 || h < 75 || h <= b {
			t.Errorf("%s: box scored %d and house %d, want box < 50 and house >= 75", style, b, h)
		}
		for _, want := range []string{"flat patch of oak_planks", "plain rectangle", "same block (oak_planks)", "no windows or doors"} {
			if style == "survival" && want == "plain rectangle" {
				continue
			}
			if !strings.Contains(box, want) {
				t.Errorf("%s: box critique is missing %q", style, want)
			}
		}
	}
	out := call(t, cs, "critique_structure", map[string]any{"name": "box", "style": "fancy"}, true)
	if !strings.Contains(out, "survival or creative") {
		t.Errorf("bad style error: %s", out)
	}
}

func TestSurvivalMaterials(t *testing.T) {
	cs := connect(t, t.TempDir())
	call(t, cs, "create_structure", map[string]any{"name": "q"}, false)
	fillBox(t, cs, "q", [3]int{0, 0, 0}, [3]int{6, 4, 6}, "quartz_block", "hollow")
	surv := call(t, cs, "critique_structure", map[string]any{"name": "q", "style": "survival"}, false)
	if !strings.Contains(surv, "quartz_block") || !strings.Contains(surv, "lavish") {
		t.Errorf("survival critique doesn't flag quartz:\n%s", surv)
	}
	if cr := call(t, cs, "critique_structure", map[string]any{"name": "q"}, false); strings.Contains(cr, "lavish") {
		t.Errorf("creative critique flags materials:\n%s", cr)
	}
}

func TestGuide(t *testing.T) {
	cs := connect(t, t.TempDir())
	out := call(t, cs, "building_guide", map[string]any{"style": "survival"}, false)
	for _, want := range []string{"# Building structures", "# Survival style", "# Style palettes"} {
		if !strings.Contains(out, want) {
			t.Errorf("guide is missing %q", want)
		}
	}
	if strings.Contains(out, "# Creative style") {
		t.Error("survival guide includes the creative section")
	}
	call(t, cs, "building_guide", map[string]any{"topics": []string{"nope"}}, true)

	ctx := context.Background()
	res, err := cs.ListResources(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Resources) != len(guideTopics) {
		t.Errorf("got %d resources, want %d", len(res.Resources), len(guideTopics))
	}
	rr, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: guideURI("creative")})
	if err != nil || !strings.Contains(rr.Contents[0].Text, "# Creative style") {
		t.Errorf("reading the creative resource: %v", err)
	}

	pr, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "build", Arguments: map[string]string{"description": "a mill", "style": "survival", "theme": "nordic"}})
	if err != nil {
		t.Fatal(err)
	}
	txt := pr.Messages[0].Content.(*mcp.TextContent).Text
	for _, want := range []string{"a mill", "critique_structure with style=survival", `"nordic"`, "# Survival style"} {
		if !strings.Contains(txt, want) {
			t.Errorf("prompt is missing %q", want)
		}
	}
	if strings.Contains(txt, "# Jigsaw structures") {
		t.Error("prompt for a mill includes the jigsaw section")
	}
	pr, err = cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "build", Arguments: map[string]string{"description": "a fishing village"}})
	if err != nil {
		t.Fatal(err)
	}
	if txt := pr.Messages[0].Content.(*mcp.TextContent).Text; !strings.Contains(txt, "# Jigsaw structures") || !strings.Contains(txt, "assemble_jigsaw over at least five seeds") {
		t.Error("prompt for a village is missing the jigsaw section")
	}
	if out := call(t, cs, "building_guide", map[string]any{"topics": []string{"jigsaw"}}, false); !strings.Contains(out, "## Jitter") {
		t.Error("jigsaw topic is missing the jitter section")
	}
	if _, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "build", Arguments: map[string]string{}}); err == nil {
		t.Error("prompt without a description succeeded")
	}
}

// TestJigsawJitter checks the guide's jitter advice against assemble_jigsaw:
// a child piece with several identical jigsaws lands at different offsets.
func TestJigsawJitter(t *testing.T) {
	cs := connect(t, t.TempDir())
	call(t, cs, "create_structure", map[string]any{"name": "street"}, false)
	call(t, cs, "fill", map[string]any{"name": "street", "from": []int{0, 0, 0}, "to": []int{8, 0, 2}, "block": "gravel"}, false)
	call(t, cs, "place_jigsaw", map[string]any{"name": "street", "pos": []int{4, 0, 0}, "facing": "north", "target": "test:house_front", "pool": "test:houses"}, false)
	call(t, cs, "create_structure", map[string]any{"name": "house"}, false)
	call(t, cs, "fill", map[string]any{"name": "house", "from": []int{0, 0, 0}, "to": []int{4, 3, 4}, "block": "oak_planks", "mode": "hollow"}, false)
	for x := 1; x <= 3; x++ {
		call(t, cs, "place_jigsaw", map[string]any{"name": "house", "pos": []int{x, 0, 4}, "facing": "south", "jigsaw_name": "test:house_front"}, false)
	}
	pools := map[string]any{"test:houses": map[string]any{"fallback": "minecraft:empty", "elements": []any{
		map[string]any{"weight": 1, "element": map[string]any{"element_type": "minecraft:single_pool_element", "location": "house", "projection": "rigid", "processors": "minecraft:empty"}},
	}}}
	boxRE := regexp.MustCompile(`#1 minecraft:house, [^\n]*?(x=[^ ]+)`)
	seen := map[string]bool{}
	for seed := 1; seed <= 12; seed++ {
		out := call(t, cs, "assemble_jigsaw", map[string]any{"into": "v", "start": "street", "pools": pools, "seed": seed}, false)
		m := boxRE.FindStringSubmatch(out)
		if m == nil {
			t.Fatalf("seed %d placed no house:\n%s", seed, out)
		}
		seen[m[1]] = true
	}
	if len(seen) < 2 {
		t.Errorf("the house always landed at %v; identical jigsaws should jitter it", seen)
	}
}

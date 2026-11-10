package main

import (
	"context"
	"embed"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//go:embed guide/*.md
var guideFS embed.FS

// guideTopics are the building guide's sections, each served as a resource
// and through the building_guide tool.
var guideTopics = []struct{ name, file, title, about string }{
	{"building", "guide/building.md", "Building principles", "how to make a structure look hand-crafted: silhouette, depth, palette gradients, detailing, asymmetry, interiors"},
	{"survival", "guide/survival.md", "Survival style", "practical builds a survival player could make: common materials, small palettes, functional detail"},
	{"creative", "guide/creative.md", "Creative style", "showpiece builds: rich palettes, texture gradients, dense detail, complex shapes"},
	{"jigsaw", "guide/jigsaw.md", "Jigsaw structures", "designing jigsaw structures that vary well: grid modules, connector naming, variants and slots, jitter with identical jigsaws, fallbacks and terminators, priorities, terrain blending, processors"},
	{"palettes", "guide/palettes.md", "Style palettes", "block palettes per theme (medieval, castle, nordic, desert, swamp, ruined, nether), each in survival and creative versions"},
}

func guideText(topic string) (string, error) {
	for _, t := range guideTopics {
		if t.name == topic {
			b, err := guideFS.ReadFile(t.file)
			return string(b), err
		}
	}
	names := make([]string, len(guideTopics))
	for i, t := range guideTopics {
		names[i] = t.name
	}
	return "", fmt.Errorf("unknown topic %q; topics: %s", topic, strings.Join(names, ", "))
}

func guideURI(topic string) string { return "nbt-mcp://guide/" + topic }

type guideIn struct {
	Topics []string `json:"topics,omitempty" enum:"building,survival,creative,jigsaw,palettes" jsonschema:"sections to return: building, survival, creative, jigsaw, palettes (default building)"`
	Style  string   `json:"style,omitempty" enum:"survival,creative" jsonschema:"survival or creative: adds that style's section and the palettes"`
}

func (s *state) guide(in guideIn) (string, error) {
	topics := append([]string(nil), in.Topics...)
	switch in.Style {
	case "":
	case "survival", "creative":
		topics = append(topics, in.Style, "palettes")
	default:
		return "", fmt.Errorf("style %q must be survival or creative", in.Style)
	}
	if len(in.Topics) == 0 {
		topics = append([]string{"building"}, topics...)
	}
	seen := map[string]bool{}
	var parts []string
	for _, t := range topics {
		if seen[t] {
			continue
		}
		seen[t] = true
		txt, err := guideText(t)
		if err != nil {
			return "", err
		}
		parts = append(parts, txt)
	}
	return strings.Join(parts, "\n\n"), nil
}

func addGuide(srv *mcp.Server, s *state) {
	for _, t := range guideTopics {
		srv.AddResource(&mcp.Resource{
			URI:         guideURI(t.name),
			Name:        t.name,
			Title:       t.title,
			Description: t.about,
			MIMEType:    "text/markdown",
		}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			txt, err := guideText(t.name)
			if err != nil {
				return nil, err
			}
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: "text/markdown", Text: txt}}}, nil
		})
	}

	topics := make([]string, len(guideTopics))
	for i, t := range guideTopics {
		topics[i] = t.name + " (" + t.about + ")"
	}
	addTool(srv, kindRead, "building_guide", "Building guide",
		"Read the building guide before designing a structure, so it looks hand-crafted rather than generated. Topics: "+strings.Join(topics, "; ")+". Pass style=survival or style=creative to get the principles plus that style and its palettes.",
		handler(s, s.guide))

	srv.AddPrompt(&mcp.Prompt{
		Name:        "build",
		Title:       "Build a structure",
		Description: "Design and build a Minecraft structure that looks hand-crafted, in survival or creative style, checking it with critique_structure and lint_structure.",
		Arguments: []*mcp.PromptArgument{
			{Name: "description", Description: "what to build, e.g. \"a blacksmith's workshop in a taiga village\"", Required: true},
			{Name: "style", Description: "survival (practical, common materials) or creative (showpiece); default creative"},
			{Name: "theme", Description: "a palette theme from the guide (medieval, castle, nordic, desert, swamp, ruined, nether) or your own"},
			{Name: "name", Description: "name for the structure; default \"build\""},
			{Name: "jigsaw", Description: "true to build it as a jigsaw structure from template pools (a village, a multi-piece ruin); on by default when the description mentions jigsaws or villages"},
		},
	}, buildPrompt)
}

func buildPrompt(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	args := req.Params.Arguments
	desc := strings.TrimSpace(args["description"])
	if desc == "" {
		return nil, fmt.Errorf("description is required")
	}
	style := strings.ToLower(strings.TrimSpace(args["style"]))
	if style == "" {
		style = "creative"
	}
	if style != "survival" && style != "creative" {
		return nil, fmt.Errorf("style %q must be survival or creative", style)
	}
	name := strings.TrimSpace(args["name"])
	if name == "" {
		name = "build"
	}
	theme := strings.TrimSpace(args["theme"])
	themeLine := "Choose a palette theme from the guide that suits the description and biome, and say which one you picked."
	if theme != "" {
		themeLine = fmt.Sprintf("Use the %q palette theme from the guide as the starting point, adapting it to the description.", theme)
	}
	jig := strings.EqualFold(strings.TrimSpace(args["jigsaw"]), "true")
	if args["jigsaw"] == "" {
		low := strings.ToLower(desc)
		jig = strings.Contains(low, "jigsaw") || strings.Contains(low, "village")
	}
	topics := []string{"building", style, "palettes"}
	jigLine := ""
	if jig {
		topics = append(topics, "jigsaw")
		jigLine = `

This is a jigsaw structure. Plan the module, the connector names, the pools and their fallbacks before building pieces; build each piece as its own structure, critique each piece, and preview the whole with assemble_jigsaw over at least five seeds. Follow the jigsaw section of the guide, including jitter with identical jigsaws.`
	}
	var guide []string
	for _, t := range topics {
		txt, err := guideText(t)
		if err != nil {
			return nil, err
		}
		guide = append(guide, txt)
	}
	task := fmt.Sprintf(`Build this as a Minecraft structure named %q: %s

Style: %s. %s%s

Work in passes, and look at the result after each one with render_image and render_view (south, east, north, west):

1. Plan. Before placing anything, write a short plan: footprint (not a plain rectangle unless it is very small), the volumes and their heights, the roof shape and overhang, and the palette for each role (foundation, frame, walls, roof, accents) as weighted mixes.
2. Block out. create_structure, then the foundation, frame and main volumes with fill and the shape tools. Leave windows and doors open.
3. Depth. Set wall infill back from the frame, add sills, overhanging eaves with brackets, and jetties or balconies where they fit.
4. Detail. Stairs, slabs, walls, fences, trapdoors and buttons on every edge; lighting; vines, moss and plants; the path to the door. Break the symmetry.
5. Interior. Furnish it for its purpose, with light.
6. Critique. Run critique_structure with style=%s. Fix the weakest categories and the listed findings, then run it again. Aim for 80 or more, but judge by eye too: a finding can be a deliberate choice.
7. Check and save. Run lint_structure and fix real problems, then save_structure.

The building guide follows.

%s`, name, desc, style, themeLine, jigLine, style, strings.Join(guide, "\n\n"))
	return &mcp.GetPromptResult{
		Description: "Build " + desc,
		Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: task}}},
	}, nil
}

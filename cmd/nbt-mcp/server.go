package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/MinecraftMetascript/nbt-go"
	"github.com/MinecraftMetascript/nbt-go/structure"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// maxVolume bounds how many positions one fill or remove call may touch, so a
// typo in a coordinate cannot exhaust memory.
const maxVolume = 1 << 21

const instructions = `Builds Minecraft Java Edition structure files (.nbt).

Before designing a build, read building_guide (with style=survival for practical builds a player could make, or style=creative for showpieces): it covers what makes a build look hand-crafted rather than generated, and palettes by theme. The build prompt walks through a whole build.

Workflow: create_structure, then place blocks with fill, set_blocks, place_grid and the shape tools (fill_ellipsoid, fill_cylinder, fill_tube, decorate_surface), check your work with render_layer, render_view, render_image, describe_structure, critique_structure and lint_structure, and finish with save_structure.

Coordinates are relative to the structure's origin and must be non-negative; x is east, y is up, z is south. Blocks use command syntax, such as "stone_bricks" or "oak_stairs[facing=east,half=bottom]"; the minecraft: namespace is optional. Anywhere a block is filled, a weighted mix such as "70%stone_bricks,20%cracked_stone_bricks,10%mossy_stone_bricks" also works; random tools take a seed and report the one they used. Block entity and entity data use SNBT, such as {LootTable:"minecraft:chests/simple_dungeon"}.

Positions without a block leave the world unchanged when the structure is placed. Use minecraft:air to clear space.

Jigsaw structures: add jigsaw blocks with place_jigsaw, check them with describe_structure (which lists and lints them), check that a child piece fits its parent with check_jigsaw_fit, and preview a whole layout with assemble_jigsaw. A child's jigsaw lands in the block directly in front of the parent's jigsaw: the two jigsaw blocks end up adjacent and face to face. When a jigsaw faces into its own piece's bounding box, the entire child piece must fit inside that box. A start piece placed with project_start_to_heightmap (as villages are) has its y=0 layer at the height of the topmost terrain block, replacing it, so y=0 is sunk one block into the ground and y=1 is the first layer above it; build y=0 as a foundation or raise start_height by 1.`

type state struct {
	mu          sync.Mutex
	dir         string
	dataVersion int32
	structures  map[string]*structure.Structure
}

func newState(dir string, dataVersion int32) *state {
	return &state{dir: dir, dataVersion: dataVersion, structures: map[string]*structure.Structure{}}
}

func (s *state) get(name string) (*structure.Structure, error) {
	st, ok := s.structures[name]
	if !ok {
		names := s.names()
		if len(names) == 0 {
			return nil, fmt.Errorf("no structure named %q; create one with create_structure", name)
		}
		return nil, fmt.Errorf("no structure named %q; open structures: %s", name, strings.Join(names, ", "))
	}
	return st, nil
}

func (s *state) names() []string {
	names := make([]string, 0, len(s.structures))
	for n := range s.structures {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (s *state) resolve(path string) (string, error) {
	if path == "" {
		return "", errors.New("path is required")
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[2:])
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.dir, path)
	}
	return filepath.Clean(path), nil
}

func text(format string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, args...)}}}
}

// handler adapts a function returning text to an MCP tool handler, holding
// the state lock for the duration of the call.
func handler[In any](s *state, f func(In) (string, error)) mcp.ToolHandlerFor[In, any] {
	return func(_ context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		out, err := f(in)
		if err != nil {
			return nil, nil, err
		}
		return text("%s", out), nil, nil
	}
}

func parseSNBT(s string) (nbt.Compound, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	t, err := nbt.ParseSNBT(s)
	if err != nil {
		return nil, err
	}
	c, ok := t.(nbt.Compound)
	if !ok {
		return nil, fmt.Errorf("NBT must be a compound like {key:value}, got %v", t.Type())
	}
	return c, nil
}

func newServer(s *state) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "nbt-mcp", Title: "Minecraft structure builder", Version: version()},
		&mcp.ServerOptions{Instructions: instructions})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_structure",
		Description: "Start a new, empty structure in memory. Replaces any open structure with the same name.",
	}, handler(s, s.create))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "load_structure",
		Description: "Open an existing .nbt structure file for viewing or editing.",
	}, handler(s, s.load))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_structures",
		Description: "List the structures currently open in memory.",
	}, handler(s, s.list))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "set_blocks",
		Description: "Place individual blocks, replacing whatever is at each position. Use this for details such as doors, stairs, chests and torches.",
	}, handler(s, s.setBlocks))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fill",
		Description: "Fill a box with one block or a weighted mix, like the /fill command. Corners are inclusive. Mode \"replace\" fills the whole box, \"hollow\" fills the shell and puts air inside, and \"outline\" fills only the shell and leaves the inside unchanged.",
	}, handler(s, s.fill))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fill_ellipsoid",
		Description: "Fill a sphere or ellipsoid, solid or as a watertight hollow shell. Good for domes, boulders, bushes, caves and organic bodies.",
	}, handler(s, s.ellipsoid))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fill_cylinder",
		Description: "Fill a cylinder along x, y or z: solid, a hollow shell, a tube or ring (hollow with open_ends), or a disc (height 1). Cross-sections can be elliptical.",
	}, handler(s, s.cylinder))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fill_tube",
		Description: "Fill a rounded tube along a path of points, optionally tapering from one radius to another. Good for trunks, branches, roots, tentacles, pipes and curved tunnels.",
	}, handler(s, s.tube))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "decorate_surface",
		Description: "Apply a block to the exposed faces of solid blocks, like a decal: moss or snow on top surfaces, vines or glow lichen on walls, grass on the top of a mound. Can be limited to a box, to certain blocks, and to a random fraction of positions.",
	}, handler(s, s.decorate))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "place_grid",
		Description: "Place many blocks at once from text layers and a legend, in the same layout render_layer draws: each layer is a list of rows from north to south, each character one block going east. Much more compact than set_blocks for detailed builds.",
	}, handler(s, s.placeGrid))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "check_sealed",
		Description: "Check that a space is watertight: flood-fill from a position through everything that isn't a full block and report whether it reaches the outside, and where the likely gap is.",
	}, handler(s, s.checkSealed))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "transform_structure",
		Description: "Rotate and/or mirror a whole structure in place, turning block states (facing, axis, connections, rails, stairs) and entities to match.",
	}, handler(s, s.transformStructure))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "paste_structure",
		Description: "Copy one open structure into another at a position, optionally rotated or mirrored. Use it to merge parts built separately or repeat a module.",
	}, handler(s, s.pasteStructure))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "crop_structure",
		Description: "Cut a structure down to a box (inclusive corners), moving that box to the origin. Optionally keep the original and open the cropped copy under a new name.",
	}, handler(s, s.crop))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "set_size",
		Description: "Set a structure's size explicitly, to add empty margin or trim it. Shrinking refuses to cut off blocks unless drop is set.",
	}, handler(s, s.setSize))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "remove_blocks",
		Description: "Remove blocks from a box (inclusive corners), so those positions are left unchanged when the structure is placed. This is not the same as placing air.",
	}, handler(s, s.remove))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "add_entity",
		Description: "Add an entity, such as an armor stand, villager or item frame.",
	}, handler(s, s.addEntity))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "describe_structure",
		Description: "Summarize a structure: size, how much is blocks, air and empty space, block counts by type, entities, block entities, and its jigsaw blocks with warnings about likely mistakes.",
	}, handler(s, s.describe))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "render_layer",
		Description: "Draw one horizontal layer (a fixed y) as a top-down text grid, one character per block, with a legend. Rows are z (north at the top), columns are x (west on the left). Use it to check your work.",
	}, handler(s, s.render))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "diff_structures",
		Description: "Compare two open structures (for example a file loaded before and after edits): size and DataVersion changes, how many positions differ, a count of each kind of change (stone → air), and the individual changes.",
	}, handler(s, s.diffStructures))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "lint_structure",
		Description: "Check a structure for common mistakes: file format problems, block ids renamed between versions, walls and fences that don't connect, torches, lanterns and vines with nothing to attach to, sand and gravel over air, doors, beds and tall plants missing their other half, fire or lava next to flammable blocks, blocks with nothing below them (legs hanging off a plinth), floating groups, containers with no loot table, high-value blocks players could mine, and jigsaw settings. Run it before saving.",
	}, handler(s, s.lint))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "render_view",
		Description: "Draw the structure as seen from one side (top, bottom, north, south, east or west) as a text grid with a legend, showing the first non-air block along each line of sight. Use south for a front elevation, east or west for a side elevation, top for a plan. show=depth draws how far in each block is instead.",
	}, handler(s, s.view))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "render_image",
		Description: "Render the structure as a PNG image, isometric by default or from one side, with an approximate flat color per block type. Use it to judge overall shape and proportions; use render_view or render_layer for exact positions.",
	}, s.renderImage)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "place_jigsaw",
		Description: "Place a jigsaw block with its settings (name, target, pool, final_state, joint, priorities) and the right orientation. final_state defaults to the block the jigsaw replaces, so it doesn't leave a hole when the structure generates. Reports where the child piece will attach and any problems.",
	}, handler(s, s.placeJigsaw))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "check_jigsaw_fit",
		Description: "Check whether a child piece can attach to a parent piece's jigsaws: for each matching jigsaw and rotation, where the child would end up in the parent's coordinates and whether it fits. A child attached by a jigsaw that faces into the parent's bounding box must fit entirely inside that box, or the game silently places nothing.",
	}, handler(s, s.checkFit))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "assemble_jigsaw",
		Description: "Assemble a jigsaw structure from a start piece and template pools, following the game's placement rules (attachment, priorities, depth, max distance, and the rule that pieces can't overlap), and open the result as a new structure to inspect with render_layer and describe_structure. Reports every piece placed and every jigsaw that placed nothing, with the reason. Pools come from a datapack folder or inline JSON; element locations can name open structures. Terrain, processors and terrain_matching projection are ignored.",
	}, handler(s, s.assemble))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "critique_structure",
		Description: "Score how hand-crafted a build looks, in survival or creative style, and list the most valuable fixes with coordinates: flat walls (depth), palette variety, share of detail blocks, silhouette, roof overhangs, foundation and colour gradient, windows and doors, asymmetry, and for survival style, lavish materials. Run it after blocking out and again after detailing; lint_structure checks correctness instead.",
	}, handler(s, s.critique))

	addGuide(srv, s)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "save_structure",
		Description: "Write a structure to a gzip-compressed .nbt file. To use it in a world, save to <world>/generated/<namespace>/structure/<name>.nbt (\"structures\" before 1.21) and load it with a structure block or /place template <namespace>:<name>.",
	}, handler(s, s.save))

	return srv
}

type createIn struct {
	Name        string `json:"name" jsonschema:"name to refer to the structure by in other tools"`
	DataVersion int32  `json:"data_version,omitempty" jsonschema:"DataVersion of the target Minecraft version, e.g. 4189 for 1.21.4 or 5023 for 26.3; defaults to the server's setting"`
}

func (s *state) create(in createIn) (string, error) {
	if in.Name == "" {
		return "", errors.New("name is required")
	}
	v := in.DataVersion
	if v == 0 {
		v = s.dataVersion
	}
	s.structures[in.Name] = structure.New(v)
	return fmt.Sprintf("Created empty structure %q (DataVersion %s).", in.Name, dataVersionLabel(v)), nil
}

type loadIn struct {
	Name string `json:"name" jsonschema:"name to refer to the structure by in other tools"`
	Path string `json:"path" jsonschema:"path of the .nbt file; relative paths are resolved against the server's directory"`
}

func (s *state) load(in loadIn) (string, error) {
	if in.Name == "" {
		return "", errors.New("name is required")
	}
	path, err := s.resolve(in.Path)
	if err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := structure.Read(f)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	s.structures[in.Name] = st
	return fmt.Sprintf("Loaded %s as %q.\n\n%s", path, in.Name, summarize(st)), nil
}

type listIn struct{}

func (s *state) list(listIn) (string, error) {
	if len(s.structures) == 0 {
		return "No structures are open.", nil
	}
	var b strings.Builder
	for _, n := range s.names() {
		st := s.structures[n]
		fmt.Fprintf(&b, "%s: size %dx%dx%d, %d blocks, %d entities\n", n, st.Size[0], st.Size[1], st.Size[2], len(st.Blocks), len(st.Entities))
	}
	return b.String(), nil
}

type blockIn struct {
	X     int    `json:"x"`
	Y     int    `json:"y"`
	Z     int    `json:"z"`
	Block string `json:"block" jsonschema:"block in command syntax, e.g. oak_stairs[facing=east,half=bottom]"`
	NBT   string `json:"nbt,omitempty" jsonschema:"block entity data as SNBT, e.g. {LootTable:\"minecraft:chests/simple_dungeon\"}"`
}

type setBlocksIn struct {
	Name   string    `json:"name"`
	Blocks []blockIn `json:"blocks" jsonschema:"blocks to place"`
}

func (s *state) setBlocks(in setBlocksIn) (string, error) {
	st, err := s.get(in.Name)
	if err != nil {
		return "", err
	}
	// Validate everything first so a bad entry leaves the structure unchanged.
	type placement struct {
		x, y, z int
		state   structure.BlockState
		data    nbt.Compound
	}
	ps := make([]placement, len(in.Blocks))
	for i, b := range in.Blocks {
		bs, err := structure.ParseBlockState(b.Block)
		if err != nil {
			return "", fmt.Errorf("blocks[%d]: %w", i, err)
		}
		data, err := parseSNBT(b.NBT)
		if err != nil {
			return "", fmt.Errorf("blocks[%d].nbt: %w", i, err)
		}
		if b.X < 0 || b.Y < 0 || b.Z < 0 {
			return "", fmt.Errorf("blocks[%d]: coordinates must be non-negative, got (%d, %d, %d)", i, b.X, b.Y, b.Z)
		}
		ps[i] = placement{b.X, b.Y, b.Z, bs, data}
	}
	for _, p := range ps {
		if err := st.SetBlock(p.x, p.y, p.z, p.state, p.data); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("Placed %d blocks. Size is now %v.", len(ps), st.Size), nil
}

type boxIn struct {
	Name string `json:"name"`
	From [3]int `json:"from" jsonschema:"first corner [x, y, z]"`
	To   [3]int `json:"to" jsonschema:"opposite corner [x, y, z], inclusive"`
}

func (b boxIn) bounds() (lo, hi [3]int, err error) {
	vol := 1
	for a := 0; a < 3; a++ {
		lo[a], hi[a] = min(b.From[a], b.To[a]), max(b.From[a], b.To[a])
		if lo[a] < 0 {
			return lo, hi, fmt.Errorf("coordinates must be non-negative, got from %v to %v", b.From, b.To)
		}
		vol *= hi[a] - lo[a] + 1
		if vol > maxVolume {
			return lo, hi, fmt.Errorf("box from %v to %v covers more than %d blocks", b.From, b.To, maxVolume)
		}
	}
	return lo, hi, nil
}

type fillIn struct {
	boxIn
	Block string `json:"block" jsonschema:"block in command syntax, e.g. stone_bricks, or a weighted mix such as 70%stone_bricks,30%cracked_stone_bricks"`
	Mode  string `json:"mode,omitempty" jsonschema:"replace (default), hollow, or outline"`
	Seed  *int64 `json:"seed,omitempty" jsonschema:"random seed for block mixes; a random one is used and reported if omitted"`
}

func (s *state) fill(in fillIn) (string, error) {
	st, err := s.get(in.Name)
	if err != nil {
		return "", err
	}
	pat, err := parsePattern(in.Block)
	if err != nil {
		return "", err
	}
	rng, seed := newRNG(in.Seed)
	lo, hi, err := in.bounds()
	if err != nil {
		return "", err
	}
	mode := in.Mode
	if mode == "" {
		mode = "replace"
	}
	if mode != "replace" && mode != "hollow" && mode != "outline" {
		return "", fmt.Errorf("unknown mode %q; use replace, hollow or outline", in.Mode)
	}
	air := structure.BlockState{Name: "minecraft:air"}
	n := 0
	for x := lo[0]; x <= hi[0]; x++ {
		for y := lo[1]; y <= hi[1]; y++ {
			for z := lo[2]; z <= hi[2]; z++ {
				edge := x == lo[0] || x == hi[0] || y == lo[1] || y == hi[1] || z == lo[2] || z == hi[2]
				place := pat.pick(rng)
				if !edge {
					switch mode {
					case "hollow":
						place = air
					case "outline":
						continue
					}
				}
				if err := st.SetBlock(x, y, z, place, nil); err != nil {
					return "", err
				}
				n++
			}
		}
	}
	msg := fmt.Sprintf("Filled %d positions from %v to %v with %s (%s)", n, lo, hi, pat, mode)
	if pat.random() {
		msg += fmt.Sprintf(", seed %d", seed)
	}
	return msg + fmt.Sprintf(". Size is now %v.", st.Size), nil
}

func (s *state) remove(in boxIn) (string, error) {
	st, err := s.get(in.Name)
	if err != nil {
		return "", err
	}
	lo, hi, err := in.bounds()
	if err != nil {
		return "", err
	}
	n := 0
	for x := lo[0]; x <= hi[0]; x++ {
		for y := lo[1]; y <= hi[1]; y++ {
			for z := lo[2]; z <= hi[2]; z++ {
				if st.RemoveBlock(x, y, z) {
					n++
				}
			}
		}
	}
	return fmt.Sprintf("Removed %d blocks.", n), nil
}

type entityIn struct {
	Name string     `json:"name"`
	Pos  [3]float64 `json:"pos" jsonschema:"position [x, y, z]; use .5 to center in a block"`
	ID   string     `json:"id" jsonschema:"entity id, e.g. armor_stand or villager"`
	NBT  string     `json:"nbt,omitempty" jsonschema:"extra entity data as SNBT, e.g. {CustomName:'\"Guard\"',NoAI:1b}"`
}

func (s *state) addEntity(in entityIn) (string, error) {
	st, err := s.get(in.Name)
	if err != nil {
		return "", err
	}
	data, err := parseSNBT(in.NBT)
	if err != nil {
		return "", fmt.Errorf("nbt: %w", err)
	}
	if data == nil {
		data = nbt.Compound{}
	}
	id := in.ID
	if id == "" {
		return "", errors.New("id is required")
	}
	if !strings.Contains(id, ":") {
		id = "minecraft:" + id
	}
	data["id"] = nbt.String(id)
	pos := make(nbt.List, 3)
	for a := range pos {
		pos[a] = nbt.Double(in.Pos[a])
	}
	data["Pos"] = pos
	if err := st.AddEntity(in.Pos, data); err != nil {
		return "", err
	}
	return fmt.Sprintf("Added %s at %v.", id, in.Pos), nil
}

type nameIn struct {
	Name string `json:"name"`
}

func (s *state) describe(in nameIn) (string, error) {
	st, err := s.get(in.Name)
	if err != nil {
		return "", err
	}
	return summarize(st), nil
}

func summarize(st *structure.Structure) string {
	var b strings.Builder
	fmt.Fprintf(&b, "DataVersion %s, size %d x %d x %d (x, y, z), %d blocks, %d entities.\n",
		dataVersionLabel(st.DataVersion), st.Size[0], st.Size[1], st.Size[2], len(st.Blocks), len(st.Entities))
	if vol := int(st.Size[0]) * int(st.Size[1]) * int(st.Size[2]); vol > 0 {
		solid, air, empty := airStats(st)
		pct := func(n int) float64 { return 100 * float64(n) / float64(vol) }
		fmt.Fprintf(&b, "Of %d positions: %d blocks (%.1f%%), %d air (%.1f%%), %d empty, left unchanged when placed (%.1f%%).\n",
			vol, solid, pct(solid), air, pct(air), empty, pct(empty))
	}
	if len(st.Palettes) > 0 {
		fmt.Fprintf(&b, "Has %d palette variants; counts use the first.\n", len(st.Palettes))
	}
	palette := st.Palette
	if len(st.Palettes) > 0 {
		palette = st.Palettes[0]
	}
	counts := make([]int, len(palette))
	withNBT := 0
	for _, bl := range st.Blocks {
		if int(bl.State) < len(counts) {
			counts[bl.State]++
		}
		if len(bl.NBT) > 0 {
			withNBT++
		}
	}
	order := make([]int, 0, len(palette))
	for i := range palette {
		if counts[i] > 0 {
			order = append(order, i)
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return counts[order[i]] > counts[order[j]] })
	if len(order) > 0 {
		b.WriteString("\nBlocks:\n")
		for _, i := range order {
			fmt.Fprintf(&b, "  %6d  %s\n", counts[i], palette[i])
		}
	}
	if withNBT > 0 {
		fmt.Fprintf(&b, "\n%d blocks have block entity data.\n", withNBT)
	}
	if len(st.Entities) > 0 {
		b.WriteString("\nEntities:\n")
		for _, e := range st.Entities {
			fmt.Fprintf(&b, "  %v at %v\n", e.NBT["id"], e.Pos)
		}
	}
	b.WriteString(jigsawReport(st))
	return b.String()
}

type renderIn struct {
	Name string `json:"name"`
	Y    int    `json:"y" jsonschema:"the layer to draw"`
}

// glyphs are assigned to block types in order of first appearance on a layer.
const glyphs = "#ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789@$%&*+=?!<>/|~^"

func (s *state) render(in renderIn) (string, error) {
	st, err := s.get(in.Name)
	if err != nil {
		return "", err
	}
	if in.Y < 0 || in.Y >= int(st.Size[1]) {
		return "", fmt.Errorf("y=%d is outside the structure, which spans y=0..%d", in.Y, st.Size[1]-1)
	}
	w, d := int(st.Size[0]), int(st.Size[2])
	if w*d > 128*128 {
		return "", fmt.Errorf("layer is %dx%d, too large to render (max 128x128)", w, d)
	}
	grid := make([][]byte, d)
	for z := range grid {
		grid[z] = []byte(strings.Repeat(".", w))
	}
	glyphOf := map[string]byte{}
	var legend []string
	for z := 0; z < d; z++ {
		for x := 0; x < w; x++ {
			bs, _, ok := st.Block(x, in.Y, z)
			if !ok {
				continue
			}
			key := bs.String()
			if bs.Name == "minecraft:air" {
				grid[z][x] = ' '
				continue
			}
			g, seen := glyphOf[key]
			if !seen {
				if len(glyphOf) < len(glyphs) {
					g = glyphs[len(glyphOf)]
				} else {
					g = '?'
				}
				glyphOf[key] = g
				legend = append(legend, fmt.Sprintf("  %c  %s", g, key))
			}
			grid[z][x] = g
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Layer y=%d of %q (%d wide on x, %d deep on z). North is up.\n\n", in.Y, in.Name, w, d)
	// Column ruler, units digit of x.
	b.WriteString("     ")
	for x := 0; x < w; x++ {
		b.WriteByte(byte('0' + x%10))
	}
	b.WriteByte('\n')
	for z := 0; z < d; z++ {
		fmt.Fprintf(&b, "%4d %s\n", z, grid[z])
	}
	b.WriteString("\nLegend:\n")
	for _, l := range legend {
		b.WriteString(l + "\n")
	}
	b.WriteString("  .  no block (world left unchanged)\n  ' ' (space) minecraft:air\n")
	return b.String(), nil
}

type saveIn struct {
	Name string `json:"name"`
	Path string `json:"path" jsonschema:"where to write the file; .nbt is added if missing; relative paths are resolved against the server's directory"`
}

func (s *state) save(in saveIn) (string, error) {
	st, err := s.get(in.Name)
	if err != nil {
		return "", err
	}
	path, err := s.resolve(in.Path)
	if err != nil {
		return "", err
	}
	if filepath.Ext(path) != ".nbt" {
		path += ".nbt"
	}
	if err := st.Validate(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".nbt-mcp-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if err := st.Write(tmp); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", err
	}
	note := ""
	if st.Size[0] > 48 || st.Size[1] > 48 || st.Size[2] > 48 {
		note = "\nNote: it is larger than 48 blocks on some axis, so a structure block cannot load all of it; use /place template instead."
	}
	return fmt.Sprintf("Saved %q (%d x %d x %d, %d blocks) to %s.%s", in.Name, st.Size[0], st.Size[1], st.Size[2], len(st.Blocks), path, note), nil
}

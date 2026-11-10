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

Workflow: create_structure, then place blocks with fill, set_blocks, place_grid and the shape tools (fill_ellipsoid, fill_cylinder, fill_tube, decorate_surface), check your work with render_layer, render_view, render_image, describe_structure, critique_structure and lint_structure, and finish with save_structure. Every edit can be taken back with undo (and redo), so try a change and back out of it if it made the build worse.

Coordinates are relative to the structure's origin and must be non-negative; x is east, y is up, z is south. Blocks use command syntax, such as "stone_bricks" or "oak_stairs[facing=east,half=bottom]"; the minecraft: namespace is optional. Anywhere a block is filled, a weighted mix such as "70%stone_bricks,20%cracked_stone_bricks,10%mossy_stone_bricks" also works; random tools take a seed and report the one they used. Block entity and entity data use SNBT, such as {LootTable:"minecraft:chests/simple_dungeon"}.

Positions without a block leave the world unchanged when the structure is placed. Use minecraft:air to clear space.

Jigsaw structures: read building_guide with topics=["jigsaw"] for how to design pieces and pools that vary well (modules, variants, jitter, terminators, processors). Add jigsaw blocks with place_jigsaw, check them with describe_structure (which lists and lints them), check that a child piece fits its parent with check_jigsaw_fit, and preview a whole layout with assemble_jigsaw. A child's jigsaw lands in the block directly in front of the parent's jigsaw: the two jigsaw blocks end up adjacent and face to face. When a jigsaw faces into its own piece's bounding box, the entire child piece must fit inside that box. A start piece placed with project_start_to_heightmap (as villages are) has its y=0 layer at the height of the topmost terrain block, replacing it, so y=0 is sunk one block into the ground and y=1 is the first layer above it; build y=0 as a foundation or raise start_height by 1.`

type state struct {
	mu          sync.Mutex
	dir         string
	restrict    bool // only read and write files inside dir
	dataVersion int32
	structures  map[string]*structure.Structure

	// undo and redo hold what edits replaced, newest last.
	undo, redo []edit
	// changed, if set, is told which structures an edit touched.
	changed func(names []string)
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
	path = filepath.Clean(path)
	if s.restrict {
		if err := s.checkInside(path); err != nil {
			return "", err
		}
	}
	return path, nil
}

// checkInside reports an error unless path is inside the server's
// directory, following symlinks in the part of it that already exists.
func (s *state) checkInside(path string) error {
	root, err := filepath.Abs(s.dir)
	if err != nil {
		return err
	}
	if root, err = filepath.EvalSymlinks(root); err != nil {
		return err
	}
	if path, err = filepath.Abs(path); err != nil {
		return err
	}
	// The file may not exist yet, so resolve the deepest ancestor that does.
	existing, rest := path, ""
	for {
		real, err := filepath.EvalSymlinks(existing)
		if err == nil {
			existing = real
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return err
		}
		rest = filepath.Join(filepath.Base(existing), rest)
		existing = parent
	}
	rel, err := filepath.Rel(root, filepath.Join(existing, rest))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("%s is outside %s, the only directory this server may read and write", path, root)
	}
	return nil
}

func text(format string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, args...)}}}
}

// handler adapts a function returning text to an MCP tool handler.
func handler[In any](f func(In) (string, error)) mcp.ToolHandlerFor[In, any] {
	return func(_ context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		out, err := f(in)
		if err != nil {
			return nil, nil, err
		}
		return text("%s", out), nil, nil
	}
}

// handlerOut adapts a function that returns text for the model and the same
// result as data, which clients get as structured content.
func handlerOut[In, Out any](f func(In) (string, Out, error)) mcp.ToolHandlerFor[In, Out] {
	return func(_ context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		txt, out, err := f(in)
		if err != nil {
			return nil, out, err
		}
		return text("%s", txt), out, nil
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
	rs := &resourceSync{s: s, listed: map[string]bool{}, subscribed: map[string]int{}}
	srv := mcp.NewServer(&mcp.Implementation{Name: "nbt-mcp", Title: "Minecraft structure builder", Version: version()},
		&mcp.ServerOptions{Instructions: instructions, SubscribeHandler: rs.subscribe, UnsubscribeHandler: rs.unsubscribe})
	rs.srv = srv
	addStructureResources(srv, rs)

	addTool(srv, s, kindEdit, "create_structure", "Create structure",
		"Start a new, empty structure in memory. Replaces any open structure with the same name.",
		handler(s.create))

	addTool(srv, s, kindEdit, "load_structure", "Load structure file",
		"Open an existing .nbt structure file for viewing or editing.",
		handler(s.load))

	addTool(srv, s, kindRead, "list_structures", "List open structures",
		"List the structures currently open in memory.",
		handler(s.list))

	addTool(srv, s, kindEdit, "set_blocks", "Set blocks",
		"Place individual blocks, replacing whatever is at each position. Use this for details such as doors, stairs, chests and torches.",
		handler(s.setBlocks))

	addTool(srv, s, kindEdit, "fill", "Fill box",
		"Fill a box with one block or a weighted mix, like the /fill command. Corners are inclusive. Mode \"replace\" fills the whole box, \"hollow\" fills the shell and puts air inside, and \"outline\" fills only the shell and leaves the inside unchanged.",
		handler(s.fill))

	addTool(srv, s, kindEdit, "fill_ellipsoid", "Fill sphere or ellipsoid",
		"Fill a sphere or ellipsoid, solid or as a watertight hollow shell. Good for domes, boulders, bushes, caves and organic bodies.",
		handler(s.ellipsoid))

	addTool(srv, s, kindEdit, "fill_cylinder", "Fill cylinder",
		"Fill a cylinder along x, y or z: solid, a hollow shell, a tube or ring (hollow with open_ends), or a disc (height 1). Cross-sections can be elliptical.",
		handler(s.cylinder))

	addTool(srv, s, kindEdit, "fill_tube", "Fill tube along a path",
		"Fill a rounded tube along a path of points, optionally tapering from one radius to another. Good for trunks, branches, roots, tentacles, pipes and curved tunnels.",
		handler(s.tube))

	addTool(srv, s, kindEdit, "decorate_surface", "Decorate surfaces",
		"Apply a block to the exposed faces of solid blocks, like a decal: moss or snow on top surfaces, vines or glow lichen on walls, grass on the top of a mound. Can be limited to a box, to certain blocks, and to a random fraction of positions.",
		handler(s.decorate))

	addTool(srv, s, kindEdit, "place_grid", "Place blocks from a text grid",
		"Place many blocks at once from text layers and a legend, in the same layout render_layer draws: each layer is a list of rows from north to south, each character one block going east. Much more compact than set_blocks for detailed builds.",
		handler(s.placeGrid))

	addTool(srv, s, kindRead, "check_sealed", "Check a space is sealed",
		"Check that a space is sealed off: flood-fill from a position and report whether it reaches the outside, and where the likely gap is. By default it checks against water, so slabs, stairs, fences and doors count as sealing; against=gaps counts any block that isn't full as open except closed doors, trapdoors and fence gates.",
		handler(s.checkSealed))

	addTool(srv, s, kindEdit, "transform_structure", "Rotate or mirror structure",
		"Rotate and/or mirror a whole structure in place, turning block states (facing, axis, connections, rails, stairs) and entities to match.",
		handler(s.transformStructure))

	addTool(srv, s, kindEdit, "paste_structure", "Paste structure",
		"Copy one open structure into another at a position, optionally rotated or mirrored. Use it to merge parts built separately or repeat a module.",
		handler(s.pasteStructure))

	addTool(srv, s, kindEdit, "crop_structure", "Crop structure",
		"Cut a structure down to a box (inclusive corners), moving that box to the origin. Optionally keep the original and open the cropped copy under a new name.",
		handler(s.crop))

	addTool(srv, s, kindEdit, "set_size", "Set structure size",
		"Set a structure's size explicitly, to add empty margin or trim it. Shrinking refuses to cut off blocks unless drop is set.",
		handler(s.setSize))

	addTool(srv, s, kindEdit, "remove_blocks", "Remove blocks",
		"Remove blocks from a box (inclusive corners), so those positions are left unchanged when the structure is placed. This is not the same as placing air.",
		handler(s.remove))

	addTool(srv, s, kindEdit, "add_entity", "Add entity",
		"Add an entity, such as an armor stand, villager or item frame. Warns if it would start inside a block's collision box (a villager at y = n.0 on a carpet sinks into it and is stuck): stand it on top, e.g. y = n.0625 on a carpet.",
		handler(s.addEntity))

	addTool(srv, s, kindEdit, "move_entity", "Move entity",
		"Move an entity, picked by its number from describe_structure or by the position it's nearest. Updates every copy of its position (pos, blockPos and the Pos in its data) and warns if it would be stuck.",
		handler(s.moveEntity))

	addTool(srv, s, kindEdit, "remove_entity", "Remove entity",
		"Remove an entity, picked by its number from describe_structure or by the position it's nearest. Later entities are renumbered.",
		handler(s.removeEntity))

	addTool(srv, s, kindRead, "describe_structure", "Describe structure",
		"Summarize a structure: size, how much is blocks, air and empty space, block counts by type, entities, block entities, and its jigsaw blocks with warnings about likely mistakes.",
		handlerOut(s.describe))

	addTool(srv, s, kindRead, "render_layer", "Render layer as text",
		"Draw one horizontal layer (a fixed y) as a top-down text grid, one character per block, with a legend. Rows are z (north at the top), columns are x (west on the left). Use it to check your work.",
		handler(s.render))

	addTool(srv, s, kindRead, "diff_structures", "Compare structures",
		"Compare two open structures (for example a file loaded before and after edits): size and DataVersion changes, how many positions differ, a count of each kind of change (stone → air), and the individual changes.",
		handlerOut(s.diffStructures))

	addTool(srv, s, kindRead, "lint_structure", "Lint structure",
		"Check a structure for common mistakes: file format problems, block ids renamed between versions, walls and fences that don't connect, torches, lanterns and vines with nothing to attach to, sand and gravel over air, doors, beds and tall plants missing their other half, fire or lava next to flammable blocks, blocks with nothing below them (legs hanging off a plinth), floating groups, containers with no loot table, high-value blocks players could mine, and jigsaw settings. Run it before saving.",
		handlerOut(s.lint))

	addTool(srv, s, kindRead, "render_view", "Render view as text",
		"Draw the structure as seen from one side (top, bottom, north, south, east or west) as a text grid with a legend, showing the first non-air block along each line of sight. Use south for a front elevation, east or west for a side elevation, top for a plan. show=depth draws how far in each block is instead.",
		handler(s.view))

	addTool(srv, s, kindRead, "render_image", "Render images",
		"Look at the structure: render PNG images from one or more viewpoints in a single call, so you can see it the way a player would. Views: isometric from each corner (iso_se, iso_sw, iso_nw, iso_ne), plan (top), elevations (south, north, east, west), or presets (corners, elevations, overview, all). Use cutaway_y to see inside a floor, and from/to to cut a cross-section. Partial blocks get their rough shape and glass and water are see-through; colors are approximate. Use render_view or render_layer for exact positions.",
		s.renderImage)

	addTool(srv, s, kindEdit, "place_jigsaw", "Place jigsaw block",
		"Place a jigsaw block with its settings (name, target, pool, final_state, joint, priorities) and the right orientation. final_state defaults to the block the jigsaw replaces, so it doesn't leave a hole when the structure generates. Reports where the child piece will attach and any problems.",
		handler(s.placeJigsaw))

	addTool(srv, s, kindRead, "check_jigsaw_fit", "Check jigsaw fit",
		"Check whether a child piece can attach to a parent piece's jigsaws: for each matching jigsaw and rotation, where the child would end up in the parent's coordinates and whether it fits. A child attached by a jigsaw that faces into the parent's bounding box must fit entirely inside that box, or the game silently places nothing.",
		handler(s.checkFit))

	addTool(srv, s, kindEdit, "assemble_jigsaw", "Assemble jigsaw structure",
		"Assemble a jigsaw structure from a start piece and template pools, following the game's placement rules (attachment, priorities, depth, max distance, and the rule that pieces can't overlap), and open the result as a new structure to inspect with render_layer and describe_structure. Reports every piece placed and every jigsaw that placed nothing, with the reason; above 40 pieces it gives a summary instead (counts per template, a depth histogram, problems grouped by reason). Entry jigsaws that point back at their parent are counted, not listed. Pass seeds to try several layouts and see which templates never get placed. Pools come from a datapack folder or inline JSON; element locations can name open structures. Terrain, processors and terrain_matching projection are ignored.",
		handler(s.assemble))

	addTool(srv, s, kindRead, "critique_structure", "Critique structure",
		"Score how hand-crafted a build looks, in survival or creative style, and list the most valuable fixes with coordinates: flat walls (depth), palette variety, share of detail blocks, silhouette, roof overhangs, foundation and colour gradient, windows and doors, asymmetry, and for survival style, lavish materials. Run it after blocking out and again after detailing; lint_structure checks correctness instead.",
		handlerOut(s.critique))

	addTool(srv, s, kindHistory, "undo", "Undo edits",
		"Undo the most recent edits, one tool call per step: a fill, a paste, a crop, creating or loading a structure, and so on. Files already saved are not changed. Use it to back out of a change that made the build worse instead of repairing it by hand.",
		handlerOut(s.undoEdits))

	addTool(srv, s, kindHistory, "redo", "Redo edits",
		"Redo edits that undo took back. A new edit after an undo clears what could be redone.",
		handlerOut(s.redoEdits))

	addGuide(srv, s)

	addTool(srv, s, kindWrite, "save_structure", "Save structure file",
		"Write a structure to a gzip-compressed .nbt file. To use it in a world, save to <world>/generated/<namespace>/structure/<name>.nbt (\"structures\" before 1.21) and load it with a structure block or /place template <namespace>:<name>.",
		handler(s.save))

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
	txt, _ := summarize(st)
	return fmt.Sprintf("Loaded %s as %q.\n\n%s", path, in.Name, txt), nil
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
	Mode  string `json:"mode,omitempty" enum:"replace,hollow,outline" jsonschema:"replace (default), hollow, or outline"`
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
	return fmt.Sprintf("Added %s at %v as entity #%d.", id, in.Pos, len(st.Entities)-1) + stuckWarning(newGrid(st), in.Pos, id), nil
}

type nameIn struct {
	Name string `json:"name"`
}

func (s *state) describe(in nameIn) (string, describeOut, error) {
	st, err := s.get(in.Name)
	if err != nil {
		return "", describeOut{}, err
	}
	txt, out := summarize(st)
	return txt, out, nil
}

// summarize describes a structure as text and as data.
func summarize(st *structure.Structure) (string, describeOut) {
	out := describeOut{DataVersion: st.DataVersion, GameVersion: versionName(st.DataVersion), Size: size3(st.Size)}
	var b strings.Builder
	fmt.Fprintf(&b, "DataVersion %s, size %d x %d x %d (x, y, z), %d blocks, %d entities.\n",
		dataVersionLabel(st.DataVersion), st.Size[0], st.Size[1], st.Size[2], len(st.Blocks), len(st.Entities))
	if vol := int(st.Size[0]) * int(st.Size[1]) * int(st.Size[2]); vol > 0 {
		solid, air, empty := airStats(st)
		out.Positions, out.Blocks, out.Air, out.Empty = vol, solid, air, empty
		pct := func(n int) float64 { return 100 * float64(n) / float64(vol) }
		fmt.Fprintf(&b, "Of %d positions: %d blocks (%.1f%%), %d air (%.1f%%), %d empty, left unchanged when placed (%.1f%%).\n",
			vol, solid, pct(solid), air, pct(air), empty, pct(empty))
	}
	if len(st.Palettes) > 0 {
		out.PaletteCount = len(st.Palettes)
		fmt.Fprintf(&b, "Has %d palette variants; counts use the first.\n", len(st.Palettes))
	}
	palette := st.Palette
	if len(st.Palettes) > 0 {
		palette = st.Palettes[0]
	}
	counts := make([]int, len(palette))
	for _, bl := range st.Blocks {
		if int(bl.State) < len(counts) {
			counts[bl.State]++
		}
		if len(bl.NBT) > 0 {
			out.BlockEntities++
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
			out.BlockCounts = append(out.BlockCounts, blockCount{Block: palette[i].String(), Count: counts[i]})
			fmt.Fprintf(&b, "  %6d  %s\n", counts[i], palette[i])
		}
	}
	if out.BlockEntities > 0 {
		fmt.Fprintf(&b, "\n%d blocks have block entity data.\n", out.BlockEntities)
	}
	if len(st.Entities) > 0 {
		b.WriteString("\nEntities:\n")
		for i, e := range st.Entities {
			id, _ := e.NBT["id"].(nbt.String)
			out.Entities = append(out.Entities, entityOut{ID: string(id), Pos: e.Pos})
			fmt.Fprintf(&b, "  #%d %v at %v\n", i, e.NBT["id"], e.Pos)
		}
	}
	jigsaws, js, warnings := jigsawReport(st)
	out.Jigsaws, out.Warnings = js, warnings
	b.WriteString(jigsaws)
	return b.String(), out
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

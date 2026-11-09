package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/MinecraftMetascript/nbt-go"
	"github.com/MinecraftMetascript/nbt-go/structure"
)

// vec is a block position or offset.
type vec [3]int

func (a vec) add(b vec) vec { return vec{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func (a vec) sub(b vec) vec { return vec{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }

func (a vec) String() string { return fmt.Sprintf("(%d, %d, %d)", a[0], a[1], a[2]) }

func toVec(p [3]int32) vec { return vec{int(p[0]), int(p[1]), int(p[2])} }

// box is an axis-aligned box with inclusive corners, like the game's
// BoundingBox.
type box struct{ min, max vec }

func sizeBox(size [3]int32) box {
	return box{vec{}, vec{int(size[0]) - 1, int(size[1]) - 1, int(size[2]) - 1}}
}

func (b box) contains(p vec) bool {
	for a := 0; a < 3; a++ {
		if p[a] < b.min[a] || p[a] > b.max[a] {
			return false
		}
	}
	return true
}

func (b box) inside(outer box) bool {
	return outer.contains(b.min) && outer.contains(b.max)
}

func (b box) intersects(o box) bool {
	for a := 0; a < 3; a++ {
		if b.max[a] < o.min[a] || o.max[a] < b.min[a] {
			return false
		}
	}
	return true
}

func (b box) moved(d vec) box { return box{b.min.add(d), b.max.add(d)} }

func (b box) size() vec {
	return vec{b.max[0] - b.min[0] + 1, b.max[1] - b.min[1] + 1, b.max[2] - b.min[2] + 1}
}

func (b box) String() string {
	return fmt.Sprintf("x=%d..%d y=%d..%d z=%d..%d", b.min[0], b.max[0], b.min[1], b.max[1], b.min[2], b.max[2])
}

// overhang describes how far b sticks out of outer on each side, such as
// "2 too far up (+y), 1 too far west (-x)".
func overhang(b, outer box) string {
	var parts []string
	names := [3][2]string{{"west (-x)", "east (+x)"}, {"down (-y)", "up (+y)"}, {"north (-z)", "south (+z)"}}
	for a := 0; a < 3; a++ {
		if d := outer.min[a] - b.min[a]; d > 0 {
			parts = append(parts, fmt.Sprintf("%d too far %s", d, names[a][0]))
		}
		if d := b.max[a] - outer.max[a]; d > 0 {
			parts = append(parts, fmt.Sprintf("%d too far %s", d, names[a][1]))
		}
	}
	return strings.Join(parts, ", ")
}

// dir is one of the six block faces.
type dir int

const (
	down dir = iota
	up
	north
	south
	west
	east
)

var dirNames = [...]string{"down", "up", "north", "south", "west", "east"}

var dirVecs = [...]vec{{0, -1, 0}, {0, 1, 0}, {0, 0, -1}, {0, 0, 1}, {-1, 0, 0}, {1, 0, 0}}

func (d dir) String() string { return dirNames[d] }
func (d dir) vec() vec       { return dirVecs[d] }
func (d dir) opposite() dir  { return d ^ 1 }
func (d dir) horizontal() bool {
	return d >= north
}

func parseDir(s string) (dir, bool) {
	for i, n := range dirNames {
		if s == n {
			return dir(i), true
		}
	}
	return 0, false
}

// rotation is a clockwise rotation about the y axis in quarter turns, seen
// from above, matching the game's Rotation enum.
type rotation int

var rotationNames = [...]string{"none", "clockwise_90", "180", "counterclockwise_90"}

func (r rotation) String() string { return rotationNames[r&3] }

func parseRotation(s string) (rotation, error) {
	switch s {
	case "", "none", "0":
		return 0, nil
	case "clockwise_90", "90", "cw":
		return 1, nil
	case "180", "clockwise_180":
		return 2, nil
	case "counterclockwise_90", "270", "-90", "ccw":
		return 3, nil
	}
	return 0, fmt.Errorf("unknown rotation %q; use none, clockwise_90, 180 or counterclockwise_90", s)
}

// apply rotates a position about the origin, as structure templates are.
func (r rotation) apply(p vec) vec {
	switch r & 3 {
	case 1:
		return vec{-p[2], p[1], p[0]}
	case 2:
		return vec{-p[0], p[1], -p[2]}
	case 3:
		return vec{p[2], p[1], -p[0]}
	}
	return p
}

func (r rotation) applyF(p [3]float64) [3]float64 {
	switch r & 3 {
	case 1:
		return [3]float64{-p[2], p[1], p[0]}
	case 2:
		return [3]float64{-p[0], p[1], -p[2]}
	case 3:
		return [3]float64{p[2], p[1], -p[0]}
	}
	return p
}

var clockwise = map[dir]dir{north: east, east: south, south: west, west: north}

func (r rotation) dir(d dir) dir {
	if !d.horizontal() {
		return d
	}
	for i := 0; i < int(r&3); i++ {
		d = clockwise[d]
	}
	return d
}

// box returns the bounding box of a template of the given size, rotated
// about its origin.
func (r rotation) box(b box) box {
	p, q := r.apply(b.min), r.apply(b.max)
	var out box
	for a := 0; a < 3; a++ {
		out.min[a], out.max[a] = min(p[a], q[a]), max(p[a], q[a])
	}
	return out
}

// rotateState rotates the direction-dependent properties of a block state:
// facing, axis, rotation, jigsaw orientation, connections and rail shapes.
func rotateState(bs structure.BlockState, r rotation) structure.BlockState {
	if r&3 == 0 || len(bs.Properties) == 0 {
		return bs
	}
	p := make(map[string]string, len(bs.Properties))
	for k, v := range bs.Properties {
		p[k] = v
	}
	rotName := func(s string) string {
		if d, ok := parseDir(s); ok {
			return r.dir(d).String()
		}
		return s
	}
	if v, ok := p["facing"]; ok {
		p["facing"] = rotName(v)
	}
	if v, ok := p["orientation"]; ok {
		if a, b, ok := strings.Cut(v, "_"); ok {
			p["orientation"] = rotName(a) + "_" + rotName(b)
		}
	}
	if v, ok := p["axis"]; ok && r&1 == 1 {
		switch v {
		case "x":
			p["axis"] = "z"
		case "z":
			p["axis"] = "x"
		}
	}
	if v, ok := p["rotation"]; ok {
		var n int
		if _, err := fmt.Sscan(v, &n); err == nil {
			p["rotation"] = fmt.Sprint((n + 4*int(r&3)) % 16)
		}
	}
	// Connection properties (fences, walls, panes, vines, redstone...).
	sides := [4]dir{north, east, south, west}
	var vals [4]string
	var has [4]bool
	for i, d := range sides {
		vals[i], has[i] = bs.Properties[d.String()]
		delete(p, d.String())
	}
	for i, d := range sides {
		if has[i] {
			p[r.dir(d).String()] = vals[i]
		}
	}
	if v, ok := p["shape"]; ok {
		p["shape"] = rotateRailShape(v, r)
	}
	return structure.BlockState{Name: bs.Name, Properties: p}
}

func rotateRailShape(s string, r rotation) string {
	switch {
	case s == "north_south" || s == "east_west":
		if r&1 == 0 {
			return s
		}
		if s == "north_south" {
			return "east_west"
		}
		return "north_south"
	case strings.HasPrefix(s, "ascending_"):
		if d, ok := parseDir(strings.TrimPrefix(s, "ascending_")); ok {
			return "ascending_" + r.dir(d).String()
		}
	default:
		a, b, ok := strings.Cut(s, "_")
		da, oka := parseDir(a)
		db, okb := parseDir(b)
		if ok && oka && okb && da.horizontal() && db.horizontal() {
			da, db = r.dir(da), r.dir(db)
			// Rail curves are named north/south first.
			if da == east || da == west {
				da, db = db, da
			}
			return da.String() + "_" + db.String()
		}
	}
	return s
}

// palette returns the palette that blocks index into (the first one for
// structures with several).
func palette(st *structure.Structure) []structure.BlockState {
	if len(st.Palettes) > 0 {
		return st.Palettes[0]
	}
	return st.Palette
}

// stateOf returns the block state of a block, or false if its palette index
// is out of range.
func stateOf(st *structure.Structure, b structure.Block) (structure.BlockState, bool) {
	p := palette(st)
	if b.State < 0 || int(b.State) >= len(p) {
		return structure.BlockState{}, false
	}
	return p[b.State], true
}

// grid is a position index over a structure's blocks.
type grid struct {
	st  *structure.Structure
	pal []structure.BlockState
	at  map[vec]int
}

func newGrid(st *structure.Structure) *grid {
	g := &grid{st: st, pal: palette(st), at: make(map[vec]int, len(st.Blocks))}
	for i, b := range st.Blocks {
		g.at[toVec(b.Pos)] = i
	}
	return g
}

// get returns the state and data at p; ok is false where there is no block.
func (g *grid) get(p vec) (structure.BlockState, nbt.Compound, bool) {
	i, ok := g.at[p]
	if !ok {
		return structure.BlockState{}, nil, false
	}
	b := g.st.Blocks[i]
	if b.State < 0 || int(b.State) >= len(g.pal) {
		return structure.BlockState{}, b.NBT, false
	}
	return g.pal[b.State], b.NBT, true
}

func isAir(name string) bool {
	switch name {
	case "minecraft:air", "minecraft:cave_air", "minecraft:void_air":
		return true
	}
	return false
}

// nonFullHints are name fragments of blocks that don't fill their whole
// cell. It is a heuristic: the server has no block registry.
var nonFullHints = []string{
	"slab", "stairs", "fence", "_wall", "pane", "iron_bars", "door", "trapdoor", "torch", "lantern",
	"sign", "carpet", "button", "lever", "rail", "flower", "sapling", "short_grass", "tall_grass", "fern",
	"vine", "ladder", "banner", "pressure_plate", "candle", "rod", "head", "skull", "pot",
	"_bed", "chest", "campfire", "anvil", "bell", "cake", "frame", "scaffolding", "snow", "water",
	"lava", "kelp", "seagrass", "coral", "pickle", "dripleaf", "lichen", "moss_carpet", "roots",
	"mushroom", "bush", "azalea", "lily", "web", "redstone_wire", "repeater", "comparator", "tripwire",
	"hopper", "brewing_stand", "cauldron", "enchanting_table", "lectern", "grindstone", "stonecutter",
	"daylight_detector", "end_rod", "amethyst_cluster", "amethyst_bud", "pointed_dripstone", "spore",
	"petals", "sculk_vein", "sculk_sensor", "sculk_shrieker", "conduit", "glow_lichen",
	"structure_void", "barrier", "jigsaw", "dirt_path", "farmland", "soul_sand", "honey_block",
	"cactus", "bamboo", "sugar_cane", "wheat", "carrots", "beetroots", "melon_stem", "pumpkin_stem",
	"sweet_berry", "cocoa", "nether_wart", "hanging_roots", "heavy_core",
}

// fullBlockOverrides are blocks whose names contain a non-full hint but fill
// their cell anyway.
var fullBlockOverrides = map[string]bool{
	"minecraft:mushroom_stem": true, "minecraft:red_mushroom_block": true, "minecraft:brown_mushroom_block": true,
	"minecraft:bamboo_planks": true, "minecraft:bamboo_mosaic": true,
	"minecraft:muddy_mangrove_roots": true, "minecraft:barrier": true, "minecraft:chain_command_block": true,
}

// isFull guesses whether a block fills its whole cell, so that things can
// attach to it and water can't pass through it.
func isFull(bs structure.BlockState) bool {
	if isAir(bs.Name) {
		return false
	}
	if full, ok := fullBlockOverrides[bs.Name]; ok {
		return full
	}
	if bs.Name == "minecraft:light" || strings.HasSuffix(bs.Name, "chain") {
		return false
	}
	if strings.HasSuffix(bs.Name, "_block") && bs.Name != "minecraft:honey_block" {
		return true
	}
	if bs.Properties["type"] == "double" && strings.HasSuffix(bs.Name, "_slab") {
		return true
	}
	if bs.Properties["waterlogged"] == "true" {
		return false
	}
	for _, h := range nonFullHints {
		if strings.Contains(bs.Name, h) {
			return false
		}
	}
	return true
}

// mostCommon returns the most frequent string, breaking ties alphabetically.
func mostCommon(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	counts := map[string]int{}
	for _, x := range xs {
		counts[x]++
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return keys[0]
}

// namespaced adds the minecraft: namespace to an id that has none.
func namespaced(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || strings.Contains(id, ":") {
		return id
	}
	return "minecraft:" + id
}

// cloneCompound copies a compound deeply enough that edits to the copy's
// top-level keys and nested compounds or lists don't affect the original.
func cloneCompound(c nbt.Compound) nbt.Compound {
	if c == nil {
		return nil
	}
	out := make(nbt.Compound, len(c))
	for k, v := range c {
		out[k] = cloneTag(v)
	}
	return out
}

func cloneTag(t nbt.Tag) nbt.Tag {
	switch v := t.(type) {
	case nbt.Compound:
		return cloneCompound(v)
	case nbt.List:
		l := make(nbt.List, len(v))
		for i, e := range v {
			l[i] = cloneTag(e)
		}
		return l
	case nbt.ByteArray:
		return append(nbt.ByteArray(nil), v...)
	case nbt.IntArray:
		return append(nbt.IntArray(nil), v...)
	case nbt.LongArray:
		return append(nbt.LongArray(nil), v...)
	}
	return t
}

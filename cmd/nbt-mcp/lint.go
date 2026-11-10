package main

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/MinecraftMetascript/nbt-go"
	"github.com/MinecraftMetascript/nbt-go/structure"
)

// lintChecks lists every check lint_structure runs, in report order.
var lintChecks = []struct{ name, about string }{
	{"format", "the file's structure: palette, block positions, entities"},
	{"renamed", "block ids that were renamed between game versions"},
	{"newer", "blocks added in a game version newer than the file's DataVersion"},
	{"connections", "walls, fences, panes and bars whose connections don't match their neighbours"},
	{"support", "torches, lanterns, vines, signs, buttons and other blocks with nothing to attach to"},
	{"gravity", "sand, gravel, concrete powder and other falling blocks with nothing under them"},
	{"halves", "doors, beds and tall plants missing their other half"},
	{"fire", "fire and lava next to blocks that burn"},
	{"entities", "mobs, armor stands and other entities that start inside a block's collision box and will be stuck"},
	{"unsupported", "solid blocks with nothing anywhere below them, such as legs hanging past the edge of a plinth"},
	{"floating", "groups of blocks that don't touch the bottom layer or anything connected to it"},
	{"containers", "chests, barrels and other containers with no loot table or items"},
	{"valuables", "high-value blocks such as gold, diamond and netherite blocks"},
	{"jigsaw", "jigsaw block settings"},
}

type lintIn struct {
	Name   string   `json:"name"`
	Checks []string `json:"checks,omitempty" enum:"format,renamed,newer,connections,support,gravity,halves,fire,entities,unsupported,floating,containers,valuables,jigsaw" jsonschema:"which checks to run (default all): format, renamed, newer, connections, support, gravity, halves, fire, entities, unsupported, floating, containers, valuables, jigsaw"`
	Limit  int      `json:"limit,omitempty" jsonschema:"most findings to list per check (default 10)"`
}

type finding struct {
	pos vec
	msg string
}

type linter struct {
	st  *structure.Structure
	g   *grid
	out map[string][]finding
}

func (l *linter) add(check string, p vec, format string, args ...any) {
	l.out[check] = append(l.out[check], finding{p, fmt.Sprintf(format, args...)})
}

// describe names what is at p for messages.
func (l *linter) describe(p vec) string {
	bs, _, ok := l.g.get(p)
	switch {
	case !ok:
		return "no block"
	case isAir(bs.Name):
		return "air"
	}
	return bs.String()
}

func (s *state) lint(in lintIn) (string, lintOut, error) {
	var out lintOut
	st, err := s.get(in.Name)
	if err != nil {
		return "", out, err
	}
	run := map[string]bool{}
	for _, c := range in.Checks {
		found := false
		for _, lc := range lintChecks {
			if lc.name == c {
				found = true
			}
		}
		if !found {
			names := make([]string, len(lintChecks))
			for i, lc := range lintChecks {
				names[i] = lc.name
			}
			return "", out, fmt.Errorf("unknown check %q; use %s", c, strings.Join(names, ", "))
		}
		run[c] = true
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}
	l := &linter{st: st, g: newGrid(st), out: map[string][]finding{}}
	checks := map[string]func(){
		"format": l.format, "renamed": l.renamed, "newer": l.newer, "connections": l.connections, "support": l.support,
		"gravity": l.gravity, "halves": l.halves, "fire": l.fire, "entities": l.entities,
		"unsupported": l.unsupported, "floating": l.floating, "containers": l.containers,
		"valuables": l.valuables, "jigsaw": l.jigsaw,
	}
	var b strings.Builder
	total := 0
	var clean []string
	for _, lc := range lintChecks {
		if len(run) > 0 && !run[lc.name] {
			continue
		}
		checks[lc.name]()
		fs := l.out[lc.name]
		if len(fs) == 0 {
			clean = append(clean, lc.name)
			continue
		}
		total += len(fs)
		co := lintCheckOut{Check: lc.name, About: lc.about, Count: len(fs), Findings: []lintFinding{}}
		fmt.Fprintf(&b, "\n%s (%d): %s\n", lc.name, len(fs), lc.about)
		for i, f := range fs {
			if i == limit {
				fmt.Fprintf(&b, "  ... and %d more\n", len(fs)-limit)
				break
			}
			if f.pos == noPos {
				co.Findings = append(co.Findings, lintFinding{Message: f.msg})
				fmt.Fprintf(&b, "  %s\n", f.msg)
			} else {
				pos := [3]int(f.pos)
				co.Findings = append(co.Findings, lintFinding{Pos: &pos, Message: f.msg})
				fmt.Fprintf(&b, "  %v %s\n", f.pos, f.msg)
			}
		}
		out.Checks = append(out.Checks, co)
	}
	out.Findings, out.Clean = total, clean
	head := fmt.Sprintf("Lint of %q: %d findings.", in.Name, total)
	if len(clean) > 0 {
		head += " No problems found by: " + strings.Join(clean, ", ") + "."
	}
	if total > 0 {
		b.WriteString("\nThese checks use name-based rules of thumb (the server has no block registry), so treat them as things to look at, not certain errors.\n")
	}
	return head + "\n" + b.String(), out, nil
}

// noPos marks a finding about the whole structure.
var noPos = vec{-1, -1, -1}

func (l *linter) blocks(f func(p vec, bs structure.BlockState, data nbt.Compound)) {
	for _, b := range l.st.Blocks {
		bs, ok := stateOf(l.st, b)
		if !ok {
			continue
		}
		f(toVec(b.Pos), bs, b.NBT)
	}
}

func (l *linter) format() {
	st := l.st
	if err := st.Validate(); err != nil {
		l.add("format", noPos, "%v", strings.TrimPrefix(err.Error(), "structure: "))
	}
	if st.DataVersion < 1343 || st.DataVersion > 20000 {
		l.add("format", noPos, "DataVersion %d doesn't look like a Java Edition version from 1.12 or later", st.DataVersion)
	}
	seen := map[vec]int{}
	used := make([]int, len(palette(st)))
	for i, b := range st.Blocks {
		p := toVec(b.Pos)
		if j, ok := seen[p]; ok {
			l.add("format", p, "blocks %d and %d are both at this position; the game keeps only one", j, i)
		}
		seen[p] = i
		if b.State >= 0 && int(b.State) < len(used) {
			used[b.State]++
		}
		for _, k := range []string{"x", "y", "z"} {
			if _, ok := b.NBT[k]; ok {
				l.add("format", p, "block entity data has an %q key; structure files leave out x, y and z", k)
				break
			}
		}
	}
	for i, n := range used {
		if n == 0 {
			l.add("format", noPos, "palette entry %d (%s) isn't used by any block", i, palette(st)[i])
		}
	}
	l.blocks(func(p vec, bs structure.BlockState, _ nbt.Compound) {
		if bs.Name == "minecraft:structure_void" {
			l.add("format", p, "is a structure_void block; the game places it as an invisible block instead of leaving the world alone. Remove the block instead")
		}
	})
	for i, e := range st.Entities {
		var want [3]int32
		for a := 0; a < 3; a++ {
			want[a] = int32(math.Floor(e.Pos[a]))
		}
		if e.BlockPos != want {
			l.add("format", toVec(e.BlockPos), "entity %d (%v) has blockPos %v but its pos %v is in block %v", i, e.NBT["id"], e.BlockPos, e.Pos, want)
		}
		for _, k := range []string{"UUID", "UUIDMost", "UUIDLeast"} {
			if _, ok := e.NBT[k]; ok {
				l.add("format", toVec(e.BlockPos), "entity %d (%v) has a fixed %s, so every copy of the structure creates entities with the same UUID; remove it", i, e.NBT["id"], k)
				break
			}
		}
	}
	if st.Size[0] > 48 || st.Size[1] > 48 || st.Size[2] > 48 {
		l.add("format", noPos, "size %v is over 48 on some axis: a structure block can't save or load it, but /place template and jigsaw structures can", st.Size)
	}
}

type rename struct {
	old, new, version string
	dataVersion       int32
}

// renames lists block ids that changed. The game upgrades the old id when
// it loads a file whose DataVersion is older than the rename.
var renames = []rename{
	{"minecraft:sign", "minecraft:oak_sign", "1.14", 1952},
	{"minecraft:wall_sign", "minecraft:oak_wall_sign", "1.14", 1952},
	{"minecraft:grass_path", "minecraft:dirt_path", "1.17", 2724},
	{"minecraft:grass", "minecraft:short_grass", "1.20.3", 3698},
	{"minecraft:chain", "minecraft:iron_chain", "1.21.9", 4554},
}

func (l *linter) renamed() {
	dv := l.st.DataVersion
	counts := map[string]int{}
	first := map[string]vec{}
	l.blocks(func(p vec, bs structure.BlockState, _ nbt.Compound) {
		if _, ok := first[bs.Name]; !ok {
			first[bs.Name] = p
		}
		counts[bs.Name]++
	})
	for _, r := range renames {
		if n := counts[r.old]; n > 0 && dv >= r.dataVersion {
			l.add("renamed", first[r.old], "%d × %s: this id was renamed to %s in %s (DataVersion %d). The file's DataVersion %d is newer, so the game won't upgrade it and these blocks will be missing",
				n, r.old, r.new, r.version, r.dataVersion, dv)
		}
		if n := counts[r.new]; n > 0 && dv < r.dataVersion {
			l.add("renamed", first[r.new], "%d × %s: this id only exists from %s (DataVersion %d), but the file says DataVersion %d. Newer games accept it, but the version the file claims to be for won't; use %s or raise the DataVersion",
				n, r.new, r.version, r.dataVersion, dv, r.old)
		}
	}
}

// newer flags blocks that the game version the file claims to be for doesn't
// have yet. Newer games load them fine, so this only matters when the file is
// meant for that older version.
func (l *linter) newer() {
	dv := l.st.DataVersion
	counts := map[string]int{}
	first := map[string]vec{}
	l.blocks(func(p vec, bs structure.BlockState, _ nbt.Compound) {
		if v, ok := addedIn[bs.Name]; ok && dv < v.dataVersion {
			if _, ok := first[bs.Name]; !ok {
				first[bs.Name] = p
			}
			counts[bs.Name]++
		}
	})
	names := make([]string, 0, len(counts))
	for n := range counts {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		v := addedIn[n]
		l.add("newer", first[n], "%d × %s: added in %s (DataVersion %d), but the file says DataVersion %s. %s and later load it; for older games pick another block, or raise the DataVersion if you only target %s+",
			counts[n], n, v.name, v.dataVersion, dataVersionLabel(dv), v.name, v.name)
	}
}

// connectFamily groups blocks with north/east/south/west connections.
func connectFamily(name string) string {
	switch {
	case strings.HasSuffix(name, "_fence_gate"):
		return ""
	case name == "minecraft:nether_brick_fence":
		return "nether_fence"
	case strings.HasSuffix(name, "_fence"):
		return "fence"
	case strings.HasSuffix(name, "_wall") && !strings.Contains(name, "wall_") && name != "minecraft:wall":
		return "wall"
	case strings.HasSuffix(name, "glass_pane"), name == "minecraft:iron_bars", strings.HasSuffix(name, "_bars"):
		return "pane"
	}
	return ""
}

// connectsTo reports whether a block of family fam would connect toward a
// neighbour on side d.
func connectsTo(fam string, nb structure.BlockState, d dir) bool {
	nf := connectFamily(nb.Name)
	switch {
	case nf == fam:
		return true
	case fam == "wall" && nf == "pane", fam == "pane" && nf == "wall":
		return true
	case strings.HasSuffix(nb.Name, "_fence_gate") && (fam == "fence" || fam == "wall"):
		if f, ok := parseDir(nb.Properties["facing"]); ok {
			return f != d && f != d.opposite()
		}
		return false
	case strings.HasSuffix(nb.Name, "_leaves"):
		return false
	}
	return isFull(nb)
}

func (l *linter) connections() {
	l.blocks(func(p vec, bs structure.BlockState, _ nbt.Compound) {
		fam := connectFamily(bs.Name)
		if fam == "" {
			return
		}
		var wrong []string
		for _, d := range []dir{north, east, south, west} {
			v, ok := bs.Properties[d.String()]
			if !ok {
				continue
			}
			has := v == "true" || v == "low" || v == "tall"
			q := p.add(d.vec())
			nb, _, exists := l.g.get(q)
			switch {
			case !exists && has:
				wrong = append(wrong, fmt.Sprintf("%s=%s but there's no block to the %s (it depends on the world)", d, v, d))
			case !exists:
			case has && !connectsTo(fam, nb, d):
				wrong = append(wrong, fmt.Sprintf("%s=%s but to the %s is %s, which it can't connect to", d, v, d, l.describe(q)))
			case !has && connectsTo(fam, nb, d):
				wrong = append(wrong, fmt.Sprintf("%s=%s but to the %s is %s, which it would connect to", d, v, d, nb))
			}
		}
		if len(wrong) > 0 {
			l.add("connections", p, "%s: %s", bs.Name, strings.Join(wrong, "; "))
		}
	})
}

// supportsNeeded returns the directions in which a block needs a neighbour
// to stay in place, with a description of the attachment.
func supportsNeeded(bs structure.BlockState) []dir {
	name := strings.TrimPrefix(bs.Name, "minecraft:")
	props := bs.Properties
	facing, hasFacing := parseDir(props["facing"])
	behind := func() []dir {
		if hasFacing {
			return []dir{facing.opposite()}
		}
		return nil
	}
	switch {
	case name == "vine", name == "glow_lichen", name == "sculk_vein", name == "resin_clump":
		var ds []dir
		for _, d := range []dir{north, east, south, west, up, down} {
			if props[d.String()] == "true" {
				ds = append(ds, d)
			}
		}
		return ds
	case name == "lantern", name == "soul_lantern", strings.HasSuffix(name, "copper_lantern"):
		if props["hanging"] == "true" {
			return []dir{up}
		}
		return []dir{down}
	case strings.HasSuffix(name, "_wall_hanging_sign"):
		return nil
	case strings.HasSuffix(name, "_hanging_sign"), name == "spore_blossom", name == "hanging_roots", name == "pale_hanging_moss":
		return []dir{up}
	case strings.HasSuffix(name, "wall_torch"), name == "ladder", strings.HasSuffix(name, "_wall_sign"),
		strings.HasSuffix(name, "_wall_banner"), strings.HasSuffix(name, "_wall_head"), strings.HasSuffix(name, "_wall_skull"),
		strings.HasSuffix(name, "_wall_fan"), name == "tripwire_hook", name == "cocoa",
		strings.HasSuffix(name, "amethyst_cluster"), strings.HasSuffix(name, "amethyst_bud"):
		if name == "cocoa" && hasFacing {
			return []dir{facing}
		}
		if strings.Contains(name, "amethyst") {
			if hasFacing {
				return []dir{facing.opposite()}
			}
			return nil
		}
		return behind()
	case strings.HasSuffix(name, "_button"), name == "lever", name == "grindstone":
		switch props["face"] {
		case "floor":
			return []dir{down}
		case "ceiling":
			return []dir{up}
		case "wall":
			return behind()
		}
		return nil
	case name == "bell":
		switch props["attachment"] {
		case "floor":
			return []dir{down}
		case "ceiling":
			return []dir{up}
		case "single_wall":
			return behind()
		case "double_wall":
			if hasFacing {
				return []dir{facing, facing.opposite()}
			}
		}
		return nil
	case name == "pointed_dripstone", name == "sulfur_spike":
		if props["vertical_direction"] == "down" {
			return []dir{up}
		}
		return []dir{down}
	case strings.HasSuffix(name, "_door"):
		return []dir{down}
	case name == "torch", name == "soul_torch", name == "redstone_torch", name == "copper_torch",
		strings.HasSuffix(name, "_sign"), strings.HasSuffix(name, "_banner"), strings.HasSuffix(name, "_carpet"),
		strings.HasSuffix(name, "_pressure_plate"), strings.HasSuffix(name, "rail"), name == "redstone_wire",
		name == "repeater", name == "comparator", strings.HasSuffix(name, "candle"), name == "flower_pot",
		strings.HasPrefix(name, "potted_"), name == "short_grass", name == "tall_grass", name == "fern",
		name == "large_fern", name == "dead_bush", strings.HasSuffix(name, "_sapling"), name == "sea_pickle",
		name == "snow", strings.HasSuffix(name, "_mushroom") && name != "shelf_mushroom", strings.HasSuffix(name, "_tulip"),
		strings.HasSuffix(name, "_coral"), name == "dandelion", name == "golden_dandelion", name == "red_shrub", name == "poppy", name == "blue_orchid",
		name == "allium", name == "azure_bluet", name == "oxeye_daisy", name == "cornflower",
		name == "lily_of_the_valley", name == "wither_rose", name == "torchflower", name == "sunflower",
		name == "lilac", name == "rose_bush", name == "peony", strings.HasSuffix(name, "_petals"),
		name == "sweet_berry_bush", name == "bush", name == "firefly_bush":
		return []dir{down}
	}
	return nil
}

func (l *linter) support() {
	l.blocks(func(p vec, bs structure.BlockState, _ nbt.Compound) {
		for _, d := range supportsNeeded(bs) {
			q := p.add(d.vec())
			nb, _, ok := l.g.get(q)
			switch {
			case !ok && q[1] < 0:
				// Resting on whatever is below the structure is normal.
			case !ok:
				l.add("support", p, "%s needs a block to the %s at %v, but the structure has no block there, so it depends on the world", bs, d, q)
			case isAir(nb.Name):
				l.add("support", p, "%s needs a block to the %s at %v, but there's only air, so it will pop off or float", bs, d, q)
			case bs.Name == "minecraft:vine" && nb.Name == "minecraft:vine" && d == up:
			case d == down && bs.Properties["half"] == "upper" && nb.Name == bs.Name:
				// The upper half of a door or tall plant stands on its lower half.
			case !isFull(nb) && !sturdyEnough(bs, nb):
				l.add("support", p, "%s attaches to the %s, to %s, which probably can't hold it", bs, d, nb)
			}
		}
	})
}

// sturdyEnough allows a few common non-full supports.
func sturdyEnough(bs, nb structure.BlockState) bool {
	n := nb.Name
	switch {
	case strings.HasSuffix(bs.Name, "lantern") || strings.HasSuffix(bs.Name, "chain"):
		return strings.Contains(n, "chain") || strings.Contains(n, "fence") || strings.HasSuffix(n, "_wall") || strings.Contains(n, "slab") || strings.Contains(n, "stairs") || strings.Contains(n, "lantern")
	case strings.HasSuffix(bs.Name, "_carpet") || strings.HasPrefix(bs.Name, "minecraft:potted_") || bs.Name == "minecraft:flower_pot" || strings.HasSuffix(bs.Name, "candle"):
		return true
	case strings.Contains(n, "slab") && nb.Properties["type"] != "bottom", strings.Contains(n, "stairs") && nb.Properties["half"] == "top":
		return true
	case n == "minecraft:farmland" || n == "minecraft:dirt_path" || n == "minecraft:soul_sand" || n == "minecraft:mud":
		return true
	case strings.HasSuffix(n, "_leaves"):
		return true
	}
	return false
}

func (l *linter) unsupported() {
	// For each column, the lowest solid block below each y.
	type col struct{ x, z int }
	lowest := map[col]int{}
	l.blocks(func(p vec, bs structure.BlockState, _ nbt.Compound) {
		if isAir(bs.Name) {
			return
		}
		c := col{p[0], p[2]}
		if y, ok := lowest[c]; !ok || p[1] < y {
			lowest[c] = p[1]
		}
	})
	type region struct {
		b box
		n int
		y int
	}
	var hits []vec
	for c, y := range lowest {
		if y == 0 {
			continue
		}
		p := vec{c.x, y, c.z}
		bs, _, _ := l.g.get(p)
		if !isFull(bs) || strings.HasSuffix(bs.Name, "_leaves") {
			continue
		}
		hits = append(hits, p)
	}
	// Group hits that touch horizontally at the same height.
	seen := map[vec]bool{}
	sort.Slice(hits, func(i, j int) bool { return lessVec(hits[i], hits[j]) })
	in := map[vec]bool{}
	for _, h := range hits {
		in[h] = true
	}
	var regions []region
	for _, h := range hits {
		if seen[h] {
			continue
		}
		r := region{b: box{h, h}, y: h[1]}
		stack := []vec{h}
		seen[h] = true
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			r.n++
			for a := 0; a < 3; a++ {
				r.b.min[a], r.b.max[a] = min(r.b.min[a], p[a]), max(r.b.max[a], p[a])
			}
			for _, d := range []dir{north, south, east, west} {
				q := p.add(d.vec())
				if in[q] && !seen[q] {
					seen[q] = true
					stack = append(stack, q)
				}
			}
		}
		regions = append(regions, r)
	}
	sort.SliceStable(regions, func(i, j int) bool { return regions[i].n > regions[j].n })
	for _, r := range regions {
		bs, _, _ := l.g.get(r.b.min)
		l.add("unsupported", r.b.min, "%d block(s) at %s, such as %s, have nothing anywhere below them in the structure, so they hang over whatever the world has there", r.n, r.b, bs)
	}
}

func lessVec(a, b vec) bool {
	if a[1] != b[1] {
		return a[1] < b[1]
	}
	if a[2] != b[2] {
		return a[2] < b[2]
	}
	return a[0] < b[0]
}

func (l *linter) floating() {
	solid := map[vec]bool{}
	l.blocks(func(p vec, bs structure.BlockState, _ nbt.Compound) {
		if !isAir(bs.Name) {
			solid[p] = true
		}
	})
	// Flood from every block on the bottom layer.
	grounded := map[vec]bool{}
	var stack []vec
	for p := range solid {
		if p[1] == 0 {
			grounded[p] = true
			stack = append(stack, p)
		}
	}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, d := range dirVecs {
			q := p.add(d)
			if solid[q] && !grounded[q] {
				grounded[q] = true
				stack = append(stack, q)
			}
		}
	}
	seen := map[vec]bool{}
	var keys []vec
	for p := range solid {
		if !grounded[p] {
			keys = append(keys, p)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return lessVec(keys[i], keys[j]) })
	type group struct {
		b box
		n int
	}
	var groups []group
	for _, start := range keys {
		if seen[start] {
			continue
		}
		g := group{b: box{start, start}}
		stack := []vec{start}
		seen[start] = true
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			g.n++
			for a := 0; a < 3; a++ {
				g.b.min[a], g.b.max[a] = min(g.b.min[a], p[a]), max(g.b.max[a], p[a])
			}
			for _, d := range dirVecs {
				q := p.add(d)
				if solid[q] && !grounded[q] && !seen[q] {
					seen[q] = true
					stack = append(stack, q)
				}
			}
		}
		groups = append(groups, g)
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].n > groups[j].n })
	for _, g := range groups {
		l.add("floating", g.b.min, "a group of %d block(s) at %s, such as %s, isn't connected to the bottom layer", g.n, g.b, l.describe(g.b.min))
	}
}

var containerNames = []string{"chest", "trapped_chest", "barrel", "shulker_box", "dispenser", "dropper", "hopper", "decorated_pot", "chiseled_bookshelf", "crafter"}

func isContainer(name string) bool {
	n := strings.TrimPrefix(name, "minecraft:")
	for _, c := range containerNames {
		if n == c || strings.HasSuffix(n, "_"+c) {
			return true
		}
	}
	return false
}

func (l *linter) containers() {
	l.blocks(func(p vec, bs structure.BlockState, data nbt.Compound) {
		if !isContainer(bs.Name) {
			return
		}
		if lt, ok := data["LootTable"].(nbt.String); ok && lt != "" {
			return
		}
		if items, ok := data["Items"].(nbt.List); ok && len(items) > 0 {
			return
		}
		if bs.Name == "minecraft:decorated_pot" || bs.Name == "minecraft:chiseled_bookshelf" || bs.Name == "minecraft:crafter" {
			return
		}
		l.add("containers", p, "%s has no LootTable and no Items, so it will be empty; set block data like {LootTable:\"minecraft:chests/simple_dungeon\"}", bs.Name)
	})
}

// valuableBlocks are blocks a player would mine for profit.
var valuableBlocks = map[string]bool{
	"minecraft:gold_block": true, "minecraft:diamond_block": true, "minecraft:emerald_block": true,
	"minecraft:netherite_block": true, "minecraft:iron_block": true, "minecraft:lapis_block": true,
	"minecraft:raw_gold_block": true, "minecraft:raw_iron_block": true, "minecraft:ancient_debris": true,
	"minecraft:beacon": true, "minecraft:diamond_ore": true, "minecraft:deepslate_diamond_ore": true,
	"minecraft:emerald_ore": true, "minecraft:deepslate_emerald_ore": true, "minecraft:gold_ore": true,
	"minecraft:deepslate_gold_ore": true, "minecraft:enchanting_table": true, "minecraft:conduit": true,
	"minecraft:heavy_core": true,
}

func (l *linter) valuables() {
	counts := map[string]int{}
	first := map[string]vec{}
	l.blocks(func(p vec, bs structure.BlockState, _ nbt.Compound) {
		if valuableBlocks[bs.Name] {
			if counts[bs.Name] == 0 {
				first[bs.Name] = p
			}
			counts[bs.Name]++
		}
	})
	names := make([]string, 0, len(counts))
	for n := range counts {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return counts[names[i]] > counts[names[j]] })
	for _, n := range names {
		l.add("valuables", first[n], "%d × %s, which players can mine and keep; make sure that is intended, or use a look-alike such as %s", counts[n], n, lookalike(n))
	}
}

func lookalike(name string) string {
	switch name {
	case "minecraft:gold_block", "minecraft:raw_gold_block":
		return "yellow_glazed_terracotta, honeycomb_block or yellow_concrete"
	case "minecraft:diamond_block":
		return "light_blue_concrete or warped_wart_block"
	case "minecraft:emerald_block":
		return "lime_concrete or green_glazed_terracotta"
	case "minecraft:iron_block", "minecraft:raw_iron_block":
		return "smooth_stone or light_gray_concrete"
	case "minecraft:netherite_block":
		return "polished_blackstone or black_concrete"
	case "minecraft:lapis_block":
		return "blue_concrete"
	}
	return "a decorative block"
}

func (l *linter) jigsaw() {
	g := l.g
	for _, j := range jigsawsOf(l.st) {
		for _, w := range lintJigsaw(l.st, g, j) {
			l.add("jigsaw", j.pos, "%s", w)
		}
	}
}

func isGravityBlock(name string) bool {
	n := strings.TrimPrefix(name, "minecraft:")
	switch n {
	case "sand", "red_sand", "gravel", "suspicious_sand", "suspicious_gravel", "anvil", "chipped_anvil",
		"damaged_anvil", "dragon_egg":
		return true
	}
	return strings.HasSuffix(n, "_concrete_powder")
}

// letsBlocksFall reports whether a falling block above this one would drop
// into it, like the game's FallingBlock.isFree: air, fire, liquids and
// replaceable plants.
func letsBlocksFall(bs structure.BlockState) bool {
	n := strings.TrimPrefix(bs.Name, "minecraft:")
	switch n {
	case "air", "cave_air", "void_air", "water", "lava", "fire", "soul_fire", "short_grass", "tall_grass",
		"fern", "large_fern", "dead_bush", "vine", "seagrass", "tall_seagrass", "glow_lichen", "light",
		"structure_void", "bush":
		return true
	case "snow":
		return bs.Properties["layers"] == "" || bs.Properties["layers"] == "1"
	}
	return false
}

func (l *linter) gravity() {
	l.blocks(func(p vec, bs structure.BlockState, _ nbt.Compound) {
		if !isGravityBlock(bs.Name) || p[1] == 0 {
			return
		}
		q := p.add(down.vec())
		nb, _, ok := l.g.get(q)
		switch {
		case !ok:
			// Unchanged world positions are only a problem if nothing below
			// in the structure holds it up.
			for y := q[1] - 1; y >= 0; y-- {
				if _, _, ok := l.g.get(vec{p[0], y, p[2]}); ok {
					l.add("gravity", p, "%s has no block under it at %v, so it will fall unless the world has something there", bs.Name, q)
					return
				}
			}
		case letsBlocksFall(nb):
			l.add("gravity", p, "%s is above %s at %v, so it will fall as soon as it's updated", bs.Name, l.describe(q), q)
		}
	})
}

func (l *linter) halves() {
	l.blocks(func(p vec, bs structure.BlockState, _ nbt.Compound) {
		props := bs.Properties
		if strings.HasSuffix(bs.Name, "_bed") {
			facing, ok := parseDir(props["facing"])
			if !ok {
				return
			}
			want, other := "head", p.add(facing.vec())
			if props["part"] == "head" {
				want, other = "foot", p.add(facing.opposite().vec())
			}
			nb, _, exists := l.g.get(other)
			if !exists || nb.Name != bs.Name || nb.Properties["part"] != want || nb.Properties["facing"] != props["facing"] {
				l.add("halves", p, "%s needs its %s at %v (same bed, facing %s), but there is %s", bs, want, other, props["facing"], l.describe(other))
			}
			return
		}
		half := props["half"]
		if half != "lower" && half != "upper" {
			return
		}
		want, other := "upper", p.add(up.vec())
		if half == "upper" {
			want, other = "lower", p.add(down.vec())
		}
		nb, _, exists := l.g.get(other)
		ok := exists && nb.Name == bs.Name && nb.Properties["half"] == want
		if ok && strings.HasSuffix(bs.Name, "_door") {
			for _, k := range []string{"facing", "hinge"} {
				if nb.Properties[k] != props[k] {
					l.add("halves", p, "%s and its %s half at %v disagree on %s (%s vs %s)", bs.Name, want, other, k, props[k], nb.Properties[k])
				}
			}
		}
		if !ok {
			l.add("halves", p, "%s needs its %s half at %v, but there is %s", bs, want, other, l.describe(other))
		}
	})
}

// woodTypes are the overworld woods; crimson and warped wood don't burn.
var woodTypes = []string{"oak", "spruce", "birch", "jungle", "acacia", "dark_oak", "mangrove", "cherry", "bamboo", "pale_oak", "poplar", "azalea"}

// isFlammable guesses whether fire can burn or spread to a block.
func isFlammable(name string) bool {
	n := strings.TrimPrefix(name, "minecraft:")
	if strings.Contains(n, "crimson") || strings.Contains(n, "warped") || strings.Contains(n, "potted") {
		return false
	}
	for _, w := range woodTypes {
		if strings.HasPrefix(n, w+"_") || strings.HasPrefix(n, "stripped_"+w+"_") {
			return true
		}
	}
	for _, f := range []string{"_wool", "_carpet", "leaves", "bookshelf", "hay_block", "tnt", "scaffolding",
		"vine", "short_grass", "tall_grass", "fern", "dried_kelp_block", "target", "beehive", "bee_nest",
		"lectern", "composter", "coal_block", "moss_block", "moss_carpet", "pale_moss", "_banner", "dead_bush", "sweet_berry_bush", "red_shrub",
		"dandelion", "poppy", "orchid", "allium", "bluet", "tulip", "daisy", "cornflower", "lily_of_the_valley",
		"sunflower", "lilac", "rose_bush", "peony", "torchflower", "_petals", "spore_blossom", "hanging_roots",
		"cave_vines", "big_dripleaf", "small_dripleaf", "glow_lichen", "_bed"} {
		if strings.Contains(n, f) {
			return true
		}
	}
	return false
}

func (l *linter) fire() {
	l.blocks(func(p vec, bs structure.BlockState, _ nbt.Compound) {
		var dy0, dy1, reach int
		switch bs.Name {
		case "minecraft:fire":
			// Fire spreads to blocks up to one away sideways, one below and
			// four above.
			dy0, dy1, reach = -1, 4, 1
		case "minecraft:lava":
			// Lava sets fire to the air beside flammable blocks within a
			// few blocks, mostly above it.
			dy0, dy1, reach = -1, 2, 1
		default:
			return
		}
		var near []vec
		for dx := -reach; dx <= reach; dx++ {
			for dz := -reach; dz <= reach; dz++ {
				for dy := dy0; dy <= dy1; dy++ {
					q := p.add(vec{dx, dy, dz})
					if nb, _, ok := l.g.get(q); ok && q != p && isFlammable(nb.Name) {
						near = append(near, q)
					}
				}
			}
		}
		if len(near) > 0 {
			first, _, _ := l.g.get(near[0])
			l.add("fire", p, "%s is within reach of %d flammable block(s), such as %s at %v, so it can start a fire", bs.Name, len(near), first.Name, near[0])
		}
	})
}

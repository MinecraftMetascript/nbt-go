package main

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/MinecraftMetascript/nbt-go/structure"
)

// critique_structure judges how a build looks, where lint_structure judges
// whether it is correct. Every measure is a heuristic over the blocks a player
// would see from outside, so the report suggests fixes rather than errors.

type critiqueIn struct {
	Name  string  `json:"name"`
	Style string  `json:"style,omitempty" jsonschema:"survival (practical, common materials, lighter detail) or creative (showpiece, rich palette, dense detail); default creative"`
	From  *[3]int `json:"from,omitempty" jsonschema:"critique only a box, such as one building in a larger structure: first corner [x, y, z]"`
	To    *[3]int `json:"to,omitempty" jsonschema:"opposite corner [x, y, z], inclusive"`
	Limit int     `json:"limit,omitempty" jsonschema:"most findings to list per category (default 3)"`
}

// styleTargets are the thresholds each style is judged against.
type styleTargets struct {
	maxFlat       int     // largest same-block, same-depth facade patch before it reads as a flat wall
	minPalette    int     // distinct visible block types
	maxPalette    int     // more than this reads as busy (0 for no limit)
	maxDominant   float64 // largest share of the visible surface one block may have
	detailShare   float64 // share of the visible surface in partial blocks for full marks
	rectPenalty   float64 // how much a plain rectangular footprint costs the silhouette score
	symmetryScore [2]float64
	lavish        bool // penalize expensive blocks
}

var styles = map[string]styleTargets{
	"survival": {maxFlat: 42, minPalette: 3, maxPalette: 10, maxDominant: 0.7, detailShare: 0.10, rectPenalty: 0.2, symmetryScore: [2]float64{0.85, 0.65}, lavish: true},
	"creative": {maxFlat: 24, minPalette: 8, maxDominant: 0.45, detailShare: 0.22, rectPenalty: 0.5, symmetryScore: [2]float64{0.6, 0.3}},
}

type category struct {
	key, title string
	weight     float64
	score      float64 // 0 to 1
	summary    string
	findings   []string
}

// facadeCell is the first non-air block seen along one line of sight.
type facadeCell struct {
	ok    bool
	pos   vec
	depth int
	bs    structure.BlockState
}

type facade struct {
	side       string
	cols, rows []int
	cells      [][]facadeCell // [row][col]
	va         viewAxes
}

func project(g *grid, r box, side string) facade {
	va, _ := parseView(side)
	f := facade{side: side, va: va,
		cols: span(r.min[va.col], r.max[va.col], va.colRev),
		rows: span(r.min[va.row], r.max[va.row], va.rowRev)}
	depths := span(r.min[va.depth], r.max[va.depth], va.depthRev)
	f.cells = make([][]facadeCell, len(f.rows))
	for i, rv := range f.rows {
		f.cells[i] = make([]facadeCell, len(f.cols))
		for j, cv := range f.cols {
			for k, dv := range depths {
				var p vec
				p[va.col], p[va.row], p[va.depth] = cv, rv, dv
				bs, _, ok := g.get(p)
				if !ok || isAir(bs.Name) {
					continue
				}
				f.cells[i][j] = facadeCell{true, p, k, bs}
				break
			}
		}
	}
	return f
}

func (s *state) critique(in critiqueIn) (string, error) {
	st, err := s.get(in.Name)
	if err != nil {
		return "", err
	}
	style := in.Style
	if style == "" {
		style = "creative"
	}
	t, ok := styles[style]
	if !ok {
		return "", fmt.Errorf("style %q must be survival or creative", in.Style)
	}
	r, err := regionOf(st, in.From, in.To)
	if err != nil {
		return "", err
	}
	if sz := r.size(); sz[0]*sz[1] > maxViewCells || sz[1]*sz[2] > maxViewCells || sz[0]*sz[2] > maxViewCells {
		return "", fmt.Errorf("the region %s is too large to critique (max %d cells per side); limit it with from and to", r, maxViewCells)
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 3
	}
	g := newGrid(st)
	c := &critic{st: st, g: g, r: r, t: t, limit: limit}
	for _, side := range []string{"south", "north", "east", "west"} {
		c.sides = append(c.sides, project(g, r, side))
	}
	c.top = project(g, r, "top")
	c.collectSurface()
	if len(c.surface) == 0 {
		return "", fmt.Errorf("there are no blocks to critique in %s", r)
	}

	cats := []*category{c.depth(), c.palette(), c.detail(), c.silhouette(), c.overhang(), c.foundation(), c.openings(), c.symmetry()}
	if t.lavish {
		cats = append(cats, c.materials())
	}
	var total, weights float64
	for _, cat := range cats {
		total += cat.score * cat.weight
		weights += cat.weight
	}
	overall := int(math.Round(100 * total / weights))

	var b strings.Builder
	fmt.Fprintf(&b, "Critique of %q in %s style, region %s: %d/100\n\n", in.Name, style, r, overall)
	for _, cat := range cats {
		n := int(math.Round(cat.score * 10))
		fmt.Fprintf(&b, "  %-11s %3d  %s%s  %s\n", cat.key, int(math.Round(cat.score*100)), strings.Repeat("#", n), strings.Repeat(".", 10-n), cat.summary)
	}
	ranked := append([]*category(nil), cats...)
	sort.SliceStable(ranked, func(i, j int) bool {
		return (1-ranked[i].score)*ranked[i].weight > (1-ranked[j].score)*ranked[j].weight
	})
	b.WriteString("\nFixes, most valuable first:\n")
	listed := false
	for _, cat := range ranked {
		if len(cat.findings) == 0 {
			continue
		}
		listed = true
		fmt.Fprintf(&b, "\n%s (%s):\n", cat.title, cat.key)
		for _, f := range cat.findings {
			b.WriteString("  - " + f + "\n")
		}
	}
	if !listed {
		b.WriteString("  None. Judge the rest by eye with render_image.\n")
	}
	b.WriteString("\nThese are heuristics over what is visible from outside; a finding can be a deliberate choice. See building_guide for the techniques.\n")
	return b.String(), nil
}

type critic struct {
	st    *structure.Structure
	g     *grid
	r     box
	t     styleTargets
	limit int
	sides []facade // south, north, east, west
	top   facade
	// surface is every position seen from the top or a side, with its block.
	surface map[vec]structure.BlockState
}

func (c *critic) collectSurface() {
	c.surface = map[vec]structure.BlockState{}
	for _, f := range append(append([]facade(nil), c.sides...), c.top) {
		for _, row := range f.cells {
			for _, cell := range row {
				if cell.ok {
					c.surface[cell.pos] = cell.bs
				}
			}
		}
	}
}

func (c *critic) add(cat *category, format string, args ...any) {
	if len(cat.findings) < c.limit {
		cat.findings = append(cat.findings, fmt.Sprintf(format, args...))
	}
}

func shortName(name string) string { return strings.TrimPrefix(name, "minecraft:") }

func pct(f float64) string { return fmt.Sprintf("%d%%", int(math.Round(100*f))) }

// depth finds flat facade patches: connected areas seen at the same depth
// with the same block.
func (c *critic) depth() *category {
	cat := &category{key: "depth", title: "Flat walls", weight: 3}
	type patch struct {
		side   string
		area   int
		lo, hi vec
		block  string
		w, h   int
	}
	var patches []patch
	total, flat := 0, 0
	for _, f := range c.sides {
		seen := make([][]bool, len(f.rows))
		for i := range seen {
			seen[i] = make([]bool, len(f.cols))
		}
		for i := range f.cells {
			for j, cell := range f.cells[i] {
				if !cell.ok {
					continue
				}
				total++
				if seen[i][j] {
					continue
				}
				// Flood the patch.
				p := patch{side: f.side, block: cell.bs.Name, lo: cell.pos, hi: cell.pos}
				minI, maxI, minJ, maxJ := i, i, j, j
				stack := [][2]int{{i, j}}
				seen[i][j] = true
				for len(stack) > 0 {
					q := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					qc := f.cells[q[0]][q[1]]
					p.area++
					for a := 0; a < 3; a++ {
						p.lo[a], p.hi[a] = min(p.lo[a], qc.pos[a]), max(p.hi[a], qc.pos[a])
					}
					minI, maxI, minJ, maxJ = min(minI, q[0]), max(maxI, q[0]), min(minJ, q[1]), max(maxJ, q[1])
					for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
						ni, nj := q[0]+d[0], q[1]+d[1]
						if ni < 0 || nj < 0 || ni >= len(f.rows) || nj >= len(f.cols) || seen[ni][nj] {
							continue
						}
						nc := f.cells[ni][nj]
						if nc.ok && nc.depth == cell.depth && nc.bs.Name == cell.bs.Name {
							seen[ni][nj] = true
							stack = append(stack, [2]int{ni, nj})
						}
					}
				}
				p.w, p.h = maxJ-minJ+1, maxI-minI+1
				// A patch only reads as a wall when it is big in both directions.
				if p.area > c.t.maxFlat && p.w >= 4 && p.h >= 3 {
					flat += p.area
					patches = append(patches, p)
				}
			}
		}
	}
	sort.Slice(patches, func(i, j int) bool { return patches[i].area > patches[j].area })
	share := 0.0
	if total > 0 {
		share = float64(flat) / float64(total)
	}
	cat.score = math.Max(0, 1-share*1.6)
	cat.summary = fmt.Sprintf("%s of the side walls is in flat patches over %d blocks", pct(share), c.t.maxFlat)
	for _, p := range patches {
		c.add(cat, "%s side: %dx%d flat patch of %s from %v to %v. Add a frame (pillars every 3 to 5 blocks, a beam at floor level) 1 block proud of it, or set windows back.",
			p.side, p.w, p.h, shortName(p.block), p.lo, p.hi)
	}
	return cat
}

// palette counts the visible block types and how dominant the main one is.
func (c *critic) palette() *category {
	cat := &category{key: "palette", title: "Palette", weight: 2}
	counts := map[string]int{}
	for _, bs := range c.surface {
		counts[bs.Name]++
	}
	type kv struct {
		name string
		n    int
	}
	var list []kv
	for k, v := range counts {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].n != list[j].n {
			return list[i].n > list[j].n
		}
		return list[i].name < list[j].name
	})
	total := len(c.surface)
	// Blocks under 0.5% of the surface are specks, not part of the palette.
	distinct := 0
	for _, e := range list {
		if float64(e.n) >= 0.005*float64(total) {
			distinct++
		}
	}
	dom := float64(list[0].n) / float64(total)
	varietyScore := math.Min(1, float64(distinct)/float64(c.t.minPalette))
	domScore := 1.0
	if dom > c.t.maxDominant {
		domScore = math.Max(0, 1-(dom-c.t.maxDominant)/(1-c.t.maxDominant))
	}
	if c.t.maxPalette > 0 && distinct > c.t.maxPalette {
		varietyScore = math.Max(0.5, 1-float64(distinct-c.t.maxPalette)/10)
	}
	cat.score = 0.5*varietyScore + 0.5*domScore
	top := make([]string, 0, 4)
	for _, e := range list[:min(4, len(list))] {
		top = append(top, fmt.Sprintf("%s %s", shortName(e.name), pct(float64(e.n)/float64(total))))
	}
	cat.summary = fmt.Sprintf("%d block types visible; %s", distinct, strings.Join(top, ", "))
	if distinct < c.t.minPalette {
		c.add(cat, "Only %d block types show from outside; aim for at least %d. Give each role (foundation, frame, wall, roof, accent) its own block.", distinct, c.t.minPalette)
	}
	if c.t.maxPalette > 0 && distinct > c.t.maxPalette {
		c.add(cat, "%d block types show from outside, which is busy for a survival build; aim for %d to %d, with one main block per role.", distinct, c.t.minPalette, c.t.maxPalette)
	}
	if dom > c.t.maxDominant {
		c.add(cat, "%s is %s of the visible surface (aim under %s). Fill it as a weighted mix with related variants, e.g. %s.",
			shortName(list[0].name), pct(dom), pct(c.t.maxDominant), mixFor(list[0].name))
	}
	return cat
}

// mixFor suggests a weighted mix to texture a block.
func mixFor(name string) string {
	n := shortName(name)
	switch {
	case n == "stone_bricks":
		return "60%stone_bricks,20%cracked_stone_bricks,15%mossy_stone_bricks,5%andesite"
	case n == "cobblestone":
		return "70%cobblestone,20%mossy_cobblestone,10%andesite"
	case n == "deepslate_bricks" || n == "deepslate_tiles":
		return fmt.Sprintf("60%%%s,20%%cracked_%s,20%%cobbled_deepslate", n, n)
	case n == "sandstone" || n == "smooth_sandstone":
		return "60%smooth_sandstone,30%sandstone,10%cut_sandstone"
	case n == "bricks":
		return "80%bricks,10%granite,10%terracotta"
	case strings.HasSuffix(n, "_planks"):
		wood := strings.TrimSuffix(n, "_planks")
		return fmt.Sprintf("75%%%s_planks,25%%stripped_%s_wood", wood, wood)
	case n == "stone":
		return "60%stone,25%andesite,15%cobblestone"
	}
	return fmt.Sprintf("70%%%s plus two blocks of similar colour and different texture", n)
}

// detail measures the share of the visible surface in partial blocks.
func (c *critic) detail() *category {
	cat := &category{key: "detail", title: "Detail blocks", weight: 2.5}
	partial := 0
	for _, bs := range c.surface {
		if !isFull(bs) {
			partial++
		}
	}
	share := float64(partial) / float64(len(c.surface))
	cat.score = math.Min(1, share/c.t.detailShare)
	cat.summary = fmt.Sprintf("%s of the visible surface is stairs, slabs, fences, trapdoors and other partial blocks (target %s)", pct(share), pct(c.t.detailShare))
	if share < c.t.detailShare {
		c.add(cat, "Add partial blocks along edges: stair sills under windows, slab or stair bevels along wall tops, trapdoor shutters, fence railings, upside-down stair brackets under the eaves.")
	}
	return cat
}

// silhouette checks the footprint and the outline seen from the sides.
func (c *critic) silhouette() *category {
	cat := &category{key: "silhouette", title: "Silhouette", weight: 2}
	// Footprint: how much of its own bounding rectangle the top view covers.
	occupied, minI, maxI, minJ, maxJ := 0, math.MaxInt, -1, math.MaxInt, -1
	for i, row := range c.top.cells {
		for j, cell := range row {
			if cell.ok {
				occupied++
				minI, maxI, minJ, maxJ = min(minI, i), max(maxI, i), min(minJ, j), max(maxJ, j)
			}
		}
	}
	rect := false
	if occupied > 0 {
		rect = float64(occupied)/float64((maxI-minI+1)*(maxJ-minJ+1)) > 0.95
	}
	// Outline: the distinct heights of the top edge seen from south and east.
	profile := func(f facade) (heights int) {
		hs := map[int]bool{}
		for j := range f.cols {
			for i := range f.rows { // rows run top first
				if f.cells[i][j].ok {
					hs[f.rows[i]] = true
					break
				}
			}
		}
		return len(hs)
	}
	south, east := profile(c.sides[0]), profile(c.sides[2])
	variety := math.Min(1, float64(max(south, east)-1)/4)
	cat.score = 0.6*variety + 0.4*(1-boolf(rect)*c.t.rectPenalty/0.5)
	cat.summary = fmt.Sprintf("roofline has %d heights from the south and %d from the east", south, east)
	if rect {
		cat.summary += "; footprint is a plain rectangle"
	}
	if rect && (c.t.rectPenalty >= 0.5 || occupied > 120) {
		c.add(cat, "The footprint is a plain rectangle. Add a wing, porch, tower or lean-to so it becomes an L or T, or offset an upper storey.")
	}
	if max(south, east) <= 2 {
		c.add(cat, "The outline is flat from every side. Pitch the roof, raise one volume 2 to 4 blocks above the rest, or add a chimney or tower.")
	}
	return cat
}

func boolf(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// overhang counts footprint columns whose top block has open space under it,
// such as roof eaves and jettied upper floors.
func (c *critic) overhang() *category {
	cat := &category{key: "overhang", title: "Overhangs", weight: 1.5}
	perimeter, eaves := 0, 0
	occ := func(i, j int) bool {
		return i >= 0 && j >= 0 && i < len(c.top.rows) && j < len(c.top.cols) && c.top.cells[i][j].ok
	}
	for i, row := range c.top.cells {
		for j, cell := range row {
			if !cell.ok {
				continue
			}
			if occ(i-1, j) && occ(i+1, j) && occ(i, j-1) && occ(i, j+1) {
				continue
			}
			perimeter++
			// An edge column whose top block has open space under it.
			p := cell.pos
			if p[1] >= c.r.min[1]+2 {
				below, _, ok := c.g.get(p.add(vec{0, -1, 0}))
				if !ok || isAir(below.Name) {
					eaves++
				}
			}
		}
	}
	if perimeter == 0 {
		cat.score, cat.summary = 1, "nothing to measure"
		return cat
	}
	share := float64(eaves) / float64(perimeter)
	cat.score = math.Min(1, share/0.5)
	cat.summary = fmt.Sprintf("%d of %d columns around the edge of the footprint overhang", eaves, perimeter)
	if share < 0.5 {
		c.add(cat, "The roof and upper floors barely overhang. Extend the roof 1 block past the walls on every side (2 on large builds), with upside-down stairs or trapdoors under it as brackets.")
	}
	return cat
}

// foundation compares the bottom layers of the facades with the wall above.
func (c *critic) foundation() *category {
	cat := &category{key: "foundation", title: "Foundation and gradient", weight: 1.5}
	var base, wall []string
	var baseLum, wallLum float64
	for _, f := range c.sides {
		for i, rv := range f.rows {
			for _, cell := range f.cells[i] {
				if !cell.ok || !isFull(cell.bs) {
					continue
				}
				h := rv - c.r.min[1]
				col := blockColor(cell.bs)
				lum := 0.299*float64(col.R) + 0.587*float64(col.G) + 0.114*float64(col.B)
				switch {
				case h == 0:
					base = append(base, cell.bs.Name)
					baseLum += lum
				case h >= 2 && h <= 6:
					wall = append(wall, cell.bs.Name)
					wallLum += lum
				}
			}
		}
	}
	if len(base) == 0 || len(wall) == 0 {
		cat.score, cat.summary = 1, "too low to have a separate base"
		return cat
	}
	baseLum /= float64(len(base))
	wallLum /= float64(len(wall))
	bm, wm := mostCommon(base), mostCommon(wall)
	distinct := bm != wm
	darker := baseLum < wallLum-8
	cat.score = 0.6*boolf(distinct) + 0.4*boolf(darker)
	if !distinct {
		cat.score = math.Max(cat.score, 0.2*boolf(darker))
	}
	cat.summary = fmt.Sprintf("base is mostly %s, walls mostly %s", shortName(bm), shortName(wm))
	if !distinct {
		c.add(cat, "The walls run into the ground in the same block (%s). Add a plinth 1 to 3 blocks tall, 1 block wider than the walls, in a heavier stone such as cobblestone, stone bricks or cobbled deepslate.", shortName(wm))
	} else if !darker {
		c.add(cat, "The base (%s) is not darker than the walls (%s). A darker, rougher base and lighter top make the building feel grounded.", shortName(bm), shortName(wm))
	}
	return cat
}

func isOpening(name string) bool {
	for _, h := range []string{"glass", "_door", "iron_bars", "_trapdoor"} {
		if strings.Contains(name, h) {
			return true
		}
	}
	return false
}

// openings checks that each big facade has windows or doors.
func (c *critic) openings() *category {
	cat := &category{key: "openings", title: "Windows and doors", weight: 1.5}
	big, with := 0, 0
	for _, f := range c.sides {
		area, open, deep := 0, 0, 0
		for i := range f.cells {
			for j, cell := range f.cells[i] {
				if !cell.ok {
					continue
				}
				area++
				switch {
				case isOpening(cell.bs.Name):
					open++
				case j > 0 && j < len(f.cols)-1 && f.cells[i][j-1].ok && f.cells[i][j+1].ok &&
					cell.depth >= f.cells[i][j-1].depth+2 && cell.depth >= f.cells[i][j+1].depth+2:
					// A hole in the wall the line of sight passes through.
					deep++
				}
			}
		}
		if area < 30 {
			continue
		}
		big++
		if open+deep > 0 {
			with++
		} else {
			c.add(cat, "The %s side has no windows or doors. Add windows (panes set 1 block back, with sills and shutters) every 3 to 4 blocks.", f.side)
		}
	}
	if big == 0 {
		cat.score, cat.summary = 1, "no large walls"
		return cat
	}
	cat.score = float64(with) / float64(big)
	cat.summary = fmt.Sprintf("%d of %d large sides have windows or doors", with, big)
	return cat
}

// symmetry measures mirror symmetry across the middle of x and of z.
func (c *critic) symmetry() *category {
	cat := &category{key: "asymmetry", title: "Asymmetry", weight: 1}
	mirror := func(axis int) float64 {
		match, total := 0, 0
		for p, b := range c.g.at {
			if !c.r.contains(p) {
				continue
			}
			bs, ok := stateOf(c.st, c.st.Blocks[b])
			if !ok || isAir(bs.Name) {
				continue
			}
			total++
			q := p
			q[axis] = c.r.min[axis] + c.r.max[axis] - p[axis]
			if o, _, ok := c.g.get(q); ok && o.Name == bs.Name {
				match++
			}
		}
		if total == 0 {
			return 0
		}
		return float64(match) / float64(total)
	}
	sx, sz := mirror(0), mirror(2)
	both := sx >= 0.97 && sz >= 0.97
	one := sx >= 0.97 || sz >= 0.97
	switch {
	case both:
		cat.score = c.t.symmetryScore[1]
	case one:
		cat.score = c.t.symmetryScore[0]
	default:
		cat.score = 1
	}
	cat.summary = fmt.Sprintf("%s mirror-symmetric east-west, %s north-south", pct(sx), pct(sz))
	if one {
		c.add(cat, "The build is a near-perfect mirror image. Keep the skeleton but break it: a chimney on one side, a lean-to or porch, a different window, a woodpile, vines on one corner.")
	}
	return cat
}

var lavishHints = []string{
	"quartz", "prismarine", "purpur", "end_stone", "concrete", "glazed_terracotta", "amethyst", "gold_block",
	"diamond_block", "emerald_block", "lapis_block", "netherite_block", "sea_lantern", "beacon", "end_rod",
	"froglight", "obsidian", "copper",
}

// materials flags blocks a survival player would rarely spend on a build.
func (c *critic) materials() *category {
	cat := &category{key: "materials", title: "Survival materials", weight: 1.5}
	counts := map[string]int{}
	n := 0
	for _, bs := range c.surface {
		for _, h := range lavishHints {
			if strings.Contains(bs.Name, h) {
				counts[bs.Name]++
				n++
				break
			}
		}
	}
	share := float64(n) / float64(len(c.surface))
	cat.score = math.Max(0, 1-share/0.08)
	cat.summary = fmt.Sprintf("%s of the visible surface is expensive or late-game blocks", pct(share))
	names := make([]string, 0, len(counts))
	for k := range counts {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool { return counts[names[i]] > counts[names[j]] })
	for _, k := range names {
		if share < 0.02 {
			break
		}
		c.add(cat, "%s (%d visible) is lavish for a survival build. Use a common block of similar colour, or keep it to a few accents.", shortName(k), counts[k])
	}
	return cat
}

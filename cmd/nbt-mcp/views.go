package main

import (
	"bytes"
	"context"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"sort"
	"strings"

	"github.com/MinecraftMetascript/nbt-go/structure"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// maxViewCells bounds the text views.
const maxViewCells = 160 * 160

type viewIn struct {
	Name string  `json:"name"`
	View string  `json:"view" jsonschema:"top, bottom, north, south, east or west: the side you look from. south is the front view (looking north, east on the right); top matches render_layer"`
	From *[3]int `json:"from,omitempty" jsonschema:"limit the view to a box: first corner [x, y, z]"`
	To   *[3]int `json:"to,omitempty" jsonschema:"limit the view to a box: opposite corner [x, y, z], inclusive"`
	Show string  `json:"show,omitempty" jsonschema:"blocks (default) draws the first block seen; depth draws how far in it is (0-9, then a-z), to tell near from far"`
}

// viewAxes describes a view: which axes become columns and rows, which way
// each runs, and the axis looked along.
type viewAxes struct {
	col, row, depth          int
	colRev, rowRev, depthRev bool // reversed: iterate from max to min
	colName, rowName         string
}

func parseView(v string) (viewAxes, error) {
	switch v {
	case "top":
		return viewAxes{col: 0, row: 2, depth: 1, depthRev: true, colName: "x (west to east)", rowName: "z (north at the top)"}, nil
	case "bottom":
		return viewAxes{col: 0, row: 2, depth: 1, colRev: true, colName: "x (east to west, as seen from below)", rowName: "z (north at the top)"}, nil
	case "south":
		return viewAxes{col: 0, row: 1, depth: 2, rowRev: true, depthRev: true, colName: "x (west to east)", rowName: "y (top at the top)"}, nil
	case "north":
		return viewAxes{col: 0, row: 1, depth: 2, colRev: true, rowRev: true, colName: "x (east to west)", rowName: "y (top at the top)"}, nil
	case "east":
		return viewAxes{col: 2, row: 1, depth: 0, colRev: true, rowRev: true, depthRev: true, colName: "z (south to north)", rowName: "y (top at the top)"}, nil
	case "west":
		return viewAxes{col: 2, row: 1, depth: 0, rowRev: true, colName: "z (north to south)", rowName: "y (top at the top)"}, nil
	}
	return viewAxes{}, fmt.Errorf("view %q must be top, bottom, north, south, east or west", v)
}

func regionOf(st *structure.Structure, from, to *[3]int) (box, error) {
	b := sizeBox(st.Size)
	if (from == nil) != (to == nil) {
		return b, fmt.Errorf("give both from and to, or neither")
	}
	if from != nil {
		lo, hi, err := boxIn{From: *from, To: *to}.bounds()
		if err != nil {
			return b, err
		}
		b = box{vec(lo), vec(hi)}
	}
	if b.size()[0] <= 0 || b.size()[1] <= 0 || b.size()[2] <= 0 {
		return b, fmt.Errorf("the structure is empty")
	}
	return b, nil
}

func span(lo, hi int, rev bool) []int {
	out := make([]int, 0, hi-lo+1)
	for i := lo; i <= hi; i++ {
		out = append(out, i)
	}
	if rev {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out
}

const depthGlyphs = "0123456789abcdefghijklmnopqrstuvwxyz"

func (s *state) view(in viewIn) (string, error) {
	st, err := s.get(in.Name)
	if err != nil {
		return "", err
	}
	va, err := parseView(in.View)
	if err != nil {
		return "", err
	}
	if in.Show != "" && in.Show != "blocks" && in.Show != "depth" {
		return "", fmt.Errorf("show %q must be blocks or depth", in.Show)
	}
	r, err := regionOf(st, in.From, in.To)
	if err != nil {
		return "", err
	}
	cols := span(r.min[va.col], r.max[va.col], va.colRev)
	rows := span(r.min[va.row], r.max[va.row], va.rowRev)
	depths := span(r.min[va.depth], r.max[va.depth], va.depthRev)
	if len(cols)*len(rows) > maxViewCells {
		return "", fmt.Errorf("the view is %dx%d, too large (max %d cells); limit it with from and to", len(cols), len(rows), maxViewCells)
	}
	g := newGrid(st)
	glyphOf := map[string]byte{}
	var legend []string
	var b strings.Builder
	fmt.Fprintf(&b, "%s view of %q, %s. Columns are %s, rows are %s.\n\n", in.View, in.Name, r, va.colName, va.rowName)
	// Column ruler: the units digit of each coordinate, in drawing order.
	b.WriteString("     ")
	for _, c := range cols {
		b.WriteByte(byte('0' + ((c%10)+10)%10))
	}
	b.WriteByte('\n')
	for _, rv := range rows {
		line := make([]byte, len(cols))
		for i, cv := range cols {
			line[i] = '.'
			sawAir := false
			for k, dv := range depths {
				var p vec
				p[va.col], p[va.row], p[va.depth] = cv, rv, dv
				bs, _, ok := g.get(p)
				if !ok {
					continue
				}
				if isAir(bs.Name) {
					sawAir = true
					continue
				}
				if in.Show == "depth" {
					line[i] = depthGlyphs[min(k, len(depthGlyphs)-1)]
				} else {
					key := bs.String()
					gl, seen := glyphOf[key]
					if !seen {
						gl = '?'
						if len(glyphOf) < len(glyphs) {
							gl = glyphs[len(glyphOf)]
						}
						glyphOf[key] = gl
						legend = append(legend, fmt.Sprintf("  %c  %s", gl, key))
					}
					line[i] = gl
				}
				sawAir = false
				break
			}
			if line[i] == '.' && sawAir {
				line[i] = ' '
			}
		}
		fmt.Fprintf(&b, "%4d %s\n", rv, line)
	}
	if in.Show == "depth" {
		fmt.Fprintf(&b, "\nDigits are how many blocks in from the %s side the first block is (0 is the nearest layer).\n", in.View)
	} else {
		b.WriteString("\nLegend (the first block seen along each line of sight; air is see-through):\n")
		for _, l := range legend {
			b.WriteString(l + "\n")
		}
	}
	b.WriteString("  .  nothing but empty positions (world left unchanged)\n  ' ' (space) only air\n")
	return b.String(), nil
}

type imageIn struct {
	Name  string  `json:"name"`
	View  string  `json:"view,omitempty" jsonschema:"isometric (default, from the south-east and above), isometric_nw (from the north-west), or a flat view: top, north, south, east, west"`
	Scale int     `json:"scale,omitempty" jsonschema:"pixels per block (default chosen to fit about 800 pixels, at most 16)"`
	From  *[3]int `json:"from,omitempty" jsonschema:"limit the render to a box: first corner [x, y, z]"`
	To    *[3]int `json:"to,omitempty" jsonschema:"limit the render to a box: opposite corner [x, y, z], inclusive"`
}

// renderImage draws the structure with flat colors per block type. Colors
// are approximations, not textures.
func (s *state) renderImage(_ context.Context, _ *mcp.CallToolRequest, in imageIn) (*mcp.CallToolResult, any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.get(in.Name)
	if err != nil {
		return nil, nil, err
	}
	r, err := regionOf(st, in.From, in.To)
	if err != nil {
		return nil, nil, err
	}
	view := in.View
	if view == "" {
		view = "isometric"
	}
	var img *image.RGBA
	switch view {
	case "isometric", "isometric_nw":
		img, err = renderIso(st, r, in.Scale, view == "isometric_nw")
	default:
		img, err = renderFlat(st, r, in.Scale, view)
	}
	if err != nil {
		return nil, nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, nil, err
	}
	caption := fmt.Sprintf("%s render of %q (%s), %dx%d pixels. Colors approximate each block's map color; textures, transparency and shapes such as stairs are not drawn.",
		view, in.Name, r, img.Bounds().Dx(), img.Bounds().Dy())
	return &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.ImageContent{Data: buf.Bytes(), MIMEType: "image/png"},
		&mcp.TextContent{Text: caption},
	}}, nil, nil
}

func pickScale(want, extent, maxScale int) int {
	if want > 0 {
		return min(want, maxScale)
	}
	s := 800 / max(extent, 1)
	return max(1, min(s, maxScale))
}

func renderFlat(st *structure.Structure, r box, scale int, view string) (*image.RGBA, error) {
	va, err := parseView(view)
	if err != nil {
		return nil, err
	}
	cols := span(r.min[va.col], r.max[va.col], va.colRev)
	rows := span(r.min[va.row], r.max[va.row], va.rowRev)
	depths := span(r.min[va.depth], r.max[va.depth], va.depthRev)
	sc := pickScale(scale, max(len(cols), len(rows)), 16)
	if len(cols)*len(rows)*sc*sc > 16<<20 {
		return nil, fmt.Errorf("image would be too large; use a smaller scale or limit it with from and to")
	}
	img := image.NewRGBA(image.Rect(0, 0, len(cols)*sc, len(rows)*sc))
	fillRect(img, img.Bounds(), background)
	g := newGrid(st)
	for j, rv := range rows {
		for i, cv := range cols {
			for k, dv := range depths {
				var p vec
				p[va.col], p[va.row], p[va.depth] = cv, rv, dv
				bs, _, ok := g.get(p)
				if !ok || isAir(bs.Name) {
					continue
				}
				// Farther blocks are drawn darker so depth reads.
				f := 1 - 0.5*float64(k)/float64(max(len(depths)-1, 1))
				fillRect(img, image.Rect(i*sc, j*sc, (i+1)*sc, (j+1)*sc), shade(blockColor(bs), f))
				break
			}
		}
	}
	return img, nil
}

var background = color.RGBA{0x20, 0x22, 0x28, 0xff}

func renderIso(st *structure.Structure, r box, scale int, fromNW bool) (*image.RGBA, error) {
	sz := r.size()
	sc := pickScale(scale, sz[0]+sz[2]+sz[1], 16)
	if sc < 2 {
		sc = 2
	}
	// Screen position of a block corner: x goes right and down, z left and
	// down, y up. Viewing from the north-west flips x and z.
	type block struct {
		p  vec
		bs structure.BlockState
	}
	g := newGrid(st)
	var blocks []block
	for _, b := range st.Blocks {
		p := toVec(b.Pos)
		if !r.contains(p) {
			continue
		}
		bs, ok := stateOf(st, b)
		if !ok || isAir(bs.Name) {
			continue
		}
		blocks = append(blocks, block{p, bs})
	}
	if len(blocks) > 1<<20 {
		return nil, fmt.Errorf("%d blocks is too many to render; limit it with from and to", len(blocks))
	}
	local := func(p vec) (int, int, int) {
		x, y, z := p[0]-r.min[0], p[1]-r.min[1], p[2]-r.min[2]
		if fromNW {
			x, z = sz[0]-1-x, sz[2]-1-z
		}
		return x, y, z
	}
	w := (sz[0] + sz[2]) * sc
	h := (sz[0]+sz[2])*sc/2 + sz[1]*sc + sc
	if w*h > 24<<20 {
		return nil, fmt.Errorf("image would be too large; use a smaller scale or limit it with from and to")
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fillRect(img, img.Bounds(), background)
	ox, oy := sz[2]*sc, sz[1]*sc
	proj := func(x, y, z float64) (float64, float64) {
		return float64(ox) + (x-z)*float64(sc), float64(oy) + (x+z)*float64(sc)/2 - y*float64(sc)
	}
	// Draw far to near: blocks with a larger x+y+z are closer to the viewer.
	sort.Slice(blocks, func(i, j int) bool {
		xi, yi, zi := local(blocks[i].p)
		xj, yj, zj := local(blocks[j].p)
		if xi+yi+zi != xj+yj+zj {
			return xi+yi+zi < xj+yj+zj
		}
		return yi < yj
	})
	// Neighbour offsets in world space for the visible faces: up, toward the
	// viewer on x, toward the viewer on z.
	sx, sz2 := 1, 1
	if fromNW {
		sx, sz2 = -1, -1
	}
	for _, b := range blocks {
		x, y, z := local(b.p)
		fx, fy, fz := float64(x), float64(y), float64(z)
		c := blockColor(b.bs)
		hidden := func(d vec) bool {
			nb, _, ok := g.get(b.p.add(d))
			return ok && isFull(nb) && r.contains(b.p.add(d))
		}
		if !hidden(vec{0, 1, 0}) {
			ax, ay := proj(fx, fy+1, fz)
			bx, by := proj(fx+1, fy+1, fz)
			cx, cy := proj(fx+1, fy+1, fz+1)
			dx, dy := proj(fx, fy+1, fz+1)
			fillPoly(img, [][2]float64{{ax, ay}, {bx, by}, {cx, cy}, {dx, dy}}, shade(c, 1))
		}
		if !hidden(vec{0, 0, sz2}) {
			ax, ay := proj(fx, fy+1, fz+1)
			bx, by := proj(fx+1, fy+1, fz+1)
			cx, cy := proj(fx+1, fy, fz+1)
			dx, dy := proj(fx, fy, fz+1)
			fillPoly(img, [][2]float64{{ax, ay}, {bx, by}, {cx, cy}, {dx, dy}}, shade(c, 0.7))
		}
		if !hidden(vec{sx, 0, 0}) {
			ax, ay := proj(fx+1, fy+1, fz)
			bx, by := proj(fx+1, fy+1, fz+1)
			cx, cy := proj(fx+1, fy, fz+1)
			dx, dy := proj(fx+1, fy, fz)
			fillPoly(img, [][2]float64{{ax, ay}, {bx, by}, {cx, cy}, {dx, dy}}, shade(c, 0.85))
		}
	}
	return img, nil
}

func fillRect(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	r = r.Intersect(img.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

// fillPoly fills a convex polygon, sampling pixel centers.
func fillPoly(img *image.RGBA, pts [][2]float64, c color.RGBA) {
	minX, minY, maxX, maxY := pts[0][0], pts[0][1], pts[0][0], pts[0][1]
	for _, p := range pts {
		minX, maxX = min(minX, p[0]), max(maxX, p[0])
		minY, maxY = min(minY, p[1]), max(maxY, p[1])
	}
	bounds := img.Bounds()
	for y := max(int(minY), bounds.Min.Y); y <= min(int(maxY), bounds.Max.Y-1); y++ {
		for x := max(int(minX), bounds.Min.X); x <= min(int(maxX), bounds.Max.X-1); x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			pos, neg := false, false
			for i := range pts {
				a, b := pts[i], pts[(i+1)%len(pts)]
				cross := (b[0]-a[0])*(py-a[1]) - (b[1]-a[1])*(px-a[0])
				if cross > 1e-9 {
					pos = true
				} else if cross < -1e-9 {
					neg = true
				}
			}
			if !(pos && neg) {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func shade(c color.RGBA, f float64) color.RGBA {
	return color.RGBA{uint8(float64(c.R) * f), uint8(float64(c.G) * f), uint8(float64(c.B) * f), 0xff}
}

func rgb(hex uint32) color.RGBA {
	return color.RGBA{uint8(hex >> 16), uint8(hex >> 8), uint8(hex), 0xff}
}

// dyeColors are the 16 dye colors, used for wool, concrete, glass and so on.
var dyeColors = map[string]uint32{
	"white": 0xe9ecec, "orange": 0xf07613, "magenta": 0xbd44b3, "light_blue": 0x3aafd9, "yellow": 0xf8c627,
	"lime": 0x70b919, "pink": 0xed8dac, "gray": 0x3e4447, "light_gray": 0x8e8e86, "cyan": 0x158991,
	"purple": 0x792aac, "blue": 0x35399d, "brown": 0x724728, "green": 0x546d1b, "red": 0xa12722, "black": 0x141519,
}

// materialColors map name fragments to colors; the first match wins, so more
// specific fragments come first.
var materialColors = []struct {
	frag string
	c    uint32
}{
	{"grass_block", 0x7cbd6b}, {"short_grass", 0x6a9a3c}, {"tall_grass", 0x6a9a3c}, {"fern", 0x5e8f35},
	{"moss", 0x5a7b2c}, {"leaves", 0x4a7a28}, {"vine", 0x3f6e1e}, {"lichen", 0x7f9f8a},
	{"dark_oak", 0x4a3420}, {"spruce", 0x6b4f2f}, {"birch", 0xd5c98c}, {"jungle", 0xa3734c},
	{"acacia", 0xad5d32}, {"mangrove", 0x763331}, {"cherry", 0xe2b2ac}, {"bamboo", 0xc6b356},
	{"crimson", 0x7e3a56}, {"warped", 0x2b6863}, {"pale_oak", 0xe3d9d0}, {"oak", 0xa2834f},
	{"deepslate", 0x4d4d51}, {"blackstone", 0x2a2328}, {"cobblestone", 0x7f7f7f}, {"mossy_stone", 0x6f7b62},
	{"stone_brick", 0x7a7a7a}, {"smooth_stone", 0x9e9e9e}, {"andesite", 0x888889}, {"diorite", 0xbcbcbc},
	{"granite", 0x956755}, {"tuff", 0x6c6d66}, {"calcite", 0xdfe0dc}, {"dripstone", 0x86634f},
	{"basalt", 0x515155}, {"netherrack", 0x6f3534}, {"nether_brick", 0x2c1519}, {"red_sandstone", 0xb5621f},
	{"sandstone", 0xd8cb9b}, {"red_sand", 0xbe6621}, {"sand", 0xdbcfa3}, {"gravel", 0x837f7e},
	{"mud_brick", 0x8a6a4f}, {"mud", 0x3c3a3e}, {"clay", 0xa0a6b3}, {"dirt", 0x866043}, {"podzol", 0x5c3f1a},
	{"mycelium", 0x6f6365}, {"farmland", 0x52341e}, {"terracotta", 0x985e43}, {"brick", 0x966053},
	{"quartz", 0xece6df}, {"purpur", 0xa97ea9}, {"end_stone", 0xdcdf9e}, {"prismarine", 0x5fa595},
	{"obsidian", 0x0f0b19}, {"glass", 0xc0dce8}, {"ice", 0x91b4fe}, {"snow", 0xf9fefe},
	{"water", 0x3f76e4}, {"lava", 0xd96514}, {"gold", 0xf6d03d}, {"iron", 0xd8d8d8}, {"diamond", 0x62ede4},
	{"emerald", 0x2acb57}, {"lapis", 0x1f43a3}, {"redstone", 0xaa0f01}, {"netherite", 0x433e40},
	{"copper", 0xc06c50}, {"amethyst", 0x8562c0}, {"glowstone", 0xfbda74}, {"sea_lantern", 0xacc7be},
	{"lantern", 0xe8a33a}, {"torch", 0xffd75e}, {"campfire", 0xd88b2c}, {"hay", 0xa58c14},
	{"pumpkin", 0xe38a1d}, {"melon", 0x6f9123}, {"cactus", 0x5a8a2a}, {"bone", 0xe1ddc9},
	{"sculk", 0x0d1e24}, {"stone", 0x7d7d7d}, {"log", 0x6b5434}, {"wood", 0x6b5434}, {"planks", 0xa2834f},
}

func blockColor(bs structure.BlockState) color.RGBA {
	name := strings.TrimPrefix(bs.Name, "minecraft:")
	// Dye-colored blocks: longest color name first, so light_blue beats blue.
	keys := make([]string, 0, len(dyeColors))
	for k := range dyeColors {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, k := range keys {
		if strings.HasPrefix(name, k+"_") && !strings.Contains(name, "sandstone") && !strings.Contains(name, "_sand") {
			c := rgb(dyeColors[k])
			if strings.Contains(name, "terracotta") {
				c = shade(c, 0.75)
			}
			return c
		}
	}
	for _, m := range materialColors {
		if strings.Contains(name, m.frag) {
			return rgb(m.c)
		}
	}
	h := fnv.New32a()
	h.Write([]byte(name))
	v := h.Sum32()
	return color.RGBA{uint8(80 + v%120), uint8(80 + (v>>8)%120), uint8(80 + (v>>16)%120), 0xff}
}

package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"sort"
	"strings"

	"github.com/MinecraftMetascript/nbt-go/structure"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// maxImagesPerCall bounds how many views one render_image call returns.
const maxImagesPerCall = 9

// isoCorners maps each isometric view to the rotation that brings that
// corner of the structure to the south-east, which is where the renderer
// always looks from.
var isoCorners = map[string]rotation{
	"iso_se": 0, // looking north-west at the south and east faces
	"iso_sw": 3, // looking north-east at the west and south faces
	"iso_nw": 2, // looking south-east at the north and west faces
	"iso_ne": 1, // looking south-west at the east and north faces
}

var viewAliases = map[string]string{"isometric": "iso_se", "isometric_nw": "iso_nw", "front": "south"}

var viewPresets = map[string][]string{
	"corners":    {"iso_se", "iso_sw", "iso_nw", "iso_ne"},
	"elevations": {"south", "east", "north", "west"},
	"overview":   {"iso_se", "iso_nw", "top", "south", "east"},
	"all":        {"iso_se", "iso_sw", "iso_nw", "iso_ne", "top", "south", "east", "north", "west"},
}

var viewDescriptions = map[string]string{
	"iso_se": "isometric from the south-east, above (south face on the left, east face on the right)",
	"iso_sw": "isometric from the south-west, above (west face on the left, south face on the right)",
	"iso_nw": "isometric from the north-west, above (north face on the left, west face on the right)",
	"iso_ne": "isometric from the north-east, above (east face on the left, north face on the right)",
	"top":    "plan from above, north at the top, east on the right",
	"bottom": "from below, north at the top, east on the left",
	"south":  "south elevation (front), looking north, east on the right",
	"north":  "north elevation, looking south, west on the right",
	"east":   "east elevation, looking west, north on the right",
	"west":   "west elevation, looking east, south on the right",
}

type imageIn struct {
	Name     string   `json:"name"`
	Views    []string `json:"views,omitempty" jsonschema:"views to render, one image each (at most 9): iso_se, iso_sw, iso_nw, iso_ne (isometric from each corner), top, bottom, south (front), north, east, west; or a preset: corners (all four isometric), elevations (all four sides), overview (iso_se, iso_nw, top, south, east), all. Default iso_se"`
	View     string   `json:"view,omitempty" jsonschema:"a single view; same names as views"`
	Scale    int      `json:"scale,omitempty" jsonschema:"pixels per block (default fits about 800 pixels for one view, 480 for several; at most 24)"`
	From     *[3]int  `json:"from,omitempty" jsonschema:"only draw a box: first corner [x, y, z]. Cut away part of a structure to look inside, e.g. from=[0,0,5] hides everything north of z=5 so a south view shows that cross-section"`
	To       *[3]int  `json:"to,omitempty" jsonschema:"only draw a box: opposite corner [x, y, z], inclusive"`
	CutawayY *int     `json:"cutaway_y,omitempty" jsonschema:"hide everything above this y to see into a floor: with an isometric view it's a cutaway, with top it's a floor plan of that level"`
}

func expandViews(in imageIn) ([]string, error) {
	var req []string
	if in.View != "" {
		req = append(req, in.View)
	}
	req = append(req, in.Views...)
	if len(req) == 0 {
		req = []string{"iso_se"}
	}
	var out []string
	seen := map[string]bool{}
	for _, v := range req {
		v = strings.ToLower(strings.TrimSpace(v))
		if a, ok := viewAliases[v]; ok {
			v = a
		}
		vs, ok := viewPresets[v]
		if !ok {
			if _, known := viewDescriptions[v]; !known {
				return nil, fmt.Errorf("unknown view %q; use iso_se, iso_sw, iso_nw, iso_ne, top, bottom, north, south, east, west, or a preset: corners, elevations, overview, all", v)
			}
			vs = []string{v}
		}
		for _, x := range vs {
			if !seen[x] {
				seen[x] = true
				out = append(out, x)
			}
		}
	}
	if len(out) > maxImagesPerCall {
		return nil, fmt.Errorf("%d views requested, at most %d per call", len(out), maxImagesPerCall)
	}
	return out, nil
}

// renderImage draws the structure from one or more viewpoints, returning
// one PNG per view. Colors approximate each block's map color; common
// partial blocks (slabs, stairs, fences, panes, plants...) get their rough
// shape, and glass and water are see-through.
func (s *state) renderImage(_ context.Context, _ *mcp.CallToolRequest, in imageIn) (*mcp.CallToolResult, any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.get(in.Name)
	if err != nil {
		return nil, nil, err
	}
	views, err := expandViews(in)
	if err != nil {
		return nil, nil, err
	}
	r, err := regionOf(st, in.From, in.To)
	if err != nil {
		return nil, nil, err
	}
	if in.CutawayY != nil {
		if *in.CutawayY < r.min[1] {
			return nil, nil, fmt.Errorf("cutaway_y=%d is below the region drawn, %s", *in.CutawayY, r)
		}
		r.max[1] = min(r.max[1], *in.CutawayY)
	}
	target := 800
	if len(views) > 1 {
		target = 480
	}
	g := newGrid(st)
	var content []mcp.Content
	for _, v := range views {
		var img *image.RGBA
		if rot, ok := isoCorners[v]; ok {
			img, err = renderIso(g, r, in.Scale, target, rot)
		} else {
			img, err = renderFlat(g, r, in.Scale, target, v)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", v, err)
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, nil, err
		}
		content = append(content,
			&mcp.TextContent{Text: fmt.Sprintf("%s: %s, %dx%d pixels.", v, viewDescriptions[v], img.Bounds().Dx(), img.Bounds().Dy())},
			&mcp.ImageContent{Data: buf.Bytes(), MIMEType: "image/png"})
	}
	note := fmt.Sprintf("Rendered %q, %s", in.Name, r)
	if in.CutawayY != nil {
		note += fmt.Sprintf(", cut away above y=%d", *in.CutawayY)
	}
	note += ". Colors approximate map colors, not textures; slabs, stairs, fences, panes, plants and other partial blocks are drawn with their rough shape, and glass, ice and water are see-through."
	content = append(content, &mcp.TextContent{Text: note})
	return &mcp.CallToolResult{Content: content}, nil, nil
}

func pickScale(want, extent, target, maxScale int) int {
	if want > 0 {
		return max(1, min(want, maxScale))
	}
	return max(1, min(target/max(extent, 1), maxScale))
}

// renderFlat draws an orthographic view from one side: every block's
// rough shape projected straight onto the image, far blocks first and
// darker, so nearer ones cover them.
func renderFlat(g *grid, r box, scale, target int, view string) (*image.RGBA, error) {
	va, err := parseView(view)
	if err != nil {
		return nil, err
	}
	sz := r.size()
	w, h, d := sz[va.col], sz[va.row], sz[va.depth]
	sc := pickScale(scale, max(w, h), target, 24)
	if w*h*sc*sc > 16<<20 {
		return nil, fmt.Errorf("image would be too large; use a smaller scale or limit it with from and to")
	}
	img := image.NewRGBA(image.Rect(0, 0, w*sc, h*sc))
	fillRect(img, img.Bounds(), background)
	// screen maps a coordinate along an axis (in blocks, possibly
	// fractional) to the distance from the image's left or top edge, or
	// from the viewer for the depth axis.
	screen := func(axis int, rev bool, v float64) float64 {
		if rev {
			return float64(r.max[axis]+1) - v
		}
		return v - float64(r.min[axis])
	}
	type cell struct {
		p     vec
		bs    structure.BlockState
		depth float64
	}
	var cells []cell
	for p, i := range g.at {
		if !r.contains(p) {
			continue
		}
		bs, ok := stateOf(g.st, g.st.Blocks[i])
		if !ok || isAir(bs.Name) {
			continue
		}
		cells = append(cells, cell{p, bs, screen(va.depth, va.depthRev, float64(p[va.depth])+0.5)})
	}
	sort.Slice(cells, func(i, j int) bool {
		if cells[i].depth != cells[j].depth {
			return cells[i].depth > cells[j].depth
		}
		return lessVec(cells[i].p, cells[j].p)
	})
	fsc := float64(sc)
	for _, c := range cells {
		// Farther blocks are darker so depth reads.
		f := 1 - 0.5*(c.depth-0.5)/float64(max(d-1, 1))
		col := shade(blockColor(c.bs), f)
		alpha := opacity(c.bs)
		for _, b := range shapeOf(c.bs) {
			x0 := screen(va.col, va.colRev, float64(c.p[va.col])+b.min[va.col])
			x1 := screen(va.col, va.colRev, float64(c.p[va.col])+b.max[va.col])
			y0 := screen(va.row, va.rowRev, float64(c.p[va.row])+b.min[va.row])
			y1 := screen(va.row, va.rowRev, float64(c.p[va.row])+b.max[va.row])
			rect := image.Rect(int(math.Round(x0*fsc)), int(math.Round(y0*fsc)), int(math.Round(x1*fsc)), int(math.Round(y1*fsc))).Canon()
			if rect.Dx() == 0 {
				rect.Max.X++
			}
			if rect.Dy() == 0 {
				rect.Max.Y++
			}
			if alpha < 1 {
				blendRect(img, rect, col, alpha)
			} else {
				fillRect(img, rect, col)
			}
			if sc >= 6 && rect.Dx() > 2 && rect.Dy() > 2 {
				outlineRect(img, rect, shade(col, 0.82))
			}
		}
	}
	return img, nil
}

func blendRect(img *image.RGBA, r image.Rectangle, c color.RGBA, a float64) {
	r = r.Intersect(img.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetRGBA(x, y, blend(img.RGBAAt(x, y), c, a))
		}
	}
}

// fbox is a box within a block, in block units (0 to 1).
type fbox struct{ min, max [3]float64 }

var fullCell = []fbox{{[3]float64{0, 0, 0}, [3]float64{1, 1, 1}}}

func fb(x0, y0, z0, x1, y1, z1 float64) fbox {
	return fbox{[3]float64{x0, y0, z0}, [3]float64{x1, y1, z1}}
}

// sideBox returns a box hugging side d of the cell, t thick, between heights
// y0 and y1.
func sideBox(d dir, t, y0, y1 float64) fbox {
	switch d {
	case north:
		return fb(0, y0, 0, 1, y1, t)
	case south:
		return fb(0, y0, 1-t, 1, y1, 1)
	case west:
		return fb(0, y0, 0, t, y1, 1)
	case east:
		return fb(1-t, y0, 0, 1, y1, 1)
	case down:
		return fb(0, 0, 0, 1, t, 1)
	}
	return fb(0, 1-t, 0, 1, 1, 1)
}

// armBox returns a bar from the center of the cell to side d, w wide.
func armBox(d dir, w, y0, y1 float64) fbox {
	lo, hi := 0.5-w/2, 0.5+w/2
	switch d {
	case north:
		return fb(lo, y0, 0, hi, y1, 0.5)
	case south:
		return fb(lo, y0, 0.5, hi, y1, 1)
	case west:
		return fb(0, y0, lo, 0.5, y1, hi)
	}
	return fb(0.5, y0, lo, 1, y1, hi)
}

func centered(w, y0, y1 float64) fbox {
	return fb(0.5-w/2, y0, 0.5-w/2, 0.5+w/2, y1, 0.5+w/2)
}

var horizontals = []dir{north, east, south, west}

// shapeOf approximates a block's shape with a few boxes.
func shapeOf(bs structure.BlockState) []fbox {
	n := strings.TrimPrefix(bs.Name, "minecraft:")
	p := bs.Properties
	facing, hasFacing := parseDir(p["facing"])
	connected := func(d dir) bool {
		v := p[d.String()]
		return v == "true" || v == "low" || v == "tall"
	}
	switch {
	case strings.HasSuffix(n, "_slab"):
		switch p["type"] {
		case "top":
			return []fbox{fb(0, 0.5, 0, 1, 1, 1)}
		case "double":
			return fullCell
		}
		return []fbox{fb(0, 0, 0, 1, 0.5, 1)}
	case strings.HasSuffix(n, "_stairs"):
		lo, hi := 0.0, 0.5
		if p["half"] == "top" {
			lo, hi = 0.5, 0
		}
		boxes := []fbox{fb(0, lo, 0, 1, lo+0.5, 1)}
		if hasFacing {
			boxes = append(boxes, sideBox(facing, 0.5, hi, hi+0.5))
		}
		return boxes
	case strings.HasSuffix(n, "_carpet"), strings.HasSuffix(n, "_pressure_plate"), n == "moss_carpet":
		return []fbox{fb(0, 0, 0, 1, 1.0/16, 1)}
	case n == "snow":
		layers := 1.0
		fmt.Sscan(p["layers"], &layers)
		return []fbox{fb(0, 0, 0, 1, layers/8, 1)}
	case strings.HasSuffix(n, "_trapdoor"):
		if p["open"] == "true" && hasFacing {
			return []fbox{sideBox(facing.opposite(), 3.0/16, 0, 1)}
		}
		if p["half"] == "top" {
			return []fbox{fb(0, 13.0/16, 0, 1, 1, 1)}
		}
		return []fbox{fb(0, 0, 0, 1, 3.0/16, 1)}
	case strings.HasSuffix(n, "_door"):
		if hasFacing {
			return []fbox{sideBox(facing.opposite(), 3.0/16, 0, 1)}
		}
	case strings.HasSuffix(n, "_fence_gate"):
		if hasFacing && (facing == north || facing == south) {
			return []fbox{fb(0, 0.3, 0.4375, 1, 0.95, 0.5625)}
		}
		return []fbox{fb(0.4375, 0.3, 0, 0.5625, 0.95, 1)}
	case connectFamily(bs.Name) == "fence", connectFamily(bs.Name) == "nether_fence":
		boxes := []fbox{centered(0.25, 0, 1)}
		for _, d := range horizontals {
			if connected(d) {
				boxes = append(boxes, armBox(d, 0.125, 0.375, 0.9375))
			}
		}
		return boxes
	case connectFamily(bs.Name) == "wall":
		boxes := []fbox{}
		if p["up"] != "false" {
			boxes = append(boxes, centered(0.5, 0, 1))
		}
		for _, d := range horizontals {
			if connected(d) {
				top := 0.875
				if p[d.String()] == "tall" {
					top = 1
				}
				boxes = append(boxes, armBox(d, 0.375, 0, top))
			}
		}
		if len(boxes) == 0 {
			return []fbox{centered(0.5, 0, 1)}
		}
		return boxes
	case connectFamily(bs.Name) == "pane":
		boxes := []fbox{centered(0.125, 0, 1)}
		for _, d := range horizontals {
			if connected(d) {
				boxes = append(boxes, armBox(d, 0.125, 0, 1))
			}
		}
		return boxes
	case n == "ladder", strings.HasSuffix(n, "wall_sign"), strings.HasSuffix(n, "wall_banner"):
		if hasFacing {
			return []fbox{sideBox(facing.opposite(), 0.125, 0.1, 0.9)}
		}
	case n == "vine", n == "glow_lichen", n == "sculk_vein":
		var boxes []fbox
		for _, d := range []dir{north, east, south, west, up, down} {
			if p[d.String()] == "true" {
				boxes = append(boxes, sideBox(d, 1.0/16, 0, 1))
			}
		}
		if len(boxes) > 0 {
			return boxes
		}
	case n == "lantern", n == "soul_lantern", strings.HasSuffix(n, "copper_lantern"):
		if p["hanging"] == "true" {
			return []fbox{centered(0.375, 0.0625, 0.5625), centered(0.125, 0.5625, 1)}
		}
		return []fbox{centered(0.375, 0, 0.5)}
	case strings.HasSuffix(n, "torch"):
		if strings.Contains(n, "wall_torch") && hasFacing {
			b := centered(0.125, 0.2, 0.85)
			off := facing.opposite().vec()
			for _, a := range []int{0, 2} {
				b.min[a] += 0.3 * float64(off[a])
				b.max[a] += 0.3 * float64(off[a])
			}
			return []fbox{b}
		}
		return []fbox{centered(0.125, 0, 0.625)}
	case strings.HasSuffix(n, "chain"), n == "end_rod", n == "lightning_rod":
		switch p["axis"] {
		case "x":
			return []fbox{fb(0, 0.4375, 0.4375, 1, 0.5625, 0.5625)}
		case "z":
			return []fbox{fb(0.4375, 0.4375, 0, 0.5625, 0.5625, 1)}
		}
		return []fbox{centered(0.125, 0, 1)}
	case strings.HasSuffix(n, "_bed"):
		return []fbox{fb(0, 0, 0, 1, 0.5625, 1)}
	case strings.HasSuffix(n, "chest"), n == "ender_chest":
		return []fbox{centered(0.875, 0, 0.875)}
	case strings.HasSuffix(n, "_button"), n == "lever", n == "tripwire_hook":
		return []fbox{centered(0.25, 0, 0.125)}
	case strings.HasSuffix(n, "_sign"), strings.HasSuffix(n, "_banner"):
		return []fbox{centered(0.125, 0, 0.6), fb(0.0625, 0.5, 0.4375, 0.9375, 1, 0.5625)}
	case strings.HasSuffix(n, "_head"), strings.HasSuffix(n, "_skull"):
		return []fbox{centered(0.5, 0, 0.5)}
	case n == "flower_pot", strings.HasPrefix(n, "potted_"):
		return []fbox{centered(0.375, 0, 0.375)}
	case strings.HasSuffix(n, "candle"):
		return []fbox{centered(0.125, 0, 0.4)}
	case n == "campfire", n == "soul_campfire", n == "daylight_detector", strings.HasSuffix(n, "_petals"), n == "lily_pad":
		return []fbox{fb(0, 0, 0, 1, 0.4, 1)}
	case n == "short_grass", n == "fern", n == "dead_bush", n == "bush", strings.HasSuffix(n, "_sapling"),
		strings.HasSuffix(n, "_mushroom"), strings.HasSuffix(n, "_tulip"), n == "dandelion", n == "poppy",
		n == "blue_orchid", n == "allium", n == "azure_bluet", n == "oxeye_daisy", n == "cornflower",
		n == "lily_of_the_valley", n == "wither_rose", n == "torchflower", n == "sweet_berry_bush",
		n == "seagrass", strings.HasSuffix(n, "_coral"), n == "wheat", n == "carrots", n == "potatoes",
		n == "beetroots", n == "redstone_wire", n == "rail", strings.HasSuffix(n, "_rail"):
		if n == "redstone_wire" || strings.HasSuffix(n, "rail") {
			return []fbox{fb(0, 0, 0, 1, 0.0625, 1)}
		}
		return []fbox{centered(0.6, 0, 0.7)}
	case n == "tall_grass", n == "large_fern", n == "sunflower", n == "lilac", n == "rose_bush", n == "peony",
		n == "tall_seagrass", n == "pitcher_plant":
		return []fbox{centered(0.7, 0, 1)}
	case n == "cactus":
		return []fbox{centered(0.875, 0, 1)}
	case n == "anvil", n == "chipped_anvil", n == "damaged_anvil", n == "enchanting_table":
		return []fbox{fb(0, 0, 0, 1, 0.75, 1)}
	case n == "dirt_path", n == "farmland":
		return []fbox{fb(0, 0, 0, 1, 0.9375, 1)}
	case n == "structure_void", n == "barrier", n == "light":
		return nil
	}
	return fullCell
}

// opacity is how much of what's behind a block it hides.
func opacity(bs structure.BlockState) float64 {
	n := strings.TrimPrefix(bs.Name, "minecraft:")
	switch {
	case n == "water", n == "bubble_column":
		return 0.45
	case strings.Contains(n, "glass"), n == "ice", n == "frosted_ice", strings.HasSuffix(n, "_bars"), n == "iron_bars":
		return 0.4
	case strings.HasSuffix(n, "_leaves"):
		return 0.9
	}
	return 1
}

func blend(dst, src color.RGBA, a float64) color.RGBA {
	mix := func(d, s uint8) uint8 { return uint8(float64(d)*(1-a) + float64(s)*a + 0.5) }
	return color.RGBA{mix(dst.R, src.R), mix(dst.G, src.G), mix(dst.B, src.B), 0xff}
}

// renderIso draws an isometric view from above the south-east corner of the
// structure after turning it by rot, which brings the other corners there.
func renderIso(g *grid, r box, scale, target int, rot rotation) (*image.RGBA, error) {
	// Turn the region into local coordinates where the viewer is always at
	// the south-east.
	rb := rot.box(box{vec{}, r.size().sub(vec{1, 1, 1})})
	shift := vec{}.sub(rb.min)
	type cell struct {
		p  vec // local position
		bs structure.BlockState
	}
	local := map[vec]structure.BlockState{}
	var cells []cell
	for p, i := range g.at {
		if !r.contains(p) {
			continue
		}
		bs, ok := stateOf(g.st, g.st.Blocks[i])
		if !ok || isAir(bs.Name) {
			continue
		}
		lp := rot.apply(p.sub(r.min)).add(shift)
		bs = rotateState(bs, rot)
		local[lp] = bs
		cells = append(cells, cell{lp, bs})
	}
	if len(cells) > 1<<20 {
		return nil, fmt.Errorf("%d blocks is too many to render; limit it with from and to", len(cells))
	}
	sz := rb.size()
	sc := pickScale(scale, sz[0]+sz[2]+sz[1], target, 24)
	sc = max(sc, 2)
	w := (sz[0]+sz[2])*sc + 2
	h := (sz[0]+sz[2])*sc/2 + sz[1]*sc + 2
	if w*h > 24<<20 {
		return nil, fmt.Errorf("image would be too large; use a smaller scale or limit it with from and to")
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fillRect(img, img.Bounds(), background)
	ox, oy := float64(sz[2]*sc+1), float64(sz[1]*sc+1)
	fsc := float64(sc)
	proj := func(x, y, z float64) [2]float64 {
		return [2]float64{ox + (x-z)*fsc, oy + (x+z)*fsc/2 - y*fsc}
	}
	// Painter's order: far to near. Larger x+y+z is nearer the viewer.
	sort.Slice(cells, func(i, j int) bool {
		a, b := cells[i].p, cells[j].p
		if sa, sb := a[0]+a[1]+a[2], b[0]+b[1]+b[2]; sa != sb {
			return sa < sb
		}
		if a[1] != b[1] {
			return a[1] < b[1]
		}
		return a[0] < b[0]
	})
	outline := sc >= 6
	for _, c := range cells {
		base := blockColor(c.bs)
		alpha := opacity(c.bs)
		boxes := shapeOf(c.bs)
		sort.Slice(boxes, func(i, j int) bool {
			return boxes[i].min[0]+boxes[i].min[1]+boxes[i].min[2] < boxes[j].min[0]+boxes[j].min[1]+boxes[j].min[2]
		})
		// A face on the cell's edge is hidden by a full neighbour, and a
		// see-through block hides its faces against the same block.
		hidden := func(d vec) bool {
			nb, ok := local[c.p.add(d)]
			if !ok {
				return false
			}
			if alpha < 1 {
				return nb.Name == c.bs.Name || (isFull(nb) && opacity(nb) >= 1)
			}
			return isFull(nb) && opacity(nb) >= 1 && len(shapeOf(nb)) == 1 && shapeOf(nb)[0] == fullCell[0]
		}
		x, y, z := float64(c.p[0]), float64(c.p[1]), float64(c.p[2])
		for _, b := range boxes {
			x0, y0, z0 := x+b.min[0], y+b.min[1], z+b.min[2]
			x1, y1, z1 := x+b.max[0], y+b.max[1], z+b.max[2]
			type face struct {
				pts    [][2]float64
				f      float64
				onEdge bool
				d      vec
			}
			faces := []face{
				{[][2]float64{proj(x0, y1, z0), proj(x1, y1, z0), proj(x1, y1, z1), proj(x0, y1, z1)}, 1, b.max[1] == 1, vec{0, 1, 0}},
				{[][2]float64{proj(x0, y1, z1), proj(x1, y1, z1), proj(x1, y0, z1), proj(x0, y0, z1)}, 0.72, b.max[2] == 1, vec{0, 0, 1}},
				{[][2]float64{proj(x1, y1, z0), proj(x1, y1, z1), proj(x1, y0, z1), proj(x1, y0, z0)}, 0.86, b.max[0] == 1, vec{1, 0, 0}},
			}
			for _, f := range faces {
				if f.onEdge && hidden(f.d) {
					continue
				}
				col := shade(base, f.f)
				if alpha < 1 {
					fillPolyBlend(img, f.pts, col, alpha)
				} else {
					fillPoly(img, f.pts, col)
				}
				if outline {
					strokePoly(img, f.pts, shade(col, 0.8))
				}
			}
		}
	}
	return img, nil
}

func fillPolyBlend(img *image.RGBA, pts [][2]float64, c color.RGBA, a float64) {
	forPolyPixels(img, pts, func(x, y int) {
		img.SetRGBA(x, y, blend(img.RGBAAt(x, y), c, a))
	})
}

// forPolyPixels calls f for each pixel whose center is inside a convex
// polygon.
func forPolyPixels(img *image.RGBA, pts [][2]float64, f func(x, y int)) {
	minX, minY, maxX, maxY := pts[0][0], pts[0][1], pts[0][0], pts[0][1]
	for _, p := range pts {
		minX, maxX = math.Min(minX, p[0]), math.Max(maxX, p[0])
		minY, maxY = math.Min(minY, p[1]), math.Max(maxY, p[1])
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
				f(x, y)
			}
		}
	}
}

func strokePoly(img *image.RGBA, pts [][2]float64, c color.RGBA) {
	for i := range pts {
		a, b := pts[i], pts[(i+1)%len(pts)]
		steps := int(math.Max(math.Abs(b[0]-a[0]), math.Abs(b[1]-a[1]))) + 1
		for k := 0; k <= steps; k++ {
			t := float64(k) / float64(steps)
			x := int(math.Floor(a[0] + (b[0]-a[0])*t))
			y := int(math.Floor(a[1] + (b[1]-a[1])*t))
			if image.Pt(x, y).In(img.Bounds()) {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func outlineRect(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	r = r.Intersect(img.Bounds())
	if r.Empty() {
		return
	}
	for x := r.Min.X; x < r.Max.X; x++ {
		img.SetRGBA(x, r.Min.Y, c)
		img.SetRGBA(x, r.Max.Y-1, c)
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		img.SetRGBA(r.Min.X, y, c)
		img.SetRGBA(r.Max.X-1, y, c)
	}
}

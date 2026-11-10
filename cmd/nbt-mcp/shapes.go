package main

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/MinecraftMetascript/nbt-go/structure"
)

// shapeOpts are the options shared by the shape tools.
type shapeOpts struct {
	Block     string `json:"block" jsonschema:"block, or a weighted mix such as 70%stone,20%andesite,10%cobblestone"`
	Hollow    bool   `json:"hollow,omitempty" jsonschema:"only build the shell. Shells are watertight: every block of the inside is cut off from the outside on all six faces"`
	Thickness int    `json:"thickness,omitempty" jsonschema:"shell thickness in blocks when hollow (default 1)"`
	Interior  string `json:"interior,omitempty" jsonschema:"when hollow: air (default) fills the inside with air so placing the shape clears it (and keeps water out), keep leaves it unchanged"`
	Seed      *int64 `json:"seed,omitempty" jsonschema:"random seed for block mixes; a random one is used and reported if omitted"`
}

// solid describes a shape by its bounding box and a membership test.
type solid struct {
	bounds box
	inside func(p vec) bool
	// shellInside is used instead of inside when deciding what counts as
	// outside for the shell, so a tube's open ends don't get capped.
	shellInside func(p vec) bool
}

// buildSolid places a solid's blocks and reports what it did.
func (s *state) buildSolid(st *structure.Structure, sh solid, o shapeOpts, what string) (string, error) {
	pat, err := parsePattern(o.Block)
	if err != nil {
		return "", err
	}
	if o.Thickness == 0 {
		o.Thickness = 1
	}
	if o.Thickness < 0 {
		return "", errors.New("thickness must be positive")
	}
	switch o.Interior {
	case "":
		o.Interior = "air"
	case "air", "keep":
	default:
		return "", fmt.Errorf("interior %q must be air or keep", o.Interior)
	}
	// Parts below 0 are cut off, so a sphere centered on the ground makes a
	// dome.
	full := sh.bounds
	clipped := false
	for a := 0; a < 3; a++ {
		if sh.bounds.min[a] < 0 {
			sh.bounds.min[a], clipped = 0, true
		}
	}
	if sh.bounds.max[0] < 0 || sh.bounds.max[1] < 0 || sh.bounds.max[2] < 0 {
		return "", fmt.Errorf("the %s spans %s, entirely below 0; coordinates must be non-negative", what, full)
	}
	sz := sh.bounds.size()
	if sz[0]*sz[1]*sz[2] > maxVolume {
		return "", fmt.Errorf("the %s spans %s, more than %d positions", what, sh.bounds, maxVolume)
	}
	shellIn := sh.inside
	if sh.shellInside != nil {
		shellIn = sh.shellInside
	}
	rng, seed := newRNG(o.Seed)
	air := structure.BlockState{Name: "minecraft:air"}
	shell, interior := 0, 0
	b := sh.bounds
	for x := b.min[0]; x <= b.max[0]; x++ {
		for y := b.min[1]; y <= b.max[1]; y++ {
			for z := b.min[2]; z <= b.max[2]; z++ {
				p := vec{x, y, z}
				if !sh.inside(p) {
					continue
				}
				place := pat.pick(rng)
				if o.Hollow && !nearOutside(p, o.Thickness, shellIn) {
					interior++
					if o.Interior == "keep" {
						continue
					}
					place = air
				} else {
					shell++
				}
				if err := st.SetBlock(x, y, z, place, nil); err != nil {
					return "", err
				}
			}
		}
	}
	if shell == 0 {
		return "", fmt.Errorf("the %s contains no blocks; check its size", what)
	}
	var out strings.Builder
	fmt.Fprintf(&out, "Built a %s spanning %s with %d blocks of %s", what, sh.bounds, shell, pat)
	if o.Hollow {
		fmt.Fprintf(&out, " (shell %d thick", o.Thickness)
		if o.Interior == "air" {
			fmt.Fprintf(&out, ", %d air inside)", interior)
		} else {
			fmt.Fprintf(&out, ", %d inside positions left unchanged)", interior)
		}
	}
	if pat.random() {
		fmt.Fprintf(&out, ", seed %d", seed)
	}
	fmt.Fprintf(&out, ". Size is now %v.", st.Size)
	if clipped {
		fmt.Fprintf(&out, " The full shape spans %s; the part below 0 was cut off.", full)
		if o.Hollow {
			out.WriteString(" The shell is open where it was cut.")
		}
	}
	return out.String(), nil
}

// nearOutside reports whether p is within t steps of the outside of a shape
// along any axis. Building the shell from these positions leaves no gaps
// that water or a player could pass through, even diagonally.
func nearOutside(p vec, t int, inside func(vec) bool) bool {
	for _, d := range dirVecs {
		q := p
		for k := 0; k < t; k++ {
			q = q.add(d)
			if !inside(q) {
				return true
			}
		}
	}
	return false
}

func floorI(f float64) int { return int(math.Floor(f)) }
func ceilI(f float64) int  { return int(math.Ceil(f)) }

type ellipsoidIn struct {
	Name   string     `json:"name"`
	Center [3]float64 `json:"center" jsonschema:"center [x, y, z]. Whole numbers center on a block; use .5 (e.g. 4.5) to center between blocks for an even width"`
	Radius float64    `json:"radius,omitempty" jsonschema:"radius for a sphere"`
	Radii  []float64  `json:"radii,omitempty" jsonschema:"radii [rx, ry, rz] for an ellipsoid; use instead of radius"`
	shapeOpts
}

func radii3(r float64, rs []float64) ([3]float64, error) {
	var out [3]float64
	switch {
	case len(rs) == 3:
		copy(out[:], rs)
	case len(rs) == 0 && r > 0:
		out = [3]float64{r, r, r}
	default:
		return out, errors.New("give radius, or radii as [rx, ry, rz]")
	}
	for _, v := range out {
		if v <= 0 {
			return out, errors.New("radii must be positive")
		}
	}
	return out, nil
}

func (s *state) ellipsoid(in ellipsoidIn) (string, error) {
	st, err := s.get(in.Name)
	if err != nil {
		return "", err
	}
	r, err := radii3(in.Radius, in.Radii)
	if err != nil {
		return "", err
	}
	c := in.Center
	// A small tolerance keeps whole-number radii from losing the extreme
	// block on each axis to rounding.
	inside := func(p vec) bool {
		sum := 0.0
		for a := 0; a < 3; a++ {
			d := (float64(p[a]) - c[a]) / (r[a] + 0.01)
			sum += d * d
		}
		return sum <= 1
	}
	var b box
	for a := 0; a < 3; a++ {
		b.min[a], b.max[a] = ceilI(c[a]-r[a]-0.01), floorI(c[a]+r[a]+0.01)
	}
	what := "sphere"
	if r[0] != r[1] || r[1] != r[2] {
		what = "ellipsoid"
	}
	return s.buildSolid(st, solid{bounds: b, inside: inside}, in.shapeOpts, what)
}

type cylinderIn struct {
	Name     string     `json:"name"`
	Base     [3]float64 `json:"base" jsonschema:"center of the bottom end [x, y, z] (for axis x or z, the end with the lowest coordinate). Use .5 on the cross-section axes for an even width"`
	Axis     string     `json:"axis,omitempty" jsonschema:"axis the cylinder runs along: y (default, upright), x or z"`
	Height   int        `json:"height" jsonschema:"length along the axis in blocks; 1 makes a disc"`
	Radius   float64    `json:"radius,omitempty" jsonschema:"radius of a round cross-section"`
	Radii    []float64  `json:"radii,omitempty" jsonschema:"radii of an elliptical cross-section, in x, y, z order skipping the axis (e.g. [rx, rz] for axis y)"`
	OpenEnds bool       `json:"open_ends,omitempty" jsonschema:"when hollow, leave the ends open to make a tube or ring"`
	shapeOpts
}

func (s *state) cylinder(in cylinderIn) (string, error) {
	st, err := s.get(in.Name)
	if err != nil {
		return "", err
	}
	ax := 1
	switch in.Axis {
	case "", "y":
	case "x":
		ax = 0
	case "z":
		ax = 2
	default:
		return "", fmt.Errorf("axis %q must be x, y or z", in.Axis)
	}
	if in.Height < 1 {
		return "", errors.New("height must be at least 1")
	}
	var perp [2]int
	for a, k := 0, 0; a < 3; a++ {
		if a != ax {
			perp[k] = a
			k++
		}
	}
	var r [2]float64
	switch {
	case len(in.Radii) == 2:
		r = [2]float64{in.Radii[0], in.Radii[1]}
	case len(in.Radii) == 0 && in.Radius > 0:
		r = [2]float64{in.Radius, in.Radius}
	default:
		return "", errors.New("give radius, or radii with two values")
	}
	if r[0] <= 0 || r[1] <= 0 {
		return "", errors.New("radii must be positive")
	}
	c := in.Base
	lo := floorI(c[ax])
	hi := lo + in.Height - 1
	inRing := func(p vec) bool {
		sum := 0.0
		for k, a := range perp {
			d := (float64(p[a]) - c[a]) / (r[k] + 0.01)
			sum += d * d
		}
		return sum <= 1
	}
	inside := func(p vec) bool { return p[ax] >= lo && p[ax] <= hi && inRing(p) }
	sh := solid{inside: inside}
	if in.OpenEnds {
		sh.shellInside = inRing
	}
	for k, a := range perp {
		sh.bounds.min[a], sh.bounds.max[a] = ceilI(c[a]-r[k]-0.01), floorI(c[a]+r[k]+0.01)
	}
	sh.bounds.min[ax], sh.bounds.max[ax] = lo, hi
	what := "cylinder"
	if in.Height == 1 {
		what = "disc"
	} else if in.Hollow && in.OpenEnds {
		what = "tube"
	}
	return s.buildSolid(st, sh, in.shapeOpts, what)
}

type tubeIn struct {
	Name   string       `json:"name"`
	Points [][3]float64 `json:"points" jsonschema:"path of the tube as a list of [x, y, z] points; each segment is joined with rounded ends, which suits trunks, branches, roots and tentacles"`
	Radius float64      `json:"radius,omitempty" jsonschema:"radius along the whole path"`
	Radii  []float64    `json:"radii,omitempty" jsonschema:"radius at each point, for a tapering tube; must have one value per point"`
	shapeOpts
}

func (s *state) tube(in tubeIn) (string, error) {
	st, err := s.get(in.Name)
	if err != nil {
		return "", err
	}
	if len(in.Points) == 0 {
		return "", errors.New("points needs at least one point")
	}
	rs := in.Radii
	if len(rs) == 0 {
		if in.Radius <= 0 {
			return "", errors.New("give radius, or radii with one value per point")
		}
		rs = make([]float64, len(in.Points))
		for i := range rs {
			rs[i] = in.Radius
		}
	}
	if len(rs) != len(in.Points) {
		return "", fmt.Errorf("radii has %d values for %d points", len(rs), len(in.Points))
	}
	var b box
	for i, p := range in.Points {
		if rs[i] <= 0 {
			return "", errors.New("radii must be positive")
		}
		for a := 0; a < 3; a++ {
			lo, hi := ceilI(p[a]-rs[i]-0.01), floorI(p[a]+rs[i]+0.01)
			if i == 0 || lo < b.min[a] {
				b.min[a] = lo
			}
			if i == 0 || hi > b.max[a] {
				b.max[a] = hi
			}
		}
	}
	pts := in.Points
	inside := func(p vec) bool {
		q := [3]float64{float64(p[0]), float64(p[1]), float64(p[2])}
		for i := range pts {
			j := min(i+1, len(pts)-1)
			a, c := pts[i], pts[j]
			var ab, aq [3]float64
			l2 := 0.0
			for k := 0; k < 3; k++ {
				ab[k], aq[k] = c[k]-a[k], q[k]-a[k]
				l2 += ab[k] * ab[k]
			}
			t := 0.0
			if l2 > 0 {
				t = (aq[0]*ab[0] + aq[1]*ab[1] + aq[2]*ab[2]) / l2
				t = math.Max(0, math.Min(1, t))
			}
			d2 := 0.0
			for k := 0; k < 3; k++ {
				d := q[k] - (a[k] + t*ab[k])
				d2 += d * d
			}
			r := rs[i] + t*(rs[j]-rs[i]) + 0.01
			if d2 <= r*r {
				return true
			}
		}
		return false
	}
	return s.buildSolid(st, solid{bounds: b, inside: inside}, in.shapeOpts, "tube")
}

type decorateIn struct {
	Name   string   `json:"name"`
	Block  string   `json:"block" jsonschema:"block or weighted mix to apply, e.g. moss_carpet or 50%vine[north=true],50%air"`
	Faces  []string `json:"faces,omitempty" jsonschema:"which exposed faces to decorate: up, down, north, south, east, west (default [up])"`
	Mode   string   `json:"mode,omitempty" jsonschema:"on (default) places the block in the empty space against the face, like moss carpet or snow on top; replace swaps the surface block itself, like turning the top of a mound into grass_block"`
	Chance float64  `json:"chance,omitempty" jsonschema:"probability from 0 to 1 that each surface position is decorated (default 1)"`
	Only   []string `json:"only,omitempty" jsonschema:"only decorate faces of these blocks, e.g. [stone, andesite]; default any solid block"`
	From   *[3]int  `json:"from,omitempty" jsonschema:"limit to a box: first corner [x, y, z]"`
	To     *[3]int  `json:"to,omitempty" jsonschema:"limit to a box: opposite corner [x, y, z], inclusive"`
	Seed   *int64   `json:"seed,omitempty" jsonschema:"random seed; a random one is used and reported if omitted"`
}

func (s *state) decorate(in decorateIn) (string, error) {
	st, err := s.get(in.Name)
	if err != nil {
		return "", err
	}
	pat, err := parsePattern(in.Block)
	if err != nil {
		return "", err
	}
	faces := in.Faces
	if len(faces) == 0 {
		faces = []string{"up"}
	}
	var ds []dir
	for _, f := range faces {
		d, ok := parseDir(strings.ToLower(f))
		if !ok {
			return "", fmt.Errorf("face %q must be up, down, north, south, east or west", f)
		}
		ds = append(ds, d)
	}
	mode := in.Mode
	if mode == "" {
		mode = "on"
	}
	if mode != "on" && mode != "replace" {
		return "", fmt.Errorf("mode %q must be on or replace", in.Mode)
	}
	chance := in.Chance
	if chance == 0 {
		chance = 1
	}
	if chance < 0 || chance > 1 {
		return "", errors.New("chance must be between 0 and 1")
	}
	only := map[string]bool{}
	for _, o := range in.Only {
		only[namespaced(o)] = true
	}
	region := sizeBox(st.Size)
	if (in.From == nil) != (in.To == nil) {
		return "", errors.New("give both from and to, or neither")
	}
	if in.From != nil {
		lo, hi, err := boxIn{From: *in.From, To: *in.To}.bounds()
		if err != nil {
			return "", err
		}
		region = box{vec(lo), vec(hi)}
	}
	rng, seed := newRNG(in.Seed)
	g := newGrid(st)
	type change struct {
		p  vec
		bs structure.BlockState
	}
	var changes []change
	done := map[vec]bool{}
	for _, b := range st.Blocks {
		p := toVec(b.Pos)
		if !region.contains(p) {
			continue
		}
		bs, ok := stateOf(st, b)
		if !ok || !isFull(bs) || (len(only) > 0 && !only[bs.Name]) {
			continue
		}
		for _, d := range ds {
			q := p.add(d.vec())
			if nb, _, ok := g.get(q); ok && !isAir(nb.Name) {
				continue
			}
			target := p
			if mode == "on" {
				target = q
			}
			if done[target] || target[0] < 0 || target[1] < 0 || target[2] < 0 {
				continue
			}
			done[target] = true
			if rng.Float64() >= chance {
				continue
			}
			changes = append(changes, change{target, pat.pick(rng)})
		}
	}
	for _, c := range changes {
		if err := st.SetBlock(c.p[0], c.p[1], c.p[2], c.bs, nil); err != nil {
			return "", err
		}
	}
	msg := fmt.Sprintf("Decorated %d of %d exposed faces with %s (mode %s)", len(changes), len(done), pat, mode)
	if pat.random() || chance < 1 {
		msg += fmt.Sprintf(", seed %d", seed)
	}
	return msg + fmt.Sprintf(". Size is now %v.", st.Size), nil
}

type gridIn struct {
	Name   string            `json:"name"`
	Origin [3]int            `json:"origin" jsonschema:"position [x, y, z] of the first character of the first row of the first layer"`
	Legend map[string]string `json:"legend" jsonschema:"what each character means: a block or weighted mix. '.' means no block and ' ' (space) means air unless redefined"`
	Layers [][]string        `json:"layers" jsonschema:"horizontal layers from bottom (origin y) to top. Each layer is a list of rows from north to south (increasing z); each character in a row is one block going east (increasing x). This is the same layout render_layer draws"`
	Seed   *int64            `json:"seed,omitempty" jsonschema:"random seed for weighted mixes in the legend"`
}

func (s *state) placeGrid(in gridIn) (string, error) {
	st, err := s.get(in.Name)
	if err != nil {
		return "", err
	}
	if in.Origin[0] < 0 || in.Origin[1] < 0 || in.Origin[2] < 0 {
		return "", fmt.Errorf("origin must be non-negative, got %v", in.Origin)
	}
	legend := map[rune]*pattern{'.': nil}
	air, _ := parsePattern("air")
	legend[' '] = &air
	random := false
	for k, v := range in.Legend {
		rs := []rune(k)
		if len(rs) != 1 {
			return "", fmt.Errorf("legend key %q must be a single character", k)
		}
		if strings.TrimSpace(v) == "" {
			legend[rs[0]] = nil
			continue
		}
		p, err := parsePattern(v)
		if err != nil {
			return "", fmt.Errorf("legend %q: %w", k, err)
		}
		legend[rs[0]] = &p
		random = random || p.random()
	}
	// Check everything before changing the structure.
	n := 0
	for y, layer := range in.Layers {
		for z, row := range layer {
			for x, c := range []rune(row) {
				if _, ok := legend[c]; !ok {
					return "", fmt.Errorf("layers[%d][%d] column %d: %q is not in the legend", y, z, x, c)
				}
				n++
			}
		}
	}
	if n > maxVolume {
		return "", fmt.Errorf("the grid has %d positions, more than %d", n, maxVolume)
	}
	rng, seed := newRNG(in.Seed)
	placed := 0
	for y, layer := range in.Layers {
		for z, row := range layer {
			for x, c := range []rune(row) {
				p := legend[c]
				if p == nil {
					continue
				}
				o := in.Origin
				if err := st.SetBlock(o[0]+x, o[1]+y, o[2]+z, p.pick(rng), nil); err != nil {
					return "", err
				}
				placed++
			}
		}
	}
	msg := fmt.Sprintf("Placed %d blocks from %d layers", placed, len(in.Layers))
	if random {
		msg += fmt.Sprintf(", seed %d", seed)
	}
	return msg + fmt.Sprintf(". Size is now %v.", st.Size), nil
}

type sealedIn struct {
	Name   string `json:"name"`
	Inside [3]int `json:"inside" jsonschema:"a position [x, y, z] inside the space that should be sealed off"`
}

// checkSealed flood-fills from a position through everything that isn't a
// full block, to see whether the space connects to the outside.
func (s *state) checkSealed(in sealedIn) (string, error) {
	st, err := s.get(in.Name)
	if err != nil {
		return "", err
	}
	start := vec(in.Inside)
	bounds := sizeBox(st.Size)
	if !bounds.contains(start) {
		return "", fmt.Errorf("%v is outside the structure, %s", start, bounds)
	}
	g := newGrid(st)
	open := func(p vec) bool {
		bs, _, ok := g.get(p)
		return !ok || !isFull(bs)
	}
	if !open(start) {
		bs, _, _ := g.get(start)
		return "", fmt.Errorf("%v is %s, a solid block; give a position in the space to check", start, bs)
	}
	parent := map[vec]vec{start: start}
	queue := []vec{start}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, d := range dirVecs {
			q := p.add(d)
			if _, seen := parent[q]; seen {
				continue
			}
			parent[q] = p
			if !bounds.contains(q) {
				return leakReport(g, parent, q, start), nil
			}
			if open(q) {
				queue = append(queue, q)
			}
		}
	}
	return fmt.Sprintf("Sealed: the space at %v (%d positions) is closed off on all sides by full blocks.", start, len(parent)), nil
}

func leakReport(g *grid, parent map[vec]vec, out, start vec) string {
	var path []vec
	for p := parent[out]; ; p = parent[p] {
		path = append(path, p)
		if p == start {
			break
		}
	}
	// path runs from the outside in. The gap is most likely the outermost
	// block on it that isn't full (a slab, a door), or else the outermost
	// opening in a wall.
	solidAround := func(p vec) int {
		n := 0
		for _, d := range dirVecs {
			if bs, _, ok := g.get(p.add(d)); ok && isFull(bs) {
				n++
			}
		}
		return n
	}
	gap, found := path[0], false
	for _, p := range path {
		if bs, _, ok := g.get(p); ok && !isAir(bs.Name) {
			gap, found = p, true
			break
		}
	}
	if !found {
		for _, p := range path {
			if solidAround(p) >= 2 {
				gap = p
				break
			}
		}
	}
	best := solidAround(gap)
	what := "no block (whatever is in the world)"
	if bs, _, ok := g.get(gap); ok {
		what = bs.String()
	}
	return fmt.Sprintf("Not sealed: the space at %v reaches the outside of the structure in %d steps, leaving at %v.\nThe likely gap is at %v, which holds %s with %d of 6 neighbours solid.\nPositions with no block, air, and blocks that don't fill their cell (slabs, stairs, fences, panes, doors and so on) count as open.",
		start, len(path), path[0], gap, what, best)
}

package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/MinecraftMetascript/nbt-go"
	"github.com/MinecraftMetascript/nbt-go/structure"
)

const jigsawBlock = "minecraft:jigsaw"

// emptyID is the id the game uses for "no name", "no target" and "no pool".
const emptyID = "minecraft:empty"

// jigsaw is a jigsaw block read from a structure.
type jigsaw struct {
	pos        vec
	front, top dir
	name       string
	target     string
	pool       string
	finalState string
	joint      string // "rollable" or "aligned"
	placement  int    // placement_priority
	selection  int    // selection_priority

	hasNBT, hasOrientation, jointSet bool
}

func (j jigsaw) rollable() bool { return j.joint == "rollable" }

func (j jigsaw) orientation() string { return j.front.String() + "_" + j.top.String() }

// targetPos is where the connecting piece's jigsaw ends up: the block
// directly in front of this one.
func (j jigsaw) targetPos() vec { return j.pos.add(j.front.vec()) }

func (j jigsaw) rotated(r rotation) jigsaw {
	j.pos = r.apply(j.pos)
	j.front, j.top = r.dir(j.front), r.dir(j.top)
	return j
}

func (j jigsaw) String() string {
	return fmt.Sprintf("jigsaw at %v facing %s", j.pos, j.front)
}

// parseOrientation reads a jigsaw orientation such as "down_east" or
// "north_up" into its front and top directions.
func parseOrientation(s string) (front, top dir, ok bool) {
	a, b, found := strings.Cut(s, "_")
	if !found {
		return 0, 0, false
	}
	front, ok1 := parseDir(a)
	top, ok2 := parseDir(b)
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	if front.horizontal() != (top == up) || (!front.horizontal() && !top.horizontal()) {
		return 0, 0, false
	}
	return front, top, true
}

func nbtString(c nbt.Compound, key string) (string, bool) {
	s, ok := c[key].(nbt.String)
	return string(s), ok
}

func nbtInt(c nbt.Compound, key string) int {
	switch v := c[key].(type) {
	case nbt.Byte:
		return int(v)
	case nbt.Short:
		return int(v)
	case nbt.Int:
		return int(v)
	case nbt.Long:
		return int(v)
	}
	return 0
}

// parseJigsaw reads a jigsaw block, applying the game's defaults for missing
// fields.
func parseJigsaw(pos vec, bs structure.BlockState, data nbt.Compound) jigsaw {
	j := jigsaw{pos: pos, front: north, top: up, name: emptyID, target: emptyID, pool: emptyID, finalState: "minecraft:air"}
	if f, t, ok := parseOrientation(bs.Properties["orientation"]); ok {
		j.front, j.top, j.hasOrientation = f, t, true
	}
	j.hasNBT = data != nil
	if v, ok := nbtString(data, "name"); ok && v != "" {
		j.name = namespaced(v)
	}
	if v, ok := nbtString(data, "target"); ok && v != "" {
		j.target = namespaced(v)
	}
	if v, ok := nbtString(data, "pool"); ok && v != "" {
		j.pool = namespaced(v)
	}
	if v, ok := nbtString(data, "final_state"); ok && v != "" {
		j.finalState = v
	}
	if v, ok := nbtString(data, "joint"); ok && (v == "rollable" || v == "aligned") {
		j.joint, j.jointSet = v, true
	} else if j.front.horizontal() {
		j.joint = "aligned"
	} else {
		j.joint = "rollable"
	}
	j.placement = nbtInt(data, "placement_priority")
	j.selection = nbtInt(data, "selection_priority")
	return j
}

// jigsawsOf lists the jigsaw blocks in a structure, ordered by position.
func jigsawsOf(st *structure.Structure) []jigsaw {
	var js []jigsaw
	for _, b := range st.Blocks {
		bs, ok := stateOf(st, b)
		if !ok || bs.Name != jigsawBlock {
			continue
		}
		js = append(js, parseJigsaw(toVec(b.Pos), bs, b.NBT))
	}
	sort.Slice(js, func(a, b int) bool {
		pa, pb := js[a].pos, js[b].pos
		if pa[1] != pb[1] {
			return pa[1] < pb[1]
		}
		if pa[2] != pb[2] {
			return pa[2] < pb[2]
		}
		return pa[0] < pb[0]
	})
	return js
}

// canAttach reports whether a piece with jigsaw tgt can be attached to the
// jigsaw src, following the game's JigsawBlock.canAttach: they must face each
// other, src's target must be tgt's name, and unless src's joint is
// rollable, their tops must match.
func canAttach(src, tgt jigsaw) bool {
	return src.front == tgt.front.opposite() && (src.rollable() || src.top == tgt.top) && src.target == tgt.name
}

// whyNot explains why tgt can't attach to src.
func whyNot(src, tgt jigsaw) string {
	switch {
	case src.target != tgt.name:
		return fmt.Sprintf("its name is %s, but the parent's target is %s", tgt.name, src.target)
	case src.front != tgt.front.opposite():
		return fmt.Sprintf("it faces %s, but must face %s (opposite the parent's)", tgt.front, src.front.opposite())
	case !src.rollable() && src.top != tgt.top:
		return fmt.Sprintf("the joint is aligned, so its top (%s) must match the parent's (%s)", tgt.top, src.top)
	}
	return ""
}

// jigsawNBT builds the block entity data of a jigsaw block.
func jigsawNBT(j jigsaw) nbt.Compound {
	return nbt.Compound{
		"id":                 nbt.String(jigsawBlock),
		"name":               nbt.String(j.name),
		"target":             nbt.String(j.target),
		"pool":               nbt.String(j.pool),
		"final_state":        nbt.String(j.finalState),
		"joint":              nbt.String(j.joint),
		"placement_priority": nbt.Int(j.placement),
		"selection_priority": nbt.Int(j.selection),
	}
}

type jigsawIn struct {
	Name              string `json:"name" jsonschema:"the structure to edit"`
	Pos               [3]int `json:"pos" jsonschema:"position [x, y, z] of the jigsaw block"`
	Facing            string `json:"facing" enum:"up,down,north,south,east,west" jsonschema:"direction the jigsaw's front points: up, down, north, south, east or west. The connecting piece is placed on this side, with its own jigsaw in the block directly in front of this one"`
	Top               string `json:"top,omitempty" enum:"up,north,south,east,west" jsonschema:"for up or down jigsaws, which horizontal direction the top points (default north); must be up for horizontal ones. Only matters with an aligned joint"`
	JigsawName        string `json:"jigsaw_name,omitempty" jsonschema:"this jigsaw's name, which other jigsaws target to attach to it (default minecraft:empty)"`
	Target            string `json:"target,omitempty" jsonschema:"name of the jigsaw on the child piece to connect to (default minecraft:empty)"`
	Pool              string `json:"pool,omitempty" jsonschema:"template pool to pick the child piece from, e.g. mypack:statues (default minecraft:empty, which means no child)"`
	FinalState        string `json:"final_state,omitempty" jsonschema:"block that replaces the jigsaw once the structure generates. Defaults to the block already at pos (the one the jigsaw replaces), else the block below it, else air"`
	Joint             string `json:"joint,omitempty" enum:"rollable,aligned" jsonschema:"rollable or aligned; only matters for up and down jigsaws (default rollable for them, aligned for horizontal ones)"`
	PlacementPriority int    `json:"placement_priority,omitempty" jsonschema:"children of higher-priority jigsaws are processed first (default 0)"`
	SelectionPriority int    `json:"selection_priority,omitempty" jsonschema:"this piece's jigsaws are tried in descending selection priority (default 0)"`
}

func (s *state) placeJigsaw(in jigsawIn) (string, error) {
	st, err := s.get(in.Name)
	if err != nil {
		return "", err
	}
	if in.Pos[0] < 0 || in.Pos[1] < 0 || in.Pos[2] < 0 {
		return "", fmt.Errorf("coordinates must be non-negative, got %v", in.Pos)
	}
	front, ok := parseDir(strings.ToLower(in.Facing))
	if !ok {
		return "", fmt.Errorf("facing %q must be one of up, down, north, south, east, west", in.Facing)
	}
	top := up
	if !front.horizontal() {
		top = north
	}
	if in.Top != "" {
		t, ok := parseDir(strings.ToLower(in.Top))
		if !ok {
			return "", fmt.Errorf("top %q must be one of up, north, south, east, west", in.Top)
		}
		top = t
	}
	if front.horizontal() && top != up {
		return "", fmt.Errorf("a jigsaw facing %s always has top up, got %s", front, top)
	}
	if !front.horizontal() && !top.horizontal() {
		return "", fmt.Errorf("a jigsaw facing %s needs a horizontal top, got %s", front, top)
	}
	j := jigsaw{
		pos: vec(in.Pos), front: front, top: top,
		name: emptyID, target: emptyID, pool: emptyID,
		placement: in.PlacementPriority, selection: in.SelectionPriority,
	}
	if in.JigsawName != "" {
		j.name = namespaced(in.JigsawName)
	}
	if in.Target != "" {
		j.target = namespaced(in.Target)
	}
	if in.Pool != "" {
		j.pool = namespaced(in.Pool)
	}
	switch in.Joint {
	case "":
		if front.horizontal() {
			j.joint = "aligned"
		} else {
			j.joint = "rollable"
		}
	case "rollable", "aligned":
		j.joint = in.Joint
	default:
		return "", fmt.Errorf("joint %q must be rollable or aligned", in.Joint)
	}

	why := ""
	if in.FinalState != "" {
		fs, err := structure.ParseBlockState(in.FinalState)
		if err != nil {
			return "", fmt.Errorf("final_state: %w", err)
		}
		j.finalState = fs.String()
	} else {
		j.finalState, why = defaultFinalState(st, j.pos)
	}

	bs := structure.BlockState{Name: jigsawBlock, Properties: map[string]string{"orientation": j.orientation()}}
	if err := st.SetBlock(j.pos[0], j.pos[1], j.pos[2], bs, jigsawNBT(j)); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Placed a jigsaw at %v (orientation %s): name %s, target %s, pool %s, joint %s, final_state %s",
		j.pos, j.orientation(), j.name, j.target, j.pool, j.joint, j.finalState)
	if why != "" {
		fmt.Fprintf(&b, " (%s)", why)
	}
	b.WriteString(".\n")
	if j.pool != emptyID {
		fmt.Fprintf(&b, "The child piece's jigsaw will sit at %v, the block in front of this one", j.targetPos())
		if sizeBox(st.Size).contains(j.targetPos()) {
			b.WriteString(", which is inside this piece's bounds, so the whole child piece must fit inside this piece's bounding box")
		}
		b.WriteString(".\n")
	}
	for _, l := range lintJigsaw(st, newGrid(st), j) {
		b.WriteString("Warning: " + l + "\n")
	}
	return b.String(), nil
}

// defaultFinalState picks the block a new jigsaw should turn into: the block
// it replaces, else the one below it, else air.
func defaultFinalState(st *structure.Structure, p vec) (string, string) {
	if bs, _, ok := st.Block(p[0], p[1], p[2]); ok && bs.Name != jigsawBlock {
		return bs.String(), "the block it replaced"
	}
	if p[1] > 0 {
		if bs, _, ok := st.Block(p[0], p[1]-1, p[2]); ok && bs.Name != jigsawBlock {
			return bs.String(), "the block below it"
		}
	}
	return "minecraft:air", "nothing was there to copy"
}

// lintJigsaw returns problems with one jigsaw block.
func lintJigsaw(st *structure.Structure, g *grid, j jigsaw) []string {
	var out []string
	if !j.hasNBT {
		out = append(out, "it has no block entity data, so it has no name, target or pool")
	}
	if !j.hasOrientation {
		out = append(out, "its orientation property is missing or invalid; the game will use north_up")
	}
	if _, err := structure.ParseBlockState(j.finalState); err != nil {
		out = append(out, fmt.Sprintf("final_state %q is not a valid block state", j.finalState))
	}
	hasPool := j.pool != emptyID
	switch {
	case hasPool && j.target == emptyID:
		out = append(out, fmt.Sprintf("it has pool %s but no target, so nothing can attach to it", j.pool))
	case !hasPool && j.target != emptyID:
		out = append(out, fmt.Sprintf("it has target %s but no pool, so it never spawns a child", j.target))
	case !hasPool && j.name == emptyID:
		out = append(out, "it has no name and no pool, so it does nothing except turn into its final_state")
	}
	if j.front.horizontal() && j.jointSet && j.joint == "rollable" {
		out = append(out, "joint is rollable, but the joint only matters for up and down jigsaws")
	}
	if fs, err := structure.ParseBlockState(j.finalState); err == nil && isAir(fs.Name) {
		var solid []string
		for _, d := range dirVecs {
			if bs, _, ok := g.get(j.pos.add(d)); ok && isFull(bs) {
				solid = append(solid, bs.String())
			}
		}
		if len(solid) >= 4 {
			out = append(out, fmt.Sprintf("final_state is air but %d of its 6 neighbours are solid, so it will probably leave a hole; consider final_state %s", len(solid), mostCommon(solid)))
		}
	}
	t := j.targetPos()
	if bounds := sizeBox(st.Size); hasPool && bounds.contains(t) {
		if bs, _, ok := g.get(t); ok && isFull(bs) && bs.Name != jigsawBlock {
			out = append(out, fmt.Sprintf("it faces into %s at %v, inside this piece; the child's jigsaw would replace that block", bs, t))
		}
	}
	return out
}

// jigsawReport lists a structure's jigsaws with their settings and lints.
func jigsawReport(st *structure.Structure) string {
	js := jigsawsOf(st)
	if len(js) == 0 {
		return ""
	}
	g := newGrid(st)
	bounds := sizeBox(st.Size)
	var b strings.Builder
	fmt.Fprintf(&b, "\nJigsaws (%d):\n", len(js))
	targets := map[vec][]vec{}
	for _, j := range js {
		fmt.Fprintf(&b, "  %v %s: name %s, target %s, pool %s, joint %s, final_state %s",
			j.pos, j.orientation(), j.name, j.target, j.pool, j.joint, j.finalState)
		if j.placement != 0 || j.selection != 0 {
			fmt.Fprintf(&b, ", placement_priority %d, selection_priority %d", j.placement, j.selection)
		}
		b.WriteByte('\n')
		if j.pool != emptyID && bounds.contains(j.targetPos()) {
			fmt.Fprintf(&b, "    connects inside this piece (child jigsaw at %v): the whole child must fit inside this piece's bounds, %s\n", j.targetPos(), bounds)
		}
		for _, l := range lintJigsaw(st, g, j) {
			fmt.Fprintf(&b, "    warning: %s\n", l)
		}
		if j.pool != emptyID {
			targets[j.targetPos()] = append(targets[j.targetPos()], j.pos)
		}
	}
	for t, from := range targets {
		if len(from) > 1 {
			fmt.Fprintf(&b, "  warning: jigsaws at %v all face %v, so their children would collide\n", from, t)
		}
	}
	return b.String()
}

type fitIn struct {
	Parent string  `json:"parent" jsonschema:"the open structure that holds the jigsaw with the pool"`
	Child  string  `json:"child" jsonschema:"the open structure that would be attached to it"`
	Jigsaw *[3]int `json:"jigsaw,omitempty" jsonschema:"only check the parent's jigsaw at this position [x, y, z]"`
}

// checkFit reports, for each jigsaw on the parent that could take the child,
// where the child would go in each rotation and whether it fits.
func (s *state) checkFit(in fitIn) (string, error) {
	parent, err := s.get(in.Parent)
	if err != nil {
		return "", err
	}
	child, err := s.get(in.Child)
	if err != nil {
		return "", err
	}
	pjs := jigsawsOf(parent)
	cjs := jigsawsOf(child)
	if len(cjs) == 0 {
		return "", fmt.Errorf("%q has no jigsaw blocks, so it can't be attached to anything", in.Child)
	}
	pbox := sizeBox(parent.Size)
	cbox := sizeBox(child.Size)
	var b strings.Builder
	checked := 0
	for _, pj := range pjs {
		if in.Jigsaw != nil && pj.pos != vec(*in.Jigsaw) {
			continue
		}
		if pj.pool == emptyID && in.Jigsaw == nil {
			continue
		}
		checked++
		t := pj.targetPos()
		inside := pbox.contains(t)
		fmt.Fprintf(&b, "Parent %s (target %s, pool %s):\n", pj, pj.target, pj.pool)
		if inside {
			fmt.Fprintf(&b, "  It connects inside the parent (at %v), so the child must fit inside the parent's bounds, %s.\n", t, pbox)
		} else {
			fmt.Fprintf(&b, "  It connects outside the parent (at %v), so the child must not overlap the parent or any other piece.\n", t)
		}
		found := false
		var reasons []string
		for _, cj := range cjs {
			for r := rotation(0); r < 4; r++ {
				rj := cj.rotated(r)
				if !canAttach(pj, rj) {
					if r == 0 {
						reasons = append(reasons, fmt.Sprintf("child %s can't attach: %s", cj, whyNot(pj, rj)))
					}
					continue
				}
				found = true
				off := t.sub(rj.pos)
				cb := r.box(cbox).moved(off)
				fmt.Fprintf(&b, "  child jigsaw at %v, rotation %s: child occupies %s in parent coordinates: ", cj.pos, r, cb)
				switch {
				case inside && cb.inside(pbox):
					b.WriteString("fits.\n")
				case inside:
					fmt.Fprintf(&b, "DOES NOT FIT, it sticks out %s.\n", overhang(cb, pbox))
				case cb.intersects(pbox):
					b.WriteString("OVERLAPS the parent, so it can't be placed.\n")
				default:
					b.WriteString("clear of the parent.\n")
				}
			}
		}
		if !found {
			b.WriteString("  No jigsaw on the child can attach here in any rotation:\n")
			for _, r := range reasons {
				b.WriteString("    " + r + "\n")
			}
		}
	}
	if checked == 0 {
		if in.Jigsaw != nil {
			return "", fmt.Errorf("%q has no jigsaw at %v", in.Parent, *in.Jigsaw)
		}
		return "", fmt.Errorf("%q has no jigsaws with a pool", in.Parent)
	}
	b.WriteString("\nThe game tries rotations in random order and uses the first that fits. Up and down jigsaws with a rollable joint accept every rotation; horizontal ones only the rotation that makes the jigsaws face each other.\n")
	return b.String(), nil
}

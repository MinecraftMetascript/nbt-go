package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/MinecraftMetascript/nbt-go"
	"github.com/MinecraftMetascript/nbt-go/structure"
)

// mirror flips a structure along one axis. mirrorX swaps east and west (the
// game's FRONT_BACK), mirrorZ swaps north and south (LEFT_RIGHT).
type mirror int

const (
	mirrorNone mirror = iota
	mirrorX
	mirrorZ
)

func parseMirror(s string) (mirror, error) {
	switch s {
	case "", "none":
		return mirrorNone, nil
	case "x", "front_back":
		return mirrorX, nil
	case "z", "left_right":
		return mirrorZ, nil
	}
	return 0, fmt.Errorf("unknown mirror %q; use none, x (swap east and west) or z (swap north and south)", s)
}

func (m mirror) String() string { return [...]string{"none", "x", "z"}[m] }

func (m mirror) apply(p vec) vec {
	switch m {
	case mirrorX:
		p[0] = -p[0]
	case mirrorZ:
		p[2] = -p[2]
	}
	return p
}

func (m mirror) applyF(p [3]float64) [3]float64 {
	switch m {
	case mirrorX:
		p[0] = -p[0]
	case mirrorZ:
		p[2] = -p[2]
	}
	return p
}

func (m mirror) dir(d dir) dir {
	switch {
	case m == mirrorX && (d == east || d == west):
		return d.opposite()
	case m == mirrorZ && (d == north || d == south):
		return d.opposite()
	}
	return d
}

// mirrorState mirrors the direction-dependent properties of a block state.
func mirrorState(bs structure.BlockState, m mirror) structure.BlockState {
	if m == mirrorNone || len(bs.Properties) == 0 {
		return bs
	}
	p := make(map[string]string, len(bs.Properties))
	for k, v := range bs.Properties {
		p[k] = v
	}
	md := func(s string) string {
		if d, ok := parseDir(s); ok {
			return m.dir(d).String()
		}
		return s
	}
	if v, ok := p["facing"]; ok {
		p["facing"] = md(v)
	}
	if v, ok := p["orientation"]; ok {
		if a, b, ok := strings.Cut(v, "_"); ok {
			p["orientation"] = md(a) + "_" + md(b)
		}
	}
	if v, ok := p["rotation"]; ok {
		var n int
		if _, err := fmt.Sscan(v, &n); err == nil {
			if m == mirrorX {
				n = (16 - n) % 16
			} else {
				n = (24 - n) % 16
			}
			p["rotation"] = fmt.Sprint(n)
		}
	}
	for _, d := range []dir{north, east, south, west} {
		delete(p, d.String())
	}
	for _, d := range []dir{north, east, south, west} {
		if v, ok := bs.Properties[d.String()]; ok {
			p[m.dir(d).String()] = v
		}
	}
	swapLR := func(s string) string {
		switch {
		case strings.HasSuffix(s, "left"):
			return strings.TrimSuffix(s, "left") + "right"
		case strings.HasSuffix(s, "right"):
			return strings.TrimSuffix(s, "right") + "left"
		}
		return s
	}
	if v, ok := p["shape"]; ok {
		if strings.HasSuffix(bs.Name, "stairs") {
			p["shape"] = swapLR(v)
		} else {
			p["shape"] = mirrorRailShape(v, m)
		}
	}
	if v, ok := p["hinge"]; ok {
		p["hinge"] = swapLR(v)
	}
	if v, ok := p["type"]; ok && strings.HasSuffix(bs.Name, "chest") {
		p["type"] = swapLR(v)
	}
	return structure.BlockState{Name: bs.Name, Properties: p}
}

func mirrorRailShape(s string, m mirror) string {
	if strings.HasPrefix(s, "ascending_") {
		if d, ok := parseDir(strings.TrimPrefix(s, "ascending_")); ok {
			return "ascending_" + m.dir(d).String()
		}
		return s
	}
	a, b, ok := strings.Cut(s, "_")
	da, oka := parseDir(a)
	db, okb := parseDir(b)
	if !ok || !oka || !okb || da.opposite() == db {
		return s
	}
	return m.dir(da).String() + "_" + m.dir(db).String()
}

// transform is a mirror followed by a rotation, the order the game uses.
type transform struct {
	rot rotation
	mir mirror
}

func (t transform) pos(p vec) vec { return t.rot.apply(t.mir.apply(p)) }

// posF transforms an exact position so that a point at the center of a block
// ends up at the center of the transformed block.
func (t transform) posF(p [3]float64) [3]float64 {
	for a := range p {
		p[a] -= 0.5
	}
	p = t.rot.applyF(t.mir.applyF(p))
	for a := range p {
		p[a] += 0.5
	}
	return p
}

func (t transform) state(bs structure.BlockState) structure.BlockState {
	return rotateState(mirrorState(bs, t.mir), t.rot)
}

func (t transform) box(b box) box {
	p, q := t.pos(b.min), t.pos(b.max)
	var out box
	for a := 0; a < 3; a++ {
		out.min[a], out.max[a] = min(p[a], q[a]), max(p[a], q[a])
	}
	return out
}

// entity transforms an entity's data for a structure moved by t then
// shifted by off, updating Pos and yaw.
func (t transform) entity(e structure.Entity, off vec) ([3]float64, nbt.Compound) {
	pos := t.posF(e.Pos)
	for a := 0; a < 3; a++ {
		pos[a] += float64(off[a])
	}
	data := cloneCompound(e.NBT)
	data["Pos"] = nbt.List{nbt.Double(pos[0]), nbt.Double(pos[1]), nbt.Double(pos[2])}
	if rl, ok := data["Rotation"].(nbt.List); ok && len(rl) == 2 {
		if yaw, ok := rl[0].(nbt.Float); ok {
			y := float32(yaw)
			switch t.mir {
			case mirrorX:
				y = -y
			case mirrorZ:
				y = 180 - y
			}
			rl[0] = nbt.Float(y + 90*float32(t.rot&3))
		}
	}
	return pos, data
}

type pasteOpts struct {
	skipAir      bool
	keepExisting bool
}

// paste copies src into dst transformed by t, with the transformed
// structure's lowest corner at `at`. It returns the number of blocks placed.
func paste(dst, src *structure.Structure, t transform, at vec, o pasteOpts) (int, error) {
	if len(src.Blocks) == 0 && len(src.Entities) == 0 {
		return 0, nil
	}
	tb := t.box(sizeBox(src.Size))
	off := at.sub(tb.min)
	for a := 0; a < 3; a++ {
		if at[a] < 0 {
			return 0, fmt.Errorf("position %v is out of range; coordinates must be non-negative", at)
		}
	}
	pal := palette(src)
	var existing *grid
	if o.keepExisting {
		existing = newGrid(dst)
	}
	n := 0
	for _, b := range src.Blocks {
		if b.State < 0 || int(b.State) >= len(pal) {
			continue
		}
		bs := pal[b.State]
		if o.skipAir && isAir(bs.Name) {
			continue
		}
		p := t.pos(toVec(b.Pos)).add(off)
		if existing != nil {
			if _, _, ok := existing.get(p); ok {
				continue
			}
		}
		if err := dst.SetBlock(p[0], p[1], p[2], t.state(bs), cloneCompound(b.NBT)); err != nil {
			return n, err
		}
		n++
	}
	for _, e := range src.Entities {
		pos, data := t.entity(e, off)
		if err := dst.AddEntity(pos, data); err != nil {
			return n, err
		}
	}
	for a := 0; a < 3; a++ {
		dst.Size[a] = max(dst.Size[a], int32(at[a]+tb.size()[a]))
	}
	return n, nil
}

type transformIn struct {
	Name     string `json:"name"`
	Rotation string `json:"rotation,omitempty" enum:"none,clockwise_90,180,counterclockwise_90" jsonschema:"clockwise rotation seen from above: none, clockwise_90, 180 or counterclockwise_90"`
	Mirror   string `json:"mirror,omitempty" enum:"none,x,z" jsonschema:"none, x (swap east and west) or z (swap north and south); applied before the rotation"`
}

func (s *state) transformStructure(in transformIn) (string, error) {
	st, err := s.get(in.Name)
	if err != nil {
		return "", err
	}
	t, err := parseTransform(in.Rotation, in.Mirror)
	if err != nil {
		return "", err
	}
	if len(st.Palettes) > 0 {
		return "", errors.New("structures with several palettes can't be transformed")
	}
	out := structure.New(st.DataVersion)
	if _, err := paste(out, st, t, vec{}, pasteOpts{}); err != nil {
		return "", err
	}
	s.structures[in.Name] = out
	return fmt.Sprintf("Transformed %q (rotation %s, mirror %s). Size is now %v; block states such as facing, axis and connections were turned to match.", in.Name, t.rot, t.mir, out.Size), nil
}

func parseTransform(rot, mir string) (transform, error) {
	r, err := parseRotation(rot)
	if err != nil {
		return transform{}, err
	}
	m, err := parseMirror(mir)
	if err != nil {
		return transform{}, err
	}
	return transform{r, m}, nil
}

type pasteIn struct {
	Source       string `json:"source" jsonschema:"the open structure to copy from"`
	Target       string `json:"target" jsonschema:"the open structure to copy into (it may be new and empty)"`
	At           [3]int `json:"at" jsonschema:"where the lowest corner of the (rotated) source goes in the target [x, y, z]"`
	Rotation     string `json:"rotation,omitempty" enum:"none,clockwise_90,180,counterclockwise_90" jsonschema:"rotate the source first: none, clockwise_90, 180 or counterclockwise_90"`
	Mirror       string `json:"mirror,omitempty" enum:"none,x,z" jsonschema:"mirror the source first: none, x or z"`
	SkipAir      bool   `json:"skip_air,omitempty" jsonschema:"don't copy the source's air blocks, so they don't overwrite the target"`
	KeepExisting bool   `json:"keep_existing,omitempty" jsonschema:"only fill positions where the target has no block yet"`
}

func (s *state) pasteStructure(in pasteIn) (string, error) {
	src, err := s.get(in.Source)
	if err != nil {
		return "", err
	}
	dst, err := s.get(in.Target)
	if err != nil {
		return "", err
	}
	if src == dst {
		return "", errors.New("source and target must be different structures")
	}
	if len(dst.Palettes) > 0 {
		return "", errors.New("can't paste into a structure with several palettes")
	}
	t, err := parseTransform(in.Rotation, in.Mirror)
	if err != nil {
		return "", err
	}
	n, err := paste(dst, src, t, vec(in.At), pasteOpts{skipAir: in.SkipAir, keepExisting: in.KeepExisting})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Pasted %d blocks and %d entities from %q into %q at %v (rotation %s, mirror %s). Size is now %v.",
		n, len(src.Entities), in.Source, in.Target, in.At, t.rot, t.mir, dst.Size), nil
}

type cropIn struct {
	boxIn
	Into string `json:"into,omitempty" jsonschema:"name for the cropped copy; omit to crop the structure in place"`
}

func (s *state) crop(in cropIn) (string, error) {
	st, err := s.get(in.Name)
	if err != nil {
		return "", err
	}
	lo, hi, err := in.bounds()
	if err != nil {
		return "", err
	}
	b := box{vec(lo), vec(hi)}
	out := structure.New(st.DataVersion)
	pal := palette(st)
	n := 0
	for _, bl := range st.Blocks {
		p := toVec(bl.Pos)
		if !b.contains(p) || bl.State < 0 || int(bl.State) >= len(pal) {
			continue
		}
		p = p.sub(b.min)
		if err := out.SetBlock(p[0], p[1], p[2], pal[bl.State], cloneCompound(bl.NBT)); err != nil {
			return "", err
		}
		n++
	}
	ents := 0
	for _, e := range st.Entities {
		if !b.contains(toVec(e.BlockPos)) {
			continue
		}
		pos, data := transform{}.entity(e, vec{}.sub(b.min))
		if err := out.AddEntity(pos, data); err != nil {
			return "", err
		}
		ents++
	}
	sz := b.size()
	out.Size = [3]int32{int32(sz[0]), int32(sz[1]), int32(sz[2])}
	name := in.Into
	if name == "" {
		name = in.Name
	}
	s.structures[name] = out
	return fmt.Sprintf("Cropped %q to %s as %q: %d blocks and %d entities, size %v.", in.Name, b, name, n, ents, out.Size), nil
}

type setSizeIn struct {
	Name string `json:"name"`
	Size [3]int `json:"size" jsonschema:"new size [x, y, z]"`
	Drop bool   `json:"drop,omitempty" jsonschema:"when shrinking, remove blocks and entities outside the new size instead of refusing"`
}

func (s *state) setSize(in setSizeIn) (string, error) {
	st, err := s.get(in.Name)
	if err != nil {
		return "", err
	}
	for _, v := range in.Size {
		if v < 1 || v > 1<<16 {
			return "", fmt.Errorf("size %v must be between 1 and 65536 on each axis", in.Size)
		}
	}
	b := box{vec{}, vec{in.Size[0] - 1, in.Size[1] - 1, in.Size[2] - 1}}
	var outside []vec
	for _, bl := range st.Blocks {
		if p := toVec(bl.Pos); !b.contains(p) {
			outside = append(outside, p)
		}
	}
	entsOut := 0
	for _, e := range st.Entities {
		if !b.contains(toVec(e.BlockPos)) {
			entsOut++
		}
	}
	if (len(outside) > 0 || entsOut > 0) && !in.Drop {
		return "", fmt.Errorf("%d blocks and %d entities lie outside size %v (the first block at %v); pass drop to remove them", len(outside), entsOut, in.Size, firstOr(outside))
	}
	for _, p := range outside {
		st.RemoveBlock(p[0], p[1], p[2])
	}
	if entsOut > 0 {
		kept := st.Entities[:0]
		for _, e := range st.Entities {
			if b.contains(toVec(e.BlockPos)) {
				kept = append(kept, e)
			}
		}
		st.Entities = kept
	}
	st.Size = [3]int32{int32(in.Size[0]), int32(in.Size[1]), int32(in.Size[2])}
	msg := fmt.Sprintf("Size of %q is now %v.", in.Name, st.Size)
	if len(outside) > 0 || entsOut > 0 {
		msg += fmt.Sprintf(" Removed %d blocks and %d entities outside it.", len(outside), entsOut)
	}
	return msg, nil
}

func firstOr(vs []vec) any {
	if len(vs) == 0 {
		return "none"
	}
	return vs[0]
}

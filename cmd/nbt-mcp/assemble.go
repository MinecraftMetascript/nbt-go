package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MinecraftMetascript/nbt-go/structure"
)

// maxPieces bounds a jigsaw assembly, as a safety net for pools that refer
// to themselves.
const maxPieces = 1024

type assembleIn struct {
	Into          string         `json:"into" jsonschema:"name for the assembled structure, which is opened like any other"`
	Start         string         `json:"start,omitempty" jsonschema:"the start piece: an open structure's name, or a structure location such as mypack:village/center found in the datapack. Give this or start_pool"`
	StartPool     string         `json:"start_pool,omitempty" jsonschema:"template pool to pick the start piece from, as the structure's start_pool does"`
	Datapack      string         `json:"datapack,omitempty" jsonschema:"datapack folder (the one containing data/) to read template pools from (data/<ns>/worldgen/template_pool/<path>.json) and structures from (data/<ns>/structure/<path>.nbt)"`
	Pools         map[string]any `json:"pools,omitempty" jsonschema:"template pools given inline, keyed by id, in the same JSON format as template_pool files, e.g. {\"mypack:statues\": {\"fallback\": \"minecraft:empty\", \"elements\": [{\"weight\": 1, \"element\": {\"element_type\": \"minecraft:single_pool_element\", \"location\": \"mypack:statue\", \"projection\": \"rigid\", \"processors\": \"minecraft:empty\"}}]}}. These override the datapack's. Element locations can name open structures"`
	MaxDepth      int            `json:"max_depth,omitempty" jsonschema:"the structure's size: how many pieces deep to go from the start piece (default 7, at most 20)"`
	MaxDistance   int            `json:"max_distance,omitempty" jsonschema:"max_distance_from_center: how far pieces may reach from the start piece's center (default 80, at most 128)"`
	StartRotation string         `json:"start_rotation,omitempty" jsonschema:"rotation of the start piece: none (default), clockwise_90, 180, counterclockwise_90 or random"`
	Seed          *int64         `json:"seed,omitempty" jsonschema:"random seed; the same seed and inputs give the same layout. A random one is used and reported if omitted"`
	KeepJigsaws   bool           `json:"keep_jigsaws,omitempty" jsonschema:"leave jigsaw blocks in the result instead of replacing them with their final_state"`
}

type poolElement struct {
	kind       string
	location   string
	projection string
	weight     int
}

type templatePool struct {
	id       string
	fallback string
	elements []poolElement
}

type template struct {
	location string
	st       *structure.Structure
	bounds   box
	jigsaws  []jigsaw
}

type piece struct {
	tpl    *template
	rot    rotation
	offset vec
	box    box
	depth  int
	free   *space // the space this piece was placed into
	parent int    // index of the parent piece, -1 for the start
	via    vec    // the parent's jigsaw that placed it
}

// space is the room left for new pieces: a container box minus the boxes of
// pieces already placed in it. It stands in for the game's VoxelShape.
type space struct {
	container box
	used      []box
	usedBy    []int
}

func (s *space) blocker(b box) (int, bool) {
	for i, u := range s.used {
		if u.intersects(b) {
			return s.usedBy[i], true
		}
	}
	return 0, false
}

type assembler struct {
	s        *state
	in       assembleIn
	rng      *rand.Rand
	datapack string
	inline   map[string]templatePool
	pools    map[string]*templatePool
	tpls     map[string]*template
	pieces   []piece
	issues   []issue
	notes    map[string]bool
	shift    vec // where the start piece's origin ends up in the result
}

func (s *state) assemble(in assembleIn) (string, error) {
	if in.Into == "" {
		return "", errors.New("into is required")
	}
	if (in.Start == "") == (in.StartPool == "") {
		return "", errors.New("give exactly one of start and start_pool")
	}
	if in.MaxDepth == 0 {
		in.MaxDepth = 7
	}
	if in.MaxDepth < 0 || in.MaxDepth > 20 {
		return "", fmt.Errorf("max_depth must be between 0 and 20, got %d", in.MaxDepth)
	}
	if in.MaxDistance == 0 {
		in.MaxDistance = 80
	}
	if in.MaxDistance < 1 || in.MaxDistance > 128 {
		return "", fmt.Errorf("max_distance must be between 1 and 128, got %d", in.MaxDistance)
	}
	seed := time.Now().UnixNano()
	if in.Seed != nil {
		seed = *in.Seed
	}
	a := &assembler{
		s: s, in: in, rng: rand.New(rand.NewSource(seed)),
		inline: map[string]templatePool{}, pools: map[string]*templatePool{}, tpls: map[string]*template{},
		notes: map[string]bool{},
	}
	if in.Datapack != "" {
		dp, err := s.resolve(in.Datapack)
		if err != nil {
			return "", err
		}
		if fi, err := os.Stat(filepath.Join(dp, "data")); err != nil || !fi.IsDir() {
			return "", fmt.Errorf("%s has no data folder; datapack should be the pack's root folder", dp)
		}
		a.datapack = dp
	}
	for id, raw := range in.Pools {
		p, err := parsePool(namespaced(id), raw)
		if err != nil {
			return "", fmt.Errorf("pools[%q]: %w", id, err)
		}
		a.inline[p.id] = *p
	}

	var startTpl *template
	if in.Start != "" {
		t, err := a.template(in.Start)
		if err != nil {
			return "", fmt.Errorf("start: %w", err)
		}
		startTpl = t
	} else {
		p, err := a.pool(namespaced(in.StartPool))
		if err != nil {
			return "", fmt.Errorf("start_pool: %w", err)
		}
		el, ok := a.pickWeighted(p.elements)
		if !ok {
			return "", fmt.Errorf("start_pool %s has no single_pool_element entries", p.id)
		}
		t, err := a.template(el.location)
		if err != nil {
			return "", fmt.Errorf("start_pool %s: %w", p.id, err)
		}
		startTpl = t
	}
	var rot rotation
	if in.StartRotation == "random" {
		rot = rotation(a.rng.Intn(4))
	} else {
		r, err := parseRotation(in.StartRotation)
		if err != nil {
			return "", err
		}
		rot = r
	}

	startBox := rot.box(startTpl.bounds)
	center := vec{(startBox.min[0] + startBox.max[0]) / 2, startBox.min[1], (startBox.min[2] + startBox.max[2]) / 2}
	d := in.MaxDistance
	global := &space{container: box{center.sub(vec{d, d, d}), center.add(vec{d, d, d})}}
	global.used = append(global.used, startBox)
	global.usedBy = append(global.usedBy, 0)
	a.pieces = append(a.pieces, piece{tpl: startTpl, rot: rot, box: startBox, free: global, parent: -1})
	a.run()

	out, err := a.merge()
	if err != nil {
		return "", err
	}
	s.structures[in.Into] = out
	return a.report(seed, out), nil
}

// run expands pieces in the game's order: a queue ordered by the placement
// priority of the jigsaw that placed each piece, first in first out within a
// priority.
func (a *assembler) run() {
	type queued struct{ piece, priority, seq int }
	queue := []queued{{0, 0, 0}}
	seq := 1
	for len(queue) > 0 {
		best := 0
		for i, q := range queue {
			if q.priority > queue[best].priority || (q.priority == queue[best].priority && q.seq < queue[best].seq) {
				best = i
			}
		}
		q := queue[best]
		queue = append(queue[:best], queue[best+1:]...)
		for _, child := range a.expand(q.piece) {
			if a.pieces[child.index].depth <= a.in.MaxDepth {
				queue = append(queue, queued{child.index, child.priority, seq})
				seq++
			}
		}
		if len(a.pieces) >= maxPieces {
			a.issues = append(a.issues, issue{msg: fmt.Sprintf("stopped after %d pieces; check for pools that keep extending themselves", maxPieces)})
			return
		}
	}
}

type placed struct{ index, priority int }

// issue is a problem found while assembling, optionally about a jigsaw on a
// piece, whose position is reported in the result's coordinates.
type issue struct {
	piece  int
	jigsaw *jigsaw
	msg    string
}

// expand tries to attach children to every jigsaw of piece i, like the
// game's JigsawPlacement.Placer.tryPlacingChildren.
func (a *assembler) expand(i int) []placed {
	p := a.pieces[i]
	var children []placed
	js := make([]jigsaw, len(p.tpl.jigsaws))
	for k, j := range p.tpl.jigsaws {
		j = j.rotated(p.rot)
		j.pos = j.pos.add(p.offset)
		js[k] = j
	}
	a.rng.Shuffle(len(js), func(x, y int) { js[x], js[y] = js[y], js[x] })
	sort.SliceStable(js, func(x, y int) bool { return js[x].selection > js[y].selection })

	var interior *space
	for _, j := range js {
		if j.pool == emptyID {
			continue
		}
		j := j
		fail := func(format string, args ...any) {
			a.issues = append(a.issues, issue{piece: i, jigsaw: &j, msg: fmt.Sprintf(format, args...)})
		}
		pool, err := a.pool(j.pool)
		if err != nil {
			fail("%v", err)
			continue
		}
		var candidates []poolElement
		if p.depth != a.in.MaxDepth {
			candidates = append(candidates, a.shuffled(pool.elements)...)
		}
		if pool.fallback != "" && pool.fallback != emptyID {
			fb, err := a.pool(pool.fallback)
			if err != nil {
				fail("fallback: %v", err)
			} else {
				candidates = append(candidates, a.shuffled(fb.elements)...)
			}
		}
		target := j.targetPos()
		inside := p.box.contains(target)
		free := p.free
		if inside {
			if interior == nil {
				interior = &space{container: p.box}
			}
			free = interior
		}

		attachable, tried := 0, 0
		var failures []string
		done := false
		for _, el := range candidates {
			if el.kind == "minecraft:empty_pool_element" {
				break
			}
			if el.location == "" {
				a.note(fmt.Sprintf("%s elements aren't supported and were skipped", el.kind))
				continue
			}
			if el.projection == "terrain_matching" {
				a.note("terrain_matching pieces are placed as if rigid; in the game they follow the terrain height")
			}
			tpl, err := a.template(el.location)
			if err != nil {
				fail("%v", err)
				continue
			}
			tried++
			rots := []rotation{0, 1, 2, 3}
			a.rng.Shuffle(4, func(x, y int) { rots[x], rots[y] = rots[y], rots[x] })
			for _, r := range rots {
				cjs := make([]jigsaw, len(tpl.jigsaws))
				for k, cj := range tpl.jigsaws {
					cjs[k] = cj.rotated(r)
				}
				a.rng.Shuffle(len(cjs), func(x, y int) { cjs[x], cjs[y] = cjs[y], cjs[x] })
				sort.SliceStable(cjs, func(x, y int) bool { return cjs[x].selection > cjs[y].selection })
				for _, cj := range cjs {
					if !canAttach(j, cj) {
						continue
					}
					attachable++
					off := target.sub(cj.pos)
					cb := r.box(tpl.bounds).moved(off)
					if !cb.inside(free.container) {
						if inside {
							failures = append(failures, fmt.Sprintf("%s rotated %s doesn't fit inside the parent (it connects inside the parent's bounds): it sticks out %s", tpl.location, r, overhang(cb, free.container)))
						} else {
							failures = append(failures, fmt.Sprintf("%s rotated %s would reach past max_distance (%s)", tpl.location, r, overhang(cb, free.container)))
						}
						continue
					}
					if by, ok := free.blocker(cb); ok {
						failures = append(failures, fmt.Sprintf("%s rotated %s would overlap piece #%d (%s)", tpl.location, r, by, a.pieces[by].tpl.location))
						continue
					}
					idx := len(a.pieces)
					free.used = append(free.used, cb)
					free.usedBy = append(free.usedBy, idx)
					a.pieces = append(a.pieces, piece{tpl: tpl, rot: r, offset: off, box: cb, depth: p.depth + 1, free: free, parent: i, via: j.pos})
					children = append(children, placed{idx, j.placement})
					done = true
					break
				}
				if done {
					break
				}
			}
			if done {
				break
			}
		}
		if done {
			continue
		}
		switch {
		case len(candidates) == 0:
			if p.depth == a.in.MaxDepth {
				fail("nothing placed, max_depth reached, so only the fallback pool %s was tried and it has nothing to place", pool.fallback)
			} else {
				fail("nothing placed, the pool is empty")
			}
		case tried > 0 && attachable == 0:
			fail("nothing placed, no piece in the pool has a jigsaw named %s that can face %s", j.target, j.front.opposite())
		case attachable > 0:
			msg := "nothing placed; every option was blocked:"
			for k, f := range failures {
				if k == 4 {
					msg += fmt.Sprintf("\n      ... and %d more", len(failures)-4)
					break
				}
				msg += "\n      " + f
			}
			fail("%s", msg)
		}
	}
	return children
}

func (a *assembler) note(s string) { a.notes[s] = true }

// shuffled lists each element once, in the order of a weighted shuffle: the
// order in which the game would first reach each element.
func (a *assembler) shuffled(els []poolElement) []poolElement {
	var expanded []int
	for i, e := range els {
		for k := 0; k < e.weight; k++ {
			expanded = append(expanded, i)
		}
	}
	a.rng.Shuffle(len(expanded), func(x, y int) { expanded[x], expanded[y] = expanded[y], expanded[x] })
	seen := map[int]bool{}
	var out []poolElement
	for _, i := range expanded {
		if !seen[i] {
			seen[i] = true
			out = append(out, els[i])
		}
	}
	return out
}

func (a *assembler) pickWeighted(els []poolElement) (poolElement, bool) {
	total := 0
	for _, e := range els {
		if e.location != "" {
			total += e.weight
		}
	}
	if total == 0 {
		return poolElement{}, false
	}
	n := a.rng.Intn(total)
	for _, e := range els {
		if e.location == "" {
			continue
		}
		if n < e.weight {
			return e, true
		}
		n -= e.weight
	}
	return poolElement{}, false
}

type poolJSON struct {
	Fallback string `json:"fallback"`
	Elements []struct {
		Weight  int `json:"weight"`
		Element struct {
			ElementType string `json:"element_type"`
			Location    any    `json:"location"`
			Projection  string `json:"projection"`
		} `json:"element"`
	} `json:"elements"`
}

func parsePool(id string, raw any) (*templatePool, error) {
	var data []byte
	switch v := raw.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		var err error
		if data, err = json.Marshal(v); err != nil {
			return nil, err
		}
	}
	var pj poolJSON
	if err := json.Unmarshal(data, &pj); err != nil {
		return nil, fmt.Errorf("not a template pool: %w", err)
	}
	p := &templatePool{id: id, fallback: namespaced(pj.Fallback)}
	for i, e := range pj.Elements {
		el := poolElement{kind: namespaced(e.Element.ElementType), projection: e.Element.Projection, weight: e.Weight}
		if el.weight <= 0 {
			el.weight = 1
		}
		switch el.kind {
		case "minecraft:single_pool_element", "minecraft:legacy_single_pool_element":
			loc, ok := e.Element.Location.(string)
			if !ok || loc == "" {
				return nil, fmt.Errorf("elements[%d]: location must be a structure id", i)
			}
			el.location = namespaced(loc)
		case "minecraft:empty_pool_element", "minecraft:list_pool_element", "minecraft:feature_pool_element":
		default:
			return nil, fmt.Errorf("elements[%d]: unknown element_type %q", i, e.Element.ElementType)
		}
		p.elements = append(p.elements, el)
	}
	return p, nil
}

func (a *assembler) pool(id string) (*templatePool, error) {
	if p, ok := a.pools[id]; ok {
		return p, nil
	}
	var p *templatePool
	if ip, ok := a.inline[id]; ok {
		p = &ip
	} else if id == emptyID {
		p = &templatePool{id: id}
	} else if a.datapack != "" {
		ns, path, _ := strings.Cut(id, ":")
		file := filepath.Join(a.datapack, "data", ns, "worldgen", "template_pool", filepath.FromSlash(path)+".json")
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("template pool %s not found (looked for %s)", id, file)
		}
		if p, err = parsePool(id, data); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
	} else {
		return nil, fmt.Errorf("template pool %s not found; pass it in pools or give a datapack", id)
	}
	a.pools[id] = p
	return p, nil
}

// template finds a structure by open name or by location in the datapack.
func (a *assembler) template(loc string) (*template, error) {
	if t, ok := a.tpls[loc]; ok {
		return t, nil
	}
	st := a.s.structures[loc]
	if st == nil {
		if ns, path, ok := strings.Cut(loc, ":"); ok {
			st = a.s.structures[path]
			if st == nil && a.datapack != "" {
				for _, dir := range []string{"structure", "structures"} {
					file := filepath.Join(a.datapack, "data", ns, dir, filepath.FromSlash(path)+".nbt")
					f, err := os.Open(file)
					if err != nil {
						continue
					}
					st, err = structure.Read(f)
					f.Close()
					if err != nil {
						return nil, fmt.Errorf("reading %s: %w", file, err)
					}
					break
				}
			}
		}
	}
	if st == nil {
		return nil, fmt.Errorf("structure %s is neither open nor in the datapack", loc)
	}
	if st.Size[0] <= 0 || st.Size[1] <= 0 || st.Size[2] <= 0 {
		return nil, fmt.Errorf("structure %s is empty", loc)
	}
	t := &template{location: loc, st: st, bounds: sizeBox(st.Size), jigsaws: jigsawsOf(st)}
	a.tpls[loc] = t
	return t, nil
}

// merge pastes every piece into one structure, the way they would be placed
// in the world: later pieces overwrite earlier ones, and positions a piece
// has no block for are left alone.
func (a *assembler) merge() (*structure.Structure, error) {
	lo := a.pieces[0].box.min
	total := 0
	for _, p := range a.pieces {
		for k := 0; k < 3; k++ {
			lo[k] = min(lo[k], p.box.min[k])
		}
		total += len(p.tpl.st.Blocks)
	}
	if total > 4*maxVolume {
		return nil, fmt.Errorf("the layout has %d blocks, too many to assemble", total)
	}
	shift := vec{-lo[0], -lo[1], -lo[2]}
	out := structure.New(a.pieces[0].tpl.st.DataVersion)
	for _, p := range a.pieces {
		st := p.tpl.st
		pal := palette(st)
		for _, b := range st.Blocks {
			if b.State < 0 || int(b.State) >= len(pal) {
				continue
			}
			bs := pal[b.State]
			data := b.NBT
			if bs.Name == jigsawBlock && !a.in.KeepJigsaws {
				j := parseJigsaw(toVec(b.Pos), bs, b.NBT)
				fs, err := structure.ParseBlockState(j.finalState)
				if err != nil {
					fs = structure.BlockState{Name: "minecraft:air"}
				}
				bs, data = fs, nil
			}
			pos := p.rot.apply(toVec(b.Pos)).add(p.offset).add(shift)
			if err := out.SetBlock(pos[0], pos[1], pos[2], rotateState(bs, p.rot), cloneCompound(data)); err != nil {
				return nil, err
			}
		}
		for _, e := range st.Entities {
			pos, data := transform{rot: p.rot}.entity(e, p.offset.add(shift))
			if err := out.AddEntity(pos, data); err != nil {
				return nil, err
			}
		}
	}
	// Grow to cover every piece's box, even where a piece's edges are empty.
	for _, p := range a.pieces {
		for k := 0; k < 3; k++ {
			out.Size[k] = max(out.Size[k], int32(p.box.max[k]+shift[k]+1))
		}
	}
	a.shift = shift
	return out, nil
}

func (a *assembler) report(seed int64, out *structure.Structure) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Assembled %d pieces into %q (size %d x %d x %d), seed %d.\n", len(a.pieces), a.in.Into, out.Size[0], out.Size[1], out.Size[2], seed)
	fmt.Fprintf(&b, "Coordinates below are in the assembled structure; the start piece's origin is at %v.\n\nPieces:\n", a.shift)
	for i, p := range a.pieces {
		bx := p.box.moved(a.shift)
		fmt.Fprintf(&b, "  #%d %s, rotation %s, depth %d, %s", i, p.tpl.location, p.rot, p.depth, bx)
		if p.parent >= 0 {
			fmt.Fprintf(&b, ", attached to #%d by the jigsaw at %v", p.parent, p.via.add(a.shift))
		}
		b.WriteByte('\n')
	}
	if len(a.issues) > 0 {
		b.WriteString("\nProblems:\n")
		for _, is := range a.issues {
			b.WriteString("  ")
			if j := is.jigsaw; j != nil {
				fmt.Fprintf(&b, "piece #%d (%s) jigsaw at %v facing %s, pool %s, target %s: ",
					is.piece, a.pieces[is.piece].tpl.location, j.pos.add(a.shift), j.front, j.pool, j.target)
			}
			b.WriteString(is.msg + "\n")
		}
	}
	if len(a.notes) > 0 {
		b.WriteString("\nNotes:\n")
		keys := make([]string, 0, len(a.notes))
		for k := range a.notes {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString("  " + k + "\n")
		}
	}
	b.WriteString("\nThis follows the game's jigsaw rules for rigid pieces but ignores terrain, processors and other structures.\n")
	return b.String()
}

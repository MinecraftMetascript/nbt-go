package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/MinecraftMetascript/nbt-go/structure"
)

// lightLevel is the block light a block gives off, by name-based rules.
func lightLevel(bs structure.BlockState) int {
	n := strings.TrimPrefix(bs.Name, "minecraft:")
	p := bs.Properties
	lit := p["lit"] != "false"
	switch {
	case n == "light":
		if v, err := strconv.Atoi(p["level"]); err == nil {
			return v
		}
		return 15
	case n == "soul_torch", n == "soul_wall_torch", n == "soul_lantern", n == "soul_fire":
		return 10
	case n == "soul_campfire":
		if lit {
			return 10
		}
		return 0
	case n == "redstone_torch", n == "redstone_wall_torch":
		if lit {
			return 7
		}
		return 0
	case strings.HasSuffix(n, "torch"), n == "end_rod":
		return 14
	case strings.HasSuffix(n, "lantern"), n == "glowstone", n == "shroomlight", n == "beacon", n == "conduit",
		strings.HasSuffix(n, "froglight"), n == "fire", n == "lava", n == "respawn_anchor" && p["charges"] == "4":
		return 15
	case n == "campfire", n == "redstone_lamp", strings.HasSuffix(n, "copper_bulb"):
		if p["lit"] == "true" || (n == "campfire" && lit) {
			return 15
		}
		return 0
	case n == "furnace", n == "blast_furnace", n == "smoker":
		if p["lit"] == "true" {
			return 13
		}
		return 0
	case strings.HasSuffix(n, "candle"):
		if p["lit"] != "true" {
			return 0
		}
		c, err := strconv.Atoi(p["candles"])
		if err != nil {
			c = 1
		}
		return 3 * c
	case n == "crying_obsidian":
		return 10
	case n == "glow_lichen":
		return 7
	case n == "sea_pickle":
		if p["waterlogged"] != "true" {
			return 0
		}
		c, err := strconv.Atoi(p["pickles"])
		if err != nil {
			c = 1
		}
		return 3 + 3*c
	case n == "magma_block":
		return 3
	case n == "amethyst_cluster":
		return 5
	case n == "brewing_stand", n == "brown_mushroom", n == "dragon_egg", n == "end_portal_frame", n == "sculk_sensor":
		return 1
	}
	return 0
}

// seeThrough reports full blocks that let light through anyway.
func seeThrough(bs structure.BlockState) bool {
	n := strings.TrimPrefix(bs.Name, "minecraft:")
	return (strings.Contains(n, "glass") && n != "tinted_glass") || strings.HasSuffix(n, "_leaves") ||
		n == "ice" || n == "frosted_ice" || n == "slime_block" || n == "honey_block" || n == "barrier" ||
		n == "spawner" || n == "beacon" || n == "glowstone" || strings.HasSuffix(n, "froglight") || n == "sea_lantern"
}

// spawnFloor reports whether hostile mobs can stand on top of a block when
// spawning: a full, opaque top face.
func spawnFloor(bs structure.BlockState) bool {
	n := strings.TrimPrefix(bs.Name, "minecraft:")
	switch {
	case strings.HasSuffix(n, "_slab"):
		return bs.Properties["type"] == "top" || bs.Properties["type"] == "double"
	case strings.HasSuffix(n, "_stairs"):
		return bs.Properties["half"] == "top"
	case seeThrough(bs), n == "magma_block":
		return false
	}
	return isFull(bs)
}

type lightingIn struct {
	Name        string `json:"name"`
	MaxLight    int    `json:"max_light,omitempty" jsonschema:"highest block light at which hostile mobs spawn: 0 (default) for Java Edition 1.18 and later, 7 for older versions"`
	IncludeOpen bool   `json:"include_open,omitempty" jsonschema:"also report positions with nothing above them in the structure. By default only roofed positions count, since open ground gets mobs at night whatever the torches do"`
	Limit       int    `json:"limit,omitempty" jsonschema:"most positions to list (default 20)"`
}

func (s *state) checkLighting(in lightingIn) (string, error) {
	st, err := s.get(in.Name)
	if err != nil {
		return "", err
	}
	if in.MaxLight < 0 || in.MaxLight > 14 {
		return "", fmt.Errorf("max_light must be between 0 and 14, got %d", in.MaxLight)
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	g := newGrid(st)
	sx, sy, sz := int(st.Size[0]), int(st.Size[1]), int(st.Size[2])
	if sx*sy*sz == 0 {
		return "", fmt.Errorf("%q is empty", in.Name)
	}
	idx := func(p vec) int { return (p[1]*sz+p[2])*sx + p[0] }
	inside := func(p vec) bool {
		return p[0] >= 0 && p[1] >= 0 && p[2] >= 0 && p[0] < sx && p[1] < sy && p[2] < sz
	}
	opaque := func(p vec) bool {
		bs, _, ok := g.get(p)
		return ok && isFull(bs) && !seeThrough(bs)
	}

	// Spread block light from every emitter, one level per step, through
	// anything that isn't an opaque full block.
	light := make([]int8, sx*sy*sz)
	var queue []vec
	emitters := 0
	for _, b := range st.Blocks {
		bs, ok := stateOf(st, b)
		if !ok {
			continue
		}
		if l := lightLevel(bs); l > 0 {
			p := toVec(b.Pos)
			if inside(p) && int(light[idx(p)]) < l {
				light[idx(p)] = int8(l)
				queue = append(queue, p)
				emitters++
			}
		}
	}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		l := light[idx(p)] - 1
		if l <= 0 {
			continue
		}
		for _, d := range []dir{north, east, south, west, up, down} {
			q := p.add(d.vec())
			if !inside(q) || opaque(q) || light[idx(q)] >= l {
				continue
			}
			light[idx(q)] = l
			queue = append(queue, q)
		}
	}

	// A position is covered if the structure has a block anywhere above it.
	covered := map[[2]int]int{} // column -> highest y with a block
	for _, b := range st.Blocks {
		bs, ok := stateOf(st, b)
		if !ok || isAir(bs.Name) {
			continue
		}
		c := [2]int{int(b.Pos[0]), int(b.Pos[2])}
		if y, ok := covered[c]; !ok || int(b.Pos[1]) > y {
			covered[c] = int(b.Pos[1])
		}
	}
	clear := func(p vec) bool {
		bs, _, ok := g.get(p)
		if !ok {
			return true
		}
		n := strings.TrimPrefix(bs.Name, "minecraft:")
		if n == "water" || n == "lava" || bs.Properties["waterlogged"] == "true" || n == "rail" || strings.HasSuffix(n, "_rail") {
			return false
		}
		return collisionOf(bs) == nil
	}

	var spots []vec
	perY := map[int]int{}
	for y := 1; y < sy; y++ {
		for z := 0; z < sz; z++ {
			for x := 0; x < sx; x++ {
				p := vec{x, y, z}
				if int(light[idx(p)]) > in.MaxLight {
					continue
				}
				floor, _, ok := g.get(p.add(down.vec()))
				if !ok || !spawnFloor(floor) || !clear(p) {
					continue
				}
				if head := p.add(up.vec()); head[1] < sy && !clear(head) {
					continue
				}
				if top, ok := covered[[2]int{x, z}]; !in.IncludeOpen && (!ok || top <= y) {
					continue
				}
				spots = append(spots, p)
				perY[y]++
			}
		}
	}

	var b strings.Builder
	what := "roofed "
	if in.IncludeOpen {
		what = ""
	}
	fmt.Fprintf(&b, "Lighting of %q: %d %sfloor positions where hostile mobs could spawn (block light %d or less, a full block below, room to stand). %d light sources found.\n",
		in.Name, len(spots), what, in.MaxLight, emitters)
	if len(spots) == 0 {
		b.WriteString("Every such floor position is lit.\n")
		return b.String(), nil
	}
	ys := make([]int, 0, len(perY))
	for y := range perY {
		ys = append(ys, y)
	}
	sort.Ints(ys)
	b.WriteString("By layer:")
	for _, y := range ys {
		fmt.Fprintf(&b, " y=%d: %d", y, perY[y])
	}
	b.WriteString("\n\nPositions:\n")
	for i, p := range spots {
		if i == limit {
			fmt.Fprintf(&b, "  ... and %d more\n", len(spots)-limit)
			break
		}
		floor, _, _ := g.get(p.add(down.vec()))
		fmt.Fprintf(&b, "  %v on %s, light %d\n", p, floor, light[idx(p)])
	}

	// Map the layer with the most dark spots, so it's clear where a light
	// would help most.
	worst := ys[0]
	for _, y := range ys {
		if perY[y] > perY[worst] {
			worst = y
		}
	}
	dark := map[vec]bool{}
	for _, p := range spots {
		dark[p] = true
	}
	fmt.Fprintf(&b, "\nLayer y=%d (! dark spawn spot, * light source, # opaque block, . anything else), rows north to south:\n", worst)
	for z := 0; z < sz; z++ {
		fmt.Fprintf(&b, "%4d ", z)
		for x := 0; x < sx; x++ {
			p := vec{x, worst, z}
			bs, _, ok := g.get(p)
			switch {
			case dark[p]:
				b.WriteByte('!')
			case ok && lightLevel(bs) > 0:
				b.WriteByte('*')
			case opaque(p):
				b.WriteByte('#')
			default:
				b.WriteByte('.')
			}
		}
		b.WriteByte('\n')
	}
	b.WriteString("\nA torch gives light 14 and a lantern 15, dropping by one per block, so a light every 12 or so blocks of floor keeps it above 0. This ignores sky light and treats slabs and stairs as letting light through.\n")
	return b.String(), nil
}

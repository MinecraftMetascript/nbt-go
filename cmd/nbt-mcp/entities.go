package main

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/MinecraftMetascript/nbt-go"
	"github.com/MinecraftMetascript/nbt-go/structure"
)

// entitySizes gives the width and height of entities whose size differs
// from a player-sized mob.
var entitySizes = map[string][2]float64{
	"villager": {0.6, 1.95}, "wandering_trader": {0.6, 1.95}, "zombie_villager": {0.6, 1.95},
	"armor_stand": {0.5, 1.975}, "iron_golem": {1.4, 2.7}, "snow_golem": {0.7, 1.9},
	"cat": {0.6, 0.7}, "ocelot": {0.6, 0.7}, "wolf": {0.6, 0.85}, "fox": {0.6, 0.7},
	"chicken": {0.4, 0.7}, "rabbit": {0.4, 0.5}, "parrot": {0.5, 0.9}, "bee": {0.7, 0.6},
	"pig": {0.9, 0.9}, "sheep": {0.9, 1.3}, "cow": {0.9, 1.4}, "mooshroom": {0.9, 1.4}, "goat": {0.9, 1.3},
	"horse": {1.3965, 1.6}, "donkey": {1.3965, 1.5}, "mule": {1.3965, 1.6}, "llama": {0.9, 1.87}, "camel": {1.7, 2.375},
	"allay": {0.35, 0.6}, "axolotl": {0.75, 0.42}, "frog": {0.5, 0.5}, "turtle": {1.2, 0.4},
	"spider": {1.4, 0.9}, "cave_spider": {0.7, 0.5}, "enderman": {0.6, 2.9}, "witch": {0.6, 1.95},
	"minecart": {0.98, 0.7}, "chest_minecart": {0.98, 0.7}, "boat": {1.375, 0.5625},
}

// entityFootprint returns an entity's width and height, and false for
// entities that don't collide with blocks: hanging entities, displays,
// markers and the like.
func entityFootprint(id string) (w, h float64, ok bool) {
	n := strings.TrimPrefix(id, "minecraft:")
	switch {
	case n == "item_frame", n == "glow_item_frame", n == "painting", n == "leash_knot", n == "marker",
		n == "interaction", strings.HasSuffix(n, "_display"), n == "item", n == "experience_orb",
		n == "area_effect_cloud", n == "falling_block", n == "lightning_bolt", n == "end_crystal":
		return 0, 0, false
	case strings.HasSuffix(n, "_boat") || strings.HasSuffix(n, "_raft"):
		return 1.375, 0.5625, true
	}
	if s, ok := entitySizes[n]; ok {
		return s[0], s[1], true
	}
	return 0.6, 1.8, true
}

// collisionOf approximates the boxes an entity collides with in a block,
// or nil for blocks that can be walked through.
func collisionOf(bs structure.BlockState) []fbox {
	n := strings.TrimPrefix(bs.Name, "minecraft:")
	switch {
	case isAir(bs.Name), letsBlocksFall(bs) && n != "snow", n == "lava", n == "water", n == "bubble_column",
		strings.HasSuffix(n, "torch"), strings.HasSuffix(n, "_sign"), strings.HasSuffix(n, "_banner"),
		strings.HasSuffix(n, "_button"), n == "lever", strings.HasPrefix(n, "tripwire"),
		strings.HasSuffix(n, "_pressure_plate"), n == "redstone_wire", n == "rail", strings.HasSuffix(n, "_rail"),
		n == "cobweb", n == "sugar_cane", n == "kelp", n == "kelp_plant", n == "nether_portal", n == "end_portal",
		n == "glow_lichen", n == "sculk_vein", n == "vine", n == "structure_void", n == "light",
		n == "wheat", n == "carrots", n == "potatoes", n == "beetroots", n == "sweet_berry_bush",
		strings.HasSuffix(n, "_sapling"), strings.HasSuffix(n, "_mushroom"), strings.HasSuffix(n, "_tulip"),
		strings.HasSuffix(n, "_coral"), strings.HasSuffix(n, "_coral_fan"), strings.HasSuffix(n, "_petals"),
		strings.HasSuffix(n, "_vines"), strings.HasSuffix(n, "_vines_plant"), n == "seagrass", n == "tall_seagrass",
		n == "dandelion", n == "poppy", n == "blue_orchid", n == "allium", n == "azure_bluet", n == "oxeye_daisy",
		n == "cornflower", n == "lily_of_the_valley", n == "wither_rose", n == "torchflower", n == "sunflower",
		n == "lilac", n == "rose_bush", n == "peony", n == "pitcher_plant", n == "firefly_bush", n == "spore_blossom",
		n == "hanging_roots", n == "small_dripleaf", n == "pink_petals", n == "leaf_litter", n == "fire", n == "soul_fire":
		return nil
	case n == "barrier":
		return fullCell
	case strings.HasSuffix(n, "_fence_gate") && bs.Properties["open"] == "true":
		return nil
	case strings.HasSuffix(n, "_door") && bs.Properties["open"] == "true":
		// An open door swings to the hinge side; treat it as passable for
		// this check rather than guessing which edge it covers.
		return nil
	}
	return shapeOf(bs)
}

// stuckIn lists blocks whose collision boxes overlap an entity standing at
// pos, which leaves it stuck (mobs suffocate or can't move) in the game.
func stuckIn(g *grid, pos [3]float64, id string) []string {
	w, h, ok := entityFootprint(id)
	if !ok {
		return nil
	}
	const eps = 1e-6
	lo := [3]float64{pos[0] - w/2, pos[1], pos[2] - w/2}
	hi := [3]float64{pos[0] + w/2, pos[1] + h, pos[2] + w/2}
	var out []string
	for y := int(math.Floor(lo[1])); y <= int(math.Floor(hi[1]-eps)); y++ {
		for x := int(math.Floor(lo[0] + eps)); x <= int(math.Floor(hi[0]-eps)); x++ {
			for z := int(math.Floor(lo[2] + eps)); z <= int(math.Floor(hi[2]-eps)); z++ {
				p := vec{x, y, z}
				bs, _, ok := g.get(p)
				if !ok {
					continue
				}
				for _, b := range collisionOf(bs) {
					hit := true
					for a := 0; a < 3; a++ {
						if lo[a] >= float64(p[a])+b.max[a]-eps || hi[a] <= float64(p[a])+b.min[a]+eps {
							hit = false
						}
					}
					if !hit {
						continue
					}
					msg := fmt.Sprintf("%s at %v", bs, p)
					if y == int(math.Floor(pos[1])) && b.min[1] < eps && b.max[1] < 1 {
						msg += fmt.Sprintf(" (stand it on top, at y = %g)", float64(y)+b.max[1])
					}
					out = append(out, msg)
					break
				}
			}
		}
	}
	return out
}

func stuckWarning(g *grid, pos [3]float64, id string) string {
	hits := stuckIn(g, pos, id)
	if len(hits) == 0 {
		return ""
	}
	return fmt.Sprintf("\nWarning: %s at %v starts inside the collision box of %s, so it will be stuck in the game. Move it with move_entity.", id, pos, strings.Join(hits, ", "))
}

type entityRefIn struct {
	Name  string      `json:"name"`
	Index *int        `json:"index,omitempty" jsonschema:"the entity's number, as describe_structure lists it (#0, #1, ...)"`
	Near  *[3]float64 `json:"near,omitempty" jsonschema:"pick the entity closest to this position (within 2 blocks) instead of by index"`
	ID    string      `json:"id,omitempty" jsonschema:"with near, only consider entities of this id, e.g. villager"`
}

// find returns the index of the entity a reference names.
func (in entityRefIn) find(st *structure.Structure) (int, error) {
	if (in.Index == nil) == (in.Near == nil) {
		return 0, errors.New("give exactly one of index and near")
	}
	if in.Index != nil {
		i := *in.Index
		if i < 0 || i >= len(st.Entities) {
			return 0, fmt.Errorf("index %d is out of range; the structure has %d entities", i, len(st.Entities))
		}
		return i, nil
	}
	want := ""
	if in.ID != "" {
		want = namespaced(in.ID)
	}
	best, bestD := -1, 4.0 // squared distance: within 2 blocks
	for i, e := range st.Entities {
		if id, _ := e.NBT["id"].(nbt.String); want != "" && string(id) != want {
			continue
		}
		d := 0.0
		for a := 0; a < 3; a++ {
			d += (e.Pos[a] - in.Near[a]) * (e.Pos[a] - in.Near[a])
		}
		if d <= bestD {
			best, bestD = i, d
		}
	}
	if best < 0 {
		return 0, fmt.Errorf("no entity within 2 blocks of %v", *in.Near)
	}
	return best, nil
}

type moveEntityIn struct {
	entityRefIn
	To *[3]float64 `json:"to,omitempty" jsonschema:"new position [x, y, z]"`
	By *[3]float64 `json:"by,omitempty" jsonschema:"offset to move it by, e.g. [0, 0.0625, 0] to lift it onto a carpet"`
}

func (s *state) moveEntity(in moveEntityIn) (string, error) {
	st, err := s.get(in.Name)
	if err != nil {
		return "", err
	}
	i, err := in.find(st)
	if err != nil {
		return "", err
	}
	if (in.To == nil) == (in.By == nil) {
		return "", errors.New("give exactly one of to and by")
	}
	e := &st.Entities[i]
	from := e.Pos
	to := e.Pos
	if in.To != nil {
		to = *in.To
	} else {
		for a := 0; a < 3; a++ {
			to[a] += in.By[a]
		}
	}
	for a := 0; a < 3; a++ {
		if to[a] < 0 {
			return "", fmt.Errorf("position %v is out of range; coordinates must be non-negative", to)
		}
	}
	setEntityPos(st, e, to)
	id, _ := e.NBT["id"].(nbt.String)
	return fmt.Sprintf("Moved entity #%d (%s) from %v to %v.", i, string(id), from, to) + stuckWarning(newGrid(st), to, string(id)), nil
}

// setEntityPos moves an entity, keeping its three copies of the position
// (pos, blockPos and the Pos inside its data) in step.
func setEntityPos(st *structure.Structure, e *structure.Entity, to [3]float64) {
	e.Pos = to
	pos := make(nbt.List, 3)
	for a := 0; a < 3; a++ {
		e.BlockPos[a] = int32(math.Floor(to[a]))
		st.Size[a] = max(st.Size[a], e.BlockPos[a]+1)
		pos[a] = nbt.Double(to[a])
	}
	if e.NBT == nil {
		e.NBT = nbt.Compound{}
	}
	e.NBT["Pos"] = pos
}

func (s *state) removeEntity(in entityRefIn) (string, error) {
	st, err := s.get(in.Name)
	if err != nil {
		return "", err
	}
	i, err := in.find(st)
	if err != nil {
		return "", err
	}
	e := st.Entities[i]
	st.Entities = append(st.Entities[:i], st.Entities[i+1:]...)
	id, _ := e.NBT["id"].(nbt.String)
	msg := fmt.Sprintf("Removed entity #%d (%s) at %v.", i, string(id), e.Pos)
	if i < len(st.Entities) {
		msg += " Entities after it are renumbered."
	}
	return msg, nil
}

func (l *linter) entities() {
	for i, e := range l.st.Entities {
		id, _ := e.NBT["id"].(nbt.String)
		if hits := stuckIn(l.g, e.Pos, string(id)); len(hits) > 0 {
			l.add("entities", toVec(e.BlockPos), "entity #%d (%s) at %v starts inside %s, so it will be stuck", i, string(id), e.Pos, strings.Join(hits, ", "))
		}
	}
}

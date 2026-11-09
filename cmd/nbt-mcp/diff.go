package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/MinecraftMetascript/nbt-go/structure"
)

type diffIn struct {
	Before string `json:"before" jsonschema:"the older open structure"`
	After  string `json:"after" jsonschema:"the newer open structure"`
	Limit  int    `json:"limit,omitempty" jsonschema:"most individual changes to list (default 20)"`
}

// diffStructures compares two structures position by position.
func (s *state) diffStructures(in diffIn) (string, error) {
	a, err := s.get(in.Before)
	if err != nil {
		return "", err
	}
	b, err := s.get(in.After)
	if err != nil {
		return "", err
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	ga, gb := newGrid(a), newGrid(b)
	describe := func(g *grid, p vec) string {
		bs, _, ok := g.get(p)
		if !ok {
			return "(no block)"
		}
		return bs.String()
	}
	positions := map[vec]bool{}
	for p := range ga.at {
		positions[p] = true
	}
	for p := range gb.at {
		positions[p] = true
	}
	type change struct {
		p    vec
		from string
		to   string
	}
	var changes []change
	dataChanged := 0
	for p := range positions {
		sa, da, oka := ga.get(p)
		sb, db, okb := gb.get(p)
		switch {
		case oka != okb || (oka && sa.String() != sb.String()):
			changes = append(changes, change{p, describe(ga, p), describe(gb, p)})
		case oka && fmt.Sprint(da) != fmt.Sprint(db):
			changes = append(changes, change{p, describe(ga, p) + " " + fmt.Sprint(da), "block entity data " + fmt.Sprint(db)})
			dataChanged++
		}
	}
	sort.Slice(changes, func(i, j int) bool { return lessVec(changes[i].p, changes[j].p) })

	var out strings.Builder
	fmt.Fprintf(&out, "Comparing %q (before) with %q (after).\n", in.Before, in.After)
	if a.Size != b.Size {
		fmt.Fprintf(&out, "Size changed from %v to %v.\n", a.Size, b.Size)
	}
	if a.DataVersion != b.DataVersion {
		fmt.Fprintf(&out, "DataVersion changed from %d to %d.\n", a.DataVersion, b.DataVersion)
	}
	if len(a.Entities) != len(b.Entities) {
		fmt.Fprintf(&out, "Entities: %d before, %d after.\n", len(a.Entities), len(b.Entities))
	}
	if len(changes) == 0 {
		out.WriteString("No blocks differ.\n")
		return out.String(), nil
	}
	bounds := box{changes[0].p, changes[0].p}
	kinds := map[string]int{}
	for _, c := range changes {
		for k := 0; k < 3; k++ {
			bounds.min[k], bounds.max[k] = min(bounds.min[k], c.p[k]), max(bounds.max[k], c.p[k])
		}
		if strings.HasPrefix(c.to, "block entity data") {
			kinds["block entity data changed"]++
		} else {
			kinds[nameOnly(c.from)+" → "+nameOnly(c.to)]++
		}
	}
	fmt.Fprintf(&out, "%d positions differ, within %s.\n\nBy kind:\n", len(changes), bounds)
	keys := make([]string, 0, len(kinds))
	for k := range kinds {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if kinds[keys[i]] != kinds[keys[j]] {
			return kinds[keys[i]] > kinds[keys[j]]
		}
		return keys[i] < keys[j]
	})
	for i, k := range keys {
		if i == limit {
			fmt.Fprintf(&out, "  ... and %d more kinds\n", len(keys)-limit)
			break
		}
		fmt.Fprintf(&out, "  %6d  %s\n", kinds[k], k)
	}
	out.WriteString("\nChanges:\n")
	for i, c := range changes {
		if i == limit {
			fmt.Fprintf(&out, "  ... and %d more\n", len(changes)-limit)
			break
		}
		fmt.Fprintf(&out, "  %v %s → %s\n", c.p, c.from, c.to)
	}
	return out.String(), nil
}

// nameOnly drops block state properties, to group changes by block type.
func nameOnly(s string) string {
	n, _, _ := strings.Cut(s, "[")
	return n
}

// airStats counts the non-air blocks, air blocks and empty positions in a
// structure.
func airStats(st *structure.Structure) (solid, air, empty int) {
	pal := palette(st)
	for _, b := range st.Blocks {
		if b.State >= 0 && int(b.State) < len(pal) && isAir(pal[b.State].Name) {
			air++
		} else {
			solid++
		}
	}
	vol := int(st.Size[0]) * int(st.Size[1]) * int(st.Size[2])
	return solid, air, max(vol-solid-air, 0)
}

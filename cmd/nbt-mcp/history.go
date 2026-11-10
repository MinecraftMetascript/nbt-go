package main

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/MinecraftMetascript/nbt-go/structure"
)

const (
	// maxUndoSteps and maxUndoBlocks bound the edit history: the oldest
	// steps are dropped past either limit.
	maxUndoSteps  = 100
	maxUndoBlocks = 8 << 20
)

// targetArgs are the arguments that name a structure an edit tool changes
// or creates. Every edit tool has at least one of them.
var targetArgs = []string{"name", "target", "into"}

// edit is one step of history: the structures a tool call could change, as
// they were before it. A nil structure means the name wasn't open.
type edit struct {
	tool   string
	before map[string]*structure.Structure
}

func (e edit) blocks() int {
	n := 0
	for _, st := range e.before {
		if st != nil {
			n += len(st.Blocks)
		}
	}
	return n
}

func (e edit) names() []string {
	names := make([]string, 0, len(e.before))
	for n := range e.before {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// snapshot copies the structures a call to an edit tool names, before the
// tool runs.
func (s *state) snapshot(tool string, args map[string]any) edit {
	e := edit{tool: tool, before: map[string]*structure.Structure{}}
	for _, key := range targetArgs {
		name, _ := args[key].(string)
		if name == "" {
			continue
		}
		e.before[name] = cloneStructure(s.structures[name])
	}
	return e
}

// restore puts back what e saved and returns what it replaced, which undoes
// the restore in turn.
func (s *state) restore(e edit) edit {
	replaced := edit{tool: e.tool, before: map[string]*structure.Structure{}}
	for name, st := range e.before {
		replaced.before[name] = s.structures[name]
		if st == nil {
			delete(s.structures, name)
		} else {
			s.structures[name] = st
		}
	}
	return replaced
}

func (e edit) touches(name string) bool {
	_, ok := e.before[name]
	return ok
}

func (e edit) overlaps(o edit) bool {
	for n := range e.before {
		if o.touches(n) {
			return true
		}
	}
	return false
}

// record adds a finished edit to the history. It ends any chance to redo
// edits to the same structures; undone edits to other structures can still
// be redone, so agents working on separate structures don't interfere.
func (s *state) record(e edit) {
	s.undo = append(s.undo, e)
	s.redo = slices.DeleteFunc(s.redo, e.overlaps)
	total := 0
	for i := len(s.undo) - 1; i >= 0; i-- {
		total += s.undo[i].blocks()
		// The latest step is always kept, however large.
		if i < len(s.undo)-1 && (total > maxUndoBlocks || len(s.undo)-i > maxUndoSteps) {
			s.undo = slices.Delete(s.undo, 0, i+1)
			break
		}
	}
	s.notify(e.names())
}

func (s *state) notify(names []string) {
	if s.changed != nil {
		s.changed(names)
	}
}

func cloneStructure(st *structure.Structure) *structure.Structure {
	if st == nil {
		return nil
	}
	out := structure.New(st.DataVersion)
	out.Size = st.Size
	out.Palette = clonePalette(st.Palette)
	if st.Palettes != nil {
		out.Palettes = make([][]structure.BlockState, len(st.Palettes))
		for i, p := range st.Palettes {
			out.Palettes[i] = clonePalette(p)
		}
	}
	out.Blocks = slices.Clone(st.Blocks)
	for i := range out.Blocks {
		out.Blocks[i].NBT = cloneCompound(out.Blocks[i].NBT)
	}
	out.Entities = slices.Clone(st.Entities)
	for i := range out.Entities {
		out.Entities[i].NBT = cloneCompound(out.Entities[i].NBT)
	}
	return out
}

func clonePalette(p []structure.BlockState) []structure.BlockState {
	if p == nil {
		return nil
	}
	out := make([]structure.BlockState, len(p))
	for i, bs := range p {
		out[i] = structure.BlockState{Name: bs.Name}
		if bs.Properties != nil {
			out[i].Properties = make(map[string]string, len(bs.Properties))
			for k, v := range bs.Properties {
				out[i].Properties[k] = v
			}
		}
	}
	return out
}

type historyIn struct {
	Name  string `json:"name,omitempty" jsonschema:"only step through edits to this structure, leaving edits to other structures alone. Pass it whenever several agents share the server"`
	Steps int    `json:"steps,omitempty" jsonschema:"how many edits to step through (default 1)"`
}

type historyOut struct {
	Steps      []historyStep `json:"steps" jsonschema:"the edits stepped through, in the order they were applied"`
	CanUndo    int           `json:"can_undo" jsonschema:"edits that can still be undone"`
	CanRedo    int           `json:"can_redo" jsonschema:"undone edits that can be redone"`
	Structures []string      `json:"structures" jsonschema:"the structures open now"`
}

type historyStep struct {
	Tool       string   `json:"tool" jsonschema:"the tool whose edit was undone or redone"`
	Structures []string `json:"structures" jsonschema:"the structures it changed"`
}

func (s *state) undoEdits(in historyIn) (string, historyOut, error) {
	return s.step(in, &s.undo, &s.redo, "undo", "Undid")
}

func (s *state) redoEdits(in historyIn) (string, historyOut, error) {
	return s.step(in, &s.redo, &s.undo, "redo", "Redid")
}

// step moves edits from one history stack to the other, restoring each.
func (s *state) step(in historyIn, from, to *[]edit, verb, done string) (string, historyOut, error) {
	n := in.Steps
	if n <= 0 {
		n = 1
	}
	out := historyOut{Steps: []historyStep{}}
	touched := map[string]bool{}
	var b strings.Builder
	for ; n > 0; n-- {
		i, err := pick(*from, in.Name, verb)
		if err != nil {
			if len(out.Steps) == 0 {
				return "", historyOut{}, err
			}
			break
		}
		e := (*from)[i]
		*from = slices.Delete(*from, i, i+1)
		*to = append(*to, s.restore(e))
		names := e.names()
		out.Steps = append(out.Steps, historyStep{Tool: e.tool, Structures: names})
		fmt.Fprintf(&b, "%s %s on %s.\n", done, e.tool, strings.Join(names, ", "))
		for _, name := range names {
			touched[name] = true
		}
	}
	names := make([]string, 0, len(touched))
	for name := range touched {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if st := s.structures[name]; st != nil {
			fmt.Fprintf(&b, "%q is now %d x %d x %d with %d blocks.\n", name, st.Size[0], st.Size[1], st.Size[2], len(st.Blocks))
		} else {
			fmt.Fprintf(&b, "%q is no longer open.\n", name)
		}
	}
	out.CanUndo, out.CanRedo, out.Structures = count(s.undo, in.Name), count(s.redo, in.Name), s.names()
	if in.Name != "" {
		fmt.Fprintf(&b, "%d more edits to %q can be undone, %d redone.", out.CanUndo, in.Name, out.CanRedo)
	} else {
		fmt.Fprintf(&b, "%d more can be undone, %d redone.", out.CanUndo, out.CanRedo)
	}
	s.notify(names)
	return b.String(), out, nil
}

// pick finds the edit to step through next: the latest one, or with a name,
// the latest one that touched that structure. An edit that also touched
// other structures can only be stepped through if none of them has a later
// edit, which restoring it would wipe out.
func pick(stack []edit, name, verb string) (int, error) {
	if len(stack) == 0 {
		return 0, errors.New("there is nothing to " + verb)
	}
	if name == "" {
		return len(stack) - 1, nil
	}
	for i := len(stack) - 1; i >= 0; i-- {
		e := stack[i]
		if !e.touches(name) {
			continue
		}
		for _, later := range stack[i+1:] {
			if later.overlaps(e) {
				var shared []string
				for _, n := range e.names() {
					if later.touches(n) {
						shared = append(shared, n)
					}
				}
				return 0, fmt.Errorf("the last %s on %q (%s) also changed %s, which has a later edit (%s); %s that first", verb, name, e.tool, strings.Join(shared, ", "), later.tool, verb)
			}
		}
		return i, nil
	}
	return 0, fmt.Errorf("there is nothing to %s on %q", verb, name)
}

// count is how many edits in stack touch name, or all of them.
func count(stack []edit, name string) int {
	if name == "" {
		return len(stack)
	}
	n := 0
	for _, e := range stack {
		if e.touches(name) {
			n++
		}
	}
	return n
}

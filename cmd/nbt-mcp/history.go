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

// record adds a finished edit to the history, which ends any chance to redo.
func (s *state) record(e edit) {
	s.undo = append(s.undo, e)
	s.redo = nil
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
	Steps int `json:"steps,omitempty" jsonschema:"how many edits to step through (default 1)"`
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
	if len(*from) == 0 {
		return "", historyOut{}, errors.New("there is nothing to " + verb)
	}
	out := historyOut{Steps: []historyStep{}}
	touched := map[string]bool{}
	var b strings.Builder
	for ; n > 0 && len(*from) > 0; n-- {
		e := (*from)[len(*from)-1]
		*from = (*from)[:len(*from)-1]
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
	out.CanUndo, out.CanRedo, out.Structures = len(s.undo), len(s.redo), s.names()
	fmt.Fprintf(&b, "%d more can be undone, %d redone.", out.CanUndo, out.CanRedo)
	s.notify(names)
	return b.String(), out, nil
}

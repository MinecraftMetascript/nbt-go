package nbt

import (
	"errors"
	"fmt"
	"strconv"
)

// A Path is an NBT path, the syntax commands such as /data and /execute
// use to point into NBT: foo.bar[0]."a key"[{id:"x"}]. Parse one with
// [ParsePath].
//
// A path is a list of nodes, each of which selects tags inside the ones the
// previous node selected:
//
//	{k:v}     the root, if it matches the compound (only first)
//	name      the compound child name; quote it ("a b") if it has
//	          spaces, quotes, dots or brackets
//	name{k:v} the child name, if it is a compound that matches
//	[n]       the list or array element n; negative n counts from the end
//	[]        every element of a list or array
//	[{k:v}]   every element of a list that is a compound that matches
//
// A compound matches a pattern when it has every key of the pattern, each
// with a matching value: compounds match recursively, a list matches if
// every element of the pattern matches some element of it (an empty pattern
// list only matches an empty list), and anything else must be equal. This
// is how Minecraft compares them.
type Path struct {
	nodes []pathNode
	src   string
}

type nodeKind uint8

const (
	nodeRoot  nodeKind = iota // {pattern}
	nodeChild                 // name, or name{pattern}
	nodeIndex                 // [n]
	nodeAll                   // [] or [{pattern}]
)

type pathNode struct {
	kind    nodeKind
	name    string
	index   int
	pattern Compound // nil when the node has none
}

// A PathError reports a path that cannot be parsed, or an operation a path
// cannot do.
type PathError struct {
	Path   string
	Offset int // byte offset in Path, for a syntax error; -1 otherwise
	msg    string
}

func (e *PathError) Error() string {
	if e.Offset >= 0 {
		return fmt.Sprintf("nbt: invalid path %q: %s at offset %d", e.Path, e.msg, e.Offset)
	}
	return fmt.Sprintf("nbt: path %s: %s", e.Path, e.msg)
}

// ParsePath parses an NBT path such as Inventory[0].components."minecraft:custom_data".
func ParsePath(s string) (Path, error) {
	p := &snbtParser{s: s}
	fail := func(msg string) (Path, error) {
		return Path{}, &PathError{Path: s, Offset: p.pos, msg: msg}
	}
	var nodes []pathNode
	if p.peek() == '{' {
		c, err := p.compound()
		if err != nil {
			return Path{}, pathSyntaxError(s, err)
		}
		nodes = append(nodes, pathNode{kind: nodeRoot, pattern: c.(Compound)})
	}
	for p.pos < len(s) {
		switch c := s[p.pos]; {
		case c == '[':
			p.pos++
			switch p.peek() {
			case ']':
				p.pos++
				nodes = append(nodes, pathNode{kind: nodeAll})
				continue
			case '{':
				c, err := p.compound()
				if err != nil {
					return Path{}, pathSyntaxError(s, err)
				}
				if p.peek() != ']' {
					return fail("expected ']'")
				}
				p.pos++
				nodes = append(nodes, pathNode{kind: nodeAll, pattern: c.(Compound)})
				continue
			}
			start := p.pos
			for p.pos < len(s) && (s[p.pos] == '-' || s[p.pos] >= '0' && s[p.pos] <= '9') {
				p.pos++
			}
			n, err := strconv.Atoi(s[start:p.pos])
			if err != nil {
				p.pos = start
				return fail("expected an index, '{' or ']'")
			}
			if p.peek() != ']' {
				return fail("expected ']'")
			}
			p.pos++
			nodes = append(nodes, pathNode{kind: nodeIndex, index: n})
			continue
		case c == '.':
			if len(nodes) == 0 {
				return fail("unexpected '.'")
			}
			p.pos++
		case len(nodes) > 0:
			return fail("expected '.' or '['")
		}
		var name string
		switch p.peek() {
		case '"', '\'':
			q, err := p.quoted()
			if err != nil {
				return Path{}, pathSyntaxError(s, err)
			}
			name = q
		default:
			start := p.pos
			for p.pos < len(s) && isPathNameChar(s[p.pos]) {
				p.pos++
			}
			if p.pos == start {
				return fail("expected a name")
			}
			name = s[start:p.pos]
		}
		n := pathNode{kind: nodeChild, name: name}
		if p.peek() == '{' {
			c, err := p.compound()
			if err != nil {
				return Path{}, pathSyntaxError(s, err)
			}
			n.pattern = c.(Compound)
		}
		nodes = append(nodes, n)
	}
	if len(nodes) == 0 {
		return fail("empty path")
	}
	return Path{nodes: nodes, src: s}, nil
}

// pathSyntaxError turns an error from the SNBT parser into a PathError.
func pathSyntaxError(path string, err error) error {
	var se *SNBTSyntaxError
	if errors.As(err, &se) {
		return &PathError{Path: path, Offset: se.Offset, msg: se.msg}
	}
	return err
}

// isPathNameChar reports whether c can be in an unquoted name in a path.
// Unlike SNBT keys, names may hold ':' (minecraft:custom_data) and any
// other character but space, quotes, '.' and brackets.
func isPathNameChar(c byte) bool {
	switch c {
	case ' ', '"', '\'', '[', ']', '.', '{', '}':
		return false
	}
	return true
}

// MustParsePath is like [ParsePath] but panics if the path is invalid.
func MustParsePath(s string) Path {
	p, err := ParsePath(s)
	if err != nil {
		panic(err)
	}
	return p
}

// String returns the path as written.
func (p Path) String() string { return p.src }

func (p Path) errorf(format string, args ...any) error {
	return &PathError{Path: p.src, Offset: -1, msg: fmt.Sprintf(format, args...)}
}

// Get returns the tags the path selects in root, in order. It returns
// none, not an error, when nothing matches.
func (p Path) Get(root Tag) []Tag {
	cur := []Tag{root}
	for _, n := range p.nodes {
		var next []Tag
		for _, t := range cur {
			next = n.get(t, next)
		}
		cur = next
		if len(cur) == 0 {
			return nil
		}
	}
	return cur
}

func (n pathNode) get(t Tag, out []Tag) []Tag {
	switch n.kind {
	case nodeRoot:
		if Matches(n.pattern, t) {
			out = append(out, t)
		}
	case nodeChild:
		if c, ok := t.(Compound); ok {
			if v, ok := c[n.name]; ok && v != nil && (n.pattern == nil || Matches(n.pattern, v)) {
				out = append(out, v)
			}
		}
	case nodeIndex:
		if i, ok := elemIndex(t, n.index); ok {
			out = append(out, elemAt(t, i))
		}
	case nodeAll:
		for i := 0; i < elemCount(t); i++ {
			if e := elemAt(t, i); n.pattern == nil || Matches(n.pattern, e) {
				out = append(out, e)
			}
		}
	}
	return out
}

// Count returns how many tags the path selects in root.
func (p Path) Count(root Tag) int { return len(p.Get(root)) }

// Set replaces every tag the path selects in root with a copy of v, and
// returns how many it changed: a tag already equal to v does not count, so
// a 0 is how /data modify ... set fails with "Nothing changed".
//
// Compounds and lists missing on the way are created, as the game does:
// name creates a compound (or a list if the next node indexes one),
// name{k:v} creates a copy of its pattern, [{k:v}] adds one to an empty or
// unmatched list, and [] adds an element to an empty list. An index past
// the end of a list creates nothing.
//
// root is changed in place, so it must be a [Compound] (or a path that
// starts below its root); Set returns an error if an array would end up
// holding another type (an [IntArray] element set to a [Byte], say).
func (p Path) Set(root Tag, v Tag) (int, error) {
	if v == nil {
		return 0, p.errorf("cannot set nil")
	}
	_, n, err := p.update(root, 0, func(old Tag) (Tag, int, error) {
		if old != nil && Equal(old, v) {
			return old, 0, nil
		}
		return Clone(v), 1, nil
	}, true)
	return n, err
}

// Modify calls f with each tag the path selects in root, creating them the
// way [Path.Set] does with create if they are missing (create is then called
// for the new tag, and may be nil to create nothing). The tag f returns
// replaces the old one. Modify returns how many tags f was called for.
//
// It is how /data modify ... append and merge work: Modify with
// create returning an empty List, appending in f.
func (p Path) Modify(root Tag, create func() Tag, f func(Tag) (Tag, error)) (int, error) {
	_, n, err := p.update(root, 0, func(old Tag) (Tag, int, error) {
		if old == nil {
			if create == nil {
				return nil, 0, nil
			}
			old = create()
		}
		t, err := f(old)
		if err != nil {
			return nil, 0, err
		}
		if t == nil {
			return nil, 0, p.errorf("cannot set nil")
		}
		return t, 1, nil
	}, create != nil)
	return n, err
}

// Remove deletes every tag the path selects in root and returns how many it
// removed. A path that selects the root itself removes nothing.
func (p Path) Remove(root Tag) int {
	if len(p.nodes) == 1 && p.nodes[0].kind == nodeRoot {
		return 0
	}
	n := 0
	p.walkParents(root, func(parent Tag, last pathNode) Tag {
		var removed int
		parent, removed = last.remove(parent)
		n += removed
		return parent
	})
	return n
}

// walkParents calls f with each tag the path's last node applies to, and
// stores the tag f returns in its place.
func (p Path) walkParents(root Tag, f func(parent Tag, last pathNode) Tag) {
	var walk func(t Tag, i int) Tag
	walk = func(t Tag, i int) Tag {
		if i == len(p.nodes)-1 {
			return f(t, p.nodes[i])
		}
		n := p.nodes[i]
		return n.each(t, func(child Tag) Tag { return walk(child, i+1) })
	}
	walk(root, 0)
}

// each calls f with each tag n selects in t, and stores what f returns in
// its place. It returns t, or the new list or array when an element was
// replaced.
func (n pathNode) each(t Tag, f func(Tag) Tag) Tag {
	switch n.kind {
	case nodeRoot:
		if Matches(n.pattern, t) {
			return f(t)
		}
	case nodeChild:
		if c, ok := t.(Compound); ok {
			if v, ok := c[n.name]; ok && v != nil && (n.pattern == nil || Matches(n.pattern, v)) {
				c[n.name] = f(v)
			}
		}
	case nodeIndex:
		if i, ok := elemIndex(t, n.index); ok {
			if nt, err := setElem(t, i, f(elemAt(t, i))); err == nil {
				return nt
			}
		}
	case nodeAll:
		for i := 0; i < elemCount(t); i++ {
			if e := elemAt(t, i); n.pattern == nil || Matches(n.pattern, e) {
				if nt, err := setElem(t, i, f(e)); err == nil {
					t = nt
				}
			}
		}
	}
	return t
}

func (n pathNode) remove(t Tag) (Tag, int) {
	switch n.kind {
	case nodeChild:
		if c, ok := t.(Compound); ok {
			if v, ok := c[n.name]; ok && (n.pattern == nil || Matches(n.pattern, v)) {
				delete(c, n.name)
				return c, 1
			}
		}
	case nodeIndex:
		if i, ok := elemIndex(t, n.index); ok {
			return removeElems(t, func(j int) bool { return j == i })
		}
	case nodeAll:
		return removeElems(t, func(j int) bool { return n.pattern == nil || Matches(n.pattern, elemAt(t, j)) })
	}
	return t, 0
}

// update applies f to every tag the path selects below t, starting at node
// i, creating missing ones when create is set. It returns the new t.
func (p Path) update(t Tag, i int, f func(old Tag) (Tag, int, error), create bool) (Tag, int, error) {
	n := p.nodes[i]
	last := i == len(p.nodes)-1
	apply := func(child Tag) (Tag, int, error) {
		if last {
			return f(child)
		}
		if child == nil {
			child = p.nodes[i+1].parentFor()
		}
		return p.update(child, i+1, f, create)
	}
	total := 0
	switch n.kind {
	case nodeRoot:
		if !Matches(n.pattern, t) {
			return t, 0, nil
		}
		if last {
			return t, 0, p.errorf("cannot replace the root")
		}
		return p.update(t, i+1, f, create)
	case nodeChild:
		c, ok := t.(Compound)
		if !ok {
			return t, 0, nil
		}
		v, exists := c[n.name]
		if exists && v != nil && n.pattern != nil && !Matches(n.pattern, v) {
			return t, 0, nil
		}
		if !exists || v == nil {
			if !create {
				return t, 0, nil
			}
			v = nil
			if n.pattern != nil {
				v = Clone(n.pattern)
			}
		}
		nv, k, err := apply(v)
		if err != nil {
			return t, 0, err
		}
		if nv != nil {
			c[n.name] = nv
		}
		return c, k, nil
	case nodeIndex:
		j, ok := elemIndex(t, n.index)
		if !ok {
			return t, 0, nil
		}
		nv, k, err := apply(elemAt(t, j))
		if err != nil || nv == nil {
			return t, 0, err
		}
		nt, err := setElem(t, j, nv)
		if err != nil {
			return t, 0, p.errorf("%v", err)
		}
		return nt, k, nil
	case nodeAll:
		if elemCount(t) < 0 {
			return t, 0, nil
		}
		matched := false
		for j := 0; j < elemCount(t); j++ {
			e := elemAt(t, j)
			if n.pattern != nil && !Matches(n.pattern, e) {
				continue
			}
			matched = true
			nv, k, err := apply(e)
			if err != nil {
				return t, 0, err
			}
			if nv == nil {
				continue
			}
			if t, err = setElem(t, j, nv); err != nil {
				return t, 0, p.errorf("%v", err)
			}
			total += k
		}
		if matched || !create {
			return t, total, nil
		}
		l, ok := t.(List)
		if !ok || n.pattern == nil && len(l) > 0 {
			return t, total, nil
		}
		var seed Tag
		if n.pattern != nil {
			seed = Clone(n.pattern)
		}
		nv, k, err := apply(seed)
		if err != nil || nv == nil {
			return t, 0, err
		}
		return append(l, nv), k, nil
	}
	return t, total, nil
}

// parentFor is the tag created for a missing parent of n.
func (n pathNode) parentFor() Tag {
	if n.kind == nodeIndex || n.kind == nodeAll {
		return List{}
	}
	return Compound{}
}

func elemCount(t Tag) int {
	switch t := t.(type) {
	case List:
		return len(t)
	case ByteArray:
		return len(t)
	case IntArray:
		return len(t)
	case LongArray:
		return len(t)
	}
	return -1
}

func elemIndex(t Tag, i int) (int, bool) {
	n := elemCount(t)
	if i < 0 {
		i += n
	}
	return i, i >= 0 && i < n
}

func elemAt(t Tag, i int) Tag {
	switch t := t.(type) {
	case List:
		return t[i]
	case ByteArray:
		return Byte(int8(t[i]))
	case IntArray:
		return Int(t[i])
	case LongArray:
		return Long(t[i])
	}
	return nil
}

// setElem sets element i of a list or array to v, in place.
func setElem(t Tag, i int, v Tag) (Tag, error) {
	switch t := t.(type) {
	case List:
		t[i] = v
		return t, nil
	case ByteArray:
		if b, ok := v.(Byte); ok {
			t[i] = byte(b)
			return t, nil
		}
	case IntArray:
		if n, ok := v.(Int); ok {
			t[i] = int32(n)
			return t, nil
		}
	case LongArray:
		if n, ok := v.(Long); ok {
			t[i] = int64(n)
			return t, nil
		}
	}
	return t, fmt.Errorf("%v cannot hold %v", t.Type(), v.Type())
}

// removeElems returns t without the elements drop selects.
func removeElems(t Tag, drop func(int) bool) (Tag, int) {
	n := 0
	keep := func(i int) bool {
		if drop(i) {
			n++
			return false
		}
		return true
	}
	switch t := t.(type) {
	case List:
		out := List{}
		for i, e := range t {
			if keep(i) {
				out = append(out, e)
			}
		}
		return out, n
	case ByteArray:
		out := ByteArray{}
		for i, e := range t {
			if keep(i) {
				out = append(out, e)
			}
		}
		return out, n
	case IntArray:
		out := IntArray{}
		for i, e := range t {
			if keep(i) {
				out = append(out, e)
			}
		}
		return out, n
	case LongArray:
		out := LongArray{}
		for i, e := range t {
			if keep(i) {
				out = append(out, e)
			}
		}
		return out, n
	}
	return t, 0
}

// Matches reports whether t matches pattern the way a path's {k:v} does:
// a compound matches when it has every key of the pattern with a matching
// value, a list when every element of the pattern matches some element of
// it (an empty pattern list only matches an empty list), and other tags
// when they are equal. A nil pattern matches anything.
func Matches(pattern, t Tag) bool {
	if pattern == nil {
		return true
	}
	if t == nil || pattern.Type() != t.Type() {
		return false
	}
	switch p := pattern.(type) {
	case Compound:
		c := t.(Compound)
		for k, v := range p {
			if v == nil {
				continue
			}
			if !Matches(v, c[k]) {
				return false
			}
		}
		return true
	case List:
		l := t.(List)
		if len(p) == 0 {
			return len(l) == 0
		}
		for _, pe := range p {
			found := false
			for _, e := range l {
				if Matches(pe, e) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	}
	return Equal(pattern, t)
}

// Equal reports whether two tags have the same type and value. An empty
// list or array equals a nil one of the same type, and compound entries
// holding nil are ignored, as they are when encoding.
func Equal(a, b Tag) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a.Type() != b.Type() {
		return false
	}
	switch a := a.(type) {
	case Compound:
		b := b.(Compound)
		n := 0
		for k, v := range a {
			if v == nil {
				continue
			}
			n++
			if !Equal(v, b[k]) {
				return false
			}
		}
		for _, v := range b {
			if v != nil {
				n--
			}
		}
		return n == 0
	case List:
		b := b.(List)
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if !Equal(a[i], b[i]) {
				return false
			}
		}
		return true
	case ByteArray:
		return string(a) == string(b.(ByteArray))
	case IntArray:
		b := b.(IntArray)
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	case LongArray:
		b := b.(LongArray)
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	return a == b
}

// Clone returns a deep copy of t, so that changing one leaves the other
// alone.
func Clone(t Tag) Tag {
	switch t := t.(type) {
	case Compound:
		c := make(Compound, len(t))
		for k, v := range t {
			c[k] = Clone(v)
		}
		return c
	case List:
		l := make(List, len(t))
		for i, v := range t {
			l[i] = Clone(v)
		}
		return l
	case ByteArray:
		return append(ByteArray{}, t...)
	case IntArray:
		return append(IntArray{}, t...)
	case LongArray:
		return append(LongArray{}, t...)
	}
	return t
}

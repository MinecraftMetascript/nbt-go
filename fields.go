package nbt

import (
	"reflect"
	"sort"
	"strings"
	"sync"
)

// field describes how one struct field maps to a compound entry.
type field struct {
	name      string
	index     []int
	typ       reflect.Type
	tagged    bool // name came from a struct tag
	omitEmpty bool
	asList    bool
}

type structFields struct {
	list   []field
	byName map[string]int
	byFold map[string]int // lower-cased names, for case-insensitive fallback
}

var fieldCache sync.Map // map[reflect.Type]*structFields

func cachedFields(t reflect.Type) *structFields {
	if f, ok := fieldCache.Load(t); ok {
		return f.(*structFields)
	}
	f, _ := fieldCache.LoadOrStore(t, typeFields(t))
	return f.(*structFields)
}

func parseTag(tag string) (name string, opts []string) {
	parts := strings.Split(tag, ",")
	return parts[0], parts[1:]
}

func hasOpt(opts []string, opt string) bool {
	for _, o := range opts {
		if o == opt {
			return true
		}
	}
	return false
}

// typeFields walks t breadth first, flattening embedded structs the same way
// encoding/json does: a shallower field hides deeper ones with the same name,
// and among fields at the same depth a tagged field wins; otherwise the
// conflicting fields are all dropped.
func typeFields(t reflect.Type) *structFields {
	type queued struct {
		typ   reflect.Type
		index []int
	}
	var (
		current []queued
		next    = []queued{{typ: t}}
		visited = map[reflect.Type]bool{}
		all     []field
	)
	for len(next) > 0 {
		current, next = next, nil
		var level []field
		for _, q := range current {
			if visited[q.typ] {
				continue
			}
			visited[q.typ] = true
			for i := 0; i < q.typ.NumField(); i++ {
				sf := q.typ.Field(i)
				tag := sf.Tag.Get("nbt")
				if tag == "-" {
					continue
				}
				name, opts := parseTag(tag)
				ft := sf.Type
				if sf.Anonymous {
					if ft.Kind() == reflect.Pointer {
						ft = ft.Elem()
					}
					if !sf.IsExported() && ft.Kind() != reflect.Struct {
						continue
					}
					if name == "" && ft.Kind() == reflect.Struct {
						next = append(next, queued{typ: ft, index: appendIndex(q.index, i)})
						continue
					}
				} else if !sf.IsExported() {
					continue
				}
				f := field{
					name:      name,
					index:     appendIndex(q.index, i),
					typ:       sf.Type,
					tagged:    name != "",
					omitEmpty: hasOpt(opts, "omitempty"),
					asList:    hasOpt(opts, "list"),
				}
				if f.name == "" {
					f.name = sf.Name
				}
				level = append(level, f)
			}
		}
		all = append(all, resolveLevel(level, all)...)
	}
	sort.SliceStable(all, func(i, j int) bool {
		return lessIndex(all[i].index, all[j].index)
	})
	sf := &structFields{list: all, byName: map[string]int{}, byFold: map[string]int{}}
	for i, f := range all {
		sf.byName[f.name] = i
		lower := strings.ToLower(f.name)
		if _, dup := sf.byFold[lower]; !dup {
			sf.byFold[lower] = i
		}
	}
	return sf
}

// resolveLevel drops fields hidden by shallower ones and resolves conflicts
// among fields found at the same depth.
func resolveLevel(level, shallower []field) []field {
	taken := map[string]bool{}
	for _, f := range shallower {
		taken[f.name] = true
	}
	groups := map[string][]field{}
	var order []string
	for _, f := range level {
		if taken[f.name] {
			continue
		}
		if _, ok := groups[f.name]; !ok {
			order = append(order, f.name)
		}
		groups[f.name] = append(groups[f.name], f)
	}
	var out []field
	for _, name := range order {
		g := groups[name]
		if len(g) == 1 {
			out = append(out, g[0])
			continue
		}
		var tagged []field
		for _, f := range g {
			if f.tagged {
				tagged = append(tagged, f)
			}
		}
		if len(tagged) == 1 {
			out = append(out, tagged[0])
		}
	}
	return out
}

func appendIndex(index []int, i int) []int {
	out := make([]int, len(index)+1)
	copy(out, index)
	out[len(index)] = i
	return out
}

func lessIndex(a, b []int) bool {
	for i := range a {
		if i >= len(b) {
			return false
		}
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

func sortedKeys(c Compound) []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

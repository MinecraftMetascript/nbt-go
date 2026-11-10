package main

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolKind says what a tool changes, which clients use to decide whether a
// call needs the user's approval.
type toolKind int

const (
	// kindRead tools only report on open structures or the guide.
	kindRead toolKind = iota
	// kindEdit tools change structures held in memory. Nothing outside the
	// server is touched until save_structure, so they are not marked
	// destructive.
	kindEdit
	// kindWrite tools write files, replacing whatever is at the path.
	kindWrite
)

func (k toolKind) annotations(title string) *mcp.ToolAnnotations {
	no, yes := false, true
	a := &mcp.ToolAnnotations{Title: title, OpenWorldHint: &no, DestructiveHint: &no}
	switch k {
	case kindRead:
		a.ReadOnlyHint = true
		a.IdempotentHint = true
	case kindWrite:
		a.DestructiveHint = &yes
		a.IdempotentHint = true
	}
	return a
}

// addTool registers a tool whose input is the struct In.
//
// Two schemas are derived from In. The one sent to clients is kept to the
// plain subset of JSON Schema that every client and model API understands:
// one type per property (no ["null", "array"] unions) and an enum wherever a
// field has an `enum:"a,b,c"` tag. Calls are checked against a looser one
// that leaves option values to the handler, which accepts a few aliases and
// explains what is allowed, and null counts as leaving a field out.
func addTool[In any](srv *mcp.Server, kind toolKind, name, title, description string, h mcp.ToolHandlerFor[In, any]) {
	lenient, err := jsonschema.For[In](nil)
	if err != nil {
		panic(fmt.Sprintf("tool %s: %v", name, err))
	}
	resolved, err := lenient.Resolve(nil)
	if err != nil {
		panic(fmt.Sprintf("tool %s: %v", name, err))
	}
	srv.AddTool(&mcp.Tool{
		Name:        name,
		Title:       title,
		Description: description,
		InputSchema: clientSchema[In](),
		Annotations: kind.annotations(title),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		in, err := decodeArgs[In](resolved, req.Params.Arguments)
		if err != nil {
			return toolError(err), nil
		}
		res, _, err := h(ctx, req, in)
		if err != nil {
			return toolError(err), nil
		}
		return res, nil
	})
}

// toolError reports a failed call as a result the model can read and act
// on, as opposed to a protocol error.
func toolError(err error) *mcp.CallToolResult {
	res := text("%s", err)
	res.IsError = true
	return res
}

func decodeArgs[In any](schema *jsonschema.Resolved, raw json.RawMessage) (In, error) {
	var in In
	if len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage("{}")
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return in, fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	// Some clients send null for every optional field they leave out.
	// Unmarshal already treats null as absent, so validation does too.
	maps.DeleteFunc(generic, func(_ string, v any) bool { return v == nil })
	if err := schema.Validate(generic); err != nil {
		return in, fmt.Errorf("invalid arguments: %w", err)
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return in, fmt.Errorf("invalid arguments: %w", err)
	}
	return in, nil
}

// clientSchema is the input schema advertised for In.
func clientSchema[In any]() *jsonschema.Schema {
	s, err := jsonschema.For[In](nil)
	if err != nil {
		panic(err)
	}
	simplify(s)
	applyFieldTags(s, reflect.TypeFor[In]())
	if p := s.Properties["name"]; p != nil && p.Description == "" {
		p.Description = "name of the open structure"
	}
	return s
}

// simplify removes what schema inference adds that some clients reject or
// that only adds noise: null as an alternative type for slices and pointers,
// and the int32 range on integers.
func simplify(s *jsonschema.Schema) {
	if s == nil {
		return
	}
	if len(s.Types) > 0 {
		var types []string
		for _, t := range s.Types {
			if t != "null" {
				types = append(types, t)
			}
		}
		if len(types) == 1 {
			s.Type, s.Types = types[0], nil
		} else {
			s.Types = types
		}
	}
	if s.Type == "integer" {
		s.Minimum, s.Maximum = nil, nil
	}
	simplify(s.Items)
	simplify(s.AdditionalProperties)
	for _, p := range s.Properties {
		simplify(p)
	}
}

// applyFieldTags sets the enum of each property whose struct field has an
// `enum:"a,b,c"` tag. On a list of strings the enum applies to the items.
func applyFieldTags(s *jsonschema.Schema, t reflect.Type) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	for i := range t.NumField() {
		f := t.Field(i)
		if f.Anonymous {
			applyFieldTags(s, f.Type)
			continue
		}
		tag, ok := f.Tag.Lookup("enum")
		if !ok {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		p := s.Properties[name]
		if p == nil {
			panic(fmt.Sprintf("%s.%s has an enum tag but no property %q", t.Name(), f.Name, name))
		}
		if p.Type == "array" {
			p = p.Items
		}
		for _, v := range strings.Split(tag, ",") {
			p.Enum = append(p.Enum, v)
		}
	}
}

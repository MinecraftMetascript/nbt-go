package main

import (
	"bytes"
	"context"
	"net/url"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Open structures are MCP resources, so a client can show a build, attach
// it to a conversation or download the file without a tool call:
//
//	nbt-mcp://structure/{name}                 a text summary
//	nbt-mcp://structure/{name}/nbt             the structure file
//	nbt-mcp://structure/{name}/render/{view}   a PNG of one view
//
// The list follows the open structures, and clients that subscribe to one
// of these are told when an edit changes it.
const structureURIPrefix = "nbt-mcp://structure/"

// listedView is the render listed for each structure. Others are read
// through the template.
const listedView = "iso_se"

func structureURI(name string, rest ...string) string {
	return structureURIPrefix + strings.Join(append([]string{url.PathEscape(name)}, rest...), "/")
}

// parseStructureURI splits a structure resource URI into the structure's
// name and what is asked of it: "", "nbt" or "render/<view>".
func parseStructureURI(uri string) (name, rest string, ok bool) {
	path, ok := strings.CutPrefix(uri, structureURIPrefix)
	if !ok {
		return "", "", false
	}
	escaped, rest, _ := strings.Cut(path, "/")
	name, err := url.PathUnescape(escaped)
	if err != nil || name == "" {
		return "", "", false
	}
	return name, rest, true
}

// resourceSync keeps the server's resource list in step with the open
// structures.
type resourceSync struct {
	srv *mcp.Server
	s   *state

	listed map[string]bool // structures with resources registered; guarded by s.mu

	mu         sync.Mutex
	subscribed map[string]int // URI to number of subscriptions
}

func (rs *resourceSync) subscribe(_ context.Context, req *mcp.SubscribeRequest) error {
	if _, _, ok := parseStructureURI(req.Params.URI); !ok {
		return nil // guide sections never change
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.subscribed[req.Params.URI]++
	return nil
}

func (rs *resourceSync) unsubscribe(_ context.Context, req *mcp.UnsubscribeRequest) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.subscribed[req.Params.URI] > 1 {
		rs.subscribed[req.Params.URI]--
	} else {
		delete(rs.subscribed, req.Params.URI)
	}
	return nil
}

// changed is called with the state locked after an edit to the named
// structures.
func (rs *resourceSync) changed(names []string) {
	for _, name := range names {
		_, open := rs.s.structures[name]
		uris := []string{structureURI(name), structureURI(name, "nbt"), structureURI(name, "render", listedView)}
		switch {
		case open && !rs.listed[name]:
			rs.listed[name] = true
			rs.srv.AddResource(&mcp.Resource{
				URI: uris[0], Name: name, Title: name,
				Description: "Summary of the open structure " + name + ": size, block counts, entities and jigsaws.",
				MIMEType:    "text/plain",
			}, rs.read)
			rs.srv.AddResource(&mcp.Resource{
				URI: uris[1], Name: name + ".nbt", Title: name + " (structure file)",
				Description: "The open structure " + name + " as a gzip-compressed .nbt structure file, as save_structure would write it now.",
				MIMEType:    "application/octet-stream",
			}, rs.read)
			rs.srv.AddResource(&mcp.Resource{
				URI: uris[2], Name: name + ".png", Title: name + " (image)",
				Description: "The open structure " + name + " drawn from the south-east, above. Other views are at render/{view}.",
				MIMEType:    "image/png",
			}, rs.read)
		case !open && rs.listed[name]:
			delete(rs.listed, name)
			rs.srv.RemoveResources(uris...)
		}

		// Tell subscribers, including those watching other views.
		prefix := structureURI(name)
		rs.mu.Lock()
		var notify []string
		for uri := range rs.subscribed {
			if uri == prefix || strings.HasPrefix(uri, prefix+"/") {
				notify = append(notify, uri)
			}
		}
		rs.mu.Unlock()
		for _, uri := range notify {
			// A client that has gone away is not this edit's problem.
			_ = rs.srv.ResourceUpdated(context.Background(), &mcp.ResourceUpdatedNotificationParams{URI: uri})
		}
	}
}

func (rs *resourceSync) read(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	uri := req.Params.URI
	name, rest, ok := parseStructureURI(uri)
	if !ok {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	rs.s.mu.Lock()
	defer rs.s.mu.Unlock()
	st, open := rs.s.structures[name]
	if !open {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	contents := &mcp.ResourceContents{URI: uri}
	view, isRender := strings.CutPrefix(rest, "render/")
	switch {
	case rest == "":
		contents.MIMEType = "text/plain"
		txt, _ := summarize(st)
		contents.Text = txt
	case rest == "nbt":
		if err := st.Validate(); err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		if err := st.Write(&buf); err != nil {
			return nil, err
		}
		contents.MIMEType = "application/octet-stream"
		contents.Blob = buf.Bytes()
	case isRender:
		if _, known := viewDescriptions[view]; !known {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		data, _, _, err := renderPNG(newGrid(st), sizeBox(st.Size), 0, 800, view)
		if err != nil {
			return nil, err
		}
		contents.MIMEType = "image/png"
		contents.Blob = data
	default:
		return nil, mcp.ResourceNotFoundError(uri)
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{contents}}, nil
}

// addStructureResources registers the templates and starts following the
// state's structures, including any already open.
func addStructureResources(srv *mcp.Server, rs *resourceSync) {
	srv.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: structureURIPrefix + "{name}",
		Name:        "structure", Title: "Structure summary",
		Description: "Summary of an open structure: size, block counts, entities and jigsaws.",
		MIMEType:    "text/plain",
	}, rs.read)
	srv.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: structureURIPrefix + "{name}/nbt",
		Name:        "structure-file", Title: "Structure file",
		Description: "An open structure as a gzip-compressed .nbt structure file.",
		MIMEType:    "application/octet-stream",
	}, rs.read)
	srv.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: structureURIPrefix + "{name}/render/{view}",
		Name:        "structure-image", Title: "Structure image",
		Description: "A PNG of an open structure. view is one of iso_se, iso_sw, iso_nw, iso_ne, top, bottom, south, north, east, west.",
		MIMEType:    "image/png",
	}, rs.read)

	rs.s.mu.Lock()
	defer rs.s.mu.Unlock()
	rs.s.changed = rs.changed
	rs.changed(rs.s.names())
}

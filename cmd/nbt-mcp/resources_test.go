package main

import (
	"bytes"
	"context"
	"image/png"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/MinecraftMetascript/nbt-go/structure"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func structureResources(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var uris []string
	for _, r := range res.Resources {
		if strings.HasPrefix(r.URI, structureURIPrefix) {
			uris = append(uris, r.URI)
		}
	}
	sort.Strings(uris)
	return uris
}

func readResource(t *testing.T, cs *mcp.ClientSession, uri string) *mcp.ResourceContents {
	t.Helper()
	res, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		t.Fatalf("%s: %v", uri, err)
	}
	if len(res.Contents) != 1 {
		t.Fatalf("%s: %d contents", uri, len(res.Contents))
	}
	return res.Contents[0]
}

func TestStructureResources(t *testing.T) {
	cs := connect(t, t.TempDir())
	if got := structureResources(t, cs); len(got) != 0 {
		t.Fatalf("resources before any structure: %v", got)
	}
	call(t, cs, "create_structure", map[string]any{"name": "my hut"}, false)
	call(t, cs, "fill", map[string]any{"name": "my hut", "from": []int{0, 0, 0}, "to": []int{2, 1, 2}, "block": "oak_planks"}, false)

	base := "nbt-mcp://structure/my%20hut"
	want := []string{base, base + "/nbt", base + "/render/iso_se"}
	if got := structureResources(t, cs); !slices.Equal(got, want) {
		t.Fatalf("resources = %v, want %v", got, want)
	}

	if c := readResource(t, cs, base); c.MIMEType != "text/plain" || !strings.Contains(c.Text, "18  minecraft:oak_planks") {
		t.Errorf("summary: %+v", c)
	}
	st, err := structure.Read(bytes.NewReader(readResource(t, cs, base+"/nbt").Blob))
	if err != nil || len(st.Blocks) != 18 {
		t.Errorf("nbt resource: %v, %v", st, err)
	}
	// Views that aren't listed are readable through the template.
	for _, view := range []string{"iso_se", "top", "north"} {
		c := readResource(t, cs, base+"/render/"+view)
		if img, err := png.Decode(bytes.NewReader(c.Blob)); err != nil || c.MIMEType != "image/png" || img.Bounds().Dx() == 0 {
			t.Errorf("%s render: %v", view, err)
		}
	}
	for _, uri := range []string{base + "/render/sideways", base + "/other", "nbt-mcp://structure/nope"} {
		if _, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri}); err == nil {
			t.Errorf("reading %s succeeded", uri)
		}
	}

	// Undoing the creation closes the structure and unlists it.
	call(t, cs, "undo", map[string]any{"steps": 2}, false)
	if got := structureResources(t, cs); len(got) != 0 {
		t.Errorf("resources after undo: %v", got)
	}

	tpl, err := cs.ListResourceTemplates(context.Background(), nil)
	if err != nil || len(tpl.ResourceTemplates) != 3 {
		t.Errorf("templates: %v, %v", tpl, err)
	}
}

func TestResourceSubscriptions(t *testing.T) {
	ctx := context.Background()
	srv := newServer(newState(t.TempDir(), defaultDataVersion))
	ct, st := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	updated := make(chan string, 10)
	listChanged := make(chan struct{}, 10)
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, &mcp.ClientOptions{
		ResourceUpdatedHandler: func(_ context.Context, req *mcp.ResourceUpdatedNotificationRequest) {
			updated <- req.Params.URI
		},
		ResourceListChangedHandler: func(context.Context, *mcp.ResourceListChangedRequest) {
			listChanged <- struct{}{}
		},
	})
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	wait := func(what string, ch <-chan string) string {
		t.Helper()
		select {
		case v := <-ch:
			return v
		case <-time.After(5 * time.Second):
			t.Fatalf("no %s notification", what)
			return ""
		}
	}

	call(t, cs, "create_structure", map[string]any{"name": "a"}, false)
	select {
	case <-listChanged:
	case <-time.After(5 * time.Second):
		t.Fatal("no list changed notification")
	}

	// Subscribing to a view that isn't listed works too.
	uri := structureURI("a", "render", "top")
	if err := cs.Subscribe(ctx, &mcp.SubscribeParams{URI: uri}); err != nil {
		t.Fatal(err)
	}
	call(t, cs, "create_structure", map[string]any{"name": "other"}, false)
	call(t, cs, "fill", map[string]any{"name": "a", "from": []int{0, 0, 0}, "to": []int{1, 1, 1}, "block": "stone"}, false)
	if got := wait("resource updated", updated); got != uri {
		t.Errorf("updated %s, want %s", got, uri)
	}

	if err := cs.Unsubscribe(ctx, &mcp.UnsubscribeParams{URI: uri}); err != nil {
		t.Fatal(err)
	}
	// Unsubscribing takes effect asynchronously, so edit until one goes
	// unannounced.
	deadline := time.Now().Add(5 * time.Second)
	for {
		call(t, cs, "fill", map[string]any{"name": "a", "from": []int{0, 0, 0}, "to": []int{1, 1, 1}, "block": "dirt"}, false)
		select {
		case got := <-updated:
			if time.Now().After(deadline) {
				t.Fatalf("still notified about %s after unsubscribing", got)
			}
			continue
		case <-time.After(200 * time.Millisecond):
		}
		break
	}
}

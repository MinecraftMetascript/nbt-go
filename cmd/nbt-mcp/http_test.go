package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func connectHTTP(t *testing.T, url string, hc *http.Client) (*mcp.ClientSession, error) {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	cs, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: url, HTTPClient: hc}, nil)
	if err == nil {
		t.Cleanup(func() { _ = cs.Close() })
	}
	return cs, err
}

// serve starts an HTTP server that is shut down after the test's clients,
// which are closed by cleanups registered later.
func serve(t *testing.T, s *state, token string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(httpHandler(s, token))
	t.Cleanup(func() {
		ts.CloseClientConnections()
		ts.Close()
	})
	return ts
}

func TestHTTP(t *testing.T) {
	dir := t.TempDir()
	st := newState(dir, defaultDataVersion)
	st.restrict = true
	ts := serve(t, st, "")

	a, err := connectHTTP(t, ts.URL+"/mcp", nil)
	if err != nil {
		t.Fatal(err)
	}
	call(t, a, "create_structure", map[string]any{"name": "hut"}, false)
	call(t, a, "fill", map[string]any{"name": "hut", "from": []int{0, 0, 0}, "to": []int{2, 2, 2}, "block": "oak_planks"}, false)

	// A second client sees the same structures.
	b, err := connectHTTP(t, ts.URL+"/mcp", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := call(t, b, "list_structures", nil, false); !strings.Contains(got, "hut") {
		t.Errorf("second session doesn't see the structure: %s", got)
	}

	call(t, b, "save_structure", map[string]any{"name": "hut", "path": "out/hut"}, false)
	if _, err := os.Stat(filepath.Join(dir, "out", "hut.nbt")); err != nil {
		t.Error(err)
	}
}

func TestHTTPToken(t *testing.T) {
	ts := serve(t, newState(t.TempDir(), defaultDataVersion), "s3cret")

	if _, err := connectHTTP(t, ts.URL+"/mcp", nil); err == nil {
		t.Error("connected without a token")
	}
	hc := &http.Client{Transport: bearer("s3cret")}
	cs, err := connectHTTP(t, ts.URL+"/mcp", hc)
	if err != nil {
		t.Fatal(err)
	}
	call(t, cs, "list_structures", nil, false)
}

type bearer string

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+string(b))
	return http.DefaultTransport.RoundTrip(r)
}

func TestHTTPRefusesOtherOrigins(t *testing.T) {
	ts := serve(t, newState(t.TempDir(), defaultDataVersion), "")
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("cross-origin request got %s, want 403", res.Status)
	}
}

func TestRestrictPaths(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	st := newState(dir, defaultDataVersion)
	st.restrict = true
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Skip("no symlinks:", err)
	}
	for _, p := range []string{"a.nbt", "sub/dir/a.nbt", filepath.Join(dir, "a.nbt")} {
		if _, err := st.resolve(p); err != nil {
			t.Errorf("resolve(%q): %v", p, err)
		}
	}
	for _, p := range []string{"../a.nbt", "sub/../../a.nbt", filepath.Join(outside, "a.nbt"), "link/a.nbt", "link/new/a.nbt", "~/a.nbt"} {
		if got, err := st.resolve(p); err == nil {
			t.Errorf("resolve(%q) = %q, want an error", p, got)
		}
	}

	// Through the tools: nothing is written or read outside.
	cs := connectState(t, st)
	call(t, cs, "create_structure", map[string]any{"name": "s"}, false)
	call(t, cs, "set_blocks", map[string]any{"name": "s", "blocks": []map[string]any{{"x": 0, "y": 0, "z": 0, "block": "stone"}}}, false)
	if msg := call(t, cs, "save_structure", map[string]any{"name": "s", "path": filepath.Join(outside, "s")}, true); !strings.Contains(msg, "outside") {
		t.Errorf("got %q", msg)
	}
	call(t, cs, "load_structure", map[string]any{"name": "t", "path": "../s.nbt"}, true)
	call(t, cs, "assemble_jigsaw", map[string]any{"into": "v", "start": "s", "datapack": outside}, true)
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("wrote outside the directory: %v", entries)
	}
}

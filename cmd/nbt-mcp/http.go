package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// tokenEnv names the environment variable holding the bearer token HTTP
// clients must send. It is read from the environment and not a flag so it
// doesn't show up in process listings.
const tokenEnv = "NBT_MCP_TOKEN"

// httpHandler serves MCP over Streamable HTTP at /mcp.
//
// Every session shares the same open structures, so two clients connected
// to one server can work on the same build. Requests from web pages on
// other origins are refused, and the SDK refuses requests that reach a
// loopback address under another host name (DNS rebinding). If token is
// set, requests must carry it as a bearer token.
func httpHandler(s *state, token string) http.Handler {
	srv := newServer(s)
	var h http.Handler = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{SessionTimeout: 30 * time.Minute})
	h = http.NewCrossOriginProtection().Handler(h)
	if token != "" {
		h = requireToken(token, h)
	}
	mux := http.NewServeMux()
	mux.Handle("/mcp", h)
	return mux
}

func requireToken(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "missing or wrong bearer token", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// serveHTTP serves until ctx is done.
func serveHTTP(ctx context.Context, s *state, addr, token string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	log.Printf("nbt-mcp %s serving MCP at http://%s/mcp, files in %s", version(), ln.Addr(), s.dir)
	if host, _, _ := net.SplitHostPort(addr); token == "" && !isLoopback(host) {
		log.Printf("warning: listening beyond this machine without a token; set %s so only your clients can use it", tokenEnv)
	}
	hs := &http.Server{Handler: httpHandler(s, token), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := hs.Shutdown(shut); err != nil {
			_ = hs.Close() // open event streams don't end on their own
		}
	}()
	if err := hs.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

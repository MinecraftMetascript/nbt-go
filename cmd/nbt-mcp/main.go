// Command nbt-mcp is a Model Context Protocol server for building Minecraft
// Java Edition structure files (.nbt).
//
// It keeps structures in memory while a model edits them with tools such as
// set_blocks, fill and render_layer, then writes them out with
// save_structure. The saved files can be loaded with a structure block, the
// /place template command, or a data pack.
//
// Install it with
//
//	go install github.com/MinecraftMetascript/nbt-go/cmd/nbt-mcp@latest
//
// and register it with an MCP client as a stdio server, for example:
//
//	claude mcp add minecraft -- nbt-mcp -dir ~/minecraft/structures
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// defaultDataVersion is Minecraft 26.3, the latest release. Files written for
// it use the 26.3 palette keys and won't load in older games; pass
// -data-version 4189 (1.21.4) or similar to target an older one.
const defaultDataVersion = 5023

func main() {
	dir := flag.String("dir", ".", "directory that relative save and load paths resolve against")
	dataVersion := flag.Int("data-version", defaultDataVersion, "DataVersion for new structures (the Minecraft version they target; 4189 is 1.21.4, 5023 is 26.3)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [flags]\n\nRuns an MCP server on stdin/stdout for building Minecraft structure files.\n\nFlags:\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Println(version())
		return
	}

	if err := run(*dir, int32(*dataVersion)); err != nil {
		log.Fatal(err)
	}
}

// run serves MCP on stdin and stdout until the client disconnects or the
// process is asked to stop.
func run(dir string, dataVersion int32) error {
	srv := newServer(newState(dir, dataVersion))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

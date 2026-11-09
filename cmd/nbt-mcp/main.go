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

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// defaultDataVersion is Minecraft 1.21.4. An older DataVersion is the safe
// default: newer games upgrade older structure data when loading it, but an
// older game cannot read data from the future.
const defaultDataVersion = 4189

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

	srv := newServer(newState(*dir, int32(*dataVersion)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}

func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

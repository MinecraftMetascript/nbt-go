# nbt-mcp

An MCP server that lets an AI model build Minecraft Java Edition structure files (`.nbt`): the files structure blocks save, and that `/place template` and data packs load.

The model works on structures held in memory. It places blocks with fills, shapes and text grids, looks at the result as text or rendered images, checks it with a linter and a style critique, and saves it as a file the game can load. It also understands jigsaw structures such as villages.

Contents: [Install](#install) · [Add it to a client](#add-it-to-a-client) · [Use the files in a world](#use-the-files-in-a-world) · [Tools](#tools) · [Resources](#resources) · [Seeing the structure](#seeing-the-structure) · [Building guide and critique](#building-guide-and-critique) · [Jigsaw structures](#jigsaw-structures) · [Flags](#flags) · [Compatibility](#compatibility)

## Install

With Go 1.25 or newer:

```sh
go install github.com/MinecraftMetascript/nbt-go/cmd/nbt-mcp@latest
```

This puts `nbt-mcp` in `$(go env GOPATH)/bin`, which should be on your `PATH`. Older Go toolchains download the right version automatically.

Without Go, download the archive for your system from the [releases page](https://github.com/MinecraftMetascript/nbt-go/releases), unpack it, and put `nbt-mcp` somewhere on your `PATH`.

## Add it to a client

The server speaks MCP over stdin/stdout by default, which every client that starts local servers supports. If the client can't find the command, use the full path from `which nbt-mcp`.

**Claude Code**

```sh
claude mcp add minecraft -- nbt-mcp -dir ~/minecraft/structures
```

**Claude Desktop, Cursor, Gemini CLI and other clients with an `mcpServers` config** (`claude_desktop_config.json`, `~/.cursor/mcp.json` or `.cursor/mcp.json`, `~/.gemini/settings.json`):

```json
{
  "mcpServers": {
    "minecraft": {
      "command": "nbt-mcp",
      "args": ["-dir", "/path/to/structures"]
    }
  }
}
```

**VS Code** (`.vscode/mcp.json`):

```json
{
  "servers": {
    "minecraft": {
      "type": "stdio",
      "command": "nbt-mcp",
      "args": ["-dir", "/path/to/structures"]
    }
  }
}
```

**Codex CLI** (`~/.codex/config.toml`):

```toml
[mcp_servers.minecraft]
command = "nbt-mcp"
args = ["-dir", "/path/to/structures"]
```

### Over HTTP

For clients that connect to a URL instead of starting a program, run the server yourself:

```sh
nbt-mcp -http localhost:8765 -dir ~/minecraft/structures
```

and point the client at `http://localhost:8765/mcp` (Streamable HTTP). In Claude Code that is `claude mcp add --transport http minecraft http://localhost:8765/mcp`.

In this mode:

- The server only reads and writes files inside `-dir`. Paths that leave it, directly or through a symlink, are refused.
- All connected clients share the same open structures, so two clients can work on one build.
- Requests from web pages on other sites are refused.
- There is no login. Keep it on `localhost`, or set the `NBT_MCP_TOKEN` environment variable so clients must send that value as a bearer token (`Authorization: Bearer ...`). Put it behind a TLS proxy if it is reachable from other machines.

## Use the files in a world

Save to `<world>/generated/<namespace>/structure/<name>.nbt` (the folder is `structures` before 1.21). Then load it with a structure block in Load mode, or run `/place template <namespace>:<name>`. Structure blocks can only load up to 48 blocks on each axis, but `/place template` has no such limit.

## Tools

**Structures**

| Tool | What it does |
| --- | --- |
| `create_structure` | Start an empty structure in memory, optionally with a specific `data_version`. |
| `load_structure` | Open an existing `.nbt` file. |
| `list_structures` | List the open structures. |
| `save_structure` | Write a gzip-compressed `.nbt` file. |
| `undo`, `redo` | Step back and forward through edits. See [Undo](#undo). |

**Placing blocks**

| Tool | What it does |
| --- | --- |
| `set_blocks` | Place a batch of blocks, each with optional block entity SNBT (chest loot and so on). |
| `fill` | Fill a box, like `/fill`, in `replace`, `hollow` or `outline` mode. |
| `fill_ellipsoid` | A sphere or ellipsoid, solid or as a watertight shell. |
| `fill_cylinder` | A cylinder along any axis: solid, shell, open tube or ring, or a disc. |
| `fill_tube` | A rounded, optionally tapering tube along a path of points (trunks, roots, pipes). |
| `place_grid` | Place many blocks from text layers and a legend, the inverse of `render_layer`. |
| `decorate_surface` | Put a block on, or swap in for, the exposed faces of solid blocks (moss, snow, vines, grass). |
| `remove_blocks` | Clear a box back to "no block", so placing the structure leaves the world unchanged there. |
| `add_entity` | Add an entity with optional SNBT. |

**Reshaping**

| Tool | What it does |
| --- | --- |
| `transform_structure` | Rotate and/or mirror a structure in place, turning block states to match. |
| `paste_structure` | Copy one structure into another, optionally rotated or mirrored. |
| `crop_structure` | Cut a structure down to a box. |
| `set_size` | Set the size explicitly, to add margin or trim. |

**Looking**

| Tool | What it does |
| --- | --- |
| `describe_structure` | Size, share of blocks, air and empty space, block counts by type, entities, and jigsaw blocks with warnings. |
| `render_image` | PNG images from several viewpoints in one call. See [Seeing the structure](#seeing-the-structure). |
| `render_view` | A text projection from the top, bottom or any side: the first block along each line of sight, or its depth. |
| `render_layer` | A top-down text drawing of one y layer. |
| `diff_structures` | Compare two structures: what changed, grouped by kind, with positions. |

**Checking**

| Tool | What it does |
| --- | --- |
| `lint_structure` | Check for common mistakes. See [Lint checks](#lint-checks). |
| `critique_structure` | Score how hand-crafted a build looks and list the fixes worth most. See [Building guide and critique](#building-guide-and-critique). |
| `check_sealed` | Flood-fill from a point to check that a space is watertight, and find the gap if it isn't. Closed doors, trapdoors and slabs hold water; `against=gaps` checks for any non-full block except closed doors. |
| `building_guide` | The building guide: principles, survival and creative styles, jigsaw design, and palettes by theme. |

**Jigsaw**

| Tool | What it does |
| --- | --- |
| `place_jigsaw` | Place a jigsaw block with typed settings. `final_state` defaults to the block it replaces. |
| `check_jigsaw_fit` | Check where a child piece lands on a parent's jigsaws in each rotation, and whether it fits. |
| `assemble_jigsaw` | Assemble a whole jigsaw layout from a start piece and template pools, and open the result. Large layouts get a summary; `seeds` reports which templates never get placed. |

Blocks use command syntax (`oak_stairs[facing=east,half=bottom]`), and the `minecraft:` namespace is optional. Wherever a tool fills an area, the block can also be a weighted mix such as `70%stone_bricks,20%cracked_stone_bricks,10%mossy_stone_bricks` (`3*stone,1*andesite` works too). Every random tool takes a `seed` and reports the seed it used, so a result can be reproduced.

Hollow shapes are watertight: every inside position is cut off from the outside on all six faces, so water and mobs can't get through diagonal gaps. By default the inside is filled with air, which clears out water or terrain when the structure is placed.

### Lint checks

`lint_structure` runs all of these unless `checks` picks some. They are name-based rules of thumb, since the server has no block registry, so treat findings as things to look at.

| Check | Finds |
| --- | --- |
| `format` | Problems with the file's structure: palette, block positions, entities. |
| `renamed` | Block ids that were renamed between game versions. |
| `newer` | Blocks added in a game version newer than the file's DataVersion. |
| `connections` | Walls, fences, panes and bars whose connections don't match their neighbours. |
| `support` | Torches, lanterns, vines, signs, buttons and other blocks with nothing to attach to. |
| `gravity` | Sand, gravel, concrete powder and other falling blocks with nothing under them. |
| `halves` | Doors, beds and tall plants missing their other half. |
| `fire` | Fire and lava next to blocks that burn. |
| `unsupported` | Solid blocks with nothing anywhere below them, such as legs hanging past the edge of a plinth. |
| `floating` | Groups of blocks that don't touch the bottom layer or anything connected to it. |
| `containers` | Chests, barrels and other containers with no loot table or items. |
| `valuables` | High-value blocks such as gold, diamond and netherite blocks. |
| `jigsaw` | Jigsaw block settings. |

### Undo

Every tool call that changes a structure is one step of history: a fill, a paste, a crop, creating or loading a structure. `undo` takes back the latest step, or several with `steps`, and `redo` puts them back until the next new edit. Files already saved are not touched.

An edit that fails changes nothing, even if it had placed some blocks before running into the problem.

The history keeps the last 100 steps, fewer when the structures are very large.

### Structured results

`describe_structure`, `lint_structure`, `critique_structure`, `diff_structures`, `undo` and `redo` return their result twice: as text for the model, and as structured content with a declared output schema for clients and scripts. A lint finding, for example, comes with its check, position and message as separate fields.

## Resources

Open structures are MCP resources, so a client can show a build, attach it to a conversation or download the file without a tool call.

| URI | Content |
| --- | --- |
| `nbt-mcp://structure/{name}` | A text summary, the same as `describe_structure`. |
| `nbt-mcp://structure/{name}/nbt` | The structure file, as `save_structure` would write it now. Over HTTP this is how a client gets the file. |
| `nbt-mcp://structure/{name}/render/{view}` | A PNG of one view: `iso_se`, `iso_sw`, `iso_nw`, `iso_ne`, `top`, `bottom`, `south`, `north`, `east` or `west`. |
| `nbt-mcp://guide/{topic}` | A section of the building guide. |

The resource list follows the open structures (each lists its summary, file and `iso_se` image), and the server sends a list-changed notification when one opens or closes. Clients can subscribe to any structure URI, including views that aren't listed, and are notified when an edit changes that structure.

## Seeing the structure

`render_image` lets a model look at what it built. It returns one PNG per view, each with a caption saying where the view is from:

- `iso_se`, `iso_sw`, `iso_nw`, `iso_ne`: isometric from above each corner.
- `top` (plan) and `bottom`.
- `south` (the front), `north`, `east`, `west`: straight-on elevations.
- Presets: `corners` (all four isometric views), `elevations` (all four sides), `overview` (`iso_se`, `iso_nw`, `top`, `south`, `east`) and `all` (nine views).

To see inside, `cutaway_y` hides everything above a height. With `top` that gives a floor plan, and with an isometric view a cutaway. `from`/`to` limit the render to a box, which gives a cross-section from any side.

Colors approximate each block's map color; there are no textures. Slabs, stairs, fences, walls, panes, doors, trapdoors, carpets, torches, lanterns, plants and other partial blocks are drawn with their rough shape. Glass, ice and water are see-through. For exact positions, use the text views (`render_view`, `render_layer`).

## Building guide and critique

Models tend to build boxes: flat walls of one block, a flat roof, no small blocks, perfect symmetry. The server ships a building guide to steer them away from that, and a critique tool to check the result.

- **Guide.** `building_guide` returns the guide as text; the same sections are MCP resources (`nbt-mcp://guide/building`, `.../survival`, `.../creative`, `.../jigsaw`, `.../palettes`). It covers silhouette, depth (frame and set-back infill, overhangs), palette gradients, detailing with partial blocks, asymmetry and interiors, plus palettes per theme (medieval, castle, nordic, desert, swamp, ruined, nether). The jigsaw section covers designing pieces and pools that vary well: grid modules, connector naming, variants and slots, jitter with several identical jigsaws, fallback terminators, priorities, terrain blending and processors for weathering.
- **Styles.** *Survival* builds are ones a player could make: common, biome-appropriate materials, 3 to 10 block types, detail where it counts. *Creative* builds are showpieces: rich palettes with texture gradients, dense detail and complex shapes.
- **Prompt.** The `build` prompt (a slash command in Claude Code: `/minecraft:build`) takes a description, style, theme, name and whether it is a jigsaw structure, and walks the model through plan, block out, depth, detail, interior, critique and lint, with the guide attached.
- **Critique.** `critique_structure` looks at what is visible from outside and scores each category from 0 to 100 against the style's targets, then lists the fixes worth most with coordinates. Use `from` and `to` to critique one building in a larger structure.

## Jigsaw structures

The jigsaw tools follow the game's placement code (`JigsawPlacement`) for rigid pieces. The rules that most often cause surprises:

- **Attachment.** A child's jigsaw lands in the block directly in front of the parent's jigsaw, so the two jigsaw blocks end up adjacent and facing each other. The parent's `target` must equal the child jigsaw's `name`. Up and down jigsaws with an `aligned` joint also need matching tops; `rollable` lets the child spin.
- **Connecting inside a piece.** If a jigsaw faces into its own piece's bounding box (a statue on a plinth whose box includes the air above it), the whole child must fit inside that box, minus anything else already placed inside it. Otherwise the game quietly places nothing. `check_jigsaw_fit` shows how far a child sticks out.
- **No overlaps.** Children that connect outside their parent must not overlap any other piece, and must stay within `max_distance_from_center` of the start piece.
- **Final state.** After generation each jigsaw is replaced by its `final_state`. The game's default is air, which leaves a hole in a floor or wall. `place_jigsaw` defaults it to the block the jigsaw replaces.
- **Start height.** With `project_start_to_heightmap` (as villages use), the start piece's bottom layer (y=0) is placed at the height of the topmost terrain block, replacing it. So y=0 is sunk one block into the ground and y=1 is the first layer above it. Build y=0 as a foundation, or set `start_height` to 1.

`assemble_jigsaw` reads template pools from a datapack folder (`data/<ns>/worldgen/template_pool/`) or from inline JSON in the same format, and finds pieces among the open structures or in the datapack's `data/<ns>/structure/` folder. It ignores terrain, processors, and `terrain_matching` height changes, so use it to check how pieces connect, not how they sit on the ground.

## Flags

| Flag              | Default | Meaning                                                                                                      |
| ----------------- | ------- | ------------------------------------------------------------------------------------------------------------ |
| `-dir`            | `.`     | Directory that relative paths in `save_structure` and `load_structure` resolve against.                      |
| `-data-version`   | `5023`  | DataVersion for new structures (`5023` is 26.3, `4189` is 1.21.4). Newer game versions upgrade older data but older ones can't read newer data, so lower it to target an older game. |
| `-http`           |         | Serve MCP over HTTP on this address, such as `localhost:8765`, instead of stdin/stdout. The endpoint is `/mcp`. |
| `-restrict-paths` | off     | Only read and write files inside `-dir`. Always on with `-http`.                                              |
| `-version`        |         | Print the version and exit.                                                                                  |

## Compatibility

Tool input schemas stick to the plain subset of JSON Schema that MCP clients and model APIs share: one type per property, enums for options, no `anyOf`, `oneOf` or `$ref`. Every tool has a title and annotations, so clients can tell the read-only tools (rendering, describing, linting) from the ones that edit a structure in memory and from `save_structure`, which writes a file.

Calls are forgiving. `null` for an optional field counts as leaving it out, and mistakes come back as tool errors that say what is allowed.

The server is built on the official Go SDK and negotiates whichever protocol version the client asks for, up to 2026-07-28.

## License

MIT. See [LICENSE](LICENSE).

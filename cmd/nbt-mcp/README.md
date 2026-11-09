# nbt-mcp

An MCP server that lets a model build Minecraft Java Edition structure files (`.nbt`).

## Install

```sh
go install github.com/MinecraftMetascript/nbt-go/cmd/nbt-mcp@latest
```

This puts `nbt-mcp` in `$(go env GOPATH)/bin`, which should be on your `PATH`. It needs Go 1.25 or newer; older Go toolchains download the right version automatically.

## Add it to a client

Claude Code:

```sh
claude mcp add minecraft -- nbt-mcp -dir ~/minecraft/structures
```

Claude Desktop, or any client that uses an `mcpServers` config:

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

If the client can't find the command, use the full path from `which nbt-mcp`.

## Flags

| Flag            | Default | Meaning                                                                                                      |
| --------------- | ------- | ------------------------------------------------------------------------------------------------------------ |
| `-dir`          | `.`     | Directory that relative paths in `save_structure` and `load_structure` resolve against.                      |
| `-data-version` | `4189`  | DataVersion for new structures (`4189` is 1.21.4). Newer game versions upgrade older data, so a lower value is the safer default. |
| `-version`      |         | Print the version and exit.                                                                                  |

## Tools

| Tool                 | What it does                                                                         |
| -------------------- | ------------------------------------------------------------------------------------ |
| `create_structure`   | Start an empty structure in memory, optionally with a specific `data_version`.       |
| `load_structure`     | Open an existing `.nbt` file.                                                        |
| `list_structures`    | List the open structures.                                                            |
| `set_blocks`         | Place a batch of blocks, each with optional block entity SNBT (chest loot and so on). |
| `fill`               | Fill a box, like `/fill`, in `replace`, `hollow` or `outline` mode.                  |
| `fill_ellipsoid`     | A sphere or ellipsoid, solid or as a watertight shell.                               |
| `fill_cylinder`      | A cylinder along any axis: solid, shell, open tube or ring, or a disc.               |
| `fill_tube`          | A rounded, optionally tapering tube along a path of points (trunks, roots, pipes).   |
| `decorate_surface`   | Put a block on, or swap in for, the exposed faces of solid blocks (moss, snow, vines, grass). |
| `place_grid`         | Place many blocks from text layers and a legend, the inverse of `render_layer`.      |
| `check_sealed`       | Flood-fill from a point to check that a space is watertight, and find the gap if it isn't. |
| `transform_structure`| Rotate and/or mirror a structure in place, turning block states to match.            |
| `paste_structure`    | Copy one structure into another, optionally rotated or mirrored.                     |
| `crop_structure`     | Cut a structure down to a box.                                                       |
| `set_size`           | Set the size explicitly, to add margin or trim.                                      |
| `remove_blocks`      | Clear a box back to "no block", so placing the structure leaves the world unchanged there. |
| `add_entity`         | Add an entity with optional SNBT.                                                    |
| `describe_structure` | Size, block counts by type, entities, and jigsaw blocks with warnings.               |
| `render_layer`       | A top-down text drawing of one y layer, so the model can check its work.            |
| `lint_structure`     | Check for common mistakes: format, renamed ids, wall and fence connections, missing support, overhangs, floating parts, empty containers, valuable blocks, jigsaw settings. |
| `render_view`        | A text projection from the top, bottom or any side: the first block along each line of sight, or its depth. |
| `render_image`       | A PNG render, isometric or from one side, with an approximate flat color per block.  |
| `place_jigsaw`       | Place a jigsaw block with typed settings. `final_state` defaults to the block it replaces. |
| `check_jigsaw_fit`   | Check where a child piece lands on a parent's jigsaws in each rotation, and whether it fits. |
| `assemble_jigsaw`    | Assemble a whole jigsaw layout from a start piece and template pools, and open the result. |
| `save_structure`     | Write a gzip-compressed `.nbt` file.                                                 |

Blocks use command syntax (`oak_stairs[facing=east,half=bottom]`), and the `minecraft:` namespace is optional. Wherever a tool fills an area, the block can also be a weighted mix such as `70%stone_bricks,20%cracked_stone_bricks,10%mossy_stone_bricks` (`3*stone,1*andesite` works too). Every random tool takes a `seed` and reports the seed it used, so a result can be reproduced.

Hollow shapes are watertight: every inside position is cut off from the outside on all six faces, so water and mobs can't get through diagonal gaps. By default the inside is filled with air, which clears out water or terrain when the structure is placed.

## Jigsaw structures

The jigsaw tools follow the game's placement code (`JigsawPlacement`) for rigid pieces. The rules that most often cause surprises:

- **Attachment.** A child's jigsaw lands in the block directly in front of the parent's jigsaw, so the two jigsaw blocks end up adjacent and facing each other. The parent's `target` must equal the child jigsaw's `name`. Up and down jigsaws with an `aligned` joint also need matching tops; `rollable` lets the child spin.
- **Connecting inside a piece.** If a jigsaw faces into its own piece's bounding box (a statue on a plinth whose box includes the air above it), the whole child must fit inside that box, minus anything else already placed inside it. Otherwise the game quietly places nothing. `check_jigsaw_fit` shows how far a child sticks out.
- **No overlaps.** Children that connect outside their parent must not overlap any other piece, and must stay within `max_distance_from_center` of the start piece.
- **Final state.** After generation each jigsaw is replaced by its `final_state`. The game's default is air, which leaves a hole in a floor or wall. `place_jigsaw` defaults it to the block the jigsaw replaces.
- **Start height.** With `project_start_to_heightmap` (as villages use), the start piece's bottom layer (y=0) is placed at the height of the topmost terrain block, replacing it. So y=0 is sunk one block into the ground and y=1 is the first layer above it. Build y=0 as a foundation, or set `start_height` to 1.

`assemble_jigsaw` reads template pools from a datapack folder (`data/<ns>/worldgen/template_pool/`) or from inline JSON in the same format, and finds pieces among the open structures or in the datapack's `data/<ns>/structure/` folder. It ignores terrain, processors, and `terrain_matching` height changes, so use it to check how pieces connect, not how they sit on the ground.

## Using the files in a world

Save to `<world>/generated/<namespace>/structure/<name>.nbt` (the folder is `structures` before 1.21). Then load it with a structure block in Load mode, or run `/place template <namespace>:<name>`. Structure blocks can only load up to 48 blocks on each axis, but `/place template` has no such limit.

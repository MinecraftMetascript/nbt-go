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
| `remove_blocks`      | Clear a box back to "no block", so placing the structure leaves the world unchanged there. |
| `add_entity`         | Add an entity with optional SNBT.                                                    |
| `describe_structure` | Size, block counts by type, and entities.                                            |
| `render_layer`       | A top-down text drawing of one y layer, so the model can check its work.            |
| `save_structure`     | Write a gzip-compressed `.nbt` file.                                                 |

Blocks use command syntax (`oak_stairs[facing=east,half=bottom]`), and the `minecraft:` namespace is optional.

## Using the files in a world

Save to `<world>/generated/<namespace>/structure/<name>.nbt` (the folder is `structures` before 1.21). Then load it with a structure block in Load mode, or run `/place template <namespace>:<name>`. Structure blocks can only load up to 48 blocks on each axis, but `/place template` has no such limit.

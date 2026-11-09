# nbt-go

A Go library for Minecraft's [NBT](https://minecraft.wiki/w/NBT_format) format, modeled on `encoding/json`.

```sh
go get github.com/MinecraftMetascript/nbt-go
```

- `Marshal` / `Unmarshal`, `Encoder` / `Decoder`, and `nbt:"name,omitempty"` struct tags, the same way `encoding/json` does it.
- A `Tag` tree (`Compound`, `List`, `Int`, …) for data whose shape you don't know ahead of time, like `map[string]any` for JSON.
- Gzip and zlib compressed input is detected automatically, and `Encoder.SetCompression` writes it.
- SNBT (the `{id:"minecraft:stone",count:1b}` syntax used in commands) both ways.
- The `structure` package reads, writes and builds Java Edition structure (`.nbt`) files.

## Usage

```go
type Item struct {
	Slot  int8   `nbt:"Slot"`
	ID    string `nbt:"id"`
	Count int32  `nbt:"count"`
}

type Chest struct {
	CustomName string `nbt:"CustomName,omitempty"`
	Items      []Item `nbt:"Items"`
}

data, err := nbt.Marshal(Chest{Items: []Item{{0, "minecraft:diamond", 3}}})

var chest Chest
err = nbt.Unmarshal(data, &chest)
```

Reading and writing files:

```go
f, _ := os.Open("level.dat")
dec := nbt.NewDecoder(f) // gzip is detected
var level struct {
	Data struct {
		LevelName string
		SpawnX, SpawnY, SpawnZ int32
	}
}
err := dec.Decode(&level)

enc := nbt.NewEncoder(out)
enc.SetCompression(nbt.Gzip)
err = enc.Encode(level)
```

Unknown data, or data you want to pass through untouched:

```go
var tree nbt.Tag // or nbt.Compound, or any
nbt.Unmarshal(data, &tree)
fmt.Println(tree) // prints SNBT: {Items:[{Slot:0b,count:3,id:"minecraft:diamond"}]}
```

SNBT:

```go
var chest Chest
err := nbt.UnmarshalSNBT(`{Items:[{Slot:0b,id:"minecraft:diamond",count:3}]}`, &chest)
s, err := nbt.MarshalSNBT(chest)
```

## Type mapping

| Go                                       | NBT                                               |
| ---------------------------------------- | ------------------------------------------------- |
| `bool`                                   | `TAG_Byte` (0/1)                                  |
| `int8`, `uint8`                          | `TAG_Byte`                                        |
| `int16`, `uint16`                        | `TAG_Short`                                       |
| `int32`, `uint32`, `int`, `uint`         | `TAG_Int` (`int`/`uint` error if out of range)    |
| `int64`, `uint64`                        | `TAG_Long`                                        |
| `float32` / `float64`                    | `TAG_Float` / `TAG_Double`                        |
| `string`                                 | `TAG_String`                                      |
| `[]byte`, `[]int8`                       | `TAG_Byte_Array`                                  |
| `[]int32`, `[]uint32`                    | `TAG_Int_Array`                                   |
| `[]int64`, `[]uint64`                    | `TAG_Long_Array`                                  |
| other slices and arrays                  | `TAG_List` (elements must share one type)         |
| `map[string]T`, structs                  | `TAG_Compound`                                    |
| pointers, interfaces                     | the value they point to; `nil` is omitted         |
| `nbt.Tag` types (`nbt.Compound`, …)      | themselves                                        |
| `nbt.Marshaler` / `encoding.TextMarshaler` | whatever `MarshalNBT` returns / `TAG_String`    |

Struct tag options:

- `omitempty` leaves out zero values, as in `encoding/json`.
- `list` writes a byte, int or long slice as a `TAG_List` instead of an array. Minecraft uses lists for things like positions: ``Pos [3]int32 `nbt:"pos,list"` ``.
- `-` skips the field.

Decoding is lenient where it's safe: any integer tag decodes into any Go integer or `bool` if the value fits, arrays and lists decode into slices, and unknown keys are ignored unless you call `Decoder.DisallowUnknownFields`. Errors report the path to the bad value, such as `blocks[3].pos[1]`.

## Structure files

```go
s := structure.New(4189) // the DataVersion of the target Minecraft version

s.Fill(0, 0, 0, 4, 0, 4, structure.MustParseBlockState("stone_bricks"))
s.SetBlock(2, 1, 2, structure.MustParseBlockState("chest[facing=north]"),
	nbt.Compound{"LootTable": nbt.String("minecraft:chests/simple_dungeon")})
s.AddEntity([3]float64{2.5, 1, 0.5}, nbt.Compound{"id": nbt.String("minecraft:armor_stand")})

f, _ := os.Create("my_structure.nbt")
defer f.Close()
err := s.Write(f) // gzip compressed, ready for a structure block or data pack
```

`structure.Read` loads existing files, including vanilla ones with multiple palettes (shipwrecks and similar). Positions with no block are left alone when the structure is placed, like `minecraft:structure_void`; place `minecraft:air` to clear a space.

Minecraft 26.3 (DataVersion 5009) renamed the palette keys from `Name`/`Properties` to `id`/`properties`. Both are read, and the structure is written with the keys that match its `DataVersion`.

## Scope

This library covers Java Edition NBT (big-endian). Bedrock's little-endian and network variants, and region (`.mca`) files, aren't supported yet.

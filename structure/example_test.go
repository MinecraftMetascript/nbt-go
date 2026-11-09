package structure_test

import (
	"bytes"
	"fmt"
	"log"

	"github.com/MinecraftMetascript/nbt-go"
	"github.com/MinecraftMetascript/nbt-go/structure"
)

func Example() {
	s := structure.New(4189) // Minecraft 1.21.4

	// A 5x5 stone brick floor with a chest in the middle.
	floor := structure.MustParseBlockState("stone_bricks")
	if err := s.Fill(0, 0, 0, 4, 0, 4, floor); err != nil {
		log.Fatal(err)
	}
	chest := structure.MustParseBlockState("chest[facing=north]")
	loot := nbt.Compound{"LootTable": nbt.String("minecraft:chests/simple_dungeon")}
	if err := s.SetBlock(2, 1, 2, chest, loot); err != nil {
		log.Fatal(err)
	}

	var buf bytes.Buffer // normally an *os.File named something.nbt
	if err := s.Write(&buf); err != nil {
		log.Fatal(err)
	}

	read, err := structure.Read(&buf)
	if err != nil {
		log.Fatal(err)
	}
	state, data, _ := read.Block(2, 1, 2)
	fmt.Println(read.Size, len(read.Blocks), state, data)
	// Output: [5 2 5] 26 minecraft:chest[facing=north] {LootTable:"minecraft:chests/simple_dungeon"}
}

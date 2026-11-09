package nbt_test

import (
	"bytes"
	"fmt"
	"log"

	"github.com/MinecraftMetascript/nbt-go"
)

type Item struct {
	Slot  int8   `nbt:"Slot"`
	ID    string `nbt:"id"`
	Count int32  `nbt:"count"`
}

type Chest struct {
	CustomName string `nbt:"CustomName,omitempty"`
	Items      []Item `nbt:"Items"`
}

func Example() {
	chest := Chest{Items: []Item{{Slot: 0, ID: "minecraft:diamond", Count: 3}}}

	data, err := nbt.Marshal(chest)
	if err != nil {
		log.Fatal(err)
	}

	var decoded Chest
	if err := nbt.Unmarshal(data, &decoded); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", decoded)
	// Output: {CustomName: Items:[{Slot:0 ID:minecraft:diamond Count:3}]}
}

func ExampleEncoder_gzip() {
	var buf bytes.Buffer
	enc := nbt.NewEncoder(&buf)
	enc.SetCompression(nbt.Gzip)
	if err := enc.Encode(map[string]int32{"DataVersion": 3955}); err != nil {
		log.Fatal(err)
	}

	// Decoders detect compression on their own.
	dec := nbt.NewDecoder(&buf)
	var v struct{ DataVersion int32 }
	if err := dec.Decode(&v); err != nil {
		log.Fatal(err)
	}
	fmt.Println(v.DataVersion, dec.Compression())
	// Output: 3955 gzip
}

func ExampleUnmarshal_tree() {
	data, _ := nbt.Marshal(map[string]any{
		"pos":  []float64{1.5, 64, -3},
		"name": "Steve",
	})

	// Decode into a Tag (or any) when the shape isn't known up front.
	var tree nbt.Tag
	if err := nbt.Unmarshal(data, &tree); err != nil {
		log.Fatal(err)
	}
	c := tree.(nbt.Compound)
	fmt.Println(c["name"], c["pos"].Type())
	fmt.Println(tree)
	// Output:
	// "Steve" TAG_List
	// {name:"Steve",pos:[1.5d,64d,-3d]}
}

func ExampleParseSNBT() {
	tag, err := nbt.ParseSNBT(`{Items:[{Slot:0b,id:"minecraft:diamond",count:3}]}`)
	if err != nil {
		log.Fatal(err)
	}
	var chest Chest
	if err := nbt.FromTag(tag, &chest); err != nil {
		log.Fatal(err)
	}
	fmt.Println(chest.Items[0].ID, chest.Items[0].Count)
	// Output: minecraft:diamond 3
}

package main

import "fmt"

// gameVersion is a Minecraft Java Edition release and its DataVersion.
type gameVersion struct {
	name        string
	dataVersion int32
}

// knownVersions are recent releases, oldest first. DataVersions come from
// each release's version.json (as mirrored by misode/mcmeta).
var knownVersions = []gameVersion{
	{"1.21.4", 4189},
	{"1.21.9", 4554},
	{"1.21.11", 4671},
	{"26.1", 4786},
	{"26.2", 4903},
	{"26.3", 5023},
}

// dataVersionLabel formats dv for messages, naming the release when dv is
// exactly a known one: "5023 (26.3)".
func dataVersionLabel(dv int32) string {
	for _, v := range knownVersions {
		if v.dataVersion == dv {
			return fmt.Sprintf("%d (%s)", dv, v.name)
		}
	}
	return fmt.Sprint(dv)
}

// versionName is the release with exactly this DataVersion, or "".
func versionName(dv int32) string {
	for _, v := range knownVersions {
		if v.dataVersion == dv {
			return v.name
		}
	}
	return ""
}

func indexOfVersion(name string) int {
	for i, v := range knownVersions {
		if v.name == name {
			return i
		}
	}
	return -1
}

// addedBlocks lists the block ids each release added, from the block
// registries of consecutive releases. A file whose DataVersion is older than
// the release can't be loaded by that older game with these blocks in it.
var addedBlocks = []struct {
	version     string
	dataVersion int32
	ids         []string
}{
	{"26.1", 4786, []string{
		"minecraft:golden_dandelion", "minecraft:potted_golden_dandelion",
	}},
	{"26.2", 4903, []string{
		"minecraft:chiseled_cinnabar", "minecraft:chiseled_sulfur", "minecraft:cinnabar", "minecraft:cinnabar_brick_slab", "minecraft:cinnabar_brick_stairs",
		"minecraft:cinnabar_brick_wall", "minecraft:cinnabar_bricks", "minecraft:cinnabar_slab", "minecraft:cinnabar_stairs", "minecraft:cinnabar_wall",
		"minecraft:polished_cinnabar", "minecraft:polished_cinnabar_slab", "minecraft:polished_cinnabar_stairs", "minecraft:polished_cinnabar_wall",
		"minecraft:polished_sulfur", "minecraft:polished_sulfur_slab", "minecraft:polished_sulfur_stairs", "minecraft:polished_sulfur_wall",
		"minecraft:potent_sulfur", "minecraft:sulfur", "minecraft:sulfur_brick_slab", "minecraft:sulfur_brick_stairs", "minecraft:sulfur_brick_wall",
		"minecraft:sulfur_bricks", "minecraft:sulfur_slab", "minecraft:sulfur_spike", "minecraft:sulfur_stairs", "minecraft:sulfur_wall",
	}},
	{"26.3", 5023, []string{
		"minecraft:black_concrete_slab", "minecraft:black_concrete_stairs", "minecraft:black_wool_slab", "minecraft:black_wool_stairs",
		"minecraft:blue_concrete_slab", "minecraft:blue_concrete_stairs", "minecraft:blue_wool_slab", "minecraft:blue_wool_stairs", "minecraft:brown_concrete_slab",
		"minecraft:brown_concrete_stairs", "minecraft:brown_wool_slab", "minecraft:brown_wool_stairs", "minecraft:cyan_concrete_slab",
		"minecraft:cyan_concrete_stairs", "minecraft:cyan_wool_slab", "minecraft:cyan_wool_stairs", "minecraft:gray_concrete_slab", "minecraft:gray_concrete_stairs",
		"minecraft:gray_wool_slab", "minecraft:gray_wool_stairs", "minecraft:green_concrete_slab", "minecraft:green_concrete_stairs", "minecraft:green_wool_slab",
		"minecraft:green_wool_stairs", "minecraft:light_blue_concrete_slab", "minecraft:light_blue_concrete_stairs", "minecraft:light_blue_wool_slab",
		"minecraft:light_blue_wool_stairs", "minecraft:light_gray_concrete_slab", "minecraft:light_gray_concrete_stairs", "minecraft:light_gray_wool_slab",
		"minecraft:light_gray_wool_stairs", "minecraft:lime_concrete_slab", "minecraft:lime_concrete_stairs", "minecraft:lime_wool_slab",
		"minecraft:lime_wool_stairs", "minecraft:magenta_concrete_slab", "minecraft:magenta_concrete_stairs", "minecraft:magenta_wool_slab",
		"minecraft:magenta_wool_stairs", "minecraft:orange_concrete_slab", "minecraft:orange_concrete_stairs", "minecraft:orange_poplar_leaves",
		"minecraft:orange_wool_slab", "minecraft:orange_wool_stairs", "minecraft:pink_concrete_slab", "minecraft:pink_concrete_stairs", "minecraft:pink_wool_slab",
		"minecraft:pink_wool_stairs", "minecraft:poplar_button", "minecraft:poplar_door", "minecraft:poplar_fence", "minecraft:poplar_fence_gate",
		"minecraft:poplar_hanging_sign", "minecraft:poplar_log", "minecraft:poplar_planks", "minecraft:poplar_pressure_plate", "minecraft:poplar_sapling",
		"minecraft:poplar_shelf", "minecraft:poplar_sign", "minecraft:poplar_slab", "minecraft:poplar_stairs", "minecraft:poplar_trapdoor",
		"minecraft:poplar_wall_hanging_sign", "minecraft:poplar_wall_sign", "minecraft:poplar_wood", "minecraft:potted_poplar_sapling",
		"minecraft:purple_concrete_slab", "minecraft:purple_concrete_stairs", "minecraft:purple_wool_slab", "minecraft:purple_wool_stairs",
		"minecraft:red_concrete_slab", "minecraft:red_concrete_stairs", "minecraft:red_poplar_leaves", "minecraft:red_shrub", "minecraft:red_wool_slab",
		"minecraft:red_wool_stairs", "minecraft:shelf_mushroom", "minecraft:straw_bed", "minecraft:stripped_poplar_log", "minecraft:stripped_poplar_wood",
		"minecraft:white_concrete_slab", "minecraft:white_concrete_stairs", "minecraft:white_wool_slab", "minecraft:white_wool_stairs",
		"minecraft:yellow_concrete_slab", "minecraft:yellow_concrete_stairs", "minecraft:yellow_poplar_leaves", "minecraft:yellow_wool_slab",
		"minecraft:yellow_wool_stairs",
	}},
}

// addedIn maps a block id to the release that added it.
var addedIn = func() map[string]gameVersion {
	m := map[string]gameVersion{}
	for _, a := range addedBlocks {
		for _, id := range a.ids {
			m[id] = gameVersion{a.version, a.dataVersion}
		}
	}
	return m
}()

# Style palettes

Starting palettes by theme. Each lists blocks per role; weighted mixes can be pasted straight into fill and the shape tools. Survival palettes stick to common materials; creative palettes add variety and rarer blocks. Swap woods and stones to match the biome.

Block ids are for 1.21.x. (In 1.21.9 and later, chain is iron_chain.) The blocks under "Newer blocks" at the end need a newer game; only use them when the structure's DataVersion is at least that release, or lint's `newer` check will flag them.

## Medieval timber house (plains, forest)

Survival
- Foundation: `70%cobblestone,30%mossy_cobblestone`, stairs cobblestone_stairs
- Frame: stripped_spruce_log or spruce_log (pillars and beams)
- Wall infill: `80%oak_planks,20%stripped_oak_wood`, or `white_wool` for a plaster look
- Roof: spruce_stairs and spruce_slab, ridge with spruce_slab
- Detail: spruce_trapdoor shutters, oak_fence railings, glass_pane windows, lantern under eaves

Creative
- Foundation: `40%cobblestone,25%mossy_cobblestone,20%stone_bricks,15%andesite`, base course cobbled_deepslate
- Frame: stripped_dark_oak_log beams, dark_oak_log corner posts, upside-down dark_oak_stairs brackets
- Wall infill: `70%calcite,20%white_terracotta,10%diorite` (plaster), or `60%mud_bricks,40%packed_mud` (wattle)
- Roof: `deepslate_tile_stairs` with `dark_oak_stairs` eaves, or `spruce_stairs` blended with `dark_oak_stairs` and `mangrove_stairs`
- Detail: spruce_trapdoor and dark_oak_trapdoor, flower_pot, decorated_pot, lantern on chain, hanging signs, moss_carpet on roof edges

## Stone castle or keep

Survival
- Walls: `65%stone_bricks,20%cobblestone,15%mossy_stone_bricks`
- Base: `cobblestone`, `mossy_cobblestone` at the very bottom
- Battlements: stone_brick_wall and stone_brick_slab
- Roofs: spruce_stairs or dark_oak_stairs on towers
- Detail: oak_fence and iron_bars in windows, torches, banners

Creative
- Walls: `45%stone_bricks,15%cracked_stone_bricks,15%mossy_stone_bricks,10%andesite,10%tuff_bricks,5%cobblestone`
- Base: `40%cobbled_deepslate,30%deepslate_bricks,20%polished_deepslate,10%mossy_cobblestone`
- Trim and quoins: polished_andesite, chiseled_stone_bricks, stone_brick_stairs used as corbels
- Battlements: stone_brick_wall, cobbled_deepslate_wall, tuff_brick_wall
- Roofs: deepslate_tile_stairs with oxidized copper accents (waxed_oxidized_cut_copper_stairs), or dark_prismarine_stairs for a slate look
- Detail: iron_bars, lantern, banners, chiseled_tuff, polished_blackstone_button rivets

## Nordic or taiga

Survival
- Foundation: `cobblestone`, `mossy_cobblestone`
- Walls: spruce_log (horizontal logs, axis=x or axis=z), spruce_planks
- Roof: spruce_stairs, steep (2 up per 1 out where possible), dark_oak_stairs ridge
- Detail: spruce_fence, spruce_trapdoor, campfire smoke out of a cobblestone chimney

Creative
- Foundation: `50%cobblestone,25%mossy_cobblestone,15%andesite,10%stone`
- Walls: spruce_log and stripped_spruce_log alternating rows, dark_oak_log corners
- Roof: `dark_oak_stairs` with `spruce_stairs` edges, moss_carpet and snow patches on top
- Detail: carved crossed gables (spruce_fence and dark_oak_fence), hay_block, barrels, lanterns, banners

## Desert

Survival
- Walls: `70%sandstone,30%smooth_sandstone`
- Trim: cut_sandstone, chiseled_sandstone over doors, sandstone_stairs and sandstone_slab
- Roof: flat with sandstone_slab, or sandstone_stairs
- Detail: oak_fence and oak_trapdoor, terracotta accents, dead_bush and cactus planting

Creative
- Walls: `50%smooth_sandstone,25%sandstone,15%cut_sandstone,10%birch_planks`
- Base: `60%sandstone,25%terracotta,15%orange_terracotta`
- Accents: cut_red_sandstone bands, chiseled_sandstone, light_blue_terracotta or cyan_terracotta tiles, waxed_oxidized_copper domes
- Detail: jungle_trapdoor shutters, decorated_pot, bamboo awnings (bamboo_mosaic_slab), palm-like jungle_log and jungle_leaves

## Swamp or mud hut

Survival
- Stilts: oak_log or spruce_log into the water
- Walls: mud_bricks, packed_mud, oak_planks
- Roof: oak_stairs, spruce_stairs with vines hanging off the edge

Creative
- Stilts: mangrove_log and mangrove_roots, muddy_mangrove_roots at the waterline
- Walls: `50%mud_bricks,30%packed_mud,20%mangrove_planks`
- Roof: mangrove_stairs blended with dark_oak_stairs, moss_block and moss_carpet patches
- Detail: vines, glow_lichen, lily_pad, lantern on chain, brown_mushroom

## Ruined or overgrown

Applies on top of any theme: start from the intact build, then remove and replace.

- Knock out roof sections and wall corners with irregular edges (remove_blocks in a few overlapping boxes, not one).
- Swap 20 to 40% of stone for cracked and mossy variants, more near the ground.
- Replace some planks with stairs and slabs facing random ways for broken edges.
- decorate_surface with moss_carpet and vines on top and north faces, grass and ferns at the base, leaves (persistent=true) creeping over walls.
- Survival: cobblestone, mossy_cobblestone, gravel, cracked_stone_bricks. Creative: add tuff, cobbled_deepslate, cracked_deepslate_bricks, rooted_dirt, azalea_leaves, flowering_azalea_leaves.

## Nether outpost

Survival (in the Nether)
- Walls: nether_bricks, blackstone, basalt pillars
- Detail: nether_brick_fence, soul_lantern, crimson or warped planks inside

Creative
- Walls: `40%polished_blackstone_bricks,25%cracked_polished_blackstone_bricks,20%blackstone,15%gilded_blackstone` (keep gilded rare)
- Pillars: polished_basalt, basalt; bands of red_nether_bricks
- Roof: crimson_stairs or nether_brick_stairs
- Detail: chains, soul_lantern, shroomlight hidden behind crimson_trapdoor, magma_block accents, crying_obsidian very sparingly

## Newer blocks (26.1 to 26.3)

Only for structures with a DataVersion of at least the release named (26.1 is 4786, 26.2 is 4903, 26.3 is 5023).

- 26.3, poplar wood: poplar_planks, poplar_log, stripped_poplar_log, poplar_stairs, poplar_slab, poplar_fence, poplar_door, poplar_trapdoor, poplar_shelf. A pale grey-brown wood that suits Nordic and coastal builds; autumn trees with yellow_poplar_leaves, orange_poplar_leaves and red_poplar_leaves.
- 26.3, wool and concrete slabs and stairs in all 16 colours (white_wool_stairs, red_concrete_slab and so on): soft, colourful roofs and awnings, and clean modern trim.
- 26.3, decoration: straw_bed for rustic or farm interiors, red_shrub and shelf_mushroom (on the side of logs) for overgrown and autumn scenes.
- 26.2, sulfur and cinnabar stone families (blocks, polished, bricks, chiseled, with slabs, stairs and walls): a yellow and a rust-red stone. Good for volcanic, desert and Nether-adjacent builds; sulfur_spike works like pointed_dripstone.
- 26.1: golden_dandelion, a flower accent for gardens.

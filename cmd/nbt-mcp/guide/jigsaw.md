# Jigsaw structures

A jigsaw structure is assembled at generation time from pieces in template pools, so it can be different every time it generates. Done well, that variety is what makes a village or ruin feel hand-made at a scale no single template can. These are the techniques that matter, roughly in the order you need them.

## How a connection works (short version)

- A parent jigsaw names a `pool` and a `target`. The game picks pieces from the pool in a weighted random order, tries the four rotations in random order, and for each tries the child's jigsaws whose `name` equals the parent's `target`, also in random order (higher `selection_priority` first). The first combination that fits wins.
- The two jigsaw blocks end up adjacent and face to face. Horizontal connections only allow the rotation that makes them face each other; up and down jigsaws with a `rollable` joint allow all four, `aligned` keeps their tops matched.
- A piece fails to place if it would overlap another piece or reach past `max_distance_from_center`. A jigsaw that places nothing turns into its `final_state`.
- A jigsaw that faces into its own piece's bounding box is an interior connection: its child must fit entirely inside the parent's box, and only competes for space with other interior children.

Use place_jigsaw to place them, check_jigsaw_fit to see where a child lands, and assemble_jigsaw with several seeds to see the range of layouts.

## Design pieces on a grid

- Pick a module and stick to it: for example streets 3 or 5 wide with the jigsaw on the centre line, house plots in multiples of 4 or 8, and the door jigsaw always in the middle of the front wall at ground level. Pieces built on the same module mix freely.
- Name connectors by role, not by piece: `mypack:street` to `mypack:street`, `mypack:street_side` to `mypack:house_front`, `mypack:room_door` to `mypack:room_door`. Any piece with the matching name can then fill the slot, and adding a variant never needs the old pieces edited.
- Build each piece so it looks right in all four rotations, because the game rotates them. Text, banners and "front" details should face the jigsaw that attaches the piece, since that is the side that ends up at the street.
- Keep pieces small: a few hundred to a few thousand blocks. Big pieces block each other and leave gaps; small ones pack.

## Variety: more variants, not bigger pieces

- Put 3 to 6 variants of every piece type in its pool: houses with different roofs, sizes and palettes, several street straights, corners and crossings. Weight the plain ones higher and the special ones lower (a weight 1 well among weight 6 houses).
- Split a building into slots so variants multiply: a shell piece with an interior jigsaw for furniture, an up jigsaw for the roof or upper storey, side jigsaws for a porch or garden. Three roofs times three interiors times two porches is eighteen houses from eight pieces.
- Add `minecraft:empty_pool_element` with some weight to make an optional slot sometimes stay empty (a garden that is sometimes there, a lamp post on some street corners).
- Decoration sets: an interior jigsaw facing into a room, pointing at a `decor` pool of small pieces (a bed corner, a workbench, a pile of crates). Because interior children must fit inside the parent, they never poke through walls.

## Jitter: break the grid on purpose

A layout where every piece snaps to the same point looks generated. Shift where pieces land with several identical jigsaws.

- **On the child piece:** place 2 to 4 jigsaws with the same `name`, side by side along the face that attaches (for example three jigsaws in a row in the front wall of a house, each 1 block apart). The game tries the child's matching jigsaws in random order, so the house lands shifted 0, 1 or 2 blocks along the street each time. Give them the same `final_state` as the wall around them so the unused ones vanish.
- **On the parent piece:** place several identical jigsaws next to each other along a street edge, all pointing at the same pool. Each one tries to place a child; once one succeeds, the others usually fail because their child would overlap it, and turn into their `final_state`. The house ends up at a random position along the plot. Use this when the plots are wider than the houses. Note that a failed jigsaw also tries its pool's fallback, so make sure the fallback pieces either fit nowhere there or are fine to place.
- Vary heights the same way: two up jigsaws stacked 1 block apart on a foundation gives a roof or upper storey at two possible heights.
- Mix rotations: for pieces attached by an up or down jigsaw (`rollable`), each generation picks a random rotation, which is an easy way to vary roofs, wells and statues.

## Ending branches: fallbacks and terminators

- Every pool that can be reached at depth should have a `fallback` pool of terminators: short dead-end street caps, a fence, a small garden, a pile of hay. At `max_depth` only the fallback is tried, so without it streets end in raw stubs.
- Terminators should be tiny (1 to 3 blocks deep) so they fit where nothing else does.
- Use `minecraft:empty` as the fallback only for optional slots that are fine to leave as their `final_state`.

## Order and priority

- `placement_priority` on a parent jigsaw: its child is processed before lower-priority ones, so the main street claims space before side buildings do. Give main streets 1 or 2 and decorations 0.
- `selection_priority` on a piece's jigsaws: the order its own jigsaws are tried in. Put the important connections (the next street segment) first so they aren't crowded out by decorations.
- Keep `max_depth` (the structure's `size`) to what you need: 5 to 7 for a village, 3 to 4 for a small ruin. Raise `max_distance_from_center` only if the layout is getting cut off.

## Blending with the world

- `final_state`: what an unused or used jigsaw becomes. Use the surrounding block for jigsaws in walls and floors, `minecraft:air` for ones in open space, and `minecraft:structure_void` only where the world should be left untouched.
- Use `terrain_matching` projection for streets and paths so they follow the ground, and `rigid` for buildings. Give rigid buildings a foundation that extends 1 to 3 blocks below their floor so they don't float on slopes.
- Leave positions with no block (not air) around the edges of pieces so terrain shows through; fill interiors with air so terrain doesn't.
- Remember the start piece sinks one block when the structure uses `project_start_to_heightmap`: build its y=0 as foundation.

## Weathering with processors, not by hand

Processor lists (data/<ns>/worldgen/processor_list) apply random changes each time a piece generates, so two copies of the same house age differently. Bake the clean build into the template and let processors age it:

- `minecraft:rule` with `random_block_match` to swap a share of `stone_bricks` for `mossy_stone_bricks` or `cracked_stone_bricks` (for example probability 0.2), `cobblestone` for `mossy_cobblestone`, and planks for stairs or air in ruins.
- `minecraft:block_rot` with an integrity below 1 (for example 0.85) to drop random blocks from ruins.
- `minecraft:gravity` on terrain-matching paths so they sit on the ground.

assemble_jigsaw ignores processors and terrain, so check variety by assembling with several seeds and judge the aging in game.

## A checklist for a village-style structure

1. Start piece: a plaza or crossroads, built on the module, with street jigsaws on each side at `placement_priority` 1.
2. Street pool: 2 to 3 straights, a corner, a T and a crossing; each with street jigsaws at the ends and `street_side` jigsaws along both edges (several identical ones for jitter). Fallback: street caps.
3. House pool: 4 to 6 houses in two or three sizes, each with 2 to 3 identical front jigsaws named `house_front`; interior decor and roof slots where it pays off. Weighted `empty_pool_element` for empty plots.
4. Decor pools: lamp posts, wells, carts, gardens, with empty elements.
5. assemble_jigsaw with 5 or more seeds; look at each with render_image. Fix pieces that never place (the report says why) and layouts that look repetitive.

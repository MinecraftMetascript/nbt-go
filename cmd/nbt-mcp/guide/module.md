# Building pieces to a module

The fastest way to build a big jigsaw structure (a village, a dungeon, a base) is to write one short spec first and then build the pieces against it, in parallel if you can: one agent per group of pieces. When every piece follows the same spec, they fit together the first time. The spec is the contract; the pieces are interchangeable within it.

This complements the jigsaw section of the guide, which explains why modules, connector names and variants make jigsaw structures vary well.

## Write the spec first

Keep it to one page. Every agent gets the whole spec, not just its part. Fill in each heading:

```
# <structure> build spec

Target: Minecraft <version>, DataVersion <n>, style <survival|creative>, theme <theme>

## Module
- Grid unit: <e.g. 8 blocks>. Every piece's footprint is a multiple of it: <list allowed sizes, e.g. 8x8, 8x16, 16x16>.
- Height: floor at y=<n>, ceiling height <n> (inside), max height <n>. y=0 is <foundation | the floor>.
- Corridors / streets: <width> wide, <height> tall, centred on the piece edge.
- Door and corridor gaps: <width> x <height>, centred, bottom at y=<n>.

## Connectors (jigsaw names)
| name           | where                                       | facing   | final_state       |
| <ns>:corridor  | centre of each open edge, floor level        | outward  | <floor block>     |
| <ns>:door      | centre of the door gap, floor level          | outward  | air               |
| <ns>:decor     | centre of each room, one above the floor     | up       | air               |
Pools: <ns>:corridors, <ns>:rooms, <ns>:decor, each with fallback <ns>:caps (or minecraft:empty).
Every jigsaw's target is the same as its name unless listed otherwise.

## Materials
- Walls: <mix>; floors: <mix>; ceilings / roofs: <mix>; trim and frames: <blocks>; accents: <blocks>.
- Blocks not to use: <list, e.g. anything renamed between versions, lavish blocks for survival>.

## Lighting
- Light every walkable floor position so block light never drops to 0 (hostile mobs spawn at block light 0).
- In practice: a light source at least every <6-8> blocks along corridors and in every room, placed <hanging from the ceiling | on walls at y=<n>>, using <lantern | torch | ...>.
- Check it with check_lighting: it lists every roofed floor position dark enough to spawn mobs, with a map of the worst layer.

## Pieces and owners
| piece              | size    | connectors                       | owner   |
| corridor_straight  | 8x8     | corridor N, S                    | agent 1 |
| corridor_corner    | 8x8     | corridor N, E                    | agent 1 |
| room_kitchen       | 8x16    | door S, decor x1                 | agent 2 |
| cap                | 8x1     | corridor S                       | agent 1 |

## Save paths
- <datapack>/data/<ns>/structure/<group>/<piece>.nbt ("structures" before 1.21)
- Pools in <datapack>/data/<ns>/worldgen/template_pool/<pool>.json, written by <who>.

## Checks before a piece is done
lint_structure clean (or every finding explained), critique_structure at <score> or above,
check_lighting finds no dark spawn spots, check_jigsaw_fit against <a reference piece>,
render_image with cutaway_y on each floor.
```

## Rules for each piece

- **Name structures with your own prefix** (for example `a1_corridor_straight`) when several agents share one server, so nobody opens or overwrites someone else's piece.
- **Pass name to undo and redo when sharing a server.** The history is shared by everyone connected, so a bare undo steps back the latest edit, which may be another agent's. With name it only steps back your own structure's edits.
- **Build exactly to the module.** The footprint is a multiple of the grid unit. Check the size with describe_structure before saving. place_grid refuses rows of different lengths, so pad every row to the full width with '.' rather than passing ragged=true.
- **Put connectors exactly where the spec says**: same height, centred, facing outward, with the spec's final_state. Use place_jigsaw so the orientation and final_state are set properly.
- **Leave door and corridor gaps exactly the spec's size**, so any corridor fits any door.
- **Light it as you build**, not at the end: the spec's spacing, in every room and along every corridor. check_lighting shows any spot you missed.
- **Use only the spec's materials.** Variety comes from different pieces and weighted mixes inside the palette, not from each agent choosing its own blocks.
- **Run the piece checks** (lint, critique, check_lighting, check_jigsaw_fit against a reference piece, a cutaway render of each floor) and save to the spec's path.

## Integrating

One agent (or you) owns the pools and the final check:

1. Write the template pools from the piece table, with fallbacks of small caps.
2. Run assemble_jigsaw against the datapack with seeds=N (say 10 to 20). It proves every connector lines up and every piece can place before anyone loads a world, and its coverage report lists templates that never placed in any layout. Large layouts come back as a summary (pass detail=summary to ask for one): pieces per template, a depth histogram and problems grouped by reason. Entry jigsaws that point back at their parent are counted separately, so the problems listed are real.
3. render_image the assembled result, with cutaway_y through each floor.
4. Fix the pieces that fail and repeat. Because pieces follow the spec, a fix stays local to one piece.

# Building structures that look hand-crafted

A generated build looks generated when it is a box: flat walls, one block type, a flat or plain pyramid roof, perfect symmetry and no small blocks. A good builder avoids each of those on purpose. Work through these in order; each one is cheap with the tools this server has.

## 1. Plan the silhouette first

- Start from a footprint that is not a plain rectangle: an L, a T, a main hall with a smaller wing, a tower set into a corner. Two or three overlapping volumes of different sizes read as "designed".
- Vary heights. The main volume is tallest, wings are 2 to 4 blocks lower, a tower or chimney breaks the roofline. Check the outline with render_view south and east: the top edge should step, not run flat.
- Proportions: walls are usually 4 to 6 blocks tall per storey (3 is cramped, 7+ looks empty). Roofs look best at 45 degrees (stairs) or steeper; a roof is often as tall as the walls under it.
- Odd widths (5, 7, 9, 11) give a centre line for a door, window or roof ridge.

## 2. Give every surface depth

Flat walls are the most common tell. Aim for no flat run of the same block longer than about 5 to 7 blocks in any direction.

- **Frame, then infill.** Build a frame of logs or a stronger material (pillars at corners and every 3 to 5 blocks, beams along each floor line), then set the infill wall 1 block back. The frame alone fixes most flatness.
- **Layers of depth:** frame at 0, wall at -1, windows at -1 or -2 with sills (stairs or trapdoors) at 0 under them, shutters (trapdoors) beside them.
- **Foundation:** a plinth 1 block wider than the walls, in a heavier, darker material (cobblestone, stone bricks, deepslate) and 1 to 3 blocks tall. Buildings should look like they sit in the ground, not on it.
- **Overhangs:** roofs extend 1 block past the walls (2 on big builds), upper storeys can jut 1 block out over the lower one on beams (jetties). Put upside-down stairs or trapdoors under the overhang as brackets.

## 3. Use palette gradients, not single blocks

- Pick 3 to 5 related blocks per material role (wall, frame, roof, foundation, accent) rather than one each.
- **Vertical gradient:** darker, rougher and mossier near the ground, lighter and cleaner going up. For stone: cobbled_deepslate or mossy_cobblestone at the base, stone_bricks with cracked and mossy variants in the middle, polished_andesite or plain stone_bricks near the top.
- **Texture by mixing:** use weighted mixes in fills, for example `60%stone_bricks,20%cracked_stone_bricks,15%mossy_stone_bricks,5%andesite`. Keep the main block above about 50% so the wall still reads as one material.
- **Contrast:** pair a warm material with a cool one (spruce or dark oak with stone; oak with white calcite or birch). Roofs should contrast with walls.
- Keep accents rare: a block used for 2 to 5% of the visible surface reads as detail; at 30% it is a second wall material.

## 4. Detail with small blocks

Full blocks make the shape; partial blocks make it look crafted. A good exterior has 15 to 30% of its visible surface in partial blocks (survival builds can sit lower, around 8 to 15%).

- Stairs and slabs: roof edges, window sills, eaves, the step at the door, the bevel along the top of a wall.
- Walls and fences: railings, chimney tops, buttresses, posts.
- Trapdoors: shutters, window boxes, roof trim, flat panels on beams.
- Buttons, levers and chains: rivets on beams, hanging hooks, lanterns on chains.
- Lighting as decor: lanterns hung under eaves, torches in pairs beside doors, candles on window sills.
- Nature: vines and moss on the shady side and near the base, flowers in pots and planters, leaves as hedges or creeping into the roof edge, a path of dirt_path, gravel and coarse_dirt to the door.

## 5. Break perfect symmetry

Use a symmetric skeleton, then break it: a chimney on one side, a lean-to shed, a window that is a different size, a woodpile, a crate stack, vines on one corner. Players read this as a lived-in place. Military and religious buildings can stay closer to symmetric than houses.

## 6. Interiors

Even a small interior should have a purpose (bed, table, storage, workstation) and lighting. Use partial blocks for furniture: stairs for chairs, a slab or trapdoor on a fence for a table, a barrel or chest for storage, item frames and flower pots on walls. Leave walking paths 1 to 2 blocks wide; don't fill every corner.

## 7. Villages and multi-piece builds

For anything assembled from template pools, read the jigsaw section of this guide: variety there comes from many small variants, slots and jittered connections rather than from one big piece.

## 8. Check and iterate

1. render_image (isometric) for overall shape; render_view south/east/north/west for each elevation.
2. critique_structure for flat walls, palette, detail ratio, silhouette, overhangs, openings and symmetry. Fix the top findings, then run it again.
3. lint_structure for correctness (support, connections, floating parts) before save_structure.

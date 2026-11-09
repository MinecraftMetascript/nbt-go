// Package structure reads, writes and builds Minecraft Java Edition
// structure files: the gzip-compressed .nbt files saved by structure blocks
// and loaded by /place template or data packs.
//
// A Structure can be decoded from an existing file with [Read], or built up
// from scratch:
//
//	s := structure.New(dataVersion)
//	s.Fill(0, 0, 0, 4, 0, 4, structure.MustParseBlockState("minecraft:stone_bricks"))
//	s.SetBlock(2, 1, 2, structure.MustParseBlockState("chest[facing=north]"),
//		nbt.Compound{"LootTable": nbt.String("minecraft:chests/simple_dungeon")})
//	err := s.Write(f)
//
// Positions that have no block in the structure are left untouched when it
// is placed, the same as minecraft:structure_void. Set minecraft:air
// explicitly to clear a space.
//
// The DataVersion must match the Minecraft version the file is meant for (see
// https://minecraft.wiki/w/Data_version); the game uses it to upgrade old
// block states and NBT when loading.
package structure

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"github.com/MinecraftMetascript/nbt-go"
)

// Structure is the root compound of a structure file.
//
// The fields can be read and modified directly. The helper methods keep
// their own indexes of block positions and palette entries, which are
// rebuilt automatically if the slices are changed behind their back.
type Structure struct {
	DataVersion int32 `nbt:"DataVersion"`
	// Size is the size of the structure along the x, y and z axes.
	Size [3]int32 `nbt:"size,list"`
	// Palette lists the block states used by Blocks. Block.State indexes
	// into it.
	Palette []BlockState `nbt:"palette,omitempty"`
	// Palettes is used instead of Palette by structures that have several
	// random variants, such as shipwrecks. Each palette must have the same
	// length. It is left empty by the helper methods.
	Palettes [][]BlockState `nbt:"palettes,omitempty"`
	Blocks   []Block        `nbt:"blocks"`
	Entities []Entity       `nbt:"entities"`

	blockIndex   map[[3]int32]int
	indexedLen   int // len(Blocks) when blockIndex was last in sync
	paletteIndex map[string]int32
}

// Block is a single block in a structure.
type Block struct {
	// State is an index into the structure's palette.
	State int32 `nbt:"state"`
	// Pos is the position of the block relative to the structure's origin.
	Pos [3]int32 `nbt:"pos,list"`
	// NBT holds block entity data (chest contents, sign text, ...), without
	// the x, y, z and id keys. It is nil for plain blocks.
	NBT nbt.Compound `nbt:"nbt,omitempty"`
}

// Entity is an entity saved in a structure.
type Entity struct {
	// Pos is the exact position of the entity relative to the origin.
	Pos [3]float64 `nbt:"pos"`
	// BlockPos is the block the entity is in.
	BlockPos [3]int32 `nbt:"blockPos,list"`
	// NBT is the entity's data. It must include an "id" such as
	// "minecraft:armor_stand".
	NBT nbt.Compound `nbt:"nbt"`
}

// MarshalNBT encodes the structure, writing palette entries in the form
// that matches s.DataVersion.
func (s Structure) MarshalNBT() (nbt.Tag, error) {
	type plain Structure // drop this method to avoid recursion
	t, err := nbt.ToTag(plain(s))
	if err != nil {
		return nil, err
	}
	c := t.(nbt.Compound)
	if len(s.Palettes) == 0 {
		// Always write "palette" unless "palettes" replaces it.
		if c["palette"] == nil {
			c["palette"] = nbt.List{}
		}
	}
	if s.DataVersion >= BlockStateIDVersion {
		return c, nil
	}
	if len(s.Palettes) == 0 {
		c["palette"] = legacyPalette(s.Palette)
	} else {
		ps := make(nbt.List, len(s.Palettes))
		for i, p := range s.Palettes {
			ps[i] = legacyPalette(p)
		}
		c["palettes"] = ps
	}
	return c, nil
}

func legacyPalette(p []BlockState) nbt.List {
	l := make(nbt.List, len(p))
	for i, b := range p {
		l[i] = b.toTag(false)
	}
	return l
}

// New returns an empty structure for the given data version.
func New(dataVersion int32) *Structure {
	return &Structure{DataVersion: dataVersion, Palette: []BlockState{}, Blocks: []Block{}, Entities: []Entity{}}
}

// Read decodes a structure file. Gzip compression (the normal format) is
// detected automatically, as is uncompressed NBT.
func Read(r io.Reader) (*Structure, error) {
	s := &Structure{}
	if err := nbt.NewDecoder(r).Decode(s); err != nil {
		return nil, err
	}
	return s, nil
}

// Write encodes the structure as a gzip-compressed structure file.
func (s *Structure) Write(w io.Writer) error {
	if err := s.Validate(); err != nil {
		return err
	}
	enc := nbt.NewEncoder(w)
	enc.SetCompression(nbt.Gzip)
	return enc.Encode(s)
}

// Validate checks that every block refers to an existing palette entry and
// lies within Size, that every entity has an id, and that DataVersion is set.
func (s *Structure) Validate() error {
	if s.DataVersion <= 0 {
		return fmt.Errorf("structure: DataVersion must be set")
	}
	n := len(s.Palette)
	for i, p := range s.Palettes {
		if i == 0 && len(s.Palette) == 0 {
			n = len(p)
		}
		if len(p) != n {
			return fmt.Errorf("structure: palettes[%d] has %d entries, want %d", i, len(p), n)
		}
	}
	for i, b := range s.Blocks {
		if b.State < 0 || int(b.State) >= n {
			return fmt.Errorf("structure: block %d at %v has state %d, palette has %d entries", i, b.Pos, b.State, n)
		}
		for a := 0; a < 3; a++ {
			if b.Pos[a] < 0 || b.Pos[a] >= s.Size[a] {
				return fmt.Errorf("structure: block %d at %v is outside size %v", i, b.Pos, s.Size)
			}
		}
	}
	for i, e := range s.Entities {
		if _, ok := e.NBT["id"].(nbt.String); !ok {
			return fmt.Errorf("structure: entity %d at %v has no id", i, e.Pos)
		}
	}
	return nil
}

func (s *Structure) blocks() map[[3]int32]int {
	if s.blockIndex == nil || s.indexedLen != len(s.Blocks) {
		s.reindexBlocks()
	}
	return s.blockIndex
}

func (s *Structure) reindexBlocks() {
	s.blockIndex = make(map[[3]int32]int, len(s.Blocks))
	for i, b := range s.Blocks {
		s.blockIndex[b.Pos] = i
	}
	s.indexedLen = len(s.Blocks)
}

// lookup finds the block at pos, verifying the index is still accurate.
func (s *Structure) lookup(pos [3]int32) (int, bool) {
	i, ok := s.blocks()[pos]
	if ok && (i >= len(s.Blocks) || s.Blocks[i].Pos != pos) {
		s.reindexBlocks()
		i, ok = s.blockIndex[pos]
	}
	return i, ok
}

// paletteID returns the index of state in the palette, adding it if needed.
func (s *Structure) paletteID(state BlockState) int32 {
	key := state.String()
	if s.paletteIndex == nil || len(s.paletteIndex) > len(s.Palette) {
		s.reindexPalette()
	}
	if id, ok := s.paletteIndex[key]; ok && int(id) < len(s.Palette) && s.Palette[id].String() == key {
		return id
	}
	s.reindexPalette()
	if id, ok := s.paletteIndex[key]; ok {
		return id
	}
	id := int32(len(s.Palette))
	s.Palette = append(s.Palette, state.clone())
	s.paletteIndex[key] = id
	return id
}

func (s *Structure) reindexPalette() {
	s.paletteIndex = make(map[string]int32, len(s.Palette))
	for i := len(s.Palette) - 1; i >= 0; i-- {
		s.paletteIndex[s.Palette[i].String()] = int32(i)
	}
}

func toPos(x, y, z int) ([3]int32, error) {
	for _, v := range [3]int{x, y, z} {
		if v < 0 || v > math.MaxInt32-1 {
			return [3]int32{}, fmt.Errorf("structure: position (%d, %d, %d) is out of range; coordinates must be non-negative", x, y, z)
		}
	}
	return [3]int32{int32(x), int32(y), int32(z)}, nil
}

// SetBlock places state at (x, y, z), replacing any block already there, and
// grows Size to fit. data is the block entity NBT, or nil.
func (s *Structure) SetBlock(x, y, z int, state BlockState, data nbt.Compound) error {
	pos, err := toPos(x, y, z)
	if err != nil {
		return err
	}
	if state.Name == "" {
		return fmt.Errorf("structure: block state at %v has no name", pos)
	}
	if len(s.Palettes) > 0 {
		return fmt.Errorf("structure: SetBlock does not support structures with multiple palettes")
	}
	b := Block{State: s.paletteID(state), Pos: pos, NBT: data}
	if i, ok := s.lookup(pos); ok {
		s.Blocks[i] = b
	} else {
		s.Blocks = append(s.Blocks, b)
		s.blockIndex[pos] = len(s.Blocks) - 1
		s.indexedLen = len(s.Blocks)
	}
	for a := 0; a < 3; a++ {
		s.Size[a] = max(s.Size[a], pos[a]+1)
	}
	return nil
}

// Fill places state in every position of the box with corners (x1, y1, z1)
// and (x2, y2, z2), inclusive, like the /fill command.
func (s *Structure) Fill(x1, y1, z1, x2, y2, z2 int, state BlockState) error {
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	if z1 > z2 {
		z1, z2 = z2, z1
	}
	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			for z := z1; z <= z2; z++ {
				if err := s.SetBlock(x, y, z, state, nil); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Block returns the block state and block entity data at (x, y, z). ok is
// false if the structure has no block there. For a structure with multiple
// palettes, the state comes from the first one.
func (s *Structure) Block(x, y, z int) (state BlockState, data nbt.Compound, ok bool) {
	pos, err := toPos(x, y, z)
	if err != nil {
		return BlockState{}, nil, false
	}
	i, ok := s.lookup(pos)
	if !ok {
		return BlockState{}, nil, false
	}
	b := s.Blocks[i]
	palette := s.Palette
	if len(s.Palettes) > 0 {
		palette = s.Palettes[0]
	}
	if b.State < 0 || int(b.State) >= len(palette) {
		return BlockState{}, b.NBT, true
	}
	return palette[b.State], b.NBT, true
}

// RemoveBlock removes the block at (x, y, z), so that placing the structure
// leaves that position unchanged. It reports whether there was a block. Size
// is not reduced.
func (s *Structure) RemoveBlock(x, y, z int) bool {
	pos, err := toPos(x, y, z)
	if err != nil {
		return false
	}
	i, ok := s.lookup(pos)
	if !ok {
		return false
	}
	last := len(s.Blocks) - 1
	s.Blocks[i] = s.Blocks[last]
	s.Blocks = s.Blocks[:last]
	s.indexedLen = len(s.Blocks)
	delete(s.blockIndex, pos)
	if i != last {
		s.blockIndex[s.Blocks[i].Pos] = i
	}
	return true
}

// AddEntity adds an entity at pos. data must contain the entity's "id".
func (s *Structure) AddEntity(pos [3]float64, data nbt.Compound) error {
	if _, ok := data["id"].(nbt.String); !ok {
		return fmt.Errorf("structure: entity data must have a string \"id\"")
	}
	var bp [3]int32
	for a := 0; a < 3; a++ {
		if pos[a] < 0 {
			return fmt.Errorf("structure: entity position %v is out of range; coordinates must be non-negative", pos)
		}
		bp[a] = int32(math.Floor(pos[a]))
		s.Size[a] = max(s.Size[a], bp[a]+1)
	}
	s.Entities = append(s.Entities, Entity{Pos: pos, BlockPos: bp, NBT: data})
	return nil
}

// BlockState is a block type and its properties, such as
// minecraft:oak_stairs[facing=east,half=bottom].
//
// Structure files written before DataVersion [BlockStateIDVersion] store a
// block state as {Name, Properties}; later ones use {id, properties}. Both
// are accepted when decoding, and [Structure] picks the right one for its
// DataVersion when encoding.
type BlockState struct {
	// Name is the namespaced block id, such as "minecraft:stone".
	Name string
	// Properties holds block state properties. Values are always strings,
	// even for numbers and booleans ("level": "15", "lit": "true").
	Properties map[string]string
}

// BlockStateIDVersion is the first DataVersion (Minecraft 26.3 snapshot 7)
// whose structure palettes use the keys "id" and "properties" instead of
// "Name" and "Properties".
const BlockStateIDVersion = 5009

// MarshalNBT encodes the block state in the current {id, properties} form.
func (b BlockState) MarshalNBT() (nbt.Tag, error) {
	return b.toTag(true), nil
}

func (b BlockState) toTag(lowercase bool) nbt.Compound {
	nameKey, propsKey := "Name", "Properties"
	if lowercase {
		nameKey, propsKey = "id", "properties"
	}
	c := nbt.Compound{nameKey: nbt.String(b.Name)}
	if len(b.Properties) > 0 {
		props := make(nbt.Compound, len(b.Properties))
		for k, v := range b.Properties {
			props[k] = nbt.String(v)
		}
		c[propsKey] = props
	}
	return c
}

// UnmarshalNBT decodes a block state in either the {Name, Properties} or the
// {id, properties} form.
func (b *BlockState) UnmarshalNBT(t nbt.Tag) error {
	c, ok := t.(nbt.Compound)
	if !ok {
		return fmt.Errorf("structure: block state is %v, want TAG_Compound", t.Type())
	}
	var raw struct {
		Name       string            `nbt:"Name"`
		ID         string            `nbt:"id"`
		Properties map[string]string `nbt:"Properties"`
		Props      map[string]string `nbt:"properties"`
	}
	if err := nbt.FromTag(c, &raw); err != nil {
		return err
	}
	b.Name = raw.Name
	if raw.ID != "" {
		b.Name = raw.ID
	}
	b.Properties = raw.Properties
	if raw.Props != nil {
		b.Properties = raw.Props
	}
	if b.Name == "" {
		return fmt.Errorf("structure: block state %v has no name", c)
	}
	return nil
}

// ParseBlockState parses the block state syntax used in commands, such as
// "oak_stairs[facing=east,half=bottom]". A missing namespace defaults to
// "minecraft".
func ParseBlockState(s string) (BlockState, error) {
	s = strings.TrimSpace(s)
	name, props, hasProps := strings.Cut(s, "[")
	name = strings.TrimSpace(name)
	if name == "" {
		return BlockState{}, fmt.Errorf("structure: empty block name in %q", s)
	}
	if !strings.Contains(name, ":") {
		name = "minecraft:" + name
	}
	bs := BlockState{Name: name}
	if hasProps {
		props, ok := strings.CutSuffix(strings.TrimSpace(props), "]")
		if !ok {
			return BlockState{}, fmt.Errorf("structure: missing ']' in block state %q", s)
		}
		if strings.TrimSpace(props) != "" {
			bs.Properties = map[string]string{}
			for _, kv := range strings.Split(props, ",") {
				k, v, ok := strings.Cut(kv, "=")
				k, v = strings.TrimSpace(k), strings.TrimSpace(v)
				if !ok || k == "" || v == "" {
					return BlockState{}, fmt.Errorf("structure: invalid property %q in block state %q", kv, s)
				}
				bs.Properties[k] = v
			}
		}
	}
	return bs, nil
}

// MustParseBlockState is like ParseBlockState but panics on error. It is
// meant for constant block states in code.
func MustParseBlockState(s string) BlockState {
	bs, err := ParseBlockState(s)
	if err != nil {
		panic(err)
	}
	return bs
}

// String formats the block state in command syntax, with properties sorted.
func (b BlockState) String() string {
	if len(b.Properties) == 0 {
		return b.Name
	}
	keys := make([]string, 0, len(b.Properties))
	for k := range b.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString(b.Name)
	sb.WriteByte('[')
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(b.Properties[k])
	}
	sb.WriteByte(']')
	return sb.String()
}

func (b BlockState) clone() BlockState {
	if b.Properties == nil {
		return b
	}
	p := make(map[string]string, len(b.Properties))
	for k, v := range b.Properties {
		p[k] = v
	}
	return BlockState{Name: b.Name, Properties: p}
}

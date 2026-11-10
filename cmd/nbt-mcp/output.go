package main

// The types in this file are the structured results of the reporting
// tools. Each tool returns the same information as text for the model and
// as one of these for clients and scripts. Lists that can be empty are
// omitted when they are, so a missing list means "none".

type regionOut struct {
	From [3]int `json:"from" jsonschema:"lowest corner [x, y, z]"`
	To   [3]int `json:"to" jsonschema:"highest corner [x, y, z], inclusive"`
}

func (b box) out() regionOut { return regionOut{From: b.min, To: b.max} }

type describeOut struct {
	DataVersion   int32        `json:"data_version" jsonschema:"the DataVersion the structure targets"`
	GameVersion   string       `json:"game_version,omitempty" jsonschema:"the Minecraft release with that DataVersion, if it is a known one"`
	Size          [3]int       `json:"size" jsonschema:"size [x, y, z] in blocks"`
	Positions     int          `json:"positions" jsonschema:"number of positions in the bounding box"`
	Blocks        int          `json:"blocks" jsonschema:"positions holding a block other than air"`
	Air           int          `json:"air" jsonschema:"positions holding air, which clears that space when the structure is placed"`
	Empty         int          `json:"empty" jsonschema:"positions with no block, left unchanged when the structure is placed"`
	PaletteCount  int          `json:"palette_variants,omitempty" jsonschema:"number of random palette variants, when the file has several; counts use the first"`
	BlockCounts   []blockCount `json:"block_counts,omitempty" jsonschema:"how many of each block state, most common first"`
	BlockEntities int          `json:"block_entities" jsonschema:"blocks carrying block entity data"`
	Entities      []entityOut  `json:"entities,omitempty" jsonschema:"the entities in the structure"`
	Jigsaws       []jigsawOut  `json:"jigsaws,omitempty" jsonschema:"the jigsaw blocks in the structure"`
	Warnings      []string     `json:"warnings,omitempty" jsonschema:"problems that involve several jigsaws"`
}

type blockCount struct {
	Block string `json:"block" jsonschema:"block state in command syntax"`
	Count int    `json:"count"`
}

type entityOut struct {
	ID  string     `json:"id" jsonschema:"entity id"`
	Pos [3]float64 `json:"pos" jsonschema:"position [x, y, z]"`
}

type jigsawOut struct {
	Pos               [3]int   `json:"pos" jsonschema:"position [x, y, z]"`
	Orientation       string   `json:"orientation" jsonschema:"front and top directions, e.g. north_up"`
	Name              string   `json:"name" jsonschema:"this jigsaw's name, which other jigsaws target"`
	Target            string   `json:"target" jsonschema:"name of the jigsaw on the child piece it connects to"`
	Pool              string   `json:"pool" jsonschema:"template pool the child piece is picked from"`
	Joint             string   `json:"joint" jsonschema:"rollable or aligned"`
	FinalState        string   `json:"final_state" jsonschema:"block that replaces the jigsaw when the structure generates"`
	PlacementPriority int      `json:"placement_priority"`
	SelectionPriority int      `json:"selection_priority"`
	ConnectsInside    bool     `json:"connects_inside" jsonschema:"true if it faces into its own piece, so the whole child must fit inside this piece's bounds"`
	Warnings          []string `json:"warnings,omitempty" jsonschema:"likely mistakes in this jigsaw's settings"`
}

type lintOut struct {
	Findings int            `json:"findings" jsonschema:"total number of findings, including any not listed"`
	Clean    []string       `json:"clean,omitempty" jsonschema:"checks that ran and found nothing"`
	Checks   []lintCheckOut `json:"checks,omitempty" jsonschema:"checks that found something"`
}

type lintCheckOut struct {
	Check    string        `json:"check" jsonschema:"name of the check"`
	About    string        `json:"about" jsonschema:"what the check looks for"`
	Count    int           `json:"count" jsonschema:"number of findings, including any not listed"`
	Findings []lintFinding `json:"findings" jsonschema:"the findings, up to the limit"`
}

type lintFinding struct {
	Pos     *[3]int `json:"pos,omitempty" jsonschema:"position [x, y, z]; absent when the finding is about the whole structure"`
	Message string  `json:"message"`
}

type critiqueOut struct {
	Style      string        `json:"style" jsonschema:"survival or creative"`
	Region     regionOut     `json:"region" jsonschema:"the box that was critiqued"`
	Score      int           `json:"score" jsonschema:"overall score from 0 to 100"`
	Categories []categoryOut `json:"categories" jsonschema:"each category, most valuable to fix first"`
}

type categoryOut struct {
	Key      string   `json:"key" jsonschema:"short name of the category"`
	Title    string   `json:"title"`
	Score    int      `json:"score" jsonschema:"score from 0 to 100"`
	Weight   float64  `json:"weight" jsonschema:"how much the category counts toward the overall score"`
	Summary  string   `json:"summary" jsonschema:"what was measured"`
	Findings []string `json:"findings,omitempty" jsonschema:"suggested fixes, up to the limit"`
}

type diffOut struct {
	SizeBefore        [3]int       `json:"size_before"`
	SizeAfter         [3]int       `json:"size_after"`
	DataVersionBefore int32        `json:"data_version_before"`
	DataVersionAfter  int32        `json:"data_version_after"`
	EntitiesBefore    int          `json:"entities_before"`
	EntitiesAfter     int          `json:"entities_after"`
	Differ            int          `json:"differ" jsonschema:"number of positions whose block or block entity data differs"`
	Bounds            *regionOut   `json:"bounds,omitempty" jsonschema:"the box containing every difference"`
	Kinds             []diffKind   `json:"kinds,omitempty" jsonschema:"differences grouped by kind, most common first, up to the limit"`
	Changes           []diffChange `json:"changes,omitempty" jsonschema:"individual differences in position order, up to the limit"`
}

type diffKind struct {
	Kind  string `json:"kind" jsonschema:"e.g. minecraft:stone → minecraft:air"`
	Count int    `json:"count"`
}

type diffChange struct {
	Pos    [3]int `json:"pos" jsonschema:"position [x, y, z]"`
	Before string `json:"before" jsonschema:"what was there, or (no block)"`
	After  string `json:"after" jsonschema:"what is there now, or (no block)"`
}

func size3(s [3]int32) [3]int { return [3]int{int(s[0]), int(s[1]), int(s[2])} }

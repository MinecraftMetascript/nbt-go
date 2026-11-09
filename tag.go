// Package nbt implements encoding and decoding of Minecraft's Named Binary Tag
// (NBT) format, as used by Java Edition for level data, player data and
// structure (.nbt) files.
//
// The API is modeled on encoding/json. Go values are converted to and from
// NBT with [Marshal] and [Unmarshal], or streamed with [Encoder] and
// [Decoder]. Struct fields are mapped with `nbt:"name,opts"` tags.
//
// When the shape of the data is not known ahead of time, decode into a [Tag]
// (or an empty interface) to get a tree of [Compound], [List], [Int], ...
// values, much like decoding JSON into map[string]any.
//
// # Type mapping
//
// Go values are encoded as follows:
//
//	bool                     Byte (0 or 1)
//	int8, uint8              Byte
//	int16, uint16            Short
//	int32, uint32            Int
//	int, uint                Int (an error if the value does not fit)
//	int64, uint64            Long
//	float32                  Float
//	float64                  Double
//	string                   String
//	[]byte, []int8           ByteArray
//	[]int32, []uint32        IntArray
//	[]int64, []uint64        LongArray
//	other slices and arrays  List (elements must all encode to the same type)
//	map[string]T             Compound
//	struct                   Compound
//	pointer, interface       the value pointed to; nil is omitted
//
// The "list" tag option forces a byte, int or long slice to be encoded as a
// List instead of an array, which some Minecraft formats require (for example
// the "pos" field in structure files is a List of Int):
//
//	Pos [3]int32 `nbt:"pos,list"`
//
// NBT has no null value, so nil pointers, interfaces and maps-of-nothing are
// left out of the enclosing compound rather than written as an empty value.
//
// Decoding is lenient in the same way encoding/json is: any integer tag can be
// decoded into any Go integer (or bool) as long as the value fits, any numeric
// tag into a float, arrays and lists into slices, and unknown compound keys
// are ignored unless [Decoder.DisallowUnknownFields] is set.
//
// # Compression
//
// [Unmarshal] and [Decoder] detect gzip and zlib compressed input
// automatically. [Encoder.SetCompression] selects compression when writing;
// Java Edition structure files are gzip compressed.
//
// # SNBT
//
// Every [Tag] formats itself as stringified NBT (the syntax used by commands
// such as /data and /give) via its String method, and [ParseSNBT],
// [MarshalSNBT] and [UnmarshalSNBT] convert in the other direction.
package nbt

import "fmt"

// TagType identifies the type of an NBT tag as it appears on the wire.
type TagType byte

// The NBT tag types, with their on-the-wire IDs.
const (
	TagEnd       TagType = 0
	TagByte      TagType = 1
	TagShort     TagType = 2
	TagInt       TagType = 3
	TagLong      TagType = 4
	TagFloat     TagType = 5
	TagDouble    TagType = 6
	TagByteArray TagType = 7
	TagString    TagType = 8
	TagList      TagType = 9
	TagCompound  TagType = 10
	TagIntArray  TagType = 11
	TagLongArray TagType = 12
)

var tagTypeNames = [...]string{
	TagEnd:       "TAG_End",
	TagByte:      "TAG_Byte",
	TagShort:     "TAG_Short",
	TagInt:       "TAG_Int",
	TagLong:      "TAG_Long",
	TagFloat:     "TAG_Float",
	TagDouble:    "TAG_Double",
	TagByteArray: "TAG_Byte_Array",
	TagString:    "TAG_String",
	TagList:      "TAG_List",
	TagCompound:  "TAG_Compound",
	TagIntArray:  "TAG_Int_Array",
	TagLongArray: "TAG_Long_Array",
}

func (t TagType) String() string {
	if int(t) < len(tagTypeNames) {
		return tagTypeNames[t]
	}
	return fmt.Sprintf("TagType(%d)", byte(t))
}

func (t TagType) valid() bool { return t <= TagLongArray }

// Tag is an NBT value. The concrete types are [Byte], [Short], [Int], [Long],
// [Float], [Double], [ByteArray], [String], [List], [Compound], [IntArray] and
// [LongArray]. The set is closed: other packages cannot implement Tag.
//
// The String method of every Tag returns its SNBT representation.
type Tag interface {
	// Type reports the wire type of the tag.
	Type() TagType
	String() string
	isTag()
}

type (
	// Byte is a TAG_Byte, a signed 8-bit integer. Minecraft also uses it
	// for booleans.
	Byte int8
	// Short is a TAG_Short, a signed 16-bit integer.
	Short int16
	// Int is a TAG_Int, a signed 32-bit integer.
	Int int32
	// Long is a TAG_Long, a signed 64-bit integer.
	Long int64
	// Float is a TAG_Float, a 32-bit IEEE 754 number.
	Float float32
	// Double is a TAG_Double, a 64-bit IEEE 754 number.
	Double float64
	// ByteArray is a TAG_Byte_Array.
	ByteArray []byte
	// String is a TAG_String.
	String string
	// List is a TAG_List. All elements must have the same type. An empty
	// List is written with element type TAG_End, as Minecraft does.
	List []Tag
	// Compound is a TAG_Compound: a set of uniquely named tags. Key order is
	// not significant in NBT; SNBT output sorts keys.
	Compound map[string]Tag
	// IntArray is a TAG_Int_Array.
	IntArray []int32
	// LongArray is a TAG_Long_Array.
	LongArray []int64
)

func (Byte) Type() TagType      { return TagByte }
func (Short) Type() TagType     { return TagShort }
func (Int) Type() TagType       { return TagInt }
func (Long) Type() TagType      { return TagLong }
func (Float) Type() TagType     { return TagFloat }
func (Double) Type() TagType    { return TagDouble }
func (ByteArray) Type() TagType { return TagByteArray }
func (String) Type() TagType    { return TagString }
func (List) Type() TagType      { return TagList }
func (Compound) Type() TagType  { return TagCompound }
func (IntArray) Type() TagType  { return TagIntArray }
func (LongArray) Type() TagType { return TagLongArray }

func (Byte) isTag()      {}
func (Short) isTag()     {}
func (Int) isTag()       {}
func (Long) isTag()      {}
func (Float) isTag()     {}
func (Double) isTag()    {}
func (ByteArray) isTag() {}
func (String) isTag()    {}
func (List) isTag()      {}
func (Compound) isTag()  {}
func (IntArray) isTag()  {}
func (LongArray) isTag() {}

// ElemType reports the type of the list's elements, or TagEnd if it is empty.
func (l List) ElemType() TagType {
	if len(l) == 0 {
		return TagEnd
	}
	return l[0].Type()
}

package nbt

import (
	"bufio"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"reflect"
)

// Compression selects how an [Encoder] compresses its output.
type Compression int

const (
	// None writes uncompressed NBT. This is the default.
	None Compression = iota
	// Gzip writes gzip-compressed NBT, the format of Java Edition .nbt
	// structure files, level.dat and player data.
	Gzip
	// Zlib writes zlib-compressed NBT, as used for chunks in region files.
	Zlib
)

func (c Compression) String() string {
	switch c {
	case None:
		return "none"
	case Gzip:
		return "gzip"
	case Zlib:
		return "zlib"
	}
	return fmt.Sprintf("Compression(%d)", int(c))
}

// An Encoder writes NBT values to an output stream.
type Encoder struct {
	w           io.Writer
	rootName    string
	compression Compression
}

// NewEncoder returns a new encoder that writes to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// SetRootName sets the name written for the root tag. It defaults to the
// empty string, which is what Java Edition uses.
func (e *Encoder) SetRootName(name string) { e.rootName = name }

// SetCompression sets the compression applied to each value written. With
// Gzip or Zlib, every call to Encode writes one complete compressed stream.
func (e *Encoder) SetCompression(c Compression) { e.compression = c }

// Encode writes the NBT encoding of v to the stream as a named root tag. See
// [Marshal] for details of the conversion.
func (e *Encoder) Encode(v any) error {
	t, err := ToTag(v)
	if err != nil {
		return err
	}
	return e.EncodeTag(t)
}

// EncodeTag writes t to the stream as a named root tag.
func (e *Encoder) EncodeTag(t Tag) error {
	var (
		w   io.Writer
		cw  io.WriteCloser
		err error
	)
	switch e.compression {
	case None:
		w = e.w
	case Gzip:
		cw = gzip.NewWriter(e.w)
	case Zlib:
		cw = zlib.NewWriter(e.w)
	default:
		return fmt.Errorf("nbt: unknown compression %v", e.compression)
	}
	if cw != nil {
		w = cw
	}
	err = (&writer{w: w}).writeRoot(e.rootName, t)
	if cw != nil {
		if cerr := cw.Close(); err == nil {
			err = cerr
		}
	}
	return err
}

// A Decoder reads NBT values from an input stream.
type Decoder struct {
	src             *bufio.Reader
	r               io.Reader // decompressed stream, set on first Decode
	rootName        string
	compression     Compression
	disallowUnknown bool
}

// NewDecoder returns a new decoder that reads from r. Gzip or zlib
// compression is detected from the first bytes of the stream.
//
// The decoder may read data from r beyond the NBT values requested.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{src: bufio.NewReader(r)}
}

// DisallowUnknownFields causes Decode to return an [UnknownFieldError] when a
// compound contains a key that does not match any field of the destination
// struct.
func (d *Decoder) DisallowUnknownFields() { d.disallowUnknown = true }

// RootName returns the name of the root tag most recently decoded.
func (d *Decoder) RootName() string { return d.rootName }

// Compression reports the compression detected on the stream. It is only
// meaningful after the first call to Decode or DecodeTag.
func (d *Decoder) Compression() Compression { return d.compression }

func (d *Decoder) init() error {
	if d.r != nil {
		return nil
	}
	head, err := d.src.Peek(2)
	if err != nil && len(head) == 0 {
		if err == io.EOF {
			return io.EOF
		}
		return err
	}
	switch {
	case len(head) == 2 && head[0] == 0x1f && head[1] == 0x8b:
		zr, err := gzip.NewReader(d.src)
		if err != nil {
			return err
		}
		d.r, d.compression = zr, Gzip
	case len(head) == 2 && head[0] == 0x78 && (uint16(head[0])<<8|uint16(head[1]))%31 == 0:
		// 0x78 is never a valid tag type, so this cannot be raw NBT.
		zr, err := zlib.NewReader(d.src)
		if err != nil {
			return err
		}
		d.r, d.compression = zr, Zlib
	default:
		d.r, d.compression = d.src, None
	}
	return nil
}

// DecodeTag reads the next root tag from the stream and returns it as a tree.
// It returns io.EOF when the stream has no more values.
func (d *Decoder) DecodeTag() (Tag, error) {
	if err := d.init(); err != nil {
		return nil, err
	}
	name, t, err := (&reader{r: d.r}).readRoot()
	if errors.Is(err, io.EOF) && d.compression == Zlib {
		// zlib has no multi-stream framing, so a later Encode with zlib
		// compression starts a fresh stream; look for one.
		d.r = nil
		if err := d.init(); err != nil {
			return nil, err
		}
		name, t, err = (&reader{r: d.r}).readRoot()
	}
	if err != nil {
		return nil, err
	}
	d.rootName = name
	return t, nil
}

// Decode reads the next root tag from the stream and stores it in the value
// pointed to by v. See [Unmarshal] for details of the conversion. It returns
// io.EOF when the stream has no more values.
func (d *Decoder) Decode(v any) error {
	if rv := reflect.ValueOf(v); rv.Kind() != reflect.Pointer || rv.IsNil() {
		return &InvalidUnmarshalError{Type: reflect.TypeOf(v)}
	}
	t, err := d.DecodeTag()
	if err != nil {
		return err
	}
	ds := &decodeState{disallowUnknown: d.disallowUnknown}
	return ds.root(t, v)
}

package nbt

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"unicode/utf16"
	"unicode/utf8"
)

// MaxDepth is the maximum nesting depth of lists and compounds accepted when
// reading, matching Minecraft's own limit.
const MaxDepth = 512

// readChunk bounds how much memory is allocated up front for a length-prefixed
// value, so that a corrupt or hostile length cannot force a huge allocation
// before any data has been read.
const readChunk = 1 << 16

// A SyntaxError describes malformed binary NBT.
type SyntaxError struct {
	msg    string
	Offset int64 // bytes read before the error occurred
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("nbt: %s (at offset %d)", e.msg, e.Offset)
}

// reader decodes the binary (Java Edition, big-endian) form of NBT.
type reader struct {
	r     io.Reader
	n     int64
	depth int
	buf   [8]byte
}

func (r *reader) errorf(format string, args ...any) error {
	return &SyntaxError{msg: fmt.Sprintf(format, args...), Offset: r.n}
}

func (r *reader) full(p []byte) error {
	n, err := io.ReadFull(r.r, p)
	r.n += int64(n)
	if err != nil {
		if errors.Is(err, io.EOF) && n == 0 {
			return err
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return io.ErrUnexpectedEOF
		}
		return err
	}
	return nil
}

func (r *reader) readN(n int) ([]byte, error) {
	if err := r.full(r.buf[:n]); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	return r.buf[:n], nil
}

func (r *reader) u8() (byte, error) {
	b, err := r.readN(1)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

func (r *reader) u16() (uint16, error) {
	b, err := r.readN(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b), nil
}

func (r *reader) u32() (uint32, error) {
	b, err := r.readN(4)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(b), nil
}

func (r *reader) u64() (uint64, error) {
	b, err := r.readN(8)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(b), nil
}

func (r *reader) length() (int, error) {
	n, err := r.u32()
	if err != nil {
		return 0, err
	}
	if int32(n) < 0 {
		return 0, r.errorf("negative length %d", int32(n))
	}
	return int(n), nil
}

// bytes reads n bytes, growing the buffer as data arrives instead of trusting
// n for the allocation size.
func (r *reader) bytes(n int) ([]byte, error) {
	if n <= readChunk {
		b := make([]byte, n)
		if err := r.full(b); err != nil {
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			return nil, err
		}
		return b, nil
	}
	b := make([]byte, 0, readChunk)
	for len(b) < n {
		k := min(n-len(b), readChunk)
		start := len(b)
		b = append(b, make([]byte, k)...)
		if err := r.full(b[start:]); err != nil {
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			return nil, err
		}
	}
	return b, nil
}

func (r *reader) string() (string, error) {
	n, err := r.u16()
	if err != nil {
		return "", err
	}
	b, err := r.bytes(int(n))
	if err != nil {
		return "", err
	}
	return decodeMUTF8(b), nil
}

// readRoot reads a named root tag. It returns io.EOF if the stream is empty.
func (r *reader) readRoot() (name string, tag Tag, err error) {
	if err := r.full(r.buf[:1]); err != nil {
		return "", nil, err // io.EOF here means a clean end of stream
	}
	t := TagType(r.buf[0])
	if t == TagEnd {
		return "", nil, r.errorf("root tag is TAG_End")
	}
	if !t.valid() {
		return "", nil, r.errorf("invalid tag type %d", byte(t))
	}
	if name, err = r.string(); err != nil {
		return "", nil, err
	}
	tag, err = r.payload(t)
	return name, tag, err
}

func (r *reader) payload(t TagType) (Tag, error) {
	switch t {
	case TagByte:
		b, err := r.u8()
		return Byte(int8(b)), err
	case TagShort:
		v, err := r.u16()
		return Short(int16(v)), err
	case TagInt:
		v, err := r.u32()
		return Int(int32(v)), err
	case TagLong:
		v, err := r.u64()
		return Long(int64(v)), err
	case TagFloat:
		v, err := r.u32()
		return Float(math.Float32frombits(v)), err
	case TagDouble:
		v, err := r.u64()
		return Double(math.Float64frombits(v)), err
	case TagByteArray:
		n, err := r.length()
		if err != nil {
			return nil, err
		}
		b, err := r.bytes(n)
		return ByteArray(b), err
	case TagString:
		s, err := r.string()
		return String(s), err
	case TagIntArray:
		n, err := r.length()
		if err != nil {
			return nil, err
		}
		if n > math.MaxInt32/4 {
			return nil, r.errorf("int array length %d too large", n)
		}
		b, err := r.bytes(n * 4)
		if err != nil {
			return nil, err
		}
		a := make(IntArray, n)
		for i := range a {
			a[i] = int32(binary.BigEndian.Uint32(b[i*4:]))
		}
		return a, nil
	case TagLongArray:
		n, err := r.length()
		if err != nil {
			return nil, err
		}
		if n > math.MaxInt32/8 {
			return nil, r.errorf("long array length %d too large", n)
		}
		b, err := r.bytes(n * 8)
		if err != nil {
			return nil, err
		}
		a := make(LongArray, n)
		for i := range a {
			a[i] = int64(binary.BigEndian.Uint64(b[i*8:]))
		}
		return a, nil
	case TagList:
		return r.list()
	case TagCompound:
		return r.compound()
	}
	return nil, r.errorf("invalid tag type %d", byte(t))
}

func (r *reader) push() error {
	r.depth++
	if r.depth > MaxDepth {
		return r.errorf("exceeded max depth of %d", MaxDepth)
	}
	return nil
}

func (r *reader) list() (Tag, error) {
	if err := r.push(); err != nil {
		return nil, err
	}
	defer func() { r.depth-- }()
	typ, err := r.u8()
	if err != nil {
		return nil, err
	}
	et := TagType(typ)
	if !et.valid() {
		return nil, r.errorf("invalid list element type %d", typ)
	}
	n, err := r.length()
	if err != nil {
		return nil, err
	}
	if et == TagEnd {
		if n != 0 {
			return nil, r.errorf("list of TAG_End with length %d", n)
		}
		return List{}, nil
	}
	l := make(List, 0, min(n, readChunk))
	for i := 0; i < n; i++ {
		v, err := r.payload(et)
		if err != nil {
			return nil, err
		}
		if et == TagCompound {
			v = unwrap(v.(Compound))
		}
		l = append(l, v)
	}
	return l, nil
}

// wrap and unwrap are how a list that mixes types is stored, as
// Minecraft stores one: as a list of compounds, with each element that is
// not a compound (or that looks like a wrapper) inside one under the key "".
func wrap(t Tag) Tag {
	if c, ok := t.(Compound); ok && !isWrapper(c) {
		return c
	}
	return Compound{"": t}
}

func unwrap(c Compound) Tag {
	if isWrapper(c) {
		return c[""]
	}
	return c
}

func isWrapper(c Compound) bool {
	v, ok := c[""]
	return ok && v != nil && len(c) == 1
}

func (r *reader) compound() (Tag, error) {
	if err := r.push(); err != nil {
		return nil, err
	}
	defer func() { r.depth-- }()
	c := Compound{}
	for {
		typ, err := r.u8()
		if err != nil {
			return nil, err
		}
		t := TagType(typ)
		if t == TagEnd {
			return c, nil
		}
		if !t.valid() {
			return nil, r.errorf("invalid tag type %d", typ)
		}
		name, err := r.string()
		if err != nil {
			return nil, err
		}
		v, err := r.payload(t)
		if err != nil {
			return nil, err
		}
		c[name] = v
	}
}

// writer encodes the binary (Java Edition, big-endian) form of NBT.
type writer struct {
	w   io.Writer
	buf []byte
	err error
}

// flushAt bounds the size of the writer's internal buffer.
const flushAt = 1 << 15

func (w *writer) flush() error {
	if w.err == nil && len(w.buf) > 0 {
		_, w.err = w.w.Write(w.buf)
	}
	w.buf = w.buf[:0]
	return w.err
}

func (w *writer) maybeFlush() {
	if len(w.buf) >= flushAt {
		_ = w.flush() // the error is kept in w.err
	}
}

func (w *writer) u8(b byte)    { w.buf = append(w.buf, b) }
func (w *writer) u16(v uint16) { w.buf = binary.BigEndian.AppendUint16(w.buf, v) }
func (w *writer) u32(v uint32) { w.buf = binary.BigEndian.AppendUint32(w.buf, v) }
func (w *writer) u64(v uint64) { w.buf = binary.BigEndian.AppendUint64(w.buf, v) }

func (w *writer) string(s string) error {
	b := encodeMUTF8(s)
	if len(b) > math.MaxUint16 {
		return fmt.Errorf("nbt: string of %d bytes exceeds the 65535 byte limit", len(b))
	}
	w.u16(uint16(len(b)))
	w.buf = append(w.buf, b...)
	return nil
}

func (w *writer) writeRoot(name string, t Tag) error {
	if t == nil {
		return errors.New("nbt: cannot write a nil root tag")
	}
	w.u8(byte(t.Type()))
	if err := w.string(name); err != nil {
		return err
	}
	if err := w.payload(t); err != nil {
		return err
	}
	return w.flush()
}

func (w *writer) payload(t Tag) error {
	switch v := t.(type) {
	case Byte:
		w.u8(byte(v))
	case Short:
		w.u16(uint16(v))
	case Int:
		w.u32(uint32(v))
	case Long:
		w.u64(uint64(v))
	case Float:
		w.u32(math.Float32bits(float32(v)))
	case Double:
		w.u64(math.Float64bits(float64(v)))
	case ByteArray:
		if err := checkLen(len(v)); err != nil {
			return err
		}
		w.u32(uint32(len(v)))
		w.buf = append(w.buf, v...)
	case String:
		return w.string(string(v))
	case IntArray:
		if err := checkLen(len(v)); err != nil {
			return err
		}
		w.u32(uint32(len(v)))
		for _, x := range v {
			w.u32(uint32(x))
			w.maybeFlush()
		}
	case LongArray:
		if err := checkLen(len(v)); err != nil {
			return err
		}
		w.u32(uint32(len(v)))
		for _, x := range v {
			w.u64(uint64(x))
			w.maybeFlush()
		}
	case List:
		if err := checkLen(len(v)); err != nil {
			return err
		}
		for i, e := range v {
			if e == nil {
				return fmt.Errorf("nbt: nil element at list index %d", i)
			}
		}
		et := v.ElemType()
		w.u8(byte(et))
		w.u32(uint32(len(v)))
		mixed := v.Mixed()
		for _, e := range v {
			if mixed {
				e = wrap(e)
			}
			if err := w.payload(e); err != nil {
				return err
			}
		}
	case Compound:
		for _, k := range sortedKeys(v) {
			e := v[k]
			if e == nil {
				continue
			}
			w.u8(byte(e.Type()))
			if err := w.string(k); err != nil {
				return err
			}
			if err := w.payload(e); err != nil {
				return err
			}
		}
		w.u8(byte(TagEnd))
	case nil:
		return errors.New("nbt: cannot write nil tag")
	default:
		return fmt.Errorf("nbt: unknown tag type %T", t)
	}
	w.maybeFlush()
	return w.err
}

func checkLen(n int) error {
	if n > math.MaxInt32 {
		return fmt.Errorf("nbt: length %d exceeds the maximum of %d", n, math.MaxInt32)
	}
	return nil
}

// encodeMUTF8 converts s to Java's "modified UTF-8": NUL is written as the two
// byte sequence C0 80, and characters outside the BMP are written as a UTF-16
// surrogate pair with each half encoded as three bytes.
func encodeMUTF8(s string) []byte {
	plain := true
	for i := 0; i < len(s); i++ {
		if s[i] == 0 || s[i] >= 0xF0 {
			plain = false
			break
		}
	}
	if plain && utf8.ValidString(s) {
		return []byte(s)
	}
	b := make([]byte, 0, len(s)+8)
	for _, r := range s {
		switch {
		case r == 0:
			b = append(b, 0xC0, 0x80)
		case r < 0x10000:
			b = utf8.AppendRune(b, r)
		default:
			hi, lo := utf16.EncodeRune(r)
			b = appendSurrogate(b, hi)
			b = appendSurrogate(b, lo)
		}
	}
	return b
}

func appendSurrogate(b []byte, r rune) []byte {
	return append(b, byte(0xE0|(r>>12)), byte(0x80|((r>>6)&0x3F)), byte(0x80|(r&0x3F)))
}

// decodeMUTF8 is the inverse of encodeMUTF8. Invalid sequences decode to
// U+FFFD rather than failing.
func decodeMUTF8(b []byte) string {
	plain := true
	for _, c := range b {
		if c == 0xC0 || c == 0xED {
			plain = false
			break
		}
	}
	if plain {
		return string(b)
	}
	out := make([]rune, 0, len(b))
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case c == 0xC0 && i+1 < len(b) && b[i+1] == 0x80:
			out = append(out, 0)
			i += 2
		case c == 0xED && i+2 < len(b):
			r := rune(c&0x0F)<<12 | rune(b[i+1]&0x3F)<<6 | rune(b[i+2]&0x3F)
			i += 3
			if utf16.IsSurrogate(r) && r < 0xDC00 && i+2 < len(b) && b[i] == 0xED {
				r2 := rune(b[i]&0x0F)<<12 | rune(b[i+1]&0x3F)<<6 | rune(b[i+2]&0x3F)
				if r2 >= 0xDC00 && r2 <= 0xDFFF {
					out = append(out, utf16.DecodeRune(r, r2))
					i += 3
					continue
				}
			}
			if utf16.IsSurrogate(r) {
				r = utf8.RuneError
			}
			out = append(out, r)
		default:
			r, size := utf8.DecodeRune(b[i:])
			out = append(out, r)
			i += size
		}
	}
	return string(out)
}

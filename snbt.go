package nbt

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// MarshalSNBT returns the stringified NBT (SNBT) form of v, the syntax used
// by Minecraft commands such as /data and /give. Compound keys are sorted so
// the output is deterministic.
func MarshalSNBT(v any) (string, error) {
	t, err := ToTag(v)
	if err != nil {
		return "", err
	}
	return t.String(), nil
}

// UnmarshalSNBT parses SNBT and stores the result in the value pointed to by
// v, using the same rules as [Unmarshal].
func UnmarshalSNBT(s string, v any) error {
	t, err := ParseSNBT(s)
	if err != nil {
		return err
	}
	return FromTag(t, v)
}

// ParseSNBT parses a single SNBT value, such as
//
//	{Items:[{Slot:0b,id:"minecraft:diamond",count:3}],CustomName:'"Loot"'}
//
// Unquoted values are typed the way Minecraft types them: 1b is a Byte, 1s a
// Short, 1 an Int, 1L a Long, 1.5f a Float, 1.5 or 1.5d a Double, true and
// false are Bytes, and anything else is a String.
func ParseSNBT(s string) (Tag, error) {
	p := &snbtParser{s: s}
	t, err := p.value()
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	if p.pos != len(p.s) {
		return nil, p.errorf("unexpected trailing data")
	}
	return t, nil
}

// An SNBTSyntaxError describes malformed SNBT.
type SNBTSyntaxError struct {
	msg    string
	Offset int // byte offset in the input
}

func (e *SNBTSyntaxError) Error() string {
	return fmt.Sprintf("nbt: invalid SNBT: %s at offset %d", e.msg, e.Offset)
}

// ---- formatting ----

func (t Byte) String() string   { return strconv.FormatInt(int64(t), 10) + "b" }
func (t Short) String() string  { return strconv.FormatInt(int64(t), 10) + "s" }
func (t Int) String() string    { return strconv.FormatInt(int64(t), 10) }
func (t Long) String() string   { return strconv.FormatInt(int64(t), 10) + "L" }
func (t Float) String() string  { return strconv.FormatFloat(float64(t), 'g', -1, 32) + "f" }
func (t Double) String() string { return strconv.FormatFloat(float64(t), 'g', -1, 64) + "d" }
func (t String) String() string { return quoteSNBT(string(t)) }

func (t ByteArray) String() string {
	var b strings.Builder
	b.WriteString("[B;")
	for i, x := range t {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(Byte(int8(x)).String())
	}
	b.WriteByte(']')
	return b.String()
}

func (t IntArray) String() string {
	var b strings.Builder
	b.WriteString("[I;")
	for i, x := range t {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatInt(int64(x), 10))
	}
	b.WriteByte(']')
	return b.String()
}

func (t LongArray) String() string {
	var b strings.Builder
	b.WriteString("[L;")
	for i, x := range t {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatInt(x, 10))
		b.WriteByte('L')
	}
	b.WriteByte(']')
	return b.String()
}

func (t List) String() string {
	var b strings.Builder
	b.WriteByte('[')
	for i, x := range t {
		if i > 0 {
			b.WriteByte(',')
		}
		if x == nil {
			continue
		}
		b.WriteString(x.String())
	}
	b.WriteByte(']')
	return b.String()
}

func (t Compound) String() string {
	var b strings.Builder
	b.WriteByte('{')
	first := true
	for _, k := range sortedKeys(t) {
		if t[k] == nil {
			continue
		}
		if !first {
			b.WriteByte(',')
		}
		first = false
		if isUnquoted(k) {
			b.WriteString(k)
		} else {
			b.WriteString(quoteSNBT(k))
		}
		b.WriteByte(':')
		b.WriteString(t[k].String())
	}
	b.WriteByte('}')
	return b.String()
}

func isUnquotedChar(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
		c == '_' || c == '-' || c == '.' || c == '+'
}

func isUnquoted(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isUnquotedChar(s[i]) {
			return false
		}
	}
	return true
}

// quoteSNBT quotes s, preferring double quotes but switching to single quotes
// when that avoids escaping (common for JSON text components).
func quoteSNBT(s string) string {
	q := byte('"')
	if strings.IndexByte(s, '"') >= 0 && strings.IndexByte(s, '\'') < 0 {
		q = '\''
	}
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte(q)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' || c == q {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	b.WriteByte(q)
	return b.String()
}

// ---- parsing ----

type snbtParser struct {
	s     string
	pos   int
	depth int
}

func (p *snbtParser) errorf(format string, args ...any) error {
	return &SNBTSyntaxError{msg: fmt.Sprintf(format, args...), Offset: p.pos}
}

func (p *snbtParser) skipSpace() {
	for p.pos < len(p.s) {
		switch p.s[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *snbtParser) peek() byte {
	if p.pos < len(p.s) {
		return p.s[p.pos]
	}
	return 0
}

func (p *snbtParser) expect(c byte) error {
	p.skipSpace()
	if p.peek() != c {
		if p.pos >= len(p.s) {
			return p.errorf("expected %q, got end of input", c)
		}
		return p.errorf("expected %q, got %q", c, p.s[p.pos])
	}
	p.pos++
	return nil
}

func (p *snbtParser) value() (Tag, error) {
	p.skipSpace()
	switch c := p.peek(); c {
	case '{':
		return p.compound()
	case '[':
		if p.pos+2 < len(p.s) && p.s[p.pos+2] == ';' {
			switch p.s[p.pos+1] {
			case 'B', 'I', 'L':
				return p.array()
			}
		}
		return p.list()
	case '"', '\'':
		s, err := p.quoted()
		return String(s), err
	case 0:
		return nil, p.errorf("expected value, got end of input")
	default:
		start := p.pos
		tok := p.token()
		if tok == "" {
			return nil, p.errorf("unexpected %q", c)
		}
		t := parseLiteral(tok)
		if t == nil {
			p.pos = start
			return nil, p.errorf("invalid value %q", tok)
		}
		return t, nil
	}
}

func (p *snbtParser) token() string {
	start := p.pos
	for p.pos < len(p.s) && isUnquotedChar(p.s[p.pos]) {
		p.pos++
	}
	return p.s[start:p.pos]
}

func (p *snbtParser) quoted() (string, error) {
	q := p.s[p.pos]
	p.pos++
	var b strings.Builder
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		switch c {
		case q:
			p.pos++
			return b.String(), nil
		case '\\':
			p.pos++
			if p.pos >= len(p.s) {
				return "", p.errorf("unterminated escape")
			}
			e := p.s[p.pos]
			p.pos++
			switch e {
			case '\\', '\'', '"':
				b.WriteByte(e)
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case 's':
				b.WriteByte(' ')
			case 'x', 'u', 'U':
				n := map[byte]int{'x': 2, 'u': 4, 'U': 8}[e]
				if p.pos+n > len(p.s) {
					return "", p.errorf("short \\%c escape", e)
				}
				r, err := strconv.ParseUint(p.s[p.pos:p.pos+n], 16, 32)
				if err != nil || !utf8.ValidRune(rune(r)) {
					return "", p.errorf("invalid \\%c escape", e)
				}
				b.WriteRune(rune(r))
				p.pos += n
			default:
				p.pos--
				return "", p.errorf("invalid escape \\%c", e)
			}
		default:
			b.WriteByte(c)
			p.pos++
		}
	}
	return "", p.errorf("unterminated string")
}

func (p *snbtParser) push() error {
	p.depth++
	if p.depth > MaxDepth {
		return p.errorf("exceeded max depth of %d", MaxDepth)
	}
	return nil
}

func (p *snbtParser) compound() (Tag, error) {
	if err := p.push(); err != nil {
		return nil, err
	}
	defer func() { p.depth-- }()
	p.pos++ // '{'
	c := Compound{}
	p.skipSpace()
	if p.peek() == '}' {
		p.pos++
		return c, nil
	}
	for {
		p.skipSpace()
		var key string
		switch p.peek() {
		case '"', '\'':
			k, err := p.quoted()
			if err != nil {
				return nil, err
			}
			key = k
		default:
			key = p.token()
			if key == "" {
				return nil, p.errorf("expected compound key")
			}
		}
		if err := p.expect(':'); err != nil {
			return nil, err
		}
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		c[key] = v
		p.skipSpace()
		switch p.peek() {
		case ',':
			p.pos++
		case '}':
			p.pos++
			return c, nil
		default:
			return nil, p.errorf("expected ',' or '}' in compound")
		}
	}
}

func (p *snbtParser) list() (Tag, error) {
	if err := p.push(); err != nil {
		return nil, err
	}
	defer func() { p.depth-- }()
	p.pos++ // '['
	l := List{}
	p.skipSpace()
	if p.peek() == ']' {
		p.pos++
		return l, nil
	}
	for {
		start := p.pos
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		if len(l) > 0 && v.Type() != l[0].Type() {
			p.pos = start
			return nil, p.errorf("list mixes %v and %v", l[0].Type(), v.Type())
		}
		l = append(l, v)
		p.skipSpace()
		switch p.peek() {
		case ',':
			p.pos++
		case ']':
			p.pos++
			return l, nil
		default:
			return nil, p.errorf("expected ',' or ']' in list")
		}
	}
}

func (p *snbtParser) array() (Tag, error) {
	kind := p.s[p.pos+1]
	p.pos += 3 // "[X;"
	var vals []int64
	p.skipSpace()
	if p.peek() == ']' {
		p.pos++
	} else {
		for {
			p.skipSpace()
			start := p.pos
			tok := p.token()
			t := parseLiteral(tok)
			n, ok := tagInt(t)
			if tok == "" || !ok {
				p.pos = start
				return nil, p.errorf("invalid element in [%c;] array", kind)
			}
			vals = append(vals, n)
			p.skipSpace()
			if p.peek() == ',' {
				p.pos++
				continue
			}
			if err := p.expect(']'); err != nil {
				return nil, err
			}
			break
		}
	}
	switch kind {
	case 'B':
		a := make(ByteArray, len(vals))
		for i, n := range vals {
			if n < math.MinInt8 || n > math.MaxInt8 {
				return nil, p.errorf("value %d out of range for byte array", n)
			}
			a[i] = byte(int8(n))
		}
		return a, nil
	case 'I':
		a := make(IntArray, len(vals))
		for i, n := range vals {
			if n < math.MinInt32 || n > math.MaxInt32 {
				return nil, p.errorf("value %d out of range for int array", n)
			}
			a[i] = int32(n)
		}
		return a, nil
	default:
		return LongArray(vals), nil
	}
}

var (
	reDoubleBare = regexp.MustCompile(`(?i)^[-+]?(?:[0-9]+[.]|[0-9]*[.][0-9]+)(?:e[-+]?[0-9]+)?$`)
	reDouble     = regexp.MustCompile(`(?i)^[-+]?(?:[0-9]+[.]?|[0-9]*[.][0-9]+)(?:e[-+]?[0-9]+)?d$`)
	reFloat      = regexp.MustCompile(`(?i)^[-+]?(?:[0-9]+[.]?|[0-9]*[.][0-9]+)(?:e[-+]?[0-9]+)?f$`)
	reInteger    = regexp.MustCompile(`(?i)^[-+]?(?:0|[1-9][0-9]*)([bslBSL]?)$`)
)

// parseLiteral types an unquoted SNBT token. Tokens that look numeric but are
// out of range become Strings, as in Minecraft.
func parseLiteral(tok string) Tag {
	if tok == "" {
		return nil
	}
	switch strings.ToLower(tok) {
	case "true":
		return Byte(1)
	case "false":
		return Byte(0)
	}
	if m := reInteger.FindStringSubmatch(tok); m != nil {
		digits := strings.TrimRight(tok, "bslBSL")
		switch strings.ToLower(m[1]) {
		case "b":
			if n, err := strconv.ParseInt(digits, 10, 8); err == nil {
				return Byte(n)
			}
		case "s":
			if n, err := strconv.ParseInt(digits, 10, 16); err == nil {
				return Short(n)
			}
		case "l":
			if n, err := strconv.ParseInt(digits, 10, 64); err == nil {
				return Long(n)
			}
		default:
			if n, err := strconv.ParseInt(digits, 10, 32); err == nil {
				return Int(n)
			}
		}
		return String(tok)
	}
	switch {
	case reFloat.MatchString(tok):
		if f, err := strconv.ParseFloat(tok[:len(tok)-1], 32); err == nil {
			return Float(f)
		}
	case reDouble.MatchString(tok):
		if f, err := strconv.ParseFloat(tok[:len(tok)-1], 64); err == nil {
			return Double(f)
		}
	case reDoubleBare.MatchString(tok):
		if f, err := strconv.ParseFloat(tok, 64); err == nil {
			return Double(f)
		}
	}
	return String(tok)
}

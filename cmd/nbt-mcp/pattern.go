package main

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/MinecraftMetascript/nbt-go/structure"
)

// pattern is one block, or a weighted random mix of blocks written like
// "60%stone_bricks,30%cracked_stone_bricks,10%mossy_stone_bricks".
type pattern struct {
	states  []structure.BlockState
	weights []int
	total   int
}

// parsePattern parses a block or a comma-separated weighted mix. Weights are
// written as "N%block" or "N*block" and default to 1; they need not add up to
// 100. Commas inside [...] belong to the block state.
func parsePattern(s string) (pattern, error) {
	var parts []string
	depth, start := 0, 0
	for i, c := range s {
		switch c {
		case '[':
			depth++
		case ']':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	var p pattern
	for _, part := range parts {
		part = strings.TrimSpace(part)
		w := 1
		if i := strings.IndexAny(part, "%*"); i > 0 && !strings.Contains(part[:i], "[") {
			n, err := strconv.ParseFloat(strings.TrimSpace(part[:i]), 64)
			if err != nil || n <= 0 {
				return pattern{}, fmt.Errorf("invalid weight %q in %q", part[:i], s)
			}
			// Keep one decimal place of precision for weights like 2.5%.
			w = int(n*10 + 0.5)
			if w == 0 {
				w = 1
			}
			part = strings.TrimSpace(part[i+1:])
		} else if len(parts) > 1 {
			w = 10
		}
		bs, err := structure.ParseBlockState(part)
		if err != nil {
			return pattern{}, err
		}
		p.states = append(p.states, bs)
		p.weights = append(p.weights, w)
		p.total += w
	}
	return p, nil
}

func (p pattern) random() bool { return len(p.states) > 1 }

func (p pattern) pick(rng *rand.Rand) structure.BlockState {
	if len(p.states) == 1 {
		return p.states[0]
	}
	n := rng.Intn(p.total)
	for i, w := range p.weights {
		if n < w {
			return p.states[i]
		}
		n -= w
	}
	return p.states[len(p.states)-1]
}

func (p pattern) String() string {
	if len(p.states) == 1 {
		return p.states[0].String()
	}
	parts := make([]string, len(p.states))
	for i, s := range p.states {
		parts[i] = fmt.Sprintf("%.3g%%%s", 100*float64(p.weights[i])/float64(p.total), s)
	}
	return strings.Join(parts, ",")
}

// newRNG returns a random source for seed, or for a fresh seed if it is nil,
// along with the seed used.
func newRNG(seed *int64) (*rand.Rand, int64) {
	s := time.Now().UnixNano()
	if seed != nil {
		s = *seed
	}
	return rand.New(rand.NewSource(s)), s
}

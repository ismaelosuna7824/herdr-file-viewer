// Package finder implements a fuzzy file finder (the "Ctrl+P" experience):
// given a query, it ranks a list of file paths by subsequence match quality.
package finder

import (
	"sort"
	"strings"
)

// Candidate is a scored path returned by a fuzzy query.
type Candidate struct {
	Path      string
	Score     int
	Positions []int // byte indices in Path that matched, for highlighting
}

// scoring weights. Higher is better. The heuristics reward matches that a
// human reader would consider "obvious": consecutive characters, matches right
// after a path separator, and matches on the file's basename.
const (
	scoreMatch       = 16 // base points per matched byte
	bonusConsecutive = 24 // matched byte immediately follows the previous match
	bonusBoundary    = 30 // match at a word/path boundary (after / _ - . or camelCase)
	bonusBasename    = 12 // match falls within the file's basename
	bonusAllBasename = 64 // the entire query matched inside the basename
)

// Matcher holds the file list plus a precomputed lowercased copy, so repeated
// queries (one per keystroke) don't re-lowercase or re-allocate the paths. This
// is the hot path in large repos, so it is deliberately allocation-light: it
// scans bytes, never []rune. Positions are byte offsets, which equal rune
// indices for ASCII paths (the overwhelming majority).
type Matcher struct {
	paths []string
	lower []string
}

// NewMatcher precomputes the lowercased paths once.
func NewMatcher(paths []string) *Matcher {
	lower := make([]string, len(paths))
	for i, p := range paths {
		lower[i] = strings.ToLower(p)
	}
	return &Matcher{paths: paths, lower: lower}
}

// Len reports how many files the matcher holds.
func (mt *Matcher) Len() int { return len(mt.paths) }

// Match ranks the files against query. An empty query returns the paths as-is
// (unscored) so the finder shows the full list before the user types. Results
// are sorted best-first, ties broken by shorter path then lexicographically.
func (mt *Matcher) Match(query string, limit int) []Candidate {
	if mt == nil {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		n := len(mt.paths)
		if limit > 0 && limit < n {
			n = limit
		}
		out := make([]Candidate, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, Candidate{Path: mt.paths[i]})
		}
		return out
	}

	qb := []byte(q)
	out := make([]Candidate, 0, 64)
	for i, low := range mt.lower {
		if c, ok := score(qb, mt.paths[i], low); ok {
			out = append(out, c)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if len(out[i].Path) != len(out[j].Path) {
			return len(out[i].Path) < len(out[j].Path)
		}
		return out[i].Path < out[j].Path
	})

	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// score matches the lowercased query bytes q against a path's lowercased form,
// scanning right-to-left so a query binds to the latest (basename-most)
// occurrence. A naive left-to-right greedy match would latch "app" onto the 'a'
// in "intern[a]l/ui/app.go" instead of the filename. Returns ok=false when q is
// not a subsequence. Allocates only the positions slice, and only on a hit.
func score(q []byte, path, lower string) (Candidate, bool) {
	// Right-to-left feasibility + latest-occurrence positions.
	qi := len(q) - 1
	// Count first so we can allocate positions exactly once.
	matched := 0
	for i := len(lower) - 1; i >= 0 && qi >= 0; i-- {
		if lower[i] == q[qi] {
			matched++
			qi--
		}
	}
	if qi != -1 {
		return Candidate{}, false
	}

	positions := make([]int, matched)
	qi = len(q) - 1
	idx := matched - 1
	for i := len(lower) - 1; i >= 0 && qi >= 0; i-- {
		if lower[i] == q[qi] {
			positions[idx] = i
			idx--
			qi--
		}
	}

	basenameStart := basenameIndex(path)
	total := 0
	prev := -2
	for _, i := range positions {
		points := scoreMatch
		if i == prev+1 {
			points += bonusConsecutive
		}
		if isBoundary(path, i) {
			points += bonusBoundary
		}
		if i >= basenameStart {
			points += bonusBasename
		}
		total += points
		prev = i
	}
	if positions[0] >= basenameStart {
		total += bonusAllBasename
	}
	return Candidate{Path: path, Score: total, Positions: positions}, true
}

// isBoundary reports whether the byte at index i starts a new "word": the first
// byte, a byte after a separator, or an uppercase byte following a lowercase one
// (camelCase transition).
func isBoundary(s string, i int) bool {
	if i == 0 {
		return true
	}
	switch s[i-1] {
	case '/', '\\', '_', '-', '.', ' ':
		return true
	}
	return isUpper(s[i]) && isLower(s[i-1])
}

func isUpper(b byte) bool { return b >= 'A' && b <= 'Z' }
func isLower(b byte) bool { return b >= 'a' && b <= 'z' }

func basenameIndex(path string) int {
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		return i + 1
	}
	return 0
}

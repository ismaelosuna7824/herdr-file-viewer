package search

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// ignore is a pragmatic .gitignore matcher. It is NOT a full implementation of
// the gitignore spec (no negation, no nested .gitignore files, no ** globbing
// beyond a single path segment). It covers the common cases — directory names,
// "*.ext" extension patterns and anchored paths — plus a built-in list of
// directories nobody wants in a code search.
type ignore struct {
	patterns []pattern
}

type pattern struct {
	glob     string // the pattern text with a leading/trailing slash stripped
	dirOnly  bool   // pattern ended in "/", so it matches directories only
	anchored bool   // pattern began with "/", so it matches from the root only
}

// defaultIgnoredDirs are always skipped regardless of any .gitignore. These are
// the noisy directories that would otherwise dominate a search.
var defaultIgnoredDirs = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	"vendor":       {},
	"dist":         {},
	"build":        {},
	".next":        {},
	".idea":        {},
	".vscode":      {},
	"target":       {},
	".DS_Store":    {},
}

// loadIgnore reads a .gitignore at the root (if present) and returns a matcher
// seeded with the default ignored directories.
func loadIgnore(root string) *ignore {
	ig := &ignore{}
	f, err := os.Open(filepath.Join(root, ".gitignore"))
	if err != nil {
		return ig
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue // blank, comment, or unsupported negation
		}
		p := pattern{glob: line}
		if strings.HasSuffix(p.glob, "/") {
			p.dirOnly = true
			p.glob = strings.TrimSuffix(p.glob, "/")
		}
		if strings.HasPrefix(p.glob, "/") {
			p.anchored = true
			p.glob = strings.TrimPrefix(p.glob, "/")
		}
		if p.glob != "" {
			ig.patterns = append(ig.patterns, p)
		}
	}
	return ig
}

// match reports whether the given path (relative to root, using OS separators)
// should be ignored. isDir distinguishes directory-only patterns.
func (ig *ignore) match(rel string, isDir bool) bool {
	base := filepath.Base(rel)
	if isDir {
		if _, ok := defaultIgnoredDirs[base]; ok {
			return true
		}
	}
	// Ignore hidden files/dirs at any depth except the root entry itself is
	// handled by the caller; keep .gitignore-style explicitness otherwise.
	slashed := filepath.ToSlash(rel)
	for _, p := range ig.patterns {
		if p.dirOnly && !isDir {
			continue
		}
		if ig.patternMatches(p, slashed, base) {
			return true
		}
	}
	return false
}

func (ig *ignore) patternMatches(p pattern, slashedRel, base string) bool {
	if p.anchored {
		ok, _ := filepath.Match(p.glob, slashedRel)
		return ok
	}
	// Unanchored: match against the basename and against every trailing path
	// suffix, mirroring how git applies a loose pattern at any depth.
	if ok, _ := filepath.Match(p.glob, base); ok {
		return true
	}
	segments := strings.Split(slashedRel, "/")
	for i := range segments {
		suffix := strings.Join(segments[i:], "/")
		if ok, _ := filepath.Match(p.glob, suffix); ok {
			return true
		}
	}
	return false
}

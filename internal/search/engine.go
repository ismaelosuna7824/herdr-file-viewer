// Package search implements a self-contained content search engine over a
// directory tree. It has no external process dependencies (no ripgrep): the
// whole thing runs on the Go standard library plus a small .gitignore matcher.
package search

import (
	"bufio"
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Options describes a single content-search request. It mirrors the toggles a
// user sees in the UI: case sensitivity, whole-word matching and regex mode.
type Options struct {
	Query         string
	CaseSensitive bool
	WholeWord     bool
	Regex         bool
}

// Match is a single hit inside a file.
type Match struct {
	Path  string // path relative to the search root
	Line  int    // 1-based line number
	Text  string // the full line the match was found on (trimmed of newline)
	Start int    // byte offset of the match start within Text
	End   int    // byte offset of the match end within Text
}

// Result is the outcome of a search over the tree.
type Result struct {
	Matches   []Match
	Files     int  // number of files that contained at least one match
	Truncated bool // true when maxMatches was reached and results were capped
}

// Limits keep a search bounded so a huge repository can't exhaust memory or
// hang the UI. They are deliberately generous but finite.
const (
	maxMatches      = 5000
	maxFileBytes    = 8 << 20 // skip files larger than 8 MiB
	scannerBufBytes = 1 << 20 // allow lines up to 1 MiB
)

// Compile turns the user-facing options into a regular expression. Plain
// (non-regex) queries are escaped so metacharacters are treated literally,
// which is what a user typing "foo(" into a search box expects.
func Compile(o Options) (*regexp.Regexp, error) {
	pattern := o.Query
	if !o.Regex {
		pattern = regexp.QuoteMeta(pattern)
	}
	if o.WholeWord {
		pattern = `\b` + pattern + `\b`
	}
	if !o.CaseSensitive {
		pattern = `(?i)` + pattern
	}
	return regexp.Compile(pattern)
}

// Search walks root and returns every line matching the compiled query. It
// honours ctx cancellation between files so a stale search can be abandoned as
// soon as the user types the next character.
func Search(ctx context.Context, root string, o Options) (Result, error) {
	var res Result
	if strings.TrimSpace(o.Query) == "" {
		return res, nil
	}

	re, err := Compile(o)
	if err != nil {
		return res, err
	}

	ig := loadIgnore(root)
	seenFiles := map[string]struct{}{}

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are skipped, not fatal
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		if rel == "." {
			return nil
		}

		if d.IsDir() {
			if ig.match(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if ig.match(rel, false) {
			return nil
		}

		info, infoErr := d.Info()
		if infoErr != nil || info.Size() > maxFileBytes {
			return nil
		}

		matched := searchFile(ctx, path, rel, re, &res)
		if matched {
			if _, ok := seenFiles[rel]; !ok {
				seenFiles[rel] = struct{}{}
				res.Files++
			}
		}
		if res.Truncated {
			return filepath.SkipAll
		}
		return nil
	})

	if walkErr != nil && ctx.Err() != nil {
		return res, ctx.Err()
	}
	return res, nil
}

// searchFile scans a single file line by line, appending matches to res. It
// returns whether the file contained at least one match. Binary files (those
// with a NUL byte in the first chunk) are skipped.
func searchFile(ctx context.Context, path, rel string, re *regexp.Regexp, res *Result) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	// Peek to reject binary files cheaply.
	head := make([]byte, 512)
	n, _ := f.Read(head)
	if bytes.IndexByte(head[:n], 0) != -1 {
		return false
	}
	if _, err := f.Seek(0, 0); err != nil {
		return false
	}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), scannerBufBytes)

	found := false
	lineNo := 0
	for sc.Scan() {
		if res.Truncated {
			return found
		}
		if lineNo%512 == 0 && ctx.Err() != nil {
			return found
		}
		lineNo++
		line := sc.Text()
		loc := re.FindStringIndex(line)
		if loc == nil {
			continue
		}
		found = true
		res.Matches = append(res.Matches, Match{
			Path:  rel,
			Line:  lineNo,
			Text:  line,
			Start: loc[0],
			End:   loc[1],
		})
		if len(res.Matches) >= maxMatches {
			res.Truncated = true
			return found
		}
	}
	return found
}

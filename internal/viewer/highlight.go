package viewer

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// highlightLimit bounds syntax highlighting: past this size we render plain text
// to keep opening a file instant. Chroma is fast, but a multi-MB minified file
// isn't worth tokenising.
const highlightLimit = 512 << 10 // 512 KiB

// highlightLines syntax-highlights source for the given file path and returns
// the result split into lines, each carrying its own ANSI escapes. When the
// language is unknown or the file is too large, it falls back to the plain
// lines unchanged.
func highlightLines(path, source string, plain []string) []string {
	if len(source) > highlightLimit {
		return plain
	}
	lexer := lexers.Match(path)
	if lexer == nil {
		lexer = lexers.Analyse(source)
	}
	if lexer == nil {
		return plain // unknown language — leave it plain rather than guessing
	}
	lexer = chroma.Coalesce(lexer)

	style := styles.Get("catppuccin-mocha")
	if style == nil {
		style = styles.Fallback
	}
	formatter := formatters.Get("terminal256")
	if formatter == nil {
		return plain
	}

	it, err := lexer.Tokenise(nil, source)
	if err != nil {
		return plain
	}
	var buf strings.Builder
	if err := formatter.Format(&buf, style, it); err != nil {
		return plain
	}

	lines := strings.Split(buf.String(), "\n")
	// Chroma may append a trailing newline the source didn't have; drop the
	// resulting empty final element so line counts line up with the gutter.
	if len(lines) == len(plain)+1 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	// Guard against any other token/line-count mismatch: fall back to plain so
	// the gutter never drifts out of sync with the content.
	if len(lines) != len(plain) {
		return plain
	}
	return lines
}

package viewer

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/glamour"
)

// markdownExts are the file extensions rendered as documents rather than source.
var markdownExts = map[string]struct{}{
	".md":       {},
	".markdown": {},
	".mdown":    {},
	".mkd":      {},
}

func isMarkdownPath(path string) bool {
	_, ok := markdownExts[strings.ToLower(filepath.Ext(path))]
	return ok
}

// renderMarkdown turns markdown source into styled terminal output wrapped to
// the given width, using glamour's dark theme. On any error it falls back to
// the raw source so a document is never lost.
func renderMarkdown(source string, width int) string {
	if width < 20 {
		width = 20
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width-2),
	)
	if err != nil {
		return source
	}
	out, err := r.Render(source)
	if err != nil {
		return source
	}
	// Glamour brackets the document with blank lines; trim the leading one so
	// the content starts at the top of the pane.
	return strings.TrimLeft(out, "\n")
}

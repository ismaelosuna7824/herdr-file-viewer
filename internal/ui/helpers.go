package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// scrollStart returns the first index to render so that cursor stays visible
// within a window of the given number of rows.
func scrollStart(cursor, total, rows int) int {
	if total <= rows || cursor < rows {
		return 0
	}
	start := cursor - rows + 1
	if start+rows > total {
		start = total - rows
	}
	if start < 0 {
		start = 0
	}
	return start
}

// truncateLine cuts a rendered line to width display cells without breaking
// ANSI escape sequences, using lipgloss's width-aware truncation.
func truncateLine(s string, width int) string {
	if width <= 0 {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}

// padRow pads a rendered (possibly ANSI-styled) string with trailing spaces to
// exactly w display cells, or truncates it if it's already wider. Unlike
// lipgloss Width, it never wraps a long line onto a second row.
func padRow(s string, w int) string {
	if w <= 0 {
		return ""
	}
	cur := lipgloss.Width(s)
	if cur >= w {
		return truncateLine(s, w)
	}
	return s + strings.Repeat(" ", w-cur)
}

// shortHash abbreviates a commit SHA for display.
func shortHash(h string) string {
	if len(h) > 7 {
		return h[:7]
	}
	return h
}

func pluralFiles(shown, total int, loaded bool) string {
	if !loaded {
		return "indexing…"
	}
	if shown == total {
		return fmt.Sprintf("%d files", total)
	}
	return fmt.Sprintf("%d / %d files", shown, total)
}

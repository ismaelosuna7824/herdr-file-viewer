package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ismaelosuna7824/herdr-file-viewer/internal/finder"
)

const finderLimit = 200

// finderPanel is the fuzzy file finder overlay (Ctrl+P). The full file list is
// loaded once; every keystroke re-ranks it in memory, which is instant for the
// tens-of-thousands of files a normal project holds.
type finderPanel struct {
	input     textinput.Model
	matcher   *finder.Matcher
	fileCount int
	results   []finder.Candidate
	cursor    int
	loaded    bool
}

func newFinderPanel() finderPanel {
	ti := textinput.New()
	ti.Placeholder = "Go to file…"
	ti.Prompt = ""
	return finderPanel{input: ti}
}

func (p *finderPanel) setFiles(files []string) {
	p.matcher = finder.NewMatcher(files) // precompute lowercased paths once
	p.fileCount = len(files)
	p.loaded = true
	p.refresh()
}

func (p *finderPanel) focus() tea.Cmd { return p.input.Focus() }
func (p *finderPanel) blur()          { p.input.Blur() }

// refresh re-runs the fuzzy match against the current query.
func (p *finderPanel) refresh() {
	p.results = p.matcher.Match(p.input.Value(), finderLimit)
	p.cursor = 0
}

func (p *finderPanel) moveUp() {
	if p.cursor > 0 {
		p.cursor--
	}
}

func (p *finderPanel) moveDown() {
	if p.cursor < len(p.results)-1 {
		p.cursor++
	}
}

// selected returns the highlighted path, or "" when there are no results.
func (p *finderPanel) selected() string {
	if p.cursor < 0 || p.cursor >= len(p.results) {
		return ""
	}
	return p.results[p.cursor].Path
}

// view renders the finder card body: title, query input, a divider, then the
// ranked/highlighted result list padded to a stable height.
func (p *finderPanel) view(st styles, innerW, rows int) string {
	p.input.Width = innerW - 3

	lines := []string{
		st.prompt.Render("⌕ ") + p.input.View(),
		st.divider.Render(strings.Repeat("─", innerW)),
	}

	start := scrollStart(p.cursor, len(p.results), rows)
	printed := 0
	for i := start; i < start+rows && i < len(p.results); i++ {
		c := p.results[i]
		var row string
		if i == p.cursor {
			row = st.modalSel.Width(innerW).Render("▸ " + highlightPositions(c.Path, c.Positions, st.match))
		} else {
			row = "  " + highlightPositions(c.Path, c.Positions, st.match)
		}
		lines = append(lines, truncateLine(row, innerW))
		printed++
	}
	for ; printed < rows; printed++ {
		lines = append(lines, "")
	}

	lines = append(lines, st.countBadge.Render(pluralFiles(len(p.results), p.fileCount, p.loaded)))
	return strings.Join(lines, "\n")
}

// highlightPositions bolds the runes at the given indices (the fuzzy match
// positions) so the user sees why a path ranked where it did.
func highlightPositions(s string, positions []int, hl lipgloss.Style) string {
	if len(positions) == 0 {
		return s
	}
	pos := make(map[int]struct{}, len(positions))
	for _, p := range positions {
		pos[p] = struct{}{}
	}
	var b strings.Builder
	for i, r := range []rune(s) {
		if _, ok := pos[i]; ok {
			b.WriteString(hl.Render(string(r)))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ismaelosuna7824/herdr-file-viewer/internal/search"
)

// searchPanel is the content search overlay (Ctrl+F). It owns the query input
// and the three toggles shown in the editor-style search bar: case sensitivity
// (Aa), whole word (ab) and regex (.*). Running the search is the app's job;
// the panel only holds state and renders.
type searchPanel struct {
	input         textinput.Model
	caseSensitive bool
	wholeWord     bool
	regex         bool

	results   []search.Match
	cursor    int
	files     int
	truncated bool
	searching bool
	errMsg    string
}

func newSearchPanel() searchPanel {
	ti := textinput.New()
	ti.Placeholder = "Search in files…"
	ti.Prompt = ""
	return searchPanel{input: ti}
}

func (p *searchPanel) focus() tea.Cmd { return p.input.Focus() }
func (p *searchPanel) blur()          { p.input.Blur() }

func (p *searchPanel) options() search.Options {
	return search.Options{
		Query:         p.input.Value(),
		CaseSensitive: p.caseSensitive,
		WholeWord:     p.wholeWord,
		Regex:         p.regex,
	}
}

func (p *searchPanel) setResult(res search.Result, err error) {
	p.searching = false
	p.cursor = 0
	if err != nil {
		p.errMsg = err.Error()
		p.results = nil
		p.files = 0
		p.truncated = false
		return
	}
	p.errMsg = ""
	p.results = res.Matches
	p.files = res.Files
	p.truncated = res.Truncated
}

func (p *searchPanel) moveUp() {
	if p.cursor > 0 {
		p.cursor--
	}
}

func (p *searchPanel) moveDown() {
	if p.cursor < len(p.results)-1 {
		p.cursor++
	}
}

func (p *searchPanel) selected() (search.Match, bool) {
	if p.cursor < 0 || p.cursor >= len(p.results) {
		return search.Match{}, false
	}
	return p.results[p.cursor], true
}

// view renders the search card body: title, the query input with the three
// editor-style toggles, a status line, a divider, and matches grouped by file.
func (p *searchPanel) view(st styles, innerW, rows int) string {
	p.input.Width = innerW - 16

	toggles := toggle(st, "Aa", p.caseSensitive) + " " +
		toggle(st, "ab", p.wholeWord) + " " +
		toggle(st, ".*", p.regex)
	inputRow := padRow(st.prompt.Render("⌕ ")+p.input.View(), innerW-lipgloss.Width(toggles)-1) + toggles

	lines := []string{
		truncateLine(inputRow, innerW),
		st.countBadge.Render(p.status()),
		st.divider.Render(strings.Repeat("─", innerW)),
	}

	start := scrollStart(p.cursor, len(p.results), rows)
	lastFile := ""
	printed := 0
	for i := start; i < start+rows && i < len(p.results) && printed < rows; i++ {
		m := p.results[i]
		if m.Path != lastFile {
			lines = append(lines, truncateLine("  "+st.dir.Render(m.Path), innerW))
			lastFile = m.Path
			printed++
			if printed >= rows {
				break
			}
		}
		lines = append(lines, truncateLine(p.matchRow(st, m, i == p.cursor, innerW), innerW))
		printed++
	}
	for ; printed < rows; printed++ {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// matchRow renders one match: "  123  the line with the ‹match› highlighted".
func (p *searchPanel) matchRow(st styles, m search.Match, selected bool, innerW int) string {
	num := st.muted.Render(fmt.Sprintf("%5d ", m.Line))
	text := m.Text
	var line string
	if m.Start >= 0 && m.End <= len(text) && m.Start < m.End {
		line = text[:m.Start] + st.match.Render(text[m.Start:m.End]) + text[m.End:]
	} else {
		line = text
	}
	line = strings.ReplaceAll(line, "\t", "    ")
	if selected {
		return st.modalSel.Width(innerW).Render("  ▸ " + num + line)
	}
	return "    " + num + line
}

func (p *searchPanel) status() string {
	toggles := "^t case:" + onOff(p.caseSensitive) +
		" · ^w word:" + onOff(p.wholeWord) +
		" · ^r regex:" + onOff(p.regex)
	switch {
	case p.searching:
		return "searching…  " + toggles
	case p.errMsg != "":
		return "error: " + p.errMsg
	case strings.TrimSpace(p.input.Value()) == "":
		return "type to search  ·  " + toggles
	case len(p.results) == 0:
		return "no matches  ·  " + toggles
	default:
		s := fmt.Sprintf("%d matches in %d files", len(p.results), p.files)
		if p.truncated {
			s += " (truncated)"
		}
		return s + "  ·  " + toggles
	}
}

func onOff(b bool) string {
	if b {
		return "ON"
	}
	return "off"
}

func toggle(st styles, label string, on bool) string {
	if on {
		return st.toggleOn.Render(label)
	}
	return st.toggleOff.Render(label)
}

// Package viewer is the read-only file content pane. It wraps a Bubble Tea
// viewport, adds a gutter with line numbers, and can highlight a target line
// when the user jumps in from search or the fuzzy finder.
package viewer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const maxViewBytes = 4 << 20 // don't try to render files larger than 4 MiB

// Model is the viewer component.
type Model struct {
	vp         viewport.Model
	path       string
	raw        string   // normalized file contents (for re-rendering on toggle/resize)
	lines      []string // plain source lines (for line count)
	rendered   []string // syntax-highlighted lines (ANSI), or plain on fallback
	target     int      // 1-based line to highlight, 0 for none
	loadErr    string
	isMarkdown bool
	rawMode    bool // show markdown source instead of the rendered document
	forceRaw   bool // this load forces raw (e.g. a search jump needs line numbers)
	width      int
	height     int
	gutterFg   lipgloss.Style
	targetGut  lipgloss.Style
	titleStyle lipgloss.Style
}

// New builds an empty viewer.
func New() Model {
	return Model{
		vp:         viewport.New(0, 0),
		gutterFg:   lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		targetGut:  lipgloss.NewStyle().Foreground(lipgloss.Color("111")).Bold(true),
		titleStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("111")).Bold(true),
	}
}

// SetSize resizes the viewport, reserving one row for the title bar.
func (m *Model) SetSize(w, h int) {
	if w == m.width && h == m.height {
		return // no change — avoid a needless full re-render
	}
	m.width, m.height = w, h
	m.vp.Width = w
	m.vp.Height = h - 1
	if m.vp.Height < 0 {
		m.vp.Height = 0
	}
	m.render()
}

// Load reads a file into the viewer. Binary and oversized files are reported
// gracefully instead of dumping garbage into the terminal.
func (m *Model) Load(path string) {
	m.path = path
	m.target = 0
	m.loadErr = ""
	m.lines = nil
	m.rendered = nil
	m.raw = ""
	m.forceRaw = false
	m.isMarkdown = isMarkdownPath(path)

	info, err := os.Stat(path)
	if err != nil {
		m.loadErr = err.Error()
		m.render()
		return
	}
	if info.IsDir() {
		m.loadErr = "(directory)"
		m.render()
		return
	}
	if info.Size() > maxViewBytes {
		m.loadErr = fmt.Sprintf("file too large to preview (%d bytes)", info.Size())
		m.render()
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		m.loadErr = err.Error()
		m.render()
		return
	}
	if isBinary(data) {
		m.loadErr = "(binary file)"
		m.render()
		return
	}
	normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
	m.raw = normalized
	m.lines = strings.Split(normalized, "\n")
	m.rendered = nil // plain now; syntax colors arrive asynchronously
	m.render()
	m.vp.GotoTop()
}

// GoToLine highlights a 1-based line and scrolls so it sits near the top third
// of the pane, the way an editor centers a jump target. Jumping into a rendered
// markdown file forces raw mode for this view, since line numbers only map to
// the source.
func (m *Model) GoToLine(line int) {
	m.target = line
	if m.isMarkdown {
		m.forceRaw = true
	}
	m.render()
	offset := line - 1 - m.vp.Height/3
	if offset < 0 {
		offset = 0
	}
	m.vp.SetYOffset(offset)
}

// Update forwards scroll and navigation messages to the viewport.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

// View renders the title bar plus the content viewport.
func (m Model) View() string {
	title := "  no file"
	if m.path != "" {
		title = "  " + filepath.Base(m.path)
		if m.isMarkdown {
			if m.markdownActive() {
				title += "  " + m.gutterFg.Render("[rendered · m for source]")
			} else {
				title += "  " + m.gutterFg.Render("[source · m to render]")
			}
		}
	}
	bar := m.titleStyle.Width(m.width).Render(truncate(title, m.width))
	return lipgloss.JoinVertical(lipgloss.Left, bar, m.vp.View())
}

// markdownActive reports whether the current file is being shown as rendered
// markdown (as opposed to raw source).
func (m Model) markdownActive() bool {
	return m.isMarkdown && !m.rawMode && !m.forceRaw && m.raw != ""
}

// Raw returns the loaded file's normalized contents.
func (m Model) Raw() string { return m.raw }

// Path returns the loaded file's path.
func (m Model) Path() string { return m.path }

// ShouldHighlight reports whether the current file wants async syntax
// highlighting (a readable text file not being shown as rendered markdown).
func (m Model) ShouldHighlight() bool {
	return m.loadErr == "" && len(m.lines) > 0 && !m.markdownActive()
}

// SetHighlighted swaps in asynchronously-computed highlighted lines, but only if
// they still belong to the file currently shown (guards against stale results).
func (m *Model) SetHighlighted(path string, lines []string) {
	if path != m.path || len(lines) != len(m.lines) {
		return
	}
	m.rendered = lines
	m.render()
}

// Highlight is the goroutine-safe syntax highlighter used by the async pipeline:
// it takes the raw source and returns one ANSI-styled line per source line.
func Highlight(path, source string) []string {
	plain := strings.Split(source, "\n")
	return highlightLines(path, source, plain)
}

// IsMarkdown reports whether the loaded file is a markdown document, so callers
// can surface the render/source toggle only when it's relevant.
func (m Model) IsMarkdown() bool { return m.isMarkdown }

// ToggleMarkdown flips between rendered markdown and raw source. It is a no-op
// on non-markdown files.
func (m *Model) ToggleMarkdown() {
	if !m.isMarkdown {
		return
	}
	m.rawMode = !m.rawMode
	m.forceRaw = false
	m.render()
	m.vp.GotoTop()
}

// render rebuilds the viewport content: either an error/status message or the
// file body with a right-aligned line-number gutter.
func (m *Model) render() {
	if m.loadErr != "" {
		m.vp.SetContent(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("\n  " + m.loadErr))
		return
	}
	if m.markdownActive() {
		m.vp.SetContent(renderMarkdown(m.raw, m.vp.Width))
		return
	}
	if len(m.lines) == 0 {
		m.vp.SetContent("")
		return
	}
	content := m.rendered
	if len(content) == 0 {
		content = m.lines
	}
	gutterW := len(fmt.Sprintf("%d", len(m.lines)))
	var b strings.Builder
	for i, line := range content {
		n := i + 1
		var gutter string
		if n == m.target {
			// A left accent bar + colored number marks the jump target; this
			// composes cleanly with syntax colors, unlike a full-row background.
			gutter = m.targetGut.Render(fmt.Sprintf("▏%*d ", gutterW, n))
		} else {
			gutter = m.gutterFg.Render(fmt.Sprintf(" %*d ", gutterW, n))
		}
		b.WriteString(gutter + expandTabs(line))
		if i < len(content)-1 {
			b.WriteByte('\n')
		}
	}
	m.vp.SetContent(b.String())
}

func isBinary(data []byte) bool {
	n := len(data)
	if n > 512 {
		n = 512
	}
	for _, b := range data[:n] {
		if b == 0 {
			return true
		}
	}
	return false
}

func expandTabs(s string) string {
	return strings.ReplaceAll(s, "\t", "    ")
}

func truncate(s string, w int) string {
	if w <= 0 || lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	if w > len(r) {
		return s
	}
	return string(r[:w])
}

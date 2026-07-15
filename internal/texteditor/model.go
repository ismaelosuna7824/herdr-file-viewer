// Package texteditor provides the small, conventional keyboard editor used by
// Herdr file tabs. It deliberately owns selection state because Bubbles'
// textarea does not expose or render text selections.
package texteditor

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

const noSelection = -1

var (
	lineNumberStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	currentLineNumberStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("111")).Bold(true)
	selectionStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Background(lipgloss.Color("62"))
	cursorStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("111"))
	selectionCountStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("111"))
)

// Model stores text and cursor positions as rune offsets, so keyboard
// selection never splits UTF-8 characters.
type Model struct {
	value        []rune
	cursor       int
	anchor       int
	preferredCol int
	width        int
	height       int
	scrollLine   int
	scrollCol    int
	status       string

	readClipboard  func() (string, error)
	writeClipboard func(string) error
}

// New creates an editor positioned at the start of value.
func New(value string) Model {
	return Model{
		value:          []rune(value),
		anchor:         noSelection,
		preferredCol:   noSelection,
		width:          40,
		height:         6,
		readClipboard:  clipboard.ReadAll,
		writeClipboard: clipboard.WriteAll,
	}
}

// Value returns the exact text currently held by the editor.
func (m Model) Value() string { return string(m.value) }

// SetValue replaces the editor contents and returns the cursor to the start.
func (m *Model) SetValue(value string) {
	m.value = []rune(value)
	m.cursor = 0
	m.anchor = noSelection
	m.preferredCol = noSelection
	m.scrollLine = 0
	m.scrollCol = 0
	m.status = ""
}

// Cursor returns the active cursor's rune offset.
func (m Model) Cursor() int { return m.cursor }

// SelectionLen reports the selected rune count.
func (m Model) SelectionLen() int {
	start, end, ok := m.selection()
	if !ok {
		return 0
	}
	return end - start
}

// SelectedText returns the selected range, or an empty string.
func (m Model) SelectedText() string {
	start, end, ok := m.selection()
	if !ok {
		return ""
	}
	return string(m.value[start:end])
}

// Status returns the latest clipboard operation result.
func (m Model) Status() string { return m.status }

// SetSize updates the visible editor area.
func (m *Model) SetSize(width, height int) {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	m.width = width
	m.height = height
	m.ensureVisible()
}

// Focus exists for the same call shape as other Bubble Tea inputs. This
// editor is focused whenever its parent file tab is in edit mode.
func (m *Model) Focus() tea.Cmd { return nil }

// Update applies conventional desktop-editor key bindings.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	m.status = ""
	switch key.String() {
	case "shift+left":
		m.moveTo(m.cursor-1, true)
	case "shift+right":
		m.moveTo(m.cursor+1, true)
	case "shift+up":
		m.moveVertical(-1, true)
	case "shift+down":
		m.moveVertical(1, true)
	case "ctrl+shift+left", "alt+shift+left":
		m.moveTo(m.previousWordBoundary(), true)
	case "ctrl+shift+right", "alt+shift+right":
		m.moveTo(m.nextWordBoundary(), true)
	case "shift+home":
		start, _ := m.lineBounds(m.cursor)
		m.moveTo(start, true)
	case "shift+end":
		_, end := m.lineBounds(m.cursor)
		m.moveTo(end, true)
	case "ctrl+shift+home":
		m.moveTo(0, true)
	case "ctrl+shift+end":
		m.moveTo(len(m.value), true)
	case "left":
		m.moveWithoutSelection(-1, m.cursor-1)
	case "right":
		m.moveWithoutSelection(1, m.cursor+1)
	case "up":
		m.collapseForDirection(-1)
		m.moveVertical(-1, false)
	case "down":
		m.collapseForDirection(1)
		m.moveVertical(1, false)
	case "ctrl+left", "alt+left":
		m.moveWithoutSelection(-1, m.previousWordBoundary())
	case "ctrl+right", "alt+right":
		m.moveWithoutSelection(1, m.nextWordBoundary())
	case "home":
		m.clearSelection()
		start, _ := m.lineBounds(m.cursor)
		m.setCursor(start)
	case "end":
		m.clearSelection()
		_, end := m.lineBounds(m.cursor)
		m.setCursor(end)
	case "ctrl+home":
		m.clearSelection()
		m.setCursor(0)
	case "ctrl+end":
		m.clearSelection()
		m.setCursor(len(m.value))
	case "ctrl+a":
		m.anchor = 0
		m.cursor = len(m.value)
		m.preferredCol = noSelection
	case "backspace", "ctrl+h":
		m.deleteBackward()
	case "delete", "ctrl+d":
		m.deleteForward()
	case "ctrl+w", "alt+backspace":
		m.deleteWordBackward()
	case "enter", "ctrl+m":
		m.insert([]rune{'\n'})
	case "tab":
		m.insert([]rune{'\t'})
	case "ctrl+c":
		m.copySelection()
	case "ctrl+x":
		m.cutSelection()
	case "ctrl+v":
		m.pasteClipboard()
	default:
		if len(key.Runes) > 0 {
			m.insert(key.Runes)
		}
	}

	m.ensureVisible()
	return m, nil
}

func (m *Model) copySelection() {
	selected := m.SelectedText()
	if selected == "" {
		return
	}
	if err := m.writeClipboard(selected); err != nil {
		m.status = "copy failed: " + err.Error()
		return
	}
	m.status = "copied"
}

func (m *Model) cutSelection() {
	selected := m.SelectedText()
	if selected == "" {
		return
	}
	if err := m.writeClipboard(selected); err != nil {
		m.status = "cut failed: " + err.Error()
		return
	}
	m.deleteSelection()
	m.status = "cut"
}

func (m *Model) pasteClipboard() {
	value, err := m.readClipboard()
	if err != nil {
		m.status = "paste failed: " + err.Error()
		return
	}
	m.insert([]rune(value))
	m.status = "pasted"
}

func (m *Model) insert(value []rune) {
	m.deleteSelection()
	if len(value) == 0 {
		return
	}
	tail := append([]rune(nil), m.value[m.cursor:]...)
	m.value = append(m.value[:m.cursor], value...)
	m.value = append(m.value, tail...)
	m.cursor += len(value)
	m.anchor = noSelection
	m.preferredCol = noSelection
}

func (m *Model) deleteBackward() {
	if m.deleteSelection() || m.cursor == 0 {
		return
	}
	m.value = append(m.value[:m.cursor-1], m.value[m.cursor:]...)
	m.cursor--
	m.preferredCol = noSelection
}

func (m *Model) deleteForward() {
	if m.deleteSelection() || m.cursor >= len(m.value) {
		return
	}
	m.value = append(m.value[:m.cursor], m.value[m.cursor+1:]...)
	m.preferredCol = noSelection
}

func (m *Model) deleteWordBackward() {
	if m.deleteSelection() || m.cursor == 0 {
		return
	}
	start := m.previousWordBoundary()
	m.value = append(m.value[:start], m.value[m.cursor:]...)
	m.cursor = start
	m.preferredCol = noSelection
}

func (m *Model) deleteSelection() bool {
	start, end, ok := m.selection()
	if !ok {
		return false
	}
	m.value = append(m.value[:start], m.value[end:]...)
	m.cursor = start
	m.anchor = noSelection
	m.preferredCol = noSelection
	return true
}

func (m Model) selection() (int, int, bool) {
	if m.anchor == noSelection || m.anchor == m.cursor {
		return 0, 0, false
	}
	if m.anchor < m.cursor {
		return m.anchor, m.cursor, true
	}
	return m.cursor, m.anchor, true
}

func (m *Model) clearSelection() {
	m.anchor = noSelection
	m.preferredCol = noSelection
}

func (m *Model) moveTo(target int, extend bool) {
	target = clamp(target, 0, len(m.value))
	if extend {
		if m.anchor == noSelection {
			m.anchor = m.cursor
		}
	} else {
		m.anchor = noSelection
	}
	m.cursor = target
	m.preferredCol = noSelection
}

func (m *Model) moveWithoutSelection(direction, target int) {
	if start, end, ok := m.selection(); ok {
		if direction < 0 {
			m.cursor = start
		} else {
			m.cursor = end
		}
		m.clearSelection()
		return
	}
	m.moveTo(target, false)
}

func (m *Model) collapseForDirection(direction int) {
	start, end, ok := m.selection()
	if !ok {
		return
	}
	if direction < 0 {
		m.cursor = start
	} else {
		m.cursor = end
	}
	m.clearSelection()
}

func (m *Model) moveVertical(delta int, extend bool) {
	line, col := m.lineAndColumn(m.cursor)
	if m.preferredCol == noSelection {
		m.preferredCol = col
	}
	starts := lineStarts(m.value)
	targetLine := clamp(line+delta, 0, len(starts)-1)
	start := starts[targetLine]
	end := len(m.value)
	if targetLine+1 < len(starts) {
		end = starts[targetLine+1] - 1
	}
	target := start + min(m.preferredCol, end-start)
	preferred := m.preferredCol
	m.moveTo(target, extend)
	m.preferredCol = preferred
}

func (m *Model) setCursor(target int) {
	m.cursor = clamp(target, 0, len(m.value))
	m.preferredCol = noSelection
}

func (m Model) previousWordBoundary() int {
	pos := m.cursor
	for pos > 0 && !isWordRune(m.value[pos-1]) {
		pos--
	}
	for pos > 0 && isWordRune(m.value[pos-1]) {
		pos--
	}
	return pos
}

func (m Model) nextWordBoundary() int {
	pos := m.cursor
	for pos < len(m.value) && isWordRune(m.value[pos]) {
		pos++
	}
	for pos < len(m.value) && !isWordRune(m.value[pos]) {
		pos++
	}
	return pos
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r)
}

func (m Model) lineBounds(pos int) (int, int) {
	pos = clamp(pos, 0, len(m.value))
	start := pos
	for start > 0 && m.value[start-1] != '\n' {
		start--
	}
	end := pos
	for end < len(m.value) && m.value[end] != '\n' {
		end++
	}
	return start, end
}

func (m Model) lineAndColumn(pos int) (int, int) {
	starts := lineStarts(m.value)
	line := 0
	for i := 1; i < len(starts) && starts[i] <= pos; i++ {
		line = i
	}
	return line, pos - starts[line]
}

func lineStarts(value []rune) []int {
	starts := []int{0}
	for i, r := range value {
		if r == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

func (m *Model) ensureVisible() {
	line, _ := m.lineAndColumn(m.cursor)
	if line < m.scrollLine {
		m.scrollLine = line
	}
	if line >= m.scrollLine+m.height {
		m.scrollLine = line - m.height + 1
	}

	start, _ := m.lineBounds(m.cursor)
	cursorCol := runewidth.StringWidth(string(m.value[start:m.cursor]))
	contentWidth := m.contentWidth()
	if label := m.selectionLabel(); label != "" && line == m.scrollLine {
		labelWidth := runewidth.StringWidth(label)
		if contentWidth > labelWidth+1 {
			contentWidth -= labelWidth + 1
		}
	}
	if cursorCol < m.scrollCol {
		m.scrollCol = cursorCol
	}
	if cursorCol >= m.scrollCol+contentWidth {
		m.scrollCol = cursorCol - contentWidth + 1
	}
	if m.scrollCol < 0 {
		m.scrollCol = 0
	}
}

func (m Model) contentWidth() int {
	_, content := m.layoutWidths()
	return content
}

func (m Model) layoutWidths() (int, int) {
	desiredGutter := len(fmt.Sprintf("%d", len(lineStarts(m.value)))) + 2
	gutter := min(desiredGutter, max(0, m.width-1))
	return gutter, max(1, m.width-gutter)
}

func (m Model) selectionLabel() string {
	if n := m.SelectionLen(); n > 0 {
		return fmt.Sprintf("%d selected", n)
	}
	return ""
}

// View renders a line-numbered editor with a visible selection and cursor.
func (m Model) View() string {
	starts := lineStarts(m.value)
	lineNumberWidth := len(fmt.Sprintf("%d", len(starts)))
	gutterWidth, contentWidth := m.layoutWidths()
	currentLine, _ := m.lineAndColumn(m.cursor)
	selectionStart, selectionEnd, hasSelection := m.selection()
	selectionLabel := m.selectionLabel()

	rows := make([]string, 0, m.height)
	for screenRow := 0; screenRow < m.height; screenRow++ {
		line := m.scrollLine + screenRow
		if line >= len(starts) {
			gutter := fitPlain(strings.Repeat(" ", lineNumberWidth)+" ~", gutterWidth)
			rows = append(rows, lineNumberStyle.Render(gutter)+strings.Repeat(" ", contentWidth))
			continue
		}

		lineStart := starts[line]
		lineEnd := len(m.value)
		if line+1 < len(starts) {
			lineEnd = starts[line+1] - 1
		}
		lineNumber := fitPlain(fmt.Sprintf("%*d ", lineNumberWidth, line+1), gutterWidth)
		if line == currentLine {
			lineNumber = currentLineNumberStyle.Render(lineNumber)
		} else {
			lineNumber = lineNumberStyle.Render(lineNumber)
		}

		available := contentWidth
		labelWidth := runewidth.StringWidth(selectionLabel)
		showSelectionLabel := screenRow == 0 && selectionLabel != "" && available > labelWidth+1
		if showSelectionLabel {
			available -= labelWidth + 1
		}
		content := m.renderLine(lineStart, lineEnd, available, selectionStart, selectionEnd, hasSelection)
		row := lineNumber + content
		if showSelectionLabel {
			used := gutterWidth + lipgloss.Width(content)
			padding := max(1, m.width-used-labelWidth)
			row += strings.Repeat(" ", padding) + selectionCountStyle.Render(selectionLabel)
		}
		rows = append(rows, row)
	}
	return strings.Join(rows, "\n")
}

func (m Model) renderLine(lineStart, lineEnd, width, selectionStart, selectionEnd int, hasSelection bool) string {
	var out strings.Builder
	displayCol := 0
	renderedWidth := 0
	for pos := lineStart; pos < lineEnd && renderedWidth < width; pos++ {
		r := m.value[pos]
		text := string(r)
		runeWidth := runewidth.RuneWidth(r)
		if r == '\t' {
			runeWidth = 4 - (displayCol % 4)
			text = strings.Repeat(" ", runeWidth)
		}
		if runeWidth < 1 {
			runeWidth = 1
		}
		nextCol := displayCol + runeWidth
		if nextCol <= m.scrollCol {
			displayCol = nextCol
			continue
		}
		if displayCol < m.scrollCol || renderedWidth+runeWidth > width {
			displayCol = nextCol
			continue
		}

		switch {
		case pos == m.cursor:
			out.WriteString(cursorStyle.Render(text))
		case hasSelection && pos >= selectionStart && pos < selectionEnd:
			out.WriteString(selectionStyle.Render(text))
		default:
			out.WriteString(text)
		}
		displayCol = nextCol
		renderedWidth += runeWidth
	}

	if renderedWidth < width && m.cursor == lineEnd {
		out.WriteString(cursorStyle.Render(" "))
		renderedWidth++
	} else if renderedWidth < width && hasSelection && lineEnd < len(m.value) && m.value[lineEnd] == '\n' && lineEnd >= selectionStart && lineEnd < selectionEnd {
		out.WriteString(selectionStyle.Render(" "))
		renderedWidth++
	}
	if renderedWidth < width {
		out.WriteString(strings.Repeat(" ", width-renderedWidth))
	}
	return out.String()
}

func clamp(value, low, high int) int {
	return min(max(value, low), high)
}

func fitPlain(value string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) > width {
		runes = runes[len(runes)-width:]
	}
	if len(runes) < width {
		return strings.Repeat(" ", width-len(runes)) + string(runes)
	}
	return string(runes)
}

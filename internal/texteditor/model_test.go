package texteditor

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func press(m Model, keys ...tea.KeyMsg) Model {
	for _, key := range keys {
		m, _ = m.Update(key)
	}
	return m
}

func TestShiftArrowsSelectAndTypingReplacesSelection(t *testing.T) {
	m := New("alpha beta")
	for range 5 {
		m = press(m, tea.KeyMsg{Type: tea.KeyShiftRight})
	}

	if got := m.SelectedText(); got != "alpha" {
		t.Fatalf("shift+right selected %q, want alpha", got)
	}
	if m.SelectionLen() != 5 {
		t.Fatalf("selection length = %d, want 5", m.SelectionLen())
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	if got := m.Value(); got != "X beta" {
		t.Fatalf("typing should replace selection, got %q", got)
	}
	if got := m.SelectedText(); got != "" {
		t.Fatalf("typing should clear selection, got %q", got)
	}
}

func TestCtrlAndOptionArrowsMoveByWordAndShiftExtendsSelection(t *testing.T) {
	m := New("alpha beta.gap")
	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlRight})
	if got := m.Cursor(); got != 6 {
		t.Fatalf("ctrl+right cursor = %d, want next word at 6", got)
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlShiftRight})
	if got := m.SelectedText(); got != "beta." {
		t.Fatalf("ctrl+shift+right selected %q, want beta.", got)
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyShiftRight, Alt: true})
	if got := m.SelectedText(); got != "beta.gap" {
		t.Fatalf("option+shift+right selected %q, want beta.gap", got)
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyLeft, Alt: true})
	if got := m.Cursor(); got != 6 {
		t.Fatalf("option+left cursor = %d, want 6", got)
	}
	if m.SelectionLen() != 0 {
		t.Fatal("movement without shift should clear the selection")
	}
}

func TestVerticalSelectionKeepsColumnAcrossLines(t *testing.T) {
	m := New("abc\n12345\nxy")
	m = press(m,
		tea.KeyMsg{Type: tea.KeyRight},
		tea.KeyMsg{Type: tea.KeyRight},
		tea.KeyMsg{Type: tea.KeyShiftDown},
	)
	if got := m.SelectedText(); got != "c\n12" {
		t.Fatalf("shift+down selected %q, want c\\n12", got)
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyShiftDown})
	if got := m.SelectedText(); got != "c\n12345\nxy" {
		t.Fatalf("second shift+down selected %q", got)
	}
}

func TestHomeEndDocumentMovementAndSelectAll(t *testing.T) {
	m := New("first line\nsecond line")
	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlEnd})
	if got := m.Cursor(); got != len([]rune(m.Value())) {
		t.Fatalf("ctrl+end cursor = %d, want document end", got)
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyHome})
	if got := m.Cursor(); got != len([]rune("first line\n")) {
		t.Fatalf("home cursor = %d, want second-line start", got)
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyShiftEnd})
	if got := m.SelectedText(); got != "second line" {
		t.Fatalf("shift+end selected %q", got)
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlA})
	if got := m.SelectedText(); got != m.Value() {
		t.Fatalf("ctrl+a selected %q, want entire document", got)
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyBackspace})
	if got := m.Value(); got != "" {
		t.Fatalf("backspace should delete selection, got %q", got)
	}
}

func TestSelectionIsRuneSafeAndVisibleInEditorView(t *testing.T) {
	m := New("a🙂 b")
	m.SetSize(30, 4)
	m = press(m,
		tea.KeyMsg{Type: tea.KeyShiftRight},
		tea.KeyMsg{Type: tea.KeyShiftRight},
	)

	if got := m.SelectedText(); got != "a🙂" {
		t.Fatalf("unicode selection = %q, want a🙂", got)
	}
	view := m.View()
	if !strings.Contains(view, "a") || !strings.Contains(view, "🙂") {
		t.Fatalf("editor view lost selected unicode text:\n%s", view)
	}
	if !strings.Contains(view, "2 selected") {
		t.Fatalf("editor view should expose visible selection state:\n%s", view)
	}
}

func TestUnshiftedArrowCollapsesSelectionAtTheExpectedEdge(t *testing.T) {
	m := New("abcd")
	m = press(m,
		tea.KeyMsg{Type: tea.KeyShiftRight},
		tea.KeyMsg{Type: tea.KeyShiftRight},
		tea.KeyMsg{Type: tea.KeyLeft},
	)
	if got := m.Cursor(); got != 0 {
		t.Fatalf("left should collapse selection to start, cursor = %d", got)
	}

	m = press(m,
		tea.KeyMsg{Type: tea.KeyShiftRight},
		tea.KeyMsg{Type: tea.KeyShiftRight},
		tea.KeyMsg{Type: tea.KeyRight},
	)
	if got := m.Cursor(); got != 2 {
		t.Fatalf("right should collapse selection to end, cursor = %d", got)
	}
}

func TestCopyCutAndPasteUseTheSelectedRange(t *testing.T) {
	m := New("alpha beta")
	var copied string
	m.writeClipboard = func(value string) error {
		copied = value
		return nil
	}
	m.readClipboard = func() (string, error) { return "gamma", nil }
	for range 5 {
		m = press(m, tea.KeyMsg{Type: tea.KeyShiftRight})
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if copied != "alpha" || m.Status() != "copied" {
		t.Fatalf("ctrl+c copied %q with status %q", copied, m.Status())
	}
	if m.Value() != "alpha beta" {
		t.Fatal("copy must not modify the document")
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlX})
	if m.Value() != " beta" || m.Status() != "cut" {
		t.Fatalf("ctrl+x produced value %q with status %q", m.Value(), m.Status())
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlV})
	if m.Value() != "gamma beta" || m.Status() != "pasted" {
		t.Fatalf("ctrl+v produced value %q with status %q", m.Value(), m.Status())
	}
}

func TestFailedCutKeepsSelectedText(t *testing.T) {
	m := New("keep")
	m.writeClipboard = func(string) error { return errors.New("clipboard unavailable") }
	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlA}, tea.KeyMsg{Type: tea.KeyCtrlX})

	if m.Value() != "keep" {
		t.Fatalf("failed cut should preserve the document, got %q", m.Value())
	}
	if !strings.Contains(m.Status(), "clipboard unavailable") {
		t.Fatalf("failed cut status = %q", m.Status())
	}
}

func TestDeletionInsertionAndAliasShortcuts(t *testing.T) {
	m := New("one two")
	m = press(m,
		tea.KeyMsg{Type: tea.KeyCtrlEnd},
		tea.KeyMsg{Type: tea.KeyCtrlW},
	)
	if got := m.Value(); got != "one " {
		t.Fatalf("ctrl+w value = %q, want trailing word deleted", got)
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlH})
	if got := m.Value(); got != "one" {
		t.Fatalf("ctrl+h value = %q, want backward deletion", got)
	}
	m = press(m,
		tea.KeyMsg{Type: tea.KeyHome},
		tea.KeyMsg{Type: tea.KeyCtrlD},
		tea.KeyMsg{Type: tea.KeyEnd},
		tea.KeyMsg{Type: tea.KeyBackspace},
		tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyTab},
	)
	if got := m.Value(); got != "n\n\t" {
		t.Fatalf("delete/enter/tab shortcuts produced %q", got)
	}
}

func TestSetValueFocusTinySizeAndNonKeyMessages(t *testing.T) {
	m := New("old")
	if cmd := m.Focus(); cmd != nil {
		t.Fatal("static editor focus should not schedule a command")
	}
	m.SetSize(0, 0)
	m.SetValue("new")
	if m.Cursor() != 0 || m.Value() != "new" {
		t.Fatalf("SetValue reset to cursor=%d value=%q", m.Cursor(), m.Value())
	}
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 1, Height: 1})
	if cmd != nil || next.Value() != "new" || next.View() == "" {
		t.Fatal("non-key messages should leave a renderable editor unchanged")
	}
}

func TestVerticalMovementCollapsesSelectionAndScrolls(t *testing.T) {
	m := New("aa\nbb\ncc\n")
	m.SetSize(8, 2)
	m = press(m,
		tea.KeyMsg{Type: tea.KeyRight},
		tea.KeyMsg{Type: tea.KeyShiftDown},
		tea.KeyMsg{Type: tea.KeyDown},
	)
	if m.SelectionLen() != 0 || m.Cursor() != 7 {
		t.Fatalf("down should collapse and continue vertically: cursor=%d selection=%d", m.Cursor(), m.SelectionLen())
	}
	if m.scrollLine == 0 {
		t.Fatal("cursor moving below the viewport should scroll the editor")
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyUp}, tea.KeyMsg{Type: tea.KeyCtrlHome})
	if m.Cursor() != 0 || m.scrollLine != 0 {
		t.Fatalf("ctrl+home should return to top: cursor=%d scroll=%d", m.Cursor(), m.scrollLine)
	}
}

func TestClipboardFailuresAreReportedWithoutChangingText(t *testing.T) {
	m := New("keep")
	m.writeClipboard = func(string) error { return errors.New("copy unavailable") }
	m.readClipboard = func() (string, error) { return "", errors.New("paste unavailable") }

	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if m.Status() != "" {
		t.Fatalf("copy with no selection should be a no-op, got %q", m.Status())
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlA}, tea.KeyMsg{Type: tea.KeyCtrlC})
	if !strings.Contains(m.Status(), "copy unavailable") || m.Value() != "keep" {
		t.Fatalf("failed copy status=%q value=%q", m.Status(), m.Value())
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlV})
	if !strings.Contains(m.Status(), "paste unavailable") || m.Value() != "keep" {
		t.Fatalf("failed paste status=%q value=%q", m.Status(), m.Value())
	}
}

func TestHorizontalScrollRendersTabsWideRunesAndSelectedNewline(t *testing.T) {
	m := New("\twide🙂text\nnext")
	m.SetSize(10, 3)
	m = press(m, tea.KeyMsg{Type: tea.KeyShiftEnd})
	if m.scrollCol == 0 {
		t.Fatal("long selected line should scroll horizontally to keep the cursor visible")
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyShiftRight})
	if got := m.SelectedText(); got != "\twide🙂text\n" {
		t.Fatalf("selected range = %q, want first line and newline", got)
	}
	view := m.View()
	if !strings.Contains(view, "next") {
		t.Fatalf("scrolled selection view lost the following line:\n%s", view)
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlShiftEnd})
	if got := m.SelectedText(); !strings.HasSuffix(got, "next") {
		t.Fatalf("ctrl+shift+end selection = %q", got)
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlShiftHome})
	if m.Cursor() != 0 {
		t.Fatalf("ctrl+shift+home cursor = %d, want 0", m.Cursor())
	}
}

func TestNarrowSelectionViewNeverExceedsEditorDimensions(t *testing.T) {
	m := New("wide selected content\nnext")
	m.SetSize(10, 6)
	m = press(m, tea.KeyMsg{Type: tea.KeyShiftEnd})

	rows := strings.Split(m.View(), "\n")
	if len(rows) != 6 {
		t.Fatalf("editor rendered %d rows, want exactly 6:\n%s", len(rows), m.View())
	}
	for i, row := range rows {
		if got := lipgloss.Width(row); got > 10 {
			t.Fatalf("row %d width = %d, exceeds editor width 10:\n%s", i+1, got, m.View())
		}
	}
}

func TestSelectionLabelReservesSpaceWhenKeepingCursorVisible(t *testing.T) {
	m := New("abcdefghijklmnopqrstuvwxyz")
	m.SetSize(20, 3)
	m = press(m, tea.KeyMsg{Type: tea.KeyShiftEnd})

	// Width 20 leaves 17 content cells after the gutter. The "26 selected"
	// label reserves 12, so the cursor at column 26 needs a scroll offset of 22.
	if m.scrollCol < 22 {
		t.Fatalf("selection label hid active cursor: scrollCol=%d, want at least 22", m.scrollCol)
	}
}

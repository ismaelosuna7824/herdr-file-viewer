package filetab

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestFileTabRendersSelectedFileAndCloses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hello.go")
	if err := os.WriteFile(path, []byte("package hello\n\nconst Answer = 42\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := model.View()
	if !strings.Contains(view, "hello.go") || !strings.Contains(view, "const Answer = 42") {
		t.Fatalf("file tab should render the selected file; got:\n%s", view)
	}

	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("q should close the file tab")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("q command should return tea.QuitMsg")
	}
}

func TestFileTabRejectsDirectory(t *testing.T) {
	if _, err := New(t.TempDir()); err == nil {
		t.Fatal("file tab should reject a directory path")
	}
}

func TestFileTabInitializesHighlightingAndHandlesUpdates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hello.go")
	if err := os.WriteFile(path, []byte("package hello\n\nfunc Hello() string { return \"hello\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := New(path)
	if err != nil {
		t.Fatal(err)
	}

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("text file should request asynchronous highlighting")
	}
	model, _ := m.Update(cmd())
	model, _ = model.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if !strings.Contains(model.View(), "Hello") {
		t.Fatalf("updated tab lost file content:\n%s", model.View())
	}
}

func TestFileTabMarkdownToggleAndTinyWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(path, []byte("# Heading\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Init() != nil {
		t.Fatal("rendered markdown should not request source highlighting")
	}
	model, _ := m.Update(tea.WindowSizeMsg{Width: 20, Height: 1})
	if model.View() == "" {
		t.Fatal("tiny window should still render without panicking")
	}
	model, _ = model.Update(tea.WindowSizeMsg{Width: 40, Height: 8})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if !strings.Contains(model.View(), "Heading") {
		t.Fatalf("markdown source should remain visible after toggle:\n%s", model.View())
	}
}

func TestFileTabRejectsMissingPath(t *testing.T) {
	if _, err := New(filepath.Join(t.TempDir(), "missing.go")); err == nil {
		t.Fatal("file tab should reject a missing path")
	}
}

func TestFileTabEditModeSavesBackToDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o640); err != nil {
		t.Fatal(err)
	}

	m, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	model, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 10})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

	tab := model.(Model)
	if !tab.editing {
		t.Fatal("pressing e should enter edit mode")
	}
	if !strings.Contains(tab.View(), "ctrl+s save") {
		t.Fatalf("edit mode should show save controls:\n%s", tab.View())
	}

	tab.edit.SetValue("new\ncontent\n")
	model, _ = tab.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	tab = model.(Model)
	if tab.editing {
		t.Fatal("ctrl+s should leave edit mode after saving")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new\ncontent\n" {
		t.Fatalf("save wrote %q, want updated file content", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("save should preserve file permissions, got %v", info.Mode().Perm())
	}
	if !strings.Contains(tab.View(), "new") || !strings.Contains(tab.View(), "saved") {
		t.Fatalf("saved tab should return to viewer with status:\n%s", tab.View())
	}
}

func TestFileTabEditModeEscapeCancelsWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	model, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 10})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	tab := model.(Model)
	tab.edit.SetValue("discard\n")

	model, _ = tab.Update(tea.KeyMsg{Type: tea.KeyEsc})
	tab = model.(Model)
	if tab.editing {
		t.Fatal("esc should leave edit mode")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep\n" {
		t.Fatalf("cancel should not write to disk, got %q", got)
	}
	if !strings.Contains(tab.View(), "edit cancelled") {
		t.Fatalf("cancel should show a status hint:\n%s", tab.View())
	}
}

func TestFileTabEditModeRejectsUneditableFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.bin")
	if err := os.WriteFile(path, []byte{'h', 'i', 0, 'x'}, 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	model, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 10})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

	tab := model.(Model)
	if tab.editing {
		t.Fatal("binary/unpreviewable files should stay in viewer mode")
	}
	if !strings.Contains(tab.View(), "not editable") {
		t.Fatalf("uneditable file should explain why edit mode did not open:\n%s", tab.View())
	}
}

func TestFileTabEditModeRoutesKeyboardSelectionIntoSavedContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("old value\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	model, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 10})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	for range 3 {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("new")})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new value\n" {
		t.Fatalf("saved content = %q, want keyboard-selected text replaced", got)
	}
}

func TestFileTabEditModeStaysInsideANarrowPane(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("wide selected content\nnext\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	model, _ := m.Update(tea.WindowSizeMsg{Width: 10, Height: 6})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyShiftEnd})

	view := model.View()
	rows := strings.Split(view, "\n")
	if len(rows) > 6 {
		t.Fatalf("file tab rendered %d rows into a 6-row pane:\n%s", len(rows), view)
	}
	for i, row := range rows {
		if got := lipgloss.Width(row); got > 10 {
			t.Fatalf("row %d width = %d, exceeds pane width 10:\n%s", i+1, got, view)
		}
	}
}

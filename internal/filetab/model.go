// Package filetab provides the standalone file pane opened in Herdr tabs by
// the explorer.
package filetab

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ismaelosuna7824/herdr-file-viewer/internal/texteditor"
	"github.com/ismaelosuna7824/herdr-file-viewer/internal/viewer"
)

// Model is a full-pane wrapper around the shared file viewer and optional
// in-pane editor.
type Model struct {
	viewer  viewer.Model
	edit    texteditor.Model
	width   int
	height  int
	editing bool
	status  string
}

// New loads a regular file into a standalone tab model.
func New(path string) (Model, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Model{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Model{}, err
	}
	if info.IsDir() {
		return Model{}, fmt.Errorf("file tab path is a directory: %s", abs)
	}
	v := viewer.New()
	v.Load(abs)
	return Model{viewer: v}, nil
}

func (m Model) Init() tea.Cmd {
	if !m.viewer.ShouldHighlight() {
		return nil
	}
	path, source := m.viewer.Path(), m.viewer.Raw()
	return func() tea.Msg {
		return highlightedMsg{path: path, lines: viewer.Highlight(path, source)}
	}
}

type highlightedMsg struct {
	path  string
	lines []string
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil
	case highlightedMsg:
		m.viewer.SetHighlighted(msg.path, msg.lines)
		return m, nil
	case tea.KeyMsg:
		if m.editing {
			return m.updateEdit(msg)
		}
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "e":
			return m.enterEdit()
		case "m":
			m.viewer.ToggleMarkdown()
			m.status = ""
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.viewer, cmd = m.viewer.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.editing {
		return m.editView()
	}
	if m.width <= 0 || m.height <= 0 {
		return m.viewer.View()
	}
	help := " q close · ↑↓ scroll · e edit · m markdown"
	if m.status != "" {
		help += " · " + m.status
	}
	foot := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Width(m.width).
		Render(truncate(help, m.width))
	return lipgloss.JoinVertical(lipgloss.Left, m.viewer.View(), strings.TrimSuffix(foot, "\n"))
}

func (m *Model) resize() {
	contentHeight := m.height - 1
	if contentHeight < 1 {
		contentHeight = 1
	}
	m.viewer.SetSize(m.width, contentHeight)
	if m.editing {
		m.resizeEditor()
	}
}

func (m Model) enterEdit() (tea.Model, tea.Cmd) {
	if !m.viewer.Editable() {
		m.status = "not editable"
		return m, nil
	}
	m.edit = texteditor.New(m.viewer.Raw())
	m.editing = true
	m.status = ""
	m.resizeEditor()
	return m, m.edit.Focus()
}

func (m Model) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+s":
		return m.saveEdit()
	case "esc":
		m.editing = false
		m.status = "edit cancelled"
		return m, nil
	}
	var cmd tea.Cmd
	m.edit, cmd = m.edit.Update(msg)
	return m, cmd
}

func (m Model) saveEdit() (tea.Model, tea.Cmd) {
	path := m.viewer.Path()
	if err := writeFilePreserveMode(path, []byte(m.edit.Value())); err != nil {
		m.status = "save failed: " + err.Error()
		return m, nil
	}
	m.viewer.Load(path)
	m.editing = false
	m.status = "saved"
	m.resize()
	return m, m.highlightCmd()
}

func (m Model) highlightCmd() tea.Cmd {
	if !m.viewer.ShouldHighlight() {
		return nil
	}
	path, source := m.viewer.Path(), m.viewer.Raw()
	return func() tea.Msg {
		return highlightedMsg{path: path, lines: viewer.Highlight(path, source)}
	}
}

func (m *Model) resizeEditor() {
	editHeight := m.height - 2
	if editHeight < 1 {
		editHeight = 1
	}
	m.edit.SetSize(m.width, editHeight)
}

func (m Model) editView() string {
	title := "  " + filepath.Base(m.viewer.Path()) + "  [editing]"
	bar := lipgloss.NewStyle().
		Foreground(lipgloss.Color("111")).
		Bold(true).
		Width(m.width).
		Render(truncate(title, m.width))
	help := " ctrl+s save · esc cancel"
	if m.edit.SelectionLen() > 0 {
		help += fmt.Sprintf(" · %d selected", m.edit.SelectionLen())
	}
	if m.edit.Status() != "" {
		help += " · " + m.edit.Status()
	}
	if m.status != "" {
		help += " · " + m.status
	}
	foot := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Width(m.width).
		Render(truncate(help, m.width))
	return lipgloss.JoinVertical(lipgloss.Left, bar, m.edit.View(), strings.TrimSuffix(foot, "\n"))
}

func writeFilePreserveMode(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("cannot save a directory: %s", path)
	}
	mode := info.Mode().Perm()
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
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

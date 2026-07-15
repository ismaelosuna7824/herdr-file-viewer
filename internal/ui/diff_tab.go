package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ismaelosuna7824/herdr-file-viewer/internal/gitdiff"
)

// DiffTabModel is the standalone review pane opened from Source Control. It
// uses the same diff renderer as the legacy full viewer, but no staging actions
// are exposed in this read-only tab.
type DiffTabModel struct {
	root   string
	rel    string
	mode   gitdiff.Mode
	panel  diffPanel
	width  int
	height int
}

// NewDiffTab validates that path belongs to root and prepares the requested
// staged/worktree/untracked review.
func NewDiffTab(root, path string, mode gitdiff.Mode) (DiffTabModel, error) {
	if !mode.Valid() || mode == gitdiff.ModeHead {
		return DiffTabModel{}, fmt.Errorf("unsupported Source Control diff mode: %q", mode)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return DiffTabModel{}, err
	}
	if info, err := os.Stat(absRoot); err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("not a directory")
		}
		return DiffTabModel{}, fmt.Errorf("diff root %s: %w", absRoot, err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return DiffTabModel{}, err
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return DiffTabModel{}, fmt.Errorf("diff path is outside project root: %s", absPath)
	}
	model := DiffTabModel{
		root:  absRoot,
		rel:   filepath.ToSlash(rel),
		mode:  mode,
		panel: newDiffPanel(),
	}
	model.panel.beginLoad(model.reviewLabel())
	return model, nil
}

func (m DiffTabModel) Init() tea.Cmd { return m.loadCmd() }

func (m DiffTabModel) loadCmd() tea.Cmd {
	root, rel, mode := m.root, m.rel, m.mode
	label := m.reviewLabel()
	return func() tea.Msg {
		diff := gitdiff.LoadMode(context.Background(), root, rel, mode)
		diff.Path = label
		return diffLoadedMsg{diff: diff}
	}
}

func (m DiffTabModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil
	case diffLoadedMsg:
		m.panel.SetDiff(msg.diff)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "s":
			m.panel.toggleLayout()
			return m, nil
		case "r":
			m.panel.beginLoad(m.reviewLabel())
			return m, m.loadCmd()
		}
	}
	var cmd tea.Cmd
	m.panel, cmd = m.panel.Update(msg)
	return m, cmd
}

func (m DiffTabModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return m.panel.View()
	}
	help := lipgloss.NewStyle().Foreground(colMuted).Width(m.width).
		Render(truncateLine(" q close · ↑↓/pgup/pgdn scroll · s split/inline · r refresh", m.width))
	return lipgloss.JoinVertical(lipgloss.Left, m.panel.View(), strings.TrimSuffix(help, "\n"))
}

func (m *DiffTabModel) resize() {
	contentHeight := m.height - 1
	if contentHeight < 1 {
		contentHeight = 1
	}
	m.panel.SetSize(m.width, contentHeight)
}

func (m DiffTabModel) reviewLabel() string {
	label := "Changes"
	switch m.mode {
	case gitdiff.ModeStaged:
		label = "Staged"
	case gitdiff.ModeUntracked:
		label = "Untracked"
	}
	return label + " · " + m.rel
}

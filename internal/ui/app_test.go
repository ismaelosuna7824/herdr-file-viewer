package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ismaelosuna7824/herdr-file-viewer/internal/gitdiff"
	"github.com/ismaelosuna7824/herdr-file-viewer/internal/gitstatus"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func fixtureRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"main.go":        "package main\nfunc main() { alpha() }\n",
		"pkg/alpha.go":   "package pkg\nfunc alpha() {}\n",
		"docs/readme.md": "alpha beta gamma\n",
	}
	for rel, c := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(c), 0o644)
	}
	return root
}

// key builds a KeyMsg from a single rune or named key string used in tests.
func send(m tea.Model, msg tea.Msg) tea.Model {
	next, _ := m.Update(msg)
	return next
}

func TestBrowseRendersAndOpensFile(t *testing.T) {
	m, err := New(fixtureRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	var model tea.Model = m
	model = send(model, tea.WindowSizeMsg{Width: 120, Height: 40})

	out := model.View()
	if !strings.Contains(out, "File Viewer") {
		t.Errorf("browse header missing; got:\n%s", firstLines(out, 2))
	}
	// The root's children should be listed (main.go, pkg/, docs/).
	if !strings.Contains(out, "main.go") {
		t.Errorf("explorer should list main.go; got:\n%s", out)
	}
}

func TestSearchPanelFindFiltersFiles(t *testing.T) {
	m, err := New(fixtureRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	var model tea.Model = m
	model = send(model, tea.WindowSizeMsg{Width: 120, Height: 40})
	model = send(model, filesLoadedMsg{files: []string{"main.go", "pkg/alpha.go", "docs/readme.md"}})
	// Ctrl+P focuses the search panel in file-find mode.
	model = send(model, tea.KeyMsg{Type: tea.KeyCtrlP})
	if model.(Model).bfocus != focusSearch || model.(Model).skind != kindFind {
		t.Fatalf("Ctrl+P should focus search in find mode, got focus=%v kind=%v",
			model.(Model).bfocus, model.(Model).skind)
	}
	for _, r := range "alpha" {
		model = send(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	fm := model.(Model)
	if len(fm.finder.results) == 0 || fm.finder.results[0].Path != "pkg/alpha.go" {
		t.Fatalf("finder should rank pkg/alpha.go first for 'alpha', got %+v", fm.finder.results)
	}
	// Esc returns focus to the tree.
	model = send(model, tea.KeyMsg{Type: tea.KeyEsc})
	if model.(Model).bfocus != focusExplorer {
		t.Errorf("Esc should return focus to the explorer")
	}
}

func TestSearchPanelContentToggles(t *testing.T) {
	m, err := New(fixtureRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	var model tea.Model = m
	model = send(model, tea.WindowSizeMsg{Width: 120, Height: 40})
	model = send(model, tea.KeyMsg{Type: tea.KeyCtrlF})
	if model.(Model).bfocus != focusSearch || model.(Model).skind != kindContent {
		t.Fatal("Ctrl+F should focus search in content mode")
	}
	// Alt+r toggles the regex option.
	model = send(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}, Alt: true})
	if !model.(Model).search.regex {
		t.Error("Alt+r should enable the regex toggle")
	}
	// The browse view must render the search panel label without panicking.
	out := model.View()
	if !strings.Contains(out, "SEARCH IN FILES") {
		t.Errorf("search panel label missing; got:\n%s", firstLines(out, 3))
	}
}

func TestDiffModeEntersRendersAndExits(t *testing.T) {
	m, err := New(fixtureRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	var model tea.Model = m
	model = send(model, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Navigate to the file main.go (root children sort dirs first:
	// docs/, pkg/, main.go), open it, then request its review diff with "d".
	for i := 0; i < 3; i++ {
		model = send(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	}
	model = send(model, tea.KeyMsg{Type: tea.KeyEnter}) // open main.go
	if model.(Model).currentFile == "" {
		t.Fatal("expected a file to be open before requesting diff")
	}
	model = send(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if model.(Model).mode != modeDiff {
		t.Fatalf("'d' should enter diff mode, got %v", model.(Model).mode)
	}

	// Feed a synthetic diff (the fixture is not a git repo) and render.
	model = send(model, diffLoadedMsg{diff: gitdiff.FileDiff{
		Path:    "main.go",
		Added:   2,
		Removed: 1,
		Lines: []gitdiff.Line{
			{Kind: gitdiff.Hunk, Text: "@@ -1,2 +1,3 @@"},
			{Kind: gitdiff.Context, OldNum: 1, NewNum: 1, Text: "package main"},
			{Kind: gitdiff.Del, OldNum: 2, Text: "old line"},
			{Kind: gitdiff.Add, NewNum: 2, Text: "new line"},
			{Kind: gitdiff.Add, NewNum: 3, Text: "another"},
		},
	}})
	out := model.View()
	if !strings.Contains(out, "Review") {
		t.Errorf("diff header missing; got:\n%s", firstLines(out, 2))
	}
	if !strings.Contains(out, "+2") || !strings.Contains(out, "1") {
		t.Errorf("diff stats bar should show +2 / -1; got:\n%s", firstLines(out, 3))
	}

	// Esc returns to the browser.
	model = send(model, tea.KeyMsg{Type: tea.KeyEsc})
	if model.(Model).mode != modeBrowse {
		t.Errorf("Esc should return to browse mode from diff")
	}
}

func TestTabCyclesFocusThroughPanels(t *testing.T) {
	m, err := New(fixtureRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	var model tea.Model = m
	model = send(model, tea.WindowSizeMsg{Width: 100, Height: 30})

	want := []browseFocus{focusViewer, focusSearch, focusLog, focusExplorer}
	for i, w := range want {
		model = send(model, tea.KeyMsg{Type: tea.KeyTab})
		if got := model.(Model).bfocus; got != w {
			t.Fatalf("tab #%d: focus = %v, want %v", i+1, got, w)
		}
	}
}

func TestCtrlPCtrlFToggleSearchKind(t *testing.T) {
	m, err := New(fixtureRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	var model tea.Model = m
	model = send(model, tea.WindowSizeMsg{Width: 100, Height: 30})

	model = send(model, tea.KeyMsg{Type: tea.KeyCtrlF})
	if model.(Model).skind != kindContent {
		t.Fatal("Ctrl+F should select content search")
	}
	model = send(model, tea.KeyMsg{Type: tea.KeyCtrlP})
	if model.(Model).skind != kindFind || model.(Model).bfocus != focusSearch {
		t.Fatal("Ctrl+P should switch to file find while keeping the panel focused")
	}
}

func TestLocateFocusesTreeOnCurrentFile(t *testing.T) {
	root := fixtureRoot(t)
	m, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	var model tea.Model = m
	model = send(model, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Open main.go, which moves focus to the file view.
	for i := 0; i < 3; i++ {
		model = send(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	}
	model = send(model, tea.KeyMsg{Type: tea.KeyEnter})
	if model.(Model).bfocus != focusViewer {
		t.Fatal("opening a file should focus the viewer")
	}

	// Locate: focus returns to the tree, on the open file.
	model = send(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	mm := model.(Model)
	if mm.bfocus != focusExplorer {
		t.Errorf("L should focus the explorer, got %v", mm.bfocus)
	}
	if n := mm.tree.Selected(); n == nil || n.Path != mm.currentFile {
		t.Errorf("L should put the tree cursor on the current file")
	}
}

func TestChangesSelectionVisibleAtBottom(t *testing.T) {
	m, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var model tea.Model = m
	model = send(model, tea.WindowSizeMsg{Width: 90, Height: 24})

	var chs []gitstatus.Change
	for i := 0; i < 30; i++ {
		chs = append(chs, gitstatus.Change{Path: fmt.Sprintf("f_%02d.ts", i), Index: ' ', Work: 'M'})
	}
	model = send(model, gitChangesMsg{changes: chs})
	model = send(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}) // focus git panel (changes)

	// Scroll well past the bottom.
	for i := 0; i < 50; i++ {
		model = send(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	}
	out := ansiRe.ReplaceAllString(model.View(), "")
	// The cursor marker must remain visible in the rendered frame (regression:
	// a trailing newline in the column separator clipped the last row).
	if !strings.Contains(out, "▸") {
		t.Errorf("selection marker should stay visible when scrolled to the bottom")
	}
	if lines := strings.Count(out, "\n") + 1; lines != 24 {
		t.Errorf("frame should be exactly 24 lines, got %d", lines)
	}
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

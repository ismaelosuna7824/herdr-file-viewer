package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ismaelosuna7824/herdr-file-viewer/internal/gitdiff"
)

func twoHunkDiff() gitdiff.FileDiff {
	return gitdiff.FileDiff{
		Path:    "main.go",
		Added:   2,
		Removed: 2,
		Lines: []gitdiff.Line{
			{Kind: gitdiff.Hunk, Text: "@@ -1,3 +1,3 @@"},
			{Kind: gitdiff.Context, OldNum: 1, NewNum: 1, Text: "alpha"},
			{Kind: gitdiff.Del, OldNum: 2, Text: "beta"},
			{Kind: gitdiff.Add, NewNum: 2, Text: "beta changed"},
			{Kind: gitdiff.Hunk, Text: "@@ -10,3 +10,3 @@"},
			{Kind: gitdiff.Context, OldNum: 10, NewNum: 10, Text: "kappa"},
			{Kind: gitdiff.Del, OldNum: 11, Text: "lambda"},
			{Kind: gitdiff.Add, NewNum: 11, Text: "lambda changed"},
		},
	}
}

func TestDiffPanelHunkCursorNavigatesAndClamps(t *testing.T) {
	p := newDiffPanel()
	p.SetSize(80, 20)
	p.SetDiff(twoHunkDiff())

	if p.hunkCount() != 2 {
		t.Fatalf("want 2 hunks, got %d", p.hunkCount())
	}
	if p.currentHunk() != 0 {
		t.Fatalf("cursor should start at hunk 0, got %d", p.currentHunk())
	}
	p.moveHunk(1)
	if p.currentHunk() != 1 {
		t.Fatalf("next should move to hunk 1, got %d", p.currentHunk())
	}
	p.moveHunk(5) // clamp at the last hunk
	if p.currentHunk() != 1 {
		t.Fatalf("cursor must clamp at the last hunk, got %d", p.currentHunk())
	}
	p.moveHunk(-9) // clamp at the first hunk
	if p.currentHunk() != 0 {
		t.Fatalf("cursor must clamp at the first hunk, got %d", p.currentHunk())
	}
}

func TestDiffPanelMarksSelectedHunk(t *testing.T) {
	p := newDiffPanel()
	p.SetSize(80, 20)
	p.SetDiff(twoHunkDiff())
	p.moveHunk(1) // select the second hunk
	if !strings.Contains(p.View(), "▶") {
		t.Fatalf("the selected hunk header should carry the ▶ marker:\n%s", p.View())
	}
}

func TestDiffViewHunkNavAndSpaceWireStaging(t *testing.T) {
	m, err := New(fixtureRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	var model tea.Model = m
	model = send(model, tea.WindowSizeMsg{Width: 120, Height: 40})

	mm := model.(Model)
	mm.mode = modeDiff
	mm.diffReturn = modeBrowse
	mm.diffMode = gitdiff.ModeWorktree
	mm.diffRel = "main.go"
	model, _ = mm.Update(diffLoadedMsg{diff: twoHunkDiff()})

	// "]" advances the hunk cursor.
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if got := model.(Model).diff.currentHunk(); got != 1 {
		t.Fatalf("] should advance the hunk cursor to 1, got %d", got)
	}
	// "[" moves it back.
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	if got := model.(Model).diff.currentHunk(); got != 0 {
		t.Fatalf("[ should move the hunk cursor back to 0, got %d", got)
	}

	// space triggers staging: the command is produced (not executed here — the
	// fixture is not a git repo, and the real staging path is covered by the
	// gitlog integration tests).
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if cmd == nil {
		t.Fatal("space should return a staging command in the diff view")
	}
}

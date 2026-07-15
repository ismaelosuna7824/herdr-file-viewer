package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ismaelosuna7824/herdr-file-viewer/internal/gitdiff"
	"github.com/ismaelosuna7824/herdr-file-viewer/internal/gitstatus"
)

func TestChangeModeMapping(t *testing.T) {
	cases := []struct {
		name   string
		change gitstatus.Change
		want   gitdiff.Mode
	}{
		{"staged", gitstatus.Change{Path: "a.go", Index: 'M', Work: ' '}, gitdiff.ModeStaged},
		{"worktree", gitstatus.Change{Path: "b.go", Index: ' ', Work: 'M'}, gitdiff.ModeWorktree},
		{"untracked", gitstatus.Change{Path: "c.go", Index: '?', Work: '?'}, gitdiff.ModeUntracked},
		// Partially staged (index and worktree both changed) prefers the staged review.
		{"partial", gitstatus.Change{Path: "d.go", Index: 'M', Work: 'M'}, gitdiff.ModeStaged},
	}
	for _, tc := range cases {
		if got := changeMode(tc.change); got != tc.want {
			t.Errorf("%s → %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestDiffKeyOpensDiffTabFromStagingView(t *testing.T) {
	root := fixtureRoot(t)
	logPath := filepath.Join(t.TempDir(), "herdr-args.log")
	fakeHerdr := filepath.Join(t.TempDir(), "herdr")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$HERDR_TEST_LOG"
if [ "$1 $2 $3" = "plugin pane open" ]; then
  printf '%s\n' '{"result":{"plugin_pane":{"pane":{"pane_id":"w-test:pd","tab_id":"w-test:td"}}}}'
else
  printf '%s\n' '{"result":{"type":"ok"}}'
fi
`
	if err := os.WriteFile(fakeHerdr, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_BIN_PATH", fakeHerdr)
	t.Setenv("HERDR_WORKSPACE_ID", "w-test")
	t.Setenv("HERDR_TEST_LOG", logPath)

	m, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	var model tea.Model = m
	model = send(model, tea.WindowSizeMsg{Width: 120, Height: 40})
	// Populate the staging view with one staged file and focus the git panel.
	model = send(model, gitChangesMsg{changes: []gitstatus.Change{{Path: "main.go", Index: 'M', Work: ' '}}})
	mm := model.(Model)
	mm.bfocus = focusLog

	_, cmd := mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd == nil {
		t.Fatal("d on a changed file should open a diff tab")
	}
	_ = cmd()

	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(logged)
	if !strings.Contains(got, "--entrypoint diff") {
		t.Fatalf("d should open the diff entrypoint; got:\n%s", got)
	}
	if !strings.Contains(got, "--env HERDR_DIFF_MODE=staged") {
		t.Fatalf("staged file should open a staged review; got:\n%s", got)
	}
	if !strings.Contains(got, "--env HERDR_DIFF_PATH="+filepath.Join(root, "main.go")) {
		t.Fatalf("diff should target the selected file; got:\n%s", got)
	}
}

package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMouseClickDirectoryTogglesExactTreeRow(t *testing.T) {
	m, err := New(fixtureRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	var model tea.Model = m
	model = send(model, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Row 0 is the root at terminal y=1 (below the header); docs/ is row 1.
	model = send(model, tea.MouseMsg{
		X:      2,
		Y:      2,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})

	got := model.(Model)
	n := got.tree.Selected()
	if n == nil || n.Name != "docs" || !n.IsDir {
		t.Fatalf("click should select docs directory, got %+v", n)
	}
	if !n.Expanded {
		t.Fatal("click should expand the selected directory")
	}
}

func TestMouseClickFileOpensHerdrTabInSameWorkspace(t *testing.T) {
	root := fixtureRoot(t)
	logPath := filepath.Join(t.TempDir(), "herdr-args.log")
	fakeHerdr := filepath.Join(t.TempDir(), "herdr")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$HERDR_TEST_LOG"
if [ "$1 $2 $3" = "plugin pane open" ]; then
  printf '%s\n' '{"result":{"plugin_pane":{"pane":{"pane_id":"w-test:p9","tab_id":"w-test:t2"}}}}'
else
  printf '%s\n' '{"result":{"type":"tab_renamed"}}'
fi
`
	if err := os.WriteFile(fakeHerdr, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_BIN_PATH", fakeHerdr)
	t.Setenv("HERDR_WORKSPACE_ID", "w-test")
	t.Setenv("HERDR_TEST_LOG", logPath)
	// Keep the attached-tree args deterministic when this suite runs inside a
	// real Herdr pane, which exports HERDR_TAB_ID.
	t.Setenv("HERDR_TAB_ID", "")

	m, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	var model tea.Model = m
	model = send(model, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Visible rows are root, docs/, pkg/, main.go. Header occupies terminal y=0.
	next, cmd := model.Update(tea.MouseMsg{
		X:      2,
		Y:      4,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	if cmd == nil {
		t.Fatal("file click should return a command that opens a Herdr tab")
	}
	selected := next.(Model).tree.Selected()
	if selected == nil || selected.Path != filepath.Join(root, "main.go") {
		t.Fatalf("click should target main.go, got %+v", selected)
	}
	_ = cmd()

	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(logged)
	wantOpen := "plugin pane open --plugin ismaelosuna.file-viewer --entrypoint file --placement tab --workspace w-test --cwd " + root + " --env HERDR_FILE_PATH=" + filepath.Join(root, "main.go") + " --focus"
	if !strings.Contains(got, wantOpen) {
		t.Fatalf("file click opened wrong tab\nwant args containing: %s\ngot:\n%s", wantOpen, got)
	}
	// A file tab is a single read-only file pane: no tree is attached.
	if strings.Contains(got, "--entrypoint viewer") {
		t.Fatalf("file tab must NOT attach a tree pane:\n%s", got)
	}
	if !strings.Contains(got, "tab rename w-test:t2 main.go") {
		t.Fatalf("new Herdr tab should be named after the file; got:\n%s", got)
	}
}

// TestMouseWheelDoesNotClickOrOpenTab confirms wheel scrolling composes with
// click-to-open: a wheel event is forwarded to the active panel rather than
// being misread as a row click that moves the cursor or opens a Herdr tab.
func TestMouseWheelDoesNotClickOrOpenTab(t *testing.T) {
	m, err := New(fixtureRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	var model tea.Model = m
	model = send(model, tea.WindowSizeMsg{Width: 120, Height: 40})
	before := model.(Model).tree.Cursor()

	next, cmd := model.Update(tea.MouseMsg{
		X:      2,
		Y:      4,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})
	if cmd != nil {
		if _, ok := cmd().(fileTabOpenedMsg); ok {
			t.Fatal("wheel scroll must not open a file tab")
		}
	}
	if got := next.(Model).tree.Cursor(); got != before {
		t.Fatalf("wheel scroll must not move the tree cursor like a click: before=%d after=%d", before, got)
	}
}

package herdr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ismaelosuna7824/herdr-file-viewer/internal/gitdiff"
)

func TestRestoreFocusedFileTabRerunsFileAndTreeInRestoredShells(t *testing.T) {
	_, logPath := fakeHerdr(t, `
if [ "$1 $2" = "pane list" ]; then
  printf '%s\n' '{"result":{"panes":[{"pane_id":"w2:p-file","tab_id":"w2:t8","workspace_id":"w2","label":"File","cwd":"/tmp/project/docs"},{"pane_id":"w2:p-tree","tab_id":"w2:t8","workspace_id":"w2","label":"File Tree","cwd":"/tmp/project"}]}}'
elif [ "$1 $2" = "pane process-info" ]; then
  printf '{"result":{"process_info":{"pane_id":"%s","shell_pid":42,"foreground_process_group_id":42,"foreground_processes":[{"name":"zsh","argv":["-zsh"]}]}}}\n' "$4"
else
  printf '%s\n' '{"result":{"type":"ok"}}'
fi`)
	stateDir := t.TempDir()
	root := t.TempDir()
	docs := filepath.Join(root, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(docs, "guide.md")
	if err := os.WriteFile(path, []byte("# guide\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)
	t.Setenv("HERDR_PLUGIN_ROOT", "/tmp/plugin root")
	t.Setenv("HERDR_SOCKET_PATH", "/tmp/herdr socket")
	state := newFileTabState()
	state.set("w2", path, "w2:t8")
	state.setRoot("w2", root)
	if err := saveFileTabState(filepath.Join(stateDir, fileTabStateFile), state); err != nil {
		t.Fatal(err)
	}

	if err := RestoreFocusedTab("w2", "w2:t8", root); err != nil {
		t.Fatal(err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(logged)
	for _, want := range []string{
		"pane run w2:p-file",
		"HERDR_FILE_PATH=" + path,
		"HERDR_WORKSPACE_ID=w2",
		"HERDR_TAB_ID=w2:t8",
		"pane run w2:p-tree",
		"HERDR_TREE_ROOT=" + root,
		"--tree-only",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("restored tab command missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "plugin pane open") {
		t.Fatalf("restored layout already has a tree and must not duplicate it:\n%s", got)
	}
}

func TestRestoreFocusedTerminalTabOpensMissingDefaultTree(t *testing.T) {
	_, logPath := fakeHerdr(t, `
if [ "$1 $2" = "pane list" ]; then
  printf '%s\n' '{"result":{"panes":[{"pane_id":"w3:p1","tab_id":"w3:t1","workspace_id":"w3","cwd":"/tmp/project"}]}}'
elif [ "$1 $2 $3" = "plugin pane open" ]; then
  printf '%s\n' '{"result":{"plugin_pane":{"pane":{"pane_id":"w3:p2","tab_id":"w3:t1"}}}}'
else
  printf '%s\n' '{"result":{"type":"ok"}}'
fi`)
	t.Setenv("HERDR_PLUGIN_STATE_DIR", t.TempDir())
	root := t.TempDir()

	if err := RestoreFocusedTab("w3", "w3:t1", root); err != nil {
		t.Fatal(err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(logged)
	want := "plugin pane open --plugin ismaelosuna.file-viewer --entrypoint viewer --placement split --target-pane w3:p1 --env HERDR_TREE_ROOT=" + root + " --env HERDR_TREE_FOLLOW_PANE_ID=w3:p1 --direction right --no-focus"
	if !strings.Contains(got, want) {
		t.Fatalf("focused terminal tab must regain its default tree\nwant: %s\ngot:\n%s", want, got)
	}
}

func TestRestoreFocusedDiffTabRerunsDiffAndTreeInRestoredShells(t *testing.T) {
	_, logPath := fakeHerdr(t, `
if [ "$1 $2" = "pane list" ]; then
  printf '%s\n' '{"result":{"panes":[{"pane_id":"w9:p-diff","tab_id":"w9:t3","workspace_id":"w9","label":"Diff","cwd":"/tmp/project/docs"},{"pane_id":"w9:p-tree","tab_id":"w9:t3","workspace_id":"w9","label":"File Tree","cwd":"/tmp/project"}]}}'
elif [ "$1 $2" = "pane process-info" ]; then
  printf '{"result":{"process_info":{"pane_id":"%s","shell_pid":42,"foreground_process_group_id":42,"foreground_processes":[{"name":"zsh","argv":["-zsh"]}]}}}\n' "$4"
else
  printf '%s\n' '{"result":{"type":"ok"}}'
fi`)
	stateDir := t.TempDir()
	root := t.TempDir()
	docs := filepath.Join(root, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(docs, "guide.md")
	if err := os.WriteFile(path, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)
	t.Setenv("HERDR_PLUGIN_ROOT", "/tmp/plugin")
	state := newFileTabState()
	state.setDiff("w9", path, gitdiff.ModeStaged, "w9:t3")
	state.setRoot("w9", root)
	if err := saveFileTabState(filepath.Join(stateDir, fileTabStateFile), state); err != nil {
		t.Fatal(err)
	}

	if err := RestoreFocusedTab("w9", "w9:t3", root); err != nil {
		t.Fatal(err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(logged)
	for _, want := range []string{
		"pane run w9:p-diff", "HERDR_DIFF_PATH=" + path,
		"HERDR_DIFF_ROOT=" + root, "HERDR_DIFF_MODE=staged",
		"pane run w9:p-tree", "--tree-only",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("restored diff tab missing %q:\n%s", want, got)
		}
	}
}

func TestRestoreFocusedTabDoesNotRestartLivePluginPanes(t *testing.T) {
	_, logPath := fakeHerdr(t, `
if [ "$1 $2" = "pane list" ]; then
  printf '%s\n' '{"result":{"panes":[{"pane_id":"w4:p1","tab_id":"w4:t1","workspace_id":"w4","cwd":"/tmp/project"},{"pane_id":"w4:p2","tab_id":"w4:t1","workspace_id":"w4","label":"File Tree","cwd":"/tmp/project"}]}}'
elif [ "$1 $2" = "pane process-info" ]; then
  printf '{"result":{"process_info":{"pane_id":"%s","shell_pid":42,"foreground_process_group_id":84,"foreground_processes":[{"name":"file-viewer","argv":["/tmp/plugin/bin/file-viewer","--tree-only"]}]}}}\n' "$4"
else
  printf '%s\n' '{"result":{"type":"ok"}}'
fi`)
	t.Setenv("HERDR_PLUGIN_STATE_DIR", t.TempDir())

	if err := RestoreFocusedTab("w4", "w4:t1", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(logged)
	if strings.Contains(got, "pane run") || strings.Contains(got, "plugin pane open") {
		t.Fatalf("live plugin pane must not be restarted or duplicated:\n%s", got)
	}
}

func TestRestoreFocusedTabPrefersSavedRootTreeOverNestedEventContext(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "scripts")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(nested, "build.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_TEST_ROOT", root)
	t.Setenv("HERDR_TEST_NESTED", nested)
	_, logPath := fakeHerdr(t, `
if [ "$1 $2" = "pane list" ]; then
  printf '{"result":{"panes":[{"pane_id":"w6:p-file","tab_id":"w6:t2","workspace_id":"w6","label":"File","cwd":"%s"},{"pane_id":"w6:p-root-tree","tab_id":"w6:t1","workspace_id":"w6","label":"File Tree","cwd":"%s"}]}}\n' "$HERDR_TEST_NESTED" "$HERDR_TEST_ROOT"
elif [ "$1 $2" = "tab get" ]; then
  printf '%s\n' '{"result":{"tab":{"tab_id":"w6:t2","workspace_id":"w6","label":"build.sh","pane_count":1}}}'
elif [ "$1 $2" = "pane process-info" ]; then
  printf '{"result":{"process_info":{"pane_id":"%s","shell_pid":42,"foreground_process_group_id":42,"foreground_processes":[{"name":"zsh","argv":["-zsh"]}]}}}\n' "$4"
elif [ "$1 $2 $3" = "plugin pane open" ]; then
  printf '%s\n' '{"result":{"plugin_pane":{"pane":{"pane_id":"w6:p-tree","tab_id":"w6:t2"}}}}'
else
  printf '%s\n' '{"result":{"type":"ok"}}'
fi`)
	stateDir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)
	t.Setenv("HERDR_PLUGIN_ROOT", "/tmp/plugin")

	if err := RestoreFocusedTab("w6", "w6:t2", nested); err != nil {
		t.Fatal(err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(logged)
	if !strings.Contains(got, "HERDR_FILE_PATH="+path) {
		t.Fatalf("legacy file path was not recovered from pane cwd and tab label:\n%s", got)
	}
	if !strings.Contains(got, "--env HERDR_TREE_ROOT="+root+" --direction") {
		t.Fatalf("restored tree must use the existing project root, not nested event cwd:\n%s", got)
	}
	if strings.Contains(got, "HERDR_TREE_FOLLOW_PANE_ID") {
		t.Fatalf("file-tab tree must stay pinned to the project root:\n%s", got)
	}
}

func TestRestoreUnrecoverableFilePaneKeepsTreePinned(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "scripts")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_TEST_ROOT", root)
	t.Setenv("HERDR_TEST_NESTED", nested)
	_, logPath := fakeHerdr(t, `
if [ "$1 $2" = "pane list" ]; then
  printf '{"result":{"panes":[{"pane_id":"w7:p-file","tab_id":"w7:t2","workspace_id":"w7","label":"File","cwd":"%s"},{"pane_id":"w7:p-tree","tab_id":"w7:t2","workspace_id":"w7","label":"File Tree","cwd":"%s"}]}}\n' "$HERDR_TEST_NESTED" "$HERDR_TEST_ROOT"
elif [ "$1 $2" = "tab get" ]; then
  printf '%s\n' '{"result":{"tab":{"tab_id":"w7:t2","workspace_id":"w7","label":"missing.go","pane_count":2}}}'
elif [ "$1 $2" = "pane process-info" ]; then
  printf '{"result":{"process_info":{"pane_id":"%s","shell_pid":42,"foreground_process_group_id":42,"foreground_processes":[{"name":"zsh","argv":["-zsh"]}]}}}\n' "$4"
else
  printf '%s\n' '{"result":{"type":"ok"}}'
fi`)
	t.Setenv("HERDR_PLUGIN_STATE_DIR", t.TempDir())
	t.Setenv("HERDR_PLUGIN_ROOT", "/tmp/plugin")

	if err := RestoreFocusedTab("w7", "w7:t2", root); err != nil {
		t.Fatal(err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(logged), "HERDR_TREE_FOLLOW_PANE_ID") {
		t.Fatalf("a file pane must never become the default tree cwd source, even when its file path cannot be recovered:\n%s", logged)
	}
}

func TestRestoreFocusedTabRejectsMissingContext(t *testing.T) {
	for name, ids := range map[string][2]string{
		"workspace": {"", "w1:t1"},
		"tab":       {"w1", ""},
	} {
		t.Run(name, func(t *testing.T) {
			if err := RestoreFocusedTab(ids[0], ids[1], "/tmp"); err == nil {
				t.Fatal("expected missing context error")
			}
		})
	}
}

package herdr

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ismaelosuna7824/herdr-file-viewer/internal/gitdiff"
)

func fakeHerdr(t *testing.T, scriptBody string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "herdr")
	logPath := filepath.Join(dir, "args.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$HERDR_TEST_LOG\"\n" + scriptBody + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_BIN_PATH", bin)
	t.Setenv("HERDR_TEST_LOG", logPath)
	return bin, logPath
}

func TestOpenFileTabArgsStayInWorkspace(t *testing.T) {
	path := filepath.Join("/tmp", "project", "docs", "read me.md")
	got := openFileTabArgs("w7", path)
	want := []string{
		"plugin", "pane", "open",
		"--plugin", "ismaelosuna.file-viewer",
		"--entrypoint", "file",
		"--placement", "tab",
		"--workspace", "w7",
		"--cwd", filepath.Dir(path),
		"--env", "HERDR_FILE_PATH=" + path,
		"--focus",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("wrong Herdr args\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestOpenFileTabArgsUseFileDirectoryAsOwnershipMarker(t *testing.T) {
	path := "/tmp/project/internal/editor/editor.go"
	got := openFileTabArgs("w7", path)
	want := []string{"--cwd", filepath.Dir(path)}
	for i := range got[:len(got)-1] {
		if reflect.DeepEqual(got[i:i+2], want) {
			return
		}
	}
	t.Fatalf("file tab must launch in its file directory: %#v", got)
}

func TestOpenDiffTabArgsCarryModeAndProjectRoot(t *testing.T) {
	path := filepath.Join("/tmp", "project", "docs", "read me.md")
	root := filepath.Join("/tmp", "project")
	got := openDiffTabArgs("w7", path, root, gitdiff.ModeStaged)
	want := []string{
		"plugin", "pane", "open",
		"--plugin", "ismaelosuna.file-viewer",
		"--entrypoint", "diff",
		"--placement", "tab",
		"--workspace", "w7",
		"--cwd", filepath.Dir(path),
		"--env", "HERDR_DIFF_PATH=" + path,
		"--env", "HERDR_DIFF_ROOT=" + root,
		"--env", "HERDR_DIFF_MODE=staged",
		"--focus",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("wrong Herdr diff args\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestOpenDiffTabReusesSameModeButSeparatesStagedAndWorktree(t *testing.T) {
	_, logPath := fakeHerdr(t, `
if [ "$1 $2 $3" = "plugin pane open" ]; then
  case "$*" in
    *"--entrypoint diff"*)
      if echo "$*" | grep -q 'HERDR_DIFF_MODE=staged'; then
        printf '%s\n' '{"result":{"plugin_pane":{"pane":{"pane_id":"w2:p-stage","tab_id":"w2:t-stage"}}}}'
      else
        printf '%s\n' '{"result":{"plugin_pane":{"pane":{"pane_id":"w2:p-work","tab_id":"w2:t-work"}}}}'
      fi
      ;;
    *) printf '%s\n' '{"result":{"plugin_pane":{"pane":{"pane_id":"w2:p-tree","tab_id":"w2:t-tree"}}}}' ;;
  esac
elif [ "$1 $2" = "tab get" ]; then
  case "$3" in
    w2:t-stage) printf '%s\n' '{"result":{"tab":{"tab_id":"w2:t-stage","workspace_id":"w2","label":"S main.go","pane_count":1}}}' ;;
    *) printf '%s\n' '{"result":{"tab":{"tab_id":"w2:t-work","workspace_id":"w2","label":"M main.go","pane_count":1}}}' ;;
  esac
elif [ "$1 $2" = "pane list" ]; then
  printf '{"result":{"panes":[{"pane_id":"w2:p-stage","tab_id":"w2:t-stage","workspace_id":"w2","label":"Diff","cwd":"%s"},{"pane_id":"w2:p-work","tab_id":"w2:t-work","workspace_id":"w2","label":"Diff","cwd":"%s"}]}}\n' "$HERDR_TEST_FILE_DIR" "$HERDR_TEST_FILE_DIR"
else
  printf '%s\n' '{"result":{"type":"ok"}}'
fi`)
	t.Setenv("HERDR_PLUGIN_STATE_DIR", t.TempDir())
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_TEST_FILE_DIR", filepath.Dir(path))

	if err := OpenDiffTab("w2", path, root, gitdiff.ModeStaged); err != nil {
		t.Fatal(err)
	}
	if err := OpenDiffTab("w2", path, root, gitdiff.ModeStaged); err != nil {
		t.Fatal(err)
	}
	if err := OpenDiffTab("w2", path, root, gitdiff.ModeWorktree); err != nil {
		t.Fatal(err)
	}

	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(logged)
	if count := strings.Count(got, "--entrypoint diff"); count != 2 {
		t.Fatalf("same mode should reuse while staged/worktree stay distinct; got %d opens:\n%s", count, got)
	}
	if !strings.Contains(got, "tab focus w2:t-stage") {
		t.Fatalf("second staged click should focus its existing tab:\n%s", got)
	}
}

func TestOpenDiffTabValidatesInputsAndOpensSinglePane(t *testing.T) {
	for name, args := range map[string]struct {
		workspace string
		path      string
		root      string
		mode      gitdiff.Mode
	}{
		"workspace": {path: "/tmp/a.go", root: "/tmp", mode: gitdiff.ModeStaged},
		"path":      {workspace: "w1", root: "/tmp", mode: gitdiff.ModeStaged},
		"root":      {workspace: "w1", path: "/tmp/a.go", mode: gitdiff.ModeStaged},
		"mode":      {workspace: "w1", path: "/tmp/a.go", root: "/tmp", mode: gitdiff.ModeHead},
	} {
		t.Run(name, func(t *testing.T) {
			if err := OpenDiffTab(args.workspace, args.path, args.root, args.mode); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}

	_, logPath := fakeHerdr(t, `
if [ "$1 $2 $3" = "plugin pane open" ]; then
  printf '%s\n' '{"result":{"plugin_pane":{"pane":{"pane_id":"w8:p-diff","tab_id":"w8:t-diff"}}}}'
else
  printf '%s\n' '{"result":{"type":"ok"}}'
fi`)
	t.Setenv("HERDR_PLUGIN_STATE_DIR", t.TempDir())
	if err := OpenDiffTab("w8", "/tmp/a.go", "/tmp", gitdiff.ModeWorktree); err != nil {
		t.Fatal(err)
	}
	logged, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	got := string(logged)
	// A diff tab is a single read-only review pane: no tree is attached.
	if strings.Count(got, "--entrypoint diff") != 1 {
		t.Fatalf("diff open should create exactly one diff pane:\n%s", got)
	}
	if strings.Contains(got, "--entrypoint viewer") {
		t.Fatalf("diff tab must NOT attach a tree pane:\n%s", got)
	}
}

func TestParseOpenedTabID(t *testing.T) {
	raw := []byte(`{"result":{"plugin_pane":{"pane":{"pane_id":"w7:p3","tab_id":"w7:t9"}}}}`)
	got, err := parseOpenedTabID(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != "w7:t9" {
		t.Fatalf("want w7:t9, got %q", got)
	}
}

func TestParseOpenedTabIDRejectsMalformedResponse(t *testing.T) {
	for _, raw := range [][]byte{[]byte("not-json"), []byte(`{"result":{}}`)} {
		if _, err := parseOpenedTabID(raw); err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
}

func TestParseTabContextRejectsMalformedOrIncompleteResponse(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte("not-json"),
		[]byte(`{"result":{"tab":{"tab_id":"w2:t8"}}}`),
		[]byte(`{"result":{"tab":{"workspace_id":"w2"}}}`),
	} {
		if _, err := parseTabContext(raw); err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
}

func TestOpenFileTabOpensRenamesSinglePane(t *testing.T) {
	_, logPath := fakeHerdr(t, `
if [ "$1 $2 $3" = "plugin pane open" ]; then
  printf '%s\n' '{"result":{"plugin_pane":{"pane":{"pane_id":"w2:p8","tab_id":"w2:t8"}}}}'
else
  printf '%s\n' '{"result":{"type":"tab_renamed"}}'
	fi`)
	t.Setenv("HERDR_PLUGIN_STATE_DIR", t.TempDir())
	t.Setenv("HERDR_TAB_ID", "")
	root := t.TempDir()
	nested := filepath.Join(root, "scripts")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(nested, "file with spaces.go")
	if err := os.WriteFile(path, []byte("package example\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := OpenFileTab("w2", path, root); err != nil {
		t.Fatal(err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(logged)
	if !strings.Contains(got, "tab rename w2:t8 file with spaces.go") {
		t.Fatalf("expected tab rename in log, got:\n%s", got)
	}
	// A file tab is a single read-only file pane: no tree is attached.
	if strings.Count(got, "--entrypoint file") != 1 {
		t.Fatalf("file click should open exactly one file pane:\n%s", got)
	}
	if strings.Contains(got, "--entrypoint viewer") {
		t.Fatalf("file tab must NOT attach a tree pane:\n%s", got)
	}
}

func TestOpenFileTabReusesExistingTabForSamePath(t *testing.T) {
	_, logPath := fakeHerdr(t, `
if [ "$1 $2 $3" = "plugin pane open" ]; then
  case "$*" in
    *"--entrypoint file"*)
      printf '%s\n' '{"result":{"plugin_pane":{"pane":{"pane_id":"w2:p8","tab_id":"w2:t8"}}}}'
      ;;
    *)
      printf '%s\n' '{"result":{"plugin_pane":{"pane":{"pane_id":"w2:p9","tab_id":"w2:t8"}}}}'
      ;;
  esac
elif [ "$1 $2" = "tab get" ]; then
  printf '%s\n' '{"result":{"tab":{"tab_id":"w2:t8","workspace_id":"w2","label":"same.go","pane_count":1}}}'
elif [ "$1 $2" = "pane list" ]; then
  printf '{"result":{"panes":[{"pane_id":"w2:p8","tab_id":"w2:t8","workspace_id":"w2","label":"File","cwd":"%s"}]}}\n' "$HERDR_TEST_FILE_DIR"
else
  printf '%s\n' '{"result":{"type":"ok"}}'
fi`)
	t.Setenv("HERDR_PLUGIN_STATE_DIR", t.TempDir())
	path := filepath.Join(t.TempDir(), "same.go")
	if err := os.WriteFile(path, []byte("package same\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_TEST_FILE_DIR", filepath.Dir(path))

	if err := OpenFileTab("w2", path, filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if err := OpenFileTab("w2", path, filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(logged)
	if count := strings.Count(got, "--entrypoint file"); count != 1 {
		t.Fatalf("same file should create one file tab, got %d opens:\n%s", count, got)
	}
	if !strings.Contains(got, "tab get w2:t8") || !strings.Contains(got, "tab focus w2:t8") {
		t.Fatalf("same file should validate and focus its existing tab:\n%s", got)
	}
}

func TestOpenFileTabReplacesMismatchedLiveTabRecord(t *testing.T) {
	_, logPath := fakeHerdr(t, `
if [ "$1 $2" = "tab get" ]; then
  printf '%s\n' '{"result":{"tab":{"tab_id":"w2:t-wrong","workspace_id":"w2","label":"main.go","pane_count":2}}}'
elif [ "$1 $2" = "pane list" ]; then
  printf '{"result":{"panes":[{"pane_id":"w2:p-wrong","tab_id":"w2:t-wrong","workspace_id":"w2","label":"File","cwd":"%s"}]}}\n' "$HERDR_TEST_OTHER_DIR"
elif [ "$1 $2 $3" = "plugin pane open" ]; then
  case "$*" in
    *"--entrypoint file"*)
      printf '%s\n' '{"result":{"plugin_pane":{"pane":{"pane_id":"w2:p-new","tab_id":"w2:t-new"}}}}'
      ;;
    *)
      printf '%s\n' '{"result":{"plugin_pane":{"pane":{"pane_id":"w2:p-tree","tab_id":"w2:t-new"}}}}'
      ;;
  esac
else
  printf '%s\n' '{"result":{"type":"ok"}}'
fi`)
	stateDir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)
	path := filepath.Join(t.TempDir(), "main.go")
	otherDir := t.TempDir()
	t.Setenv("HERDR_TEST_OTHER_DIR", otherDir)
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	state := newFileTabState()
	state.set("w2", abs, "w2:t-wrong")
	if err := saveFileTabState(filepath.Join(stateDir, fileTabStateFile), state); err != nil {
		t.Fatal(err)
	}

	if err := OpenFileTab("w2", path, filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(logged)
	if strings.Contains(got, "tab focus w2:t-wrong") {
		t.Fatalf("mismatched record must not focus unrelated live tab:\n%s", got)
	}
	if strings.Count(got, "--entrypoint file") != 1 {
		t.Fatalf("mismatched record should be replaced with a fresh file tab:\n%s", got)
	}
}

func TestOpenFileTabReplacesClosedTabRecord(t *testing.T) {
	_, logPath := fakeHerdr(t, `
if [ "$1 $2" = "tab get" ]; then
  printf '%s\n' 'tab closed' >&2
  exit 4
elif [ "$1 $2 $3" = "plugin pane open" ]; then
  case "$*" in
    *"--entrypoint file"*)
      printf '%s\n' '{"result":{"plugin_pane":{"pane":{"pane_id":"w2:p-new","tab_id":"w2:t-new"}}}}'
      ;;
    *)
      printf '%s\n' '{"result":{"plugin_pane":{"pane":{"pane_id":"w2:p-tree","tab_id":"w2:t-new"}}}}'
      ;;
  esac
else
  printf '%s\n' '{"result":{"type":"ok"}}'
fi`)
	stateDir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)
	path := filepath.Join(t.TempDir(), "reopened.go")
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	state := newFileTabState()
	state.set("w2", abs, "w2:t-closed")
	if err := saveFileTabState(filepath.Join(stateDir, fileTabStateFile), state); err != nil {
		t.Fatal(err)
	}

	if err := OpenFileTab("w2", path, filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(logged)
	if !strings.Contains(got, "tab get w2:t-closed") || strings.Count(got, "--entrypoint file") != 1 {
		t.Fatalf("closed record should be checked then replaced:\n%s", got)
	}
	saved, err := loadFileTabState(filepath.Join(stateDir, fileTabStateFile))
	if err != nil {
		t.Fatal(err)
	}
	if got := saved.tabID("w2", abs); got != "w2:t-new" {
		t.Fatalf("closed record not replaced: got %q", got)
	}
}

func TestOpenFileTabRecoversFromMalformedState(t *testing.T) {
	fakeHerdr(t, `
if [ "$1 $2 $3" = "plugin pane open" ]; then
  case "$*" in
    *"--entrypoint file"*)
      printf '%s\n' '{"result":{"plugin_pane":{"pane":{"pane_id":"w3:p3","tab_id":"w3:t3"}}}}'
      ;;
    *)
      printf '%s\n' '{"result":{"plugin_pane":{"pane":{"pane_id":"w3:p4","tab_id":"w3:t3"}}}}'
      ;;
  esac
else
  printf '%s\n' '{"result":{"type":"ok"}}'
fi`)
	stateDir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)
	statePath := filepath.Join(stateDir, fileTabStateFile)
	if err := os.WriteFile(statePath, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "valid.go")

	if err := OpenFileTab("w3", path, filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	saved, err := loadFileTabState(statePath)
	if err != nil {
		t.Fatalf("malformed state should be atomically replaced: %v", err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := saved.tabID("w3", abs); got != "w3:t3" {
		t.Fatalf("recovered state missing file record: got %q", got)
	}
}

func TestFileTabStateDistinguishesSameBasenamePaths(t *testing.T) {
	state := newFileTabState()
	first := filepath.Join(t.TempDir(), "main.go")
	second := filepath.Join(t.TempDir(), "main.go")
	state.set("w4", first, "w4:t1")
	state.set("w4", second, "w4:t2")

	if got := state.tabID("w4", first); got != "w4:t1" {
		t.Fatalf("first path got %q", got)
	}
	if got := state.tabID("w4", second); got != "w4:t2" {
		t.Fatalf("second path got %q", got)
	}
}

func TestFileTabStateInitializesNilMaps(t *testing.T) {
	state := fileTabState{}
	state.set("w4", "/tmp/main.go", "w4:t1")
	if got := state.tabID("w4", "/tmp/main.go"); got != "w4:t1" {
		t.Fatalf("initialized state got %q", got)
	}
	if state.Version != 1 {
		t.Fatalf("initialized state version got %d", state.Version)
	}
}

func TestOpenFileTabValidatesContextAndPath(t *testing.T) {
	if err := OpenFileTab("", "/tmp/file", "/tmp"); err == nil || !strings.Contains(err.Error(), "workspace ID") {
		t.Fatalf("expected missing workspace error, got %v", err)
	}
	if err := OpenFileTab("w1", "", "/tmp"); err == nil || !strings.Contains(err.Error(), "file path") {
		t.Fatalf("expected missing path error, got %v", err)
	}
	if err := OpenFileTab("w1", "/tmp/file", ""); err == nil || !strings.Contains(err.Error(), "project root") {
		t.Fatalf("expected missing project root error, got %v", err)
	}
}

func TestOpenFileTabReportsOpenAndRenameFailures(t *testing.T) {
	t.Run("open", func(t *testing.T) {
		fakeHerdr(t, `printf '%s\n' 'open failed' >&2; exit 7`)
		err := OpenFileTab("w1", "/tmp/example.go", "/tmp")
		if err == nil || !strings.Contains(err.Error(), "open failed") {
			t.Fatalf("expected open stderr, got %v", err)
		}
	})

	t.Run("rename", func(t *testing.T) {
		fakeHerdr(t, `
if [ "$1 $2 $3" = "plugin pane open" ]; then
  printf '%s\n' '{"result":{"plugin_pane":{"pane":{"tab_id":"w1:t2"}}}}'
else
  printf '%s\n' 'rename failed' >&2
  exit 9
fi`)
		err := OpenFileTab("w1", "/tmp/example.go", "/tmp")
		if err == nil || !strings.Contains(err.Error(), "rename failed") {
			t.Fatalf("expected rename stderr, got %v", err)
		}
	})
}

func TestOpenFileTabRollsBackWhenStateSaveFails(t *testing.T) {
	_, logPath := fakeHerdr(t, `
if [ "$1 $2 $3" = "plugin pane open" ]; then
  chmod 0500 "$HERDR_TEST_STATE_DIR"
  printf '%s\n' '{"result":{"plugin_pane":{"pane":{"pane_id":"w2:p8","tab_id":"w2:t8"}}}}'
else
  printf '%s\n' '{"result":{"type":"ok"}}'
fi`)
	stateDir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)
	t.Setenv("HERDR_TEST_STATE_DIR", stateDir)
	defer os.Chmod(stateDir, 0o700) //nolint:errcheck

	err := OpenFileTab("w2", "/tmp/example.go", "/tmp")
	if err == nil || !strings.Contains(err.Error(), "save Herdr file-tab state") {
		t.Fatalf("expected state-save failure, got %v", err)
	}
	logged, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(logged), "tab close w2:t8") {
		t.Fatalf("untracked file tab must be rolled back after state-save failure:\n%s", logged)
	}
}

package herdr

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestEnsureWorkspaceTreeWaitsForInitialPaneAndOpensRightSplit(t *testing.T) {
	_, logPath := fakeHerdr(t, `
if [ "$1 $2" = "pane list" ]; then
  count=$(cat "$HERDR_TEST_COUNT_FILE" 2>/dev/null || printf '0')
  count=$((count + 1))
  printf '%s' "$count" > "$HERDR_TEST_COUNT_FILE"
  if [ "$count" -eq 1 ]; then
    printf '%s\n' '{"result":{"panes":[]}}'
  else
    printf '%s\n' '{"result":{"panes":[{"pane_id":"w9:p1","tab_id":"w9:t1","workspace_id":"w9","cwd":"/tmp/project"}]}}'
  fi
elif [ "$1 $2 $3" = "plugin pane open" ]; then
  printf '%s\n' '{"result":{"plugin_pane":{"pane":{"pane_id":"w9:p2","tab_id":"w9:t1"}}}}'
else
  printf '%s\n' '{"result":{"type":"ok"}}'
fi`)
	t.Setenv("HERDR_TEST_COUNT_FILE", filepath.Join(t.TempDir(), "count"))

	if err := EnsureWorkspaceTree("w9"); err != nil {
		t.Fatal(err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(logged)
	if strings.Count(got, "pane list --workspace w9") < 2 {
		t.Fatalf("workspace hook must wait for the initial pane:\n%s", got)
	}
	want := "plugin pane open --plugin ismaelosuna.file-viewer --entrypoint viewer --placement split --target-pane w9:p1 --env HERDR_TREE_ROOT=/tmp/project --env HERDR_TREE_FOLLOW_PANE_ID=w9:p1 --direction right --no-focus"
	if !strings.Contains(got, want) {
		t.Fatalf("workspace hook opened wrong layout\nwant: %s\ngot:\n%s", want, got)
	}
	if strings.Contains(got, "--workspace w9 --target-pane") {
		t.Fatalf("Herdr rejects workspace plus target-pane for a split:\n%s", got)
	}
}

func TestEnsureWorkspaceTreeDoesNotDuplicateExistingTree(t *testing.T) {
	_, logPath := fakeHerdr(t, `
if [ "$1 $2" = "pane list" ]; then
  printf '%s\n' '{"result":{"panes":[{"pane_id":"w8:p1","tab_id":"w8:t1","workspace_id":"w8","cwd":"/tmp/project"},{"pane_id":"w8:p2","tab_id":"w8:t1","workspace_id":"w8","label":"File Viewer","cwd":"/tmp/project"}]}}'
else
  printf '%s\n' '{"result":{"type":"ok"}}'
fi`)

	if err := EnsureWorkspaceTree("w8"); err != nil {
		t.Fatal(err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(logged), "plugin pane open") {
		t.Fatalf("existing tree must not be duplicated:\n%s", logged)
	}
}

func TestEnsureWorkspaceTreeRejectsMissingWorkspace(t *testing.T) {
	if err := EnsureWorkspaceTree(""); err == nil || !strings.Contains(err.Error(), "workspace ID") {
		t.Fatalf("expected missing workspace error, got %v", err)
	}
}

func TestEnsureWorkspaceTreeSerializesDuplicateDelivery(t *testing.T) {
	_, logPath := fakeHerdr(t, `
if [ "$1 $2" = "pane list" ]; then
  if [ -f "$HERDR_TEST_TREE_MARKER" ]; then
    printf '%s\n' '{"result":{"panes":[{"pane_id":"w7:p1","tab_id":"w7:t1","workspace_id":"w7","cwd":"/tmp/project"},{"pane_id":"w7:p2","tab_id":"w7:t1","workspace_id":"w7","label":"File Viewer","cwd":"/tmp/project"}]}}'
  else
    sleep 0.1
    printf '%s\n' '{"result":{"panes":[{"pane_id":"w7:p1","tab_id":"w7:t1","workspace_id":"w7","cwd":"/tmp/project"}]}}'
  fi
elif [ "$1 $2 $3" = "plugin pane open" ]; then
  : > "$HERDR_TEST_TREE_MARKER"
  printf '%s\n' '{"result":{"plugin_pane":{"pane":{"pane_id":"w7:p2","tab_id":"w7:t1"}}}}'
else
  printf '%s\n' '{"result":{"type":"ok"}}'
fi`)
	t.Setenv("HERDR_PLUGIN_STATE_DIR", t.TempDir())
	t.Setenv("HERDR_TEST_TREE_MARKER", filepath.Join(t.TempDir(), "tree-opened"))

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- EnsureWorkspaceTree("w7")
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(logged), "plugin pane open"); count != 1 {
		t.Fatalf("duplicate delivery opened %d trees:\n%s", count, logged)
	}
}

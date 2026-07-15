package herdr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPaneCWDUsesLiveForegroundDirectory(t *testing.T) {
	launchRoot := t.TempDir()
	liveRoot := t.TempDir()
	t.Setenv("HERDR_TEST_LAUNCH_ROOT", launchRoot)
	t.Setenv("HERDR_TEST_LIVE_ROOT", liveRoot)
	fakeHerdr(t, `
if [ "$1 $2" = "pane list" ]; then
  printf '{"result":{"panes":[{"pane_id":"w1:p1","tab_id":"w1:t1","workspace_id":"w1","cwd":"%s","foreground_cwd":"%s"}]}}\n' "$HERDR_TEST_LAUNCH_ROOT" "$HERDR_TEST_LIVE_ROOT"
else
  printf '%s\n' '{"result":{"type":"ok"}}'
fi`)

	got, err := PaneCWD("w1", "w1:p1")
	if err != nil {
		t.Fatal(err)
	}
	if got != liveRoot {
		t.Fatalf("tree should follow live pane cwd: got %q, want %q", got, liveRoot)
	}
}

func TestPaneCWDFallsBackToLaunchDirectoryAndValidatesTarget(t *testing.T) {
	launchRoot := t.TempDir()
	t.Setenv("HERDR_TEST_LAUNCH_ROOT", launchRoot)
	fakeHerdr(t, `
if [ "$1 $2" = "pane list" ]; then
  printf '{"result":{"panes":[{"pane_id":"w1:p1","tab_id":"w1:t1","workspace_id":"w1","cwd":"%s"}]}}\n' "$HERDR_TEST_LAUNCH_ROOT"
else
  printf '%s\n' '{"result":{"type":"ok"}}'
fi`)

	got, err := PaneCWD("w1", "w1:p1")
	if err != nil || got != launchRoot {
		t.Fatalf("launch cwd fallback: got %q, err %v", got, err)
	}
	if _, err := PaneCWD("w1", "w1:missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing followed pane should be reported, got %v", err)
	}
	if _, err := PaneCWD("", "w1:p1"); err == nil {
		t.Fatal("missing workspace must be rejected")
	}
	if _, err := PaneCWD("w1", ""); err == nil {
		t.Fatal("missing pane must be rejected")
	}

	missing := filepath.Join(t.TempDir(), "missing")
	t.Setenv("HERDR_TEST_LAUNCH_ROOT", missing)
	if _, err := PaneCWD("w1", "w1:p1"); err == nil || !os.IsNotExist(err) && !strings.Contains(err.Error(), "directory") {
		t.Fatalf("missing cwd must be rejected, got %v", err)
	}
}

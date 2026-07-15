package gitdiff

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitRawDiffSeparatesHeaderAndHunks(t *testing.T) {
	raw := "diff --git a/a.txt b/a.txt\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/a.txt\n" +
		"+++ b/a.txt\n" +
		"@@ -1,3 +1,3 @@\n" +
		" line01\n" +
		"-line02\n" +
		"+line02 CHANGED\n" +
		" line03\n" +
		"@@ -11,3 +11,3 @@\n" +
		" line11\n" +
		"-line12\n" +
		"+line12 CHANGED\n" +
		" line13\n"

	rd := splitRawDiff([]byte(raw))
	if !strings.HasPrefix(rd.Header, "diff --git a/a.txt b/a.txt") ||
		!strings.HasSuffix(rd.Header, "+++ b/a.txt") {
		t.Fatalf("header block wrong:\n%q", rd.Header)
	}
	if len(rd.Hunks) != 2 {
		t.Fatalf("want 2 hunks, got %d:\n%q", len(rd.Hunks), rd.Hunks)
	}
	if !strings.HasPrefix(rd.Hunks[0], "@@ -1,3 +1,3 @@") || !strings.Contains(rd.Hunks[0], "+line02 CHANGED") {
		t.Fatalf("hunk 0 wrong:\n%q", rd.Hunks[0])
	}
	if !strings.HasPrefix(rd.Hunks[1], "@@ -11,3 +11,3 @@") || !strings.Contains(rd.Hunks[1], "+line12 CHANGED") {
		t.Fatalf("hunk 1 wrong:\n%q", rd.Hunks[1])
	}
	// A reassembled patch is header + that hunk + trailing newline.
	patch := rd.Patch(1)
	if !strings.HasPrefix(patch, rd.Header) || !strings.Contains(patch, "@@ -11,3 +11,3 @@") || !strings.HasSuffix(patch, "\n") {
		t.Fatalf("Patch(1) is not a standalone patch:\n%q", patch)
	}
	if strings.Contains(patch, "line02 CHANGED") {
		t.Fatalf("Patch(1) leaked hunk 0 content:\n%q", patch)
	}
	if rd.Patch(9) != "" {
		t.Fatal("out-of-range hunk index should yield an empty patch")
	}
}

func TestSplitRawDiffPreservesNoNewlineMarker(t *testing.T) {
	raw := "diff --git a/a.txt b/a.txt\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/a.txt\n" +
		"+++ b/a.txt\n" +
		"@@ -1,2 +1,2 @@\n" +
		" a\n" +
		"-b\n" +
		"+b changed\n" +
		"\\ No newline at end of file\n"

	rd := splitRawDiff([]byte(raw))
	if len(rd.Hunks) != 1 {
		t.Fatalf("want 1 hunk, got %d", len(rd.Hunks))
	}
	if !strings.Contains(rd.Hunks[0], "\\ No newline at end of file") {
		t.Fatalf("no-newline marker must be preserved in the hunk:\n%q", rd.Hunks[0])
	}
	if !strings.HasSuffix(rd.Patch(0), "\\ No newline at end of file\n") {
		t.Fatalf("reassembled patch must keep the marker:\n%q", rd.Patch(0))
	}
}

func TestSplitRawDiffNoHunks(t *testing.T) {
	rd := splitRawDiff([]byte("Binary files a/x and b/x differ\n"))
	if len(rd.Hunks) != 0 {
		t.Fatalf("binary diff has no hunks, got %d", len(rd.Hunks))
	}
	if splitRawDiff(nil).Header != "" {
		t.Fatal("empty input should give an empty RawDiff")
	}
}

// gitRun runs git in root with a deterministic identity for tests.
func gitRun(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// repoTwoHunks builds a throwaway git repo (NOT the working repo) whose a.txt
// has exactly two separated worktree hunks.
func repoTwoHunks(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	var base strings.Builder
	for i := 1; i <= 14; i++ {
		fmt.Fprintf(&base, "line%02d\n", i)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte(base.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "init", "-q")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-q", "-m", "base")
	// Change line02 (top) and line13 (bottom): two hunks separated by >6 lines.
	changed := strings.ReplaceAll(base.String(), "line02\n", "line02 CHANGED\n")
	changed = strings.ReplaceAll(changed, "line13\n", "line13 CHANGED\n")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestLoadRawWorktreeYieldsTwoHunksMatchingParsed(t *testing.T) {
	root := repoTwoHunks(t)
	raw, err := LoadRaw(context.Background(), root, "a.txt", ModeWorktree)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw.Hunks) != 2 {
		t.Fatalf("want 2 raw hunks, got %d:\n%q", len(raw.Hunks), raw.Hunks)
	}
	// The raw hunk count must equal the parsed FileDiff's Hunk-line count so the
	// UI hunk cursor maps 1:1.
	parsed := LoadMode(context.Background(), root, "a.txt", ModeWorktree)
	parsedHunks := 0
	for _, ln := range parsed.Lines {
		if ln.Kind == Hunk {
			parsedHunks++
		}
	}
	if parsedHunks != len(raw.Hunks) {
		t.Fatalf("raw hunks (%d) must match parsed hunks (%d)", len(raw.Hunks), parsedHunks)
	}
	if !strings.HasPrefix(raw.Header, "diff --git a/a.txt b/a.txt") {
		t.Fatalf("header missing diff --git line:\n%q", raw.Header)
	}
}

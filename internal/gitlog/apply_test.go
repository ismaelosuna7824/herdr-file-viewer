package gitlog

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ismaelosuna7824/herdr-file-viewer/internal/gitdiff"
)

// runGitHunk runs git in a THROWAWAY test repo (never the working repo) with a
// deterministic identity, returning combined output.
func runGitHunk(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func writeFileT(t *testing.T, root, rel, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, rel), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// hunkRepo creates an isolated git repo whose a.txt has exactly two separated
// worktree hunks (a change near the top and one near the bottom).
func hunkRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	var base strings.Builder
	for i := 1; i <= 14; i++ {
		fmt.Fprintf(&base, "line%02d\n", i)
	}
	writeFileT(t, root, "a.txt", base.String())
	runGitHunk(t, root, "init", "-q")
	runGitHunk(t, root, "add", "-A")
	runGitHunk(t, root, "commit", "-q", "-m", "base")

	changed := strings.ReplaceAll(base.String(), "line02\n", "line02 CHANGED\n")
	changed = strings.ReplaceAll(changed, "line13\n", "line13 CHANGED\n")
	writeFileT(t, root, "a.txt", changed)
	return root
}

func TestApplyCachedStagesExactlyOneHunk(t *testing.T) {
	root := hunkRepo(t)

	raw, err := gitdiff.LoadRaw(context.Background(), root, "a.txt", gitdiff.ModeWorktree)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw.Hunks) != 2 {
		t.Fatalf("expected two hunks to stage independently, got %d", len(raw.Hunks))
	}

	if err := ApplyCachedPatch(context.Background(), root, raw.Patch(0), false); err != nil {
		t.Fatalf("staging hunk 0 failed: %v", err)
	}

	cached := runGitHunk(t, root, "diff", "--cached")
	worktree := runGitHunk(t, root, "diff")

	if !strings.Contains(cached, "line02 CHANGED") {
		t.Fatalf("hunk 0 should be staged (present in git diff --cached):\n%s", cached)
	}
	if strings.Contains(cached, "line13 CHANGED") {
		t.Fatalf("hunk 1 must NOT be staged:\n%s", cached)
	}
	if !strings.Contains(worktree, "line13 CHANGED") {
		t.Fatalf("hunk 1 should remain unstaged (present in git diff):\n%s", worktree)
	}
	if strings.Contains(worktree, "line02 CHANGED") {
		t.Fatalf("hunk 0 should be fully staged (absent from git diff):\n%s", worktree)
	}
}

func TestApplyCachedReverseUnstagesOneHunk(t *testing.T) {
	root := hunkRepo(t)
	runGitHunk(t, root, "add", "a.txt") // stage both hunks first

	staged, err := gitdiff.LoadRaw(context.Background(), root, "a.txt", gitdiff.ModeStaged)
	if err != nil {
		t.Fatal(err)
	}
	if len(staged.Hunks) != 2 {
		t.Fatalf("expected two staged hunks, got %d", len(staged.Hunks))
	}

	if err := ApplyCachedPatch(context.Background(), root, staged.Patch(0), true); err != nil {
		t.Fatalf("unstaging hunk 0 failed: %v", err)
	}

	cached := runGitHunk(t, root, "diff", "--cached")
	worktree := runGitHunk(t, root, "diff")

	if strings.Contains(cached, "line02 CHANGED") {
		t.Fatalf("hunk 0 should be unstaged (absent from git diff --cached):\n%s", cached)
	}
	if !strings.Contains(cached, "line13 CHANGED") {
		t.Fatalf("hunk 1 should remain staged:\n%s", cached)
	}
	if !strings.Contains(worktree, "line02 CHANGED") {
		t.Fatalf("hunk 0 should return to the worktree diff:\n%s", worktree)
	}
}

func TestApplyCachedStagesNoNewlineAtEOF(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	writeFileT(t, root, "a.txt", "alpha\nbeta\n")
	runGitHunk(t, root, "init", "-q")
	runGitHunk(t, root, "add", "-A")
	runGitHunk(t, root, "commit", "-q", "-m", "base")
	// Change the last line AND drop the trailing newline.
	writeFileT(t, root, "a.txt", "alpha\nbeta changed")

	raw, err := gitdiff.LoadRaw(context.Background(), root, "a.txt", gitdiff.ModeWorktree)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw.Hunks) != 1 {
		t.Fatalf("expected one hunk, got %d", len(raw.Hunks))
	}
	if !strings.Contains(raw.Hunks[0], "No newline at end of file") {
		t.Fatalf("raw hunk must carry the no-newline marker:\n%q", raw.Hunks[0])
	}

	if err := ApplyCachedPatch(context.Background(), root, raw.Patch(0), false); err != nil {
		t.Fatalf("staging a no-newline hunk must succeed: %v", err)
	}
	cached := runGitHunk(t, root, "diff", "--cached")
	if !strings.Contains(cached, "beta changed") || !strings.Contains(cached, "No newline at end of file") {
		t.Fatalf("staged diff should reflect the missing newline:\n%s", cached)
	}
}

func TestUntrackedFileStagesWhole(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	writeFileT(t, root, "seed.txt", "seed\n")
	runGitHunk(t, root, "init", "-q")
	runGitHunk(t, root, "add", "-A")
	runGitHunk(t, root, "commit", "-q", "-m", "seed")
	// A brand-new untracked file.
	writeFileT(t, root, "new.txt", "fresh\ncontent\n")

	raw, err := gitdiff.LoadRaw(context.Background(), root, "new.txt", gitdiff.ModeUntracked)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw.Hunks) != 1 {
		t.Fatalf("an untracked file is one whole-file hunk, got %d", len(raw.Hunks))
	}
	// The UI stages untracked files with StageFile (git add), not apply --cached.
	if err := StageFile(context.Background(), root, "new.txt"); err != nil {
		t.Fatalf("staging untracked file failed: %v", err)
	}
	if names := runGitHunk(t, root, "diff", "--cached", "--name-only"); !strings.Contains(names, "new.txt") {
		t.Fatalf("untracked file should be staged whole:\n%s", names)
	}
}

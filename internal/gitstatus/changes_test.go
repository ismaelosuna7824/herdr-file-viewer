package gitstatus

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestChangesStagedUnstagedUntracked(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	run := func(a ...string) {
		c := exec.Command("git", append([]string{"-C", root}, a...)...)
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if o, e := c.CombinedOutput(); e != nil {
			t.Fatalf("git %v: %s", a, o)
		}
	}
	write := func(p, c string) {
		if err := os.WriteFile(filepath.Join(root, p), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run("init", "-q")
	write("keep.txt", "1\n")
	run("add", "-A")
	run("commit", "-qm", "init")

	write("keep.txt", "2\n")      // modified, unstaged
	write("staged.txt", "new\n")  // will be staged
	run("add", "staged.txt")      // staged (added)
	write("untracked.txt", "u\n") // untracked

	byPath := map[string]Change{}
	for _, c := range Changes(context.Background(), root) {
		byPath[c.Path] = c
	}
	if !byPath["staged.txt"].Staged() {
		t.Errorf("staged.txt should be staged: %+v", byPath["staged.txt"])
	}
	if byPath["keep.txt"].Staged() {
		t.Errorf("keep.txt (worktree-only change) should not be staged")
	}
	if !byPath["untracked.txt"].Untracked() {
		t.Errorf("untracked.txt should be untracked")
	}
}

func TestChangesNonRepoIsEmpty(t *testing.T) {
	if c := Changes(context.Background(), t.TempDir()); len(c) != 0 {
		t.Errorf("a non-git dir should yield no changes, got %d", len(c))
	}
}

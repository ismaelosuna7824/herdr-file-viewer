package gitdiff

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func repoWithChange(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(rel, content string) {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q")
	write("a.txt", "one\ntwo\nthree\n")
	run("add", "-A")
	run("commit", "-q", "-m", "init")
	// Change: keep "one", drop "two", change "three" -> "THREE", add "four".
	write("a.txt", "one\nTHREE\nfour\n")
	return root
}

func TestLoadTrackedDiff(t *testing.T) {
	root := repoWithChange(t)
	d := Load(context.Background(), root, "a.txt", false)
	if d.Empty {
		t.Fatal("expected a non-empty diff")
	}
	if d.Added == 0 || d.Removed == 0 {
		t.Fatalf("expected both additions and removals, got +%d -%d", d.Added, d.Removed)
	}
	// Ensure the parser assigned kinds and at least one hunk header exists.
	var hunks, adds, dels int
	for _, ln := range d.Lines {
		switch ln.Kind {
		case Hunk:
			hunks++
		case Add:
			adds++
		case Del:
			dels++
		}
	}
	if hunks == 0 {
		t.Error("expected at least one hunk header")
	}
	if adds != d.Added || dels != d.Removed {
		t.Errorf("line-kind counts (add=%d del=%d) disagree with totals (+%d -%d)", adds, dels, d.Added, d.Removed)
	}
}

func TestLoadUntrackedShowsAllAdditions(t *testing.T) {
	root := repoWithChange(t)
	if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("x\ny\nz\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := Load(context.Background(), root, "new.txt", true)
	if d.Empty {
		t.Fatal("untracked file should produce a diff of additions")
	}
	if d.Added != 3 || d.Removed != 0 {
		t.Errorf("untracked new.txt should be +3 -0, got +%d -%d", d.Added, d.Removed)
	}
}

func TestLoadNoChangesIsEmpty(t *testing.T) {
	root := repoWithChange(t)
	// b.txt does not exist and a committed-but-unchanged file has no diff.
	d := Load(context.Background(), root, "does-not-exist.txt", false)
	if !d.Empty {
		t.Errorf("a path with no diff should be Empty")
	}
}

func TestLoadModeSeparatesStagedAndWorktreeDiffs(t *testing.T) {
	root := repoWithChange(t)
	path := filepath.Join(root, "a.txt")

	// Stage one version, then make a second worktree-only edit. A partially
	// staged file must produce two different reviews in Source Control.
	if err := os.WriteFile(path, []byte("one\nstaged version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", root, "add", "a.txt").CombinedOutput(); err != nil {
		t.Fatalf("stage a.txt: %v\n%s", err, out)
	}
	if err := os.WriteFile(path, []byte("one\nstaged version\nworktree version\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	staged := LoadMode(context.Background(), root, "a.txt", ModeStaged)
	worktree := LoadMode(context.Background(), root, "a.txt", ModeWorktree)
	stagedText := diffText(staged)
	worktreeText := diffText(worktree)

	if !strings.Contains(stagedText, "staged version") || strings.Contains(stagedText, "worktree version") {
		t.Fatalf("staged diff crossed into the worktree:\n%s", stagedText)
	}
	if !strings.Contains(worktreeText, "worktree version") || strings.Contains(worktreeText, "THREE") {
		t.Fatalf("worktree diff was not based on the index:\n%s", worktreeText)
	}
}

func TestLoadModeUntrackedShowsWholeFile(t *testing.T) {
	root := repoWithChange(t)
	if err := os.WriteFile(filepath.Join(root, "fresh.txt"), []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := LoadMode(context.Background(), root, "fresh.txt", ModeUntracked)
	if d.Empty || d.Added != 2 || d.Removed != 0 {
		t.Fatalf("untracked diff = empty:%v +%d -%d, want whole file as +2", d.Empty, d.Added, d.Removed)
	}
}

func TestModeValidation(t *testing.T) {
	for _, mode := range []Mode{ModeHead, ModeStaged, ModeWorktree, ModeUntracked} {
		if !mode.Valid() {
			t.Errorf("expected %q to be valid", mode)
		}
	}
	if Mode("mystery").Valid() {
		t.Fatal("unknown diff mode must be rejected")
	}
	if d := LoadMode(context.Background(), t.TempDir(), "a.txt", Mode("mystery")); !d.Empty {
		t.Fatal("unknown diff mode should return an empty diff")
	}
}

func TestLoadRefShowsCommittedChanges(t *testing.T) {
	root := repoWithChange(t)
	d := LoadRef(context.Background(), root, "HEAD", "latest commit")
	if d.Empty || len(d.Lines) == 0 {
		t.Fatal("HEAD should produce the committed file diff")
	}
	if d.Path != "latest commit" {
		t.Fatalf("diff label = %q, want latest commit", d.Path)
	}
	if d := LoadRef(context.Background(), t.TempDir(), "HEAD", "missing"); !d.Empty {
		t.Fatal("non-repository ref should return an empty diff")
	}
}

func diffText(d FileDiff) string {
	var b strings.Builder
	for _, line := range d.Lines {
		b.WriteString(line.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

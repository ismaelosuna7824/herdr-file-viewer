package gitlog

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func repoWithCommits(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Ada", "GIT_AUTHOR_EMAIL=a@a",
			"GIT_COMMITTER_NAME=Ada", "GIT_COMMITTER_EMAIL=a@a")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	// Set repo-local identity so package git ops (CommitAll, Merge, …) work on
	// CI runners that have no global git config.
	run("config", "user.email", "a@a")
	run("config", "user.name", "Ada")
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("one\n"), 0o644)
	run("add", "-A")
	run("commit", "-q", "-m", "first commit")
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("two\n"), 0o644)
	run("commit", "-qam", "second commit")
	return root
}

func TestLoadReturnsCommitsNewestFirst(t *testing.T) {
	root := repoWithCommits(t)
	commits := Load(context.Background(), root, 10)
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}
	if commits[0].Subject != "second commit" {
		t.Errorf("newest commit should be first, got %q", commits[0].Subject)
	}
	if commits[0].Author != "Ada" || commits[0].Short == "" || commits[0].Hash == "" {
		t.Errorf("commit fields not populated: %+v", commits[0])
	}
	if commits[0].When == "" {
		t.Errorf("relative date missing")
	}
}

func TestLoadNonRepoIsEmpty(t *testing.T) {
	if c := Load(context.Background(), t.TempDir(), 10); len(c) != 0 {
		t.Errorf("non-repo should yield no commits, got %d", len(c))
	}
}

func TestBranchesAndSwitch(t *testing.T) {
	root := repoWithCommits(t)
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Ada", "GIT_AUTHOR_EMAIL=a@a",
			"GIT_COMMITTER_NAME=Ada", "GIT_COMMITTER_EMAIL=a@a")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("branch", "feature")

	bs := Branches(context.Background(), root)
	if len(bs) != 2 {
		t.Fatalf("expected 2 branches, got %d: %+v", len(bs), bs)
	}
	// Exactly one is current.
	var currents int
	for _, b := range bs {
		if b.Current {
			currents++
		}
	}
	if currents != 1 {
		t.Errorf("expected exactly one current branch, got %d", currents)
	}

	// Switching to an existing branch succeeds and updates "current".
	if err := Switch(context.Background(), root, "feature"); err != nil {
		t.Fatalf("switch to feature failed: %v", err)
	}
	for _, b := range Branches(context.Background(), root) {
		if b.Name == "feature" && !b.Current {
			t.Errorf("feature should be current after switch")
		}
	}

	// Switching to a missing branch returns an error.
	if err := Switch(context.Background(), root, "nope"); err == nil {
		t.Errorf("switching to a nonexistent branch should error")
	}
}

func TestCreateCommitMergeDelete(t *testing.T) {
	root := repoWithCommits(t) // current branch has commits
	ctx := context.Background()

	// Create + switch to a new branch.
	if err := CreateBranch(ctx, root, "feature"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	cur := currentBranchName(t, root)
	if cur != "feature" {
		t.Fatalf("after CreateBranch, current = %q, want feature", cur)
	}

	// Commit a change on the feature branch.
	if err := os.WriteFile(filepath.Join(root, "feat.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CommitAll(ctx, root, "add feat.txt"); err != nil {
		t.Fatalf("CommitAll: %v", err)
	}
	if got := Load(ctx, root, 10); got[0].Subject != "add feat.txt" {
		t.Errorf("latest commit = %q, want 'add feat.txt'", got[0].Subject)
	}

	// Back to the base branch and merge feature in.
	base := baseBranchName(t, root)
	if err := Switch(ctx, root, base); err != nil {
		t.Fatalf("switch to base: %v", err)
	}
	if err := Merge(ctx, root, "feature"); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	// Now feature is merged, safe delete should succeed.
	if err := DeleteBranch(ctx, root, "feature"); err != nil {
		t.Fatalf("DeleteBranch after merge: %v", err)
	}
}

func TestTagCherryPickResetStage(t *testing.T) {
	root := repoWithCommits(t)
	ctx := context.Background()

	// Tag HEAD.
	if err := CreateTag(ctx, root, "v1.0.0"); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	// Make a commit on a side branch to cherry-pick later.
	if err := CreateBranch(ctx, root, "side"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "picked.txt"), []byte("pick me\n"), 0o644)
	if err := CommitAll(ctx, root, "add picked.txt"); err != nil {
		t.Fatal(err)
	}
	pick := Load(ctx, root, 1)[0].Hash

	// Back to base, cherry-pick the side commit.
	base := baseBranchName2(t, root)
	if err := Switch(ctx, root, base); err != nil {
		t.Fatal(err)
	}
	if err := CherryPick(ctx, root, pick); err != nil {
		t.Fatalf("CherryPick: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "picked.txt")); err != nil {
		t.Errorf("cherry-pick should have brought picked.txt over: %v", err)
	}

	// StageAll then reset --hard HEAD discards a fresh uncommitted change.
	os.WriteFile(filepath.Join(root, "dirty.txt"), []byte("junk\n"), 0o644)
	if err := StageAll(ctx, root); err != nil {
		t.Fatalf("StageAll: %v", err)
	}
	if err := ResetHard(ctx, root, "HEAD"); err != nil {
		t.Fatalf("ResetHard: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "dirty.txt")); err == nil {
		t.Errorf("reset --hard HEAD should have discarded dirty.txt")
	}
}

func baseBranchName2(t *testing.T, root string) string {
	t.Helper()
	for _, b := range Branches(context.Background(), root) {
		if b.Name != "side" {
			return b.Name
		}
	}
	t.Fatal("no base branch")
	return ""
}

func currentBranchName(t *testing.T, root string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", root, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func baseBranchName(t *testing.T, root string) string {
	t.Helper()
	for _, b := range Branches(context.Background(), root) {
		if b.Name != "feature" {
			return b.Name
		}
	}
	t.Fatal("no base branch found")
	return ""
}

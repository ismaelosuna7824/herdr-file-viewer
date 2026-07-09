package gitstatus

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initRepo creates a git repo with one committed file, then leaves the tree in
// a known dirty state: one modified file, one new untracked file, one deleted.
func initRepo(t *testing.T) string {
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
		p := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run("init", "-q")
	write("keep.txt", "original\n")
	write("gone.txt", "delete me\n")
	write("pkg/mod.txt", "v1\n")
	run("add", "-A")
	run("commit", "-q", "-m", "init")

	write("pkg/mod.txt", "v2\n")               // modified
	os.Remove(filepath.Join(root, "gone.txt")) // deleted
	write("brand-new.txt", "hi\n")             // untracked
	return root
}

func TestLoadClassifiesStates(t *testing.T) {
	root := initRepo(t)
	st := Load(context.Background(), root)
	if st.Empty() {
		t.Fatal("expected a dirty status, got empty")
	}

	cases := map[string]Code{
		"pkg/mod.txt":   Modified,
		"gone.txt":      Deleted,
		"brand-new.txt": Untracked,
	}
	for rel, want := range cases {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if got := st.FileCode(abs); got != want {
			t.Errorf("%s: got code %v, want %v", rel, got, want)
		}
	}

	// keep.txt is unchanged — no decoration.
	if st.FileCode(filepath.Join(root, "keep.txt")) != None {
		t.Errorf("keep.txt should have no status")
	}

	// The pkg/ directory contains a modified file, so it must be dirty.
	if !st.DirDirty(filepath.Join(root, "pkg")) {
		t.Errorf("pkg/ should be marked dirty (contains modified mod.txt)")
	}
}

func TestLoadNonRepoIsEmpty(t *testing.T) {
	st := Load(context.Background(), t.TempDir())
	if !st.Empty() {
		t.Errorf("a non-git directory should yield an empty status")
	}
}

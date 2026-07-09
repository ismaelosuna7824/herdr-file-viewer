package explorer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRefreshPicksUpNewFiles(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644)
	os.Mkdir(filepath.Join(root, "sub"), 0o755)
	os.WriteFile(filepath.Join(root, "sub", "old.txt"), []byte("x"), 0o644)

	tr, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	// Expand sub/.
	for i, n := range tr.Visible() {
		if n.Name == "sub" {
			tr.SetCursor(i)
			tr.Toggle()
			break
		}
	}
	before := len(tr.Visible())

	// Files appear on disk while the tree is open.
	os.WriteFile(filepath.Join(root, "b.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(root, "sub", "new.txt"), []byte("x"), 0o644)

	tr.Refresh()

	if got := len(tr.Visible()); got != before+2 {
		t.Fatalf("expected 2 new visible nodes, before=%d after=%d", before, got)
	}
	// sub/ stays expanded, so its new file is visible.
	var sawNew bool
	for _, n := range tr.Visible() {
		if n.Name == "new.txt" {
			sawNew = true
		}
	}
	if !sawNew {
		t.Error("new.txt under expanded sub/ should be visible after refresh")
	}
}

func TestRefreshDropsDeletedFiles(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "keep.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(root, "gone.txt"), []byte("x"), 0o644)
	tr, _ := New(root)
	os.Remove(filepath.Join(root, "gone.txt"))
	tr.Refresh()
	for _, n := range tr.Visible() {
		if n.Name == "gone.txt" {
			t.Error("deleted file should drop from the tree after refresh")
		}
	}
}

package explorer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotAndRestoreExpandedFoldersAndSelection(t *testing.T) {
	root := t.TempDir()
	docs := filepath.Join(root, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(docs, "readme.md")
	if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tree, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if !tree.RevealPath(path) {
		t.Fatal("test path should be revealable")
	}
	saved := tree.Snapshot()

	restored, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	restored.Restore(saved)
	if selected := restored.Selected(); selected == nil || selected.Path != path {
		t.Fatalf("selected path was not restored: %+v", selected)
	}
	if len(restored.Visible()) < 3 || !restored.Visible()[1].Expanded {
		t.Fatalf("expanded directory was not restored: %+v", restored.Visible())
	}
}

func TestRestoreIgnoresOutOfRootAndMissingPaths(t *testing.T) {
	tree, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tree.Restore(State{
		Expanded: []string{".", "../outside", "/absolute"},
		Selected: "missing/file.txt",
	})
	if len(tree.Visible()) != 1 || tree.Selected() != tree.Root {
		t.Fatalf("stale state must leave a safe root-only tree: %+v", tree.Visible())
	}
}

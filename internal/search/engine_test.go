package search

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// writeTree lays down a small fixture project inside a temp dir.
func writeTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"main.go":           "package main\n\nfunc main() { TODO() }\n",
		"util/helper.go":    "package util\n\n// TODO: refactor\nfunc Helper() {}\n",
		"README.md":         "# Title\nsome todo here and TODO there\n",
		".gitignore":        "ignored/\n*.log\n",
		"ignored/secret.go": "package ignored\nfunc TODO() {}\n",
		"app.log":           "TODO in a log file\n",
		"node_modules/x.js": "// TODO default-ignored\n",
	}
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestSearchPlainRespectsIgnores(t *testing.T) {
	root := writeTree(t)
	res, err := Search(context.Background(), root, Options{Query: "TODO"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, m := range res.Matches {
		got[filepath.ToSlash(m.Path)]++
	}
	// main.go, util/helper.go, README.md should match. .gitignore excludes
	// ignored/ and *.log; node_modules is default-ignored.
	for _, banned := range []string{"ignored/secret.go", "app.log", "node_modules/x.js"} {
		if got[banned] > 0 {
			t.Errorf("expected %s to be ignored, but it matched", banned)
		}
	}
	if got["main.go"] == 0 || got["util/helper.go"] == 0 || got["README.md"] == 0 {
		t.Errorf("expected matches in main.go, util/helper.go, README.md; got %v", got)
	}
}

func TestCaseSensitivity(t *testing.T) {
	root := writeTree(t)
	insensitive, _ := Search(context.Background(), root, Options{Query: "todo"})
	if len(insensitive.Matches) == 0 {
		t.Fatal("case-insensitive search should find TODO via 'todo'")
	}
	sensitive, _ := Search(context.Background(), root, Options{Query: "todo", CaseSensitive: true})
	for _, m := range sensitive.Matches {
		if m.Text[m.Start:m.End] != "todo" {
			t.Errorf("case-sensitive match returned %q", m.Text[m.Start:m.End])
		}
	}
	if len(sensitive.Matches) >= len(insensitive.Matches) {
		t.Errorf("expected fewer case-sensitive matches (%d) than insensitive (%d)",
			len(sensitive.Matches), len(insensitive.Matches))
	}
}

func TestWholeWord(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("cat catalog concatenate\n"), 0o644)
	res, _ := Search(context.Background(), root, Options{Query: "cat", WholeWord: true})
	if len(res.Matches) != 1 {
		t.Fatalf("whole-word 'cat' should match one line once at word boundary, got %d matches", len(res.Matches))
	}
	m := res.Matches[0]
	if m.Text[m.Start:m.End] != "cat" || m.Start != 0 {
		t.Errorf("whole-word match landed wrong: start=%d text=%q", m.Start, m.Text[m.Start:m.End])
	}
}

func TestRegex(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("id=42 id=7 name=x\n"), 0o644)
	res, err := Search(context.Background(), root, Options{Query: `id=\d+`, Regex: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Matches) != 1 { // one line, FindStringIndex returns first hit per line
		t.Fatalf("expected 1 regex match line, got %d", len(res.Matches))
	}
	if res.Matches[0].Text[res.Matches[0].Start:res.Matches[0].End] != "id=42" {
		t.Errorf("regex matched %q, want id=42", res.Matches[0].Text[res.Matches[0].Start:res.Matches[0].End])
	}
}

func TestInvalidRegexReturnsError(t *testing.T) {
	root := t.TempDir()
	_, err := Search(context.Background(), root, Options{Query: "(unclosed", Regex: true})
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
}

func TestEmptyQueryNoMatches(t *testing.T) {
	root := writeTree(t)
	res, err := Search(context.Background(), root, Options{Query: "   "})
	if err != nil || len(res.Matches) != 0 {
		t.Fatalf("blank query should return no matches, got %d (err=%v)", len(res.Matches), err)
	}
}

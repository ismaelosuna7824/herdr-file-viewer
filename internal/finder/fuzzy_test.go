package finder

import "testing"

func match(query string, paths []string, limit int) []Candidate {
	return NewMatcher(paths).Match(query, limit)
}

func TestMatchSubsequenceOnly(t *testing.T) {
	paths := []string{"internal/ui/app.go", "cmd/file-viewer/main.go", "README.md"}
	got := match("xyz", paths, 0)
	if len(got) != 0 {
		t.Fatalf("query 'xyz' should match nothing, got %d", len(got))
	}
}

func TestMatchRanksBasenameAndBoundaries(t *testing.T) {
	paths := []string{
		"internal/searchbar/aaa.go", // 'app' not a clean subsequence here
		"internal/ui/app.go",        // basename starts with 'app' — should win
		"apparatus/other.txt",
	}
	got := match("app", paths, 0)
	if len(got) == 0 {
		t.Fatal("expected matches for 'app'")
	}
	if got[0].Path != "internal/ui/app.go" {
		t.Errorf("expected app.go to rank first, got %q", got[0].Path)
	}
}

func TestMatchIsCaseInsensitive(t *testing.T) {
	got := match("app", []string{"internal/ui/App.go"}, 0)
	if len(got) != 1 {
		t.Fatalf("expected case-insensitive match, got %d", len(got))
	}
}

func TestMatchEmptyQueryReturnsAllUpToLimit(t *testing.T) {
	paths := []string{"a", "b", "c", "d"}
	got := match("", paths, 2)
	if len(got) != 2 {
		t.Fatalf("empty query with limit 2 should return 2, got %d", len(got))
	}
}

func TestMatchPositionsAreValidIndices(t *testing.T) {
	got := match("mn", []string{"main.go"}, 0)
	if len(got) != 1 {
		t.Fatalf("expected 1 match, got %d", len(got))
	}
	for _, pos := range got[0].Positions {
		if pos < 0 || pos >= len("main.go") {
			t.Errorf("position %d out of range for %q", pos, "main.go")
		}
	}
}

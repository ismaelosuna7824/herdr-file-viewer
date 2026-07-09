package viewer

import (
	"strings"
	"testing"
)

func TestHighlightProducesANSIForGo(t *testing.T) {
	src := "package main\n\nfunc main() { x := 42 }\n"
	got := Highlight("main.go", src)
	if len(got) != len(strings.Split(src, "\n")) {
		t.Fatalf("highlighted line count %d != source lines %d", len(got), len(strings.Split(src, "\n")))
	}
	if !strings.Contains(strings.Join(got, "\n"), "\x1b[") {
		t.Errorf("expected ANSI escapes in highlighted Go")
	}
}

func TestSetHighlightedGuardsStaleAndMismatched(t *testing.T) {
	m := New()
	m.path = "a.go"
	m.lines = []string{"one", "two"}

	// Wrong path is ignored.
	m.SetHighlighted("other.go", []string{"x", "y"})
	if m.rendered != nil {
		t.Errorf("highlight for a different path must be ignored")
	}
	// Mismatched line count is ignored.
	m.SetHighlighted("a.go", []string{"only-one"})
	if m.rendered != nil {
		t.Errorf("highlight with wrong line count must be ignored")
	}
	// Matching path + count is applied.
	m.SetHighlighted("a.go", []string{"ONE", "TWO"})
	if len(m.rendered) != 2 || m.rendered[0] != "ONE" {
		t.Errorf("matching highlight should be applied, got %v", m.rendered)
	}
}

package viewer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTerminalHyperlinksLeavesOneHerdrDetectableURL(t *testing.T) {
	input := "visit https://example.com/docs?q=1"
	got := terminalHyperlinks(input)
	if got != input {
		t.Fatalf("visible URL must remain the only Herdr link source:\nwant %q\ngot  %q", input, got)
	}
	if strings.Contains(got, "\x1b]8;;") {
		t.Fatalf("explicit OSC 8 plus Herdr visible-URL detection opens two tabs:\n%q", got)
	}
}

func TestTerminalHyperlinksLeavesTrailingSentencePunctuationOutsideLink(t *testing.T) {
	input := "see https://example.com/docs."
	if got := terminalHyperlinks(input); got != input {
		t.Fatalf("host-detected URL text and punctuation should remain unchanged:\n%q", got)
	}
}

func TestViewerRendersOneHostDetectablePlainURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "links.txt")
	if err := os.WriteFile(path, []byte("open https://example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m.Load(path)
	m.SetSize(80, 12)
	view := m.View()
	if !strings.Contains(view, "https://example.com") {
		t.Fatalf("viewer dropped the host-detectable URL:\n%q", view)
	}
	if strings.Contains(view, "\x1b]8;;") {
		t.Fatalf("viewer emitted a second OSC 8 activation source:\n%q", view)
	}
}

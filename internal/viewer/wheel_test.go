package viewer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestViewerWheelDownScrollsContent verifies mouse-wheel events reach the
// underlying viewport and move the visible content, so wheel scrolling works in
// the file viewer without any tree-only machinery.
func TestViewerWheelDownScrollsContent(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 200; i++ {
		b.WriteString("line ")
		b.WriteByte(byte('a' + i%26))
		b.WriteByte('\n')
	}
	path := filepath.Join(t.TempDir(), "long.txt")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New()
	m.Load(path)
	m.SetSize(40, 6)
	before := m.View()

	var model Model = m
	for range 5 {
		model, _ = model.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	}
	after := model.View()

	if before == after {
		t.Fatalf("wheel-down should scroll the viewer content, but the view did not change:\n%s", after)
	}
}

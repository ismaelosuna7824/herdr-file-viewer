// Package explorer models a navigable, lazily-expanded directory tree — the
// left-hand file browser. It holds no rendering logic; the UI layer walks the
// visible rows and draws them.
package explorer

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Node is a single entry in the tree. Directory children are loaded lazily the
// first time a directory is expanded.
type Node struct {
	Name     string
	Path     string // absolute path
	IsDir    bool
	Depth    int
	Expanded bool
	loaded   bool
	children []*Node
}

// Tree is the rooted file browser.
type Tree struct {
	Root    *Node
	visible []*Node // cached flattened view of expanded nodes
	cursor  int
}

// New builds a tree rooted at the given directory and expands the root.
func New(root string) (*Tree, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	r := &Node{
		Name:  filepath.Base(abs),
		Path:  abs,
		IsDir: info.IsDir(),
		Depth: 0,
	}
	t := &Tree{Root: r}
	t.expand(r)
	r.Expanded = true
	t.rebuild()
	return t, nil
}

// loadChildren reads a directory's entries once, sorting directories first then
// names case-insensitively. Hidden dotfiles are kept — the explorer shows
// everything so the user is never surprised by a missing file.
func (t *Tree) loadChildren(n *Node) {
	if n.loaded || !n.IsDir {
		return
	}
	n.loaded = true
	entries, err := os.ReadDir(n.Path)
	if err != nil {
		return
	}
	for _, e := range entries {
		n.children = append(n.children, &Node{
			Name:  e.Name(),
			Path:  filepath.Join(n.Path, e.Name()),
			IsDir: e.IsDir(),
			Depth: n.Depth + 1,
		})
	}
	sortChildren(n.children)
}

func sortChildren(nodes []*Node) {
	sort.SliceStable(nodes, func(i, j int) bool {
		a, b := nodes[i], nodes[j]
		if a.IsDir != b.IsDir {
			return a.IsDir
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
}

// Refresh re-reads every directory already loaded from disk, so files created,
// renamed or deleted while the viewer is open show up. Expansion state and the
// cursor (by path) are preserved.
func (t *Tree) Refresh() {
	var selPath string
	if n := t.Selected(); n != nil {
		selPath = n.Path
	}
	t.refreshNode(t.Root)
	t.rebuild()
	if selPath != "" {
		for i, v := range t.visible {
			if v.Path == selPath {
				t.cursor = i
				break
			}
		}
	}
	if t.cursor >= len(t.visible) {
		t.cursor = len(t.visible) - 1
	}
	if t.cursor < 0 {
		t.cursor = 0
	}
}

// refreshNode re-reads a loaded directory, merging disk contents with the
// in-memory tree so existing nodes keep their expanded/loaded state while new
// entries appear and deleted ones drop. Recurses into expanded subdirectories.
func (t *Tree) refreshNode(n *Node) {
	if !n.IsDir || !n.loaded {
		return
	}
	entries, err := os.ReadDir(n.Path)
	if err != nil {
		return
	}
	existing := make(map[string]*Node, len(n.children))
	for _, c := range n.children {
		existing[c.Name] = c
	}
	merged := make([]*Node, 0, len(entries))
	for _, e := range entries {
		if old, ok := existing[e.Name()]; ok {
			merged = append(merged, old)
		} else {
			merged = append(merged, &Node{
				Name:  e.Name(),
				Path:  filepath.Join(n.Path, e.Name()),
				IsDir: e.IsDir(),
				Depth: n.Depth + 1,
			})
		}
	}
	sortChildren(merged)
	n.children = merged
	for _, c := range n.children {
		if c.IsDir && c.Expanded {
			t.refreshNode(c)
		}
	}
}

func (t *Tree) expand(n *Node) {
	t.loadChildren(n)
	n.Expanded = true
}

// rebuild recomputes the flattened list of visible nodes via a depth-first walk
// of expanded directories.
func (t *Tree) rebuild() {
	t.visible = t.visible[:0]
	var walk func(n *Node)
	walk = func(n *Node) {
		t.visible = append(t.visible, n)
		if n.IsDir && n.Expanded {
			for _, c := range n.children {
				walk(c)
			}
		}
	}
	walk(t.Root)
	if t.cursor >= len(t.visible) {
		t.cursor = len(t.visible) - 1
	}
	if t.cursor < 0 {
		t.cursor = 0
	}
}

// Visible returns the current flattened rows.
func (t *Tree) Visible() []*Node { return t.visible }

// Cursor returns the index of the selected row.
func (t *Tree) Cursor() int { return t.cursor }

// Selected returns the node under the cursor, or nil for an empty tree.
func (t *Tree) Selected() *Node {
	if t.cursor < 0 || t.cursor >= len(t.visible) {
		return nil
	}
	return t.visible[t.cursor]
}

// MoveUp / MoveDown move the cursor within the visible rows.
func (t *Tree) MoveUp() {
	if t.cursor > 0 {
		t.cursor--
	}
}

func (t *Tree) MoveDown() {
	if t.cursor < len(t.visible)-1 {
		t.cursor++
	}
}

// Toggle expands or collapses the selected directory. On a file it does
// nothing and returns false so the caller knows to open it instead.
func (t *Tree) Toggle() bool {
	n := t.Selected()
	if n == nil || !n.IsDir {
		return false
	}
	if n.Expanded {
		n.Expanded = false
	} else {
		t.expand(n)
	}
	t.rebuild()
	return true
}

// SetCursor clamps and sets the cursor position.
func (t *Tree) SetCursor(i int) {
	t.cursor = i
	if t.cursor < 0 {
		t.cursor = 0
	}
	if t.cursor >= len(t.visible) {
		t.cursor = len(t.visible) - 1
	}
}

// RevealPath expands the tree down to the given absolute path and puts the
// cursor on it. It is used to sync the browser when the user jumps to a file
// from the finder or search results. Returns true when the path was found.
func (t *Tree) RevealPath(target string) bool {
	target = filepath.Clean(target)
	rel, err := filepath.Rel(t.Root.Path, target)
	if err != nil || strings.HasPrefix(rel, "..") {
		return false
	}
	node := t.Root
	if rel != "." {
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			t.expand(node)
			var next *Node
			for _, c := range node.children {
				if c.Name == part {
					next = c
					break
				}
			}
			if next == nil {
				return false
			}
			node = next
		}
	}
	t.rebuild()
	for i, v := range t.visible {
		if v == node {
			t.cursor = i
			return true
		}
	}
	return false
}

// State is the user-visible explorer state that survives a plugin restart.
// Paths are slash-separated and relative to the tree root; "." is the root.
type State struct {
	Expanded []string
	Selected string
}

// Snapshot returns expanded directories and the selected row without exposing
// the tree's lazy-loading internals to the UI persistence layer.
func (t *Tree) Snapshot() State {
	state := State{}
	var walk func(*Node)
	walk = func(node *Node) {
		if node.IsDir && node.Expanded {
			if rel, ok := t.relativePath(node.Path); ok {
				state.Expanded = append(state.Expanded, rel)
			}
		}
		for _, child := range node.children {
			walk(child)
		}
	}
	walk(t.Root)
	if selected := t.Selected(); selected != nil {
		state.Selected, _ = t.relativePath(selected.Path)
	}
	return state
}

// Restore applies a previous Snapshot. Missing or out-of-root paths are
// ignored so deleted files and stale/corrupt state cannot break the explorer.
func (t *Tree) Restore(state State) {
	var collapse func(*Node)
	collapse = func(node *Node) {
		if node.IsDir {
			node.Expanded = false
		}
		for _, child := range node.children {
			collapse(child)
		}
	}
	collapse(t.Root)

	expanded := append([]string(nil), state.Expanded...)
	sort.SliceStable(expanded, func(i, j int) bool {
		return strings.Count(expanded[i], "/") < strings.Count(expanded[j], "/")
	})
	for _, rel := range expanded {
		t.expandRelative(rel)
	}
	t.rebuild()
	if target, ok := t.absolutePath(state.Selected); ok {
		t.RevealPath(target)
	}
}

func (t *Tree) expandRelative(rel string) {
	target, ok := t.absolutePath(rel)
	if !ok {
		return
	}
	cleanRel, _ := filepath.Rel(t.Root.Path, target)
	node := t.Root
	if cleanRel != "." {
		for _, part := range strings.Split(cleanRel, string(filepath.Separator)) {
			t.expand(node)
			var next *Node
			for _, child := range node.children {
				if child.Name == part {
					next = child
					break
				}
			}
			if next == nil {
				return
			}
			node = next
		}
	}
	if node.IsDir {
		t.expand(node)
	}
}

func (t *Tree) relativePath(path string) (string, bool) {
	rel, err := filepath.Rel(t.Root.Path, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func (t *Tree) absolutePath(rel string) (string, bool) {
	if rel == "" || filepath.IsAbs(rel) {
		return "", false
	}
	target := filepath.Clean(filepath.Join(t.Root.Path, filepath.FromSlash(rel)))
	if _, ok := t.relativePath(target); !ok {
		return "", false
	}
	return target, true
}

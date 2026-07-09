package ui

import (
	"sort"
	"strings"

	"github.com/ismaelosuna7824/herdr-file-viewer/internal/gitstatus"
)

type stageState int

const (
	stageNone stageState = iota
	stageSome
	stageAll
)

// changeRow is one flattened, indented row of the staging tree: either a
// directory (aggregating the files beneath it) or a file leaf.
type changeRow struct {
	depth  int
	name   string // dir chain ("a/b") or file basename
	isDir  bool
	change gitstatus.Change   // valid when !isDir
	files  []gitstatus.Change // when isDir: every file beneath it
	staged stageState
}

// ctNode is an intermediate tree node used while building the rows.
type ctNode struct {
	name     string
	children map[string]*ctNode
	change   *gitstatus.Change // non-nil only for file leaves
}

func (n *ctNode) isDir() bool { return n.change == nil }

// buildChangeRows turns a flat list of changes into a lazygit-style directory
// tree: nested by folder, with single-child directory chains compressed into one
// row ("a/b/c"). Directory rows aggregate the staged state of their subtree.
func buildChangeRows(changes []gitstatus.Change) []changeRow {
	root := &ctNode{children: map[string]*ctNode{}}
	for i := range changes {
		c := changes[i]
		parts := strings.Split(c.Path, "/")
		cur := root
		for j, part := range parts {
			if j == len(parts)-1 {
				cur.children[part] = &ctNode{name: part, change: &c}
				break
			}
			next := cur.children[part]
			if next == nil {
				next = &ctNode{name: part, children: map[string]*ctNode{}}
				cur.children[part] = next
			}
			cur = next
		}
	}
	compress(root)

	var rows []changeRow
	appendNode(root, 0, &rows)
	return rows
}

// compress collapses directories that have a single sub-directory child into a
// combined "parent/child" name — the way lazygit shows deep unique paths.
func compress(node *ctNode) {
	for _, c := range node.children {
		if !c.isDir() {
			continue
		}
		for len(c.children) == 1 {
			var only *ctNode
			for _, x := range c.children {
				only = x
			}
			if only.isDir() {
				c.name += "/" + only.name
				c.children = only.children
				continue
			}
			break
		}
		compress(c)
	}
}

// appendNode emits rows for a directory's children (dirs first, then files, each
// alphabetical) and returns every file beneath it, so directory rows can
// aggregate their subtree's staged state.
func appendNode(node *ctNode, depth int, rows *[]changeRow) []gitstatus.Change {
	var dirs, files []*ctNode
	for _, c := range node.children {
		if c.isDir() {
			dirs = append(dirs, c)
		} else {
			files = append(files, c)
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })

	var all []gitstatus.Change
	for _, d := range dirs {
		idx := len(*rows)
		*rows = append(*rows, changeRow{depth: depth, name: d.name, isDir: true})
		sub := appendNode(d, depth+1, rows)
		(*rows)[idx].files = sub
		(*rows)[idx].staged = aggregate(sub)
		all = append(all, sub...)
	}
	for _, f := range files {
		st := stageNone
		if f.change.Staged() {
			st = stageAll
		}
		*rows = append(*rows, changeRow{depth: depth, name: f.name, isDir: false, change: *f.change, staged: st})
		all = append(all, *f.change)
	}
	return all
}

// aggregate returns the combined staged state of a set of files.
func aggregate(files []gitstatus.Change) stageState {
	staged := 0
	for _, f := range files {
		if f.Staged() {
			staged++
		}
	}
	switch {
	case staged == 0:
		return stageNone
	case staged == len(files):
		return stageAll
	default:
		return stageSome
	}
}

// paths returns the file paths of a row (one for a file, the whole subtree for a
// directory).
func (r changeRow) paths() []string {
	if !r.isDir {
		return []string{r.change.Path}
	}
	out := make([]string, len(r.files))
	for i, f := range r.files {
		out[i] = f.Path
	}
	return out
}

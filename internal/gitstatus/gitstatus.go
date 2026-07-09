// Package gitstatus reports the git working-tree status of files under a root,
// so the file browser can flag what changed — modified, new, deleted, renamed —
// the way an editor's source-control decorations do.
//
// It shells out to `git`, which is the only sane source of truth for status
// (ignore rules, submodules, rename detection). If the root is not a git
// repository, or git is absent, Load returns an empty, harmless result: the
// viewer simply shows no decorations.
package gitstatus

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
)

// Code is the decoration shown for a file.
type Code int

const (
	None Code = iota
	Untracked
	Added
	Modified
	Deleted
	Renamed
	Conflicted
)

// Letter is the single-character badge for a status, matching common editor
// conventions (U for untracked/new, M modified, A added, D deleted, …).
func (c Code) Letter() string {
	switch c {
	case Untracked:
		return "U"
	case Added:
		return "A"
	case Modified:
		return "M"
	case Deleted:
		return "D"
	case Renamed:
		return "R"
	case Conflicted:
		return "!"
	default:
		return " "
	}
}

// Status is the parsed result: the current branch, per-file codes, plus the set
// of directories that contain at least one changed file (keyed by absolute
// path).
type Status struct {
	Branch string
	Files  map[string]Code
	Dirs   map[string]struct{}
}

// Empty reports whether there is nothing to decorate.
func (s Status) Empty() bool { return len(s.Files) == 0 }

// FileCode returns the status of an absolute file path.
func (s Status) FileCode(abs string) Code {
	if s.Files == nil {
		return None
	}
	return s.Files[filepath.Clean(abs)]
}

// DirDirty reports whether an absolute directory path contains changes.
func (s Status) DirDirty(abs string) bool {
	if s.Dirs == nil {
		return false
	}
	_, ok := s.Dirs[filepath.Clean(abs)]
	return ok
}

// Change is one entry in the staging view: a changed file with its raw index
// (staged) and worktree (unstaged) status columns from git's porcelain output.
type Change struct {
	Path  string // relative slash path
	Index byte   // X column — staged status
	Work  byte   // Y column — worktree status
}

// Staged reports whether the file has changes in the index.
func (c Change) Staged() bool { return c.Index != ' ' && c.Index != '?' }

// Untracked reports whether git isn't tracking the file yet.
func (c Change) Untracked() bool { return c.Index == '?' }

// Code collapses the two columns into a display code (same mapping as the tree).
func (c Change) Code() Code { return classify(c.Index, c.Work) }

// Changes lists every changed file with its staged/unstaged columns, for the
// staging view. Returns nil outside a repo.
func Changes(ctx context.Context, root string) []Change {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil
	}
	cmd := exec.CommandContext(ctx, "git", "-C", abs,
		"status", "--porcelain=v1", "-z", "--untracked-files=all")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil
	}

	var changes []Change
	records := strings.Split(out.String(), "\x00")
	for i := 0; i < len(records); i++ {
		rec := records[i]
		if len(rec) < 3 {
			continue
		}
		x, y := rec[0], rec[1]
		path := rec[3:]
		if x == 'R' || x == 'C' || y == 'R' || y == 'C' {
			i++ // rename/copy: consume the original-path field
		}
		changes = append(changes, Change{Path: filepath.ToSlash(path), Index: x, Work: y})
	}
	return changes
}

// Load runs `git status` for root and parses the porcelain output. It never
// returns an error for the "not a repo / no git" case — that yields an empty
// Status, since a viewer without decorations is a perfectly valid state.
func Load(ctx context.Context, root string) Status {
	empty := Status{Files: map[string]Code{}, Dirs: map[string]struct{}{}}

	abs, err := filepath.Abs(root)
	if err != nil {
		return empty
	}
	cmd := exec.CommandContext(ctx, "git",
		"-C", abs,
		"status", "--porcelain=v1", "-z", "--untracked-files=all")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return empty // not a repo, git missing, etc.
	}
	st := parse(abs, out.Bytes())
	st.Branch = currentBranch(ctx, abs)
	return st
}

// currentBranch returns the short branch name, or "" outside a repo / on a
// detached HEAD where the symbolic name is unavailable.
func currentBranch(ctx context.Context, root string) string {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--abbrev-ref", "HEAD")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	b := strings.TrimSpace(out.String())
	if b == "HEAD" {
		return "" // detached
	}
	return b
}

// parse decodes NUL-separated porcelain v1 records into a Status. Each record
// is "XY <path>"; rename/copy records ("R"/"C") are followed by a second record
// holding the original path, which we consume and ignore.
func parse(root string, data []byte) Status {
	s := Status{Files: map[string]Code{}, Dirs: map[string]struct{}{}}
	records := strings.Split(string(data), "\x00")

	for i := 0; i < len(records); i++ {
		rec := records[i]
		if len(rec) < 3 {
			continue
		}
		x, y := rec[0], rec[1]
		path := rec[3:] // skip "XY "

		// Rename/copy: the very next NUL field is the original path.
		if x == 'R' || x == 'C' || y == 'R' || y == 'C' {
			i++ // consume the original-path field
		}

		abs := filepath.Join(root, filepath.FromSlash(path))
		code := classify(x, y)
		if code == None {
			continue
		}
		s.Files[abs] = code
		markDirs(s.Dirs, root, abs)
	}
	return s
}

// classify collapses the two-column git status into a single display code,
// preferring the most salient state.
func classify(x, y byte) Code {
	switch {
	case x == '?' && y == '?':
		return Untracked
	case x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D'):
		return Conflicted
	case x == 'R' || y == 'R':
		return Renamed
	case x == 'A':
		return Added
	case x == 'D' || y == 'D':
		return Deleted
	case x == 'M' || y == 'M' || x == 'T' || y == 'T':
		return Modified
	default:
		return Modified
	}
}

// markDirs records every ancestor directory of abs (up to root) as dirty.
func markDirs(dirs map[string]struct{}, root, abs string) {
	dir := filepath.Dir(abs)
	for {
		if len(dir) < len(root) || !strings.HasPrefix(dir, root) {
			return
		}
		dirs[dir] = struct{}{}
		if dir == root {
			return
		}
		dir = filepath.Dir(dir)
	}
}

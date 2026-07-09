// Package gitlog reads a repository's commit history — the data behind the git
// log panel. Like the other git packages it shells out to `git` and degrades to
// an empty slice outside a repo.
package gitlog

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
)

// Commit is one entry in the history.
type Commit struct {
	Hash    string // full SHA (used to load the commit's diff)
	Short   string // abbreviated SHA
	Author  string
	When    string // relative date, e.g. "3 days ago"
	Subject string
}

// Branch is a local branch.
type Branch struct {
	Name    string
	Current bool
}

// fieldSep and recordSep are unlikely-to-collide delimiters for --pretty so we
// can parse without worrying about spaces in subjects/authors.
const (
	fieldSep  = "\x1f" // unit separator
	recordSep = "\x1e" // record separator
)

// Load returns up to limit most-recent commits reachable from HEAD.
func Load(ctx context.Context, root string, limit int) []Commit {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil
	}
	format := strings.Join([]string{"%H", "%h", "%an", "%cr", "%s"}, fieldSep) + recordSep
	cmd := exec.CommandContext(ctx, "git", "-C", abs,
		"log", "--no-color", "-n", itoa(limit), "--pretty=format:"+format)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil
	}

	var commits []Commit
	for _, rec := range strings.Split(out.String(), recordSep) {
		rec = strings.Trim(rec, "\n")
		if rec == "" {
			continue
		}
		f := strings.Split(rec, fieldSep)
		if len(f) < 5 {
			continue
		}
		commits = append(commits, Commit{
			Hash:    f[0],
			Short:   f[1],
			Author:  f[2],
			When:    f[3],
			Subject: f[4],
		})
	}
	return commits
}

// Branches lists local branches, newest-committed first, marking the current
// one. Returns nil outside a repo.
func Branches(ctx context.Context, root string) []Branch {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil
	}
	// %(HEAD) is "*" for the current branch, a space otherwise.
	cmd := exec.CommandContext(ctx, "git", "-C", abs,
		"for-each-ref", "--sort=-committerdate", "--format=%(HEAD)%(refname:short)", "refs/heads/")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil
	}
	var branches []Branch
	for _, line := range strings.Split(out.String(), "\n") {
		if line == "" {
			continue
		}
		current := strings.HasPrefix(line, "*")
		name := strings.TrimSpace(strings.TrimPrefix(line, "*"))
		if name != "" {
			branches = append(branches, Branch{Name: name, Current: current})
		}
	}
	return branches
}

// runGit runs a git subcommand in root and returns an error carrying git's own
// output on failure (so conflict/refusal messages reach the user).
func runGit(ctx context.Context, root string, args ...string) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", abs}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

// Switch checks out the named branch.
func Switch(ctx context.Context, root, name string) error {
	return runGit(ctx, root, "switch", name)
}

// CreateBranch creates a new branch from HEAD and switches to it.
func CreateBranch(ctx context.Context, root, name string) error {
	return runGit(ctx, root, "switch", "-c", name)
}

// DeleteBranch deletes a branch (safe delete: git refuses if unmerged).
func DeleteBranch(ctx context.Context, root, name string) error {
	return runGit(ctx, root, "branch", "-d", name)
}

// CommitAll stages every change (tracked and untracked) and commits it.
func CommitAll(ctx context.Context, root, message string) error {
	if err := runGit(ctx, root, "add", "-A"); err != nil {
		return err
	}
	return runGit(ctx, root, "commit", "-m", message)
}

// Merge merges the named branch into the current branch.
func Merge(ctx context.Context, root, name string) error {
	return runGit(ctx, root, "merge", name)
}

// Rebase rebases the current branch onto the named branch.
func Rebase(ctx context.Context, root, name string) error {
	return runGit(ctx, root, "rebase", name)
}

// Fetch updates all remotes and prunes deleted remote branches.
func Fetch(ctx context.Context, root string) error {
	return runGit(ctx, root, "fetch", "--all", "--prune")
}

// Pull fast-forwards / merges the upstream into the current branch.
func Pull(ctx context.Context, root string) error {
	return runGit(ctx, root, "pull")
}

// Push publishes the current branch, setting upstream on first push.
func Push(ctx context.Context, root string) error {
	return runGit(ctx, root, "push")
}

// ForcePush force-pushes with --force-with-lease, which refuses to clobber
// commits on the remote that you haven't seen — the safe way to force push.
func ForcePush(ctx context.Context, root string) error {
	return runGit(ctx, root, "push", "--force-with-lease")
}

// Stash saves the working tree (including untracked files).
func Stash(ctx context.Context, root string) error {
	return runGit(ctx, root, "stash", "push", "--include-untracked")
}

// StashPop restores the most recently stashed changes.
func StashPop(ctx context.Context, root string) error {
	return runGit(ctx, root, "stash", "pop")
}

// Amend folds all current changes into the last commit, keeping its message.
func Amend(ctx context.Context, root string) error {
	if err := runGit(ctx, root, "add", "-A"); err != nil {
		return err
	}
	return runGit(ctx, root, "commit", "--amend", "--no-edit")
}

// UndoLastCommit undoes the last commit but keeps its changes staged.
func UndoLastCommit(ctx context.Context, root string) error {
	return runGit(ctx, root, "reset", "--soft", "HEAD~1")
}

// StageAll stages every change (equivalent to `git add -A` / `git add .`).
func StageAll(ctx context.Context, root string) error {
	return runGit(ctx, root, "add", "-A")
}

// StageFile stages a single path (relative to root).
func StageFile(ctx context.Context, root, path string) error {
	return runGit(ctx, root, "add", "--", path)
}

// UnstageFile removes a single path from the index (keeps working-tree changes).
func UnstageFile(ctx context.Context, root, path string) error {
	return runGit(ctx, root, "restore", "--staged", "--", path)
}

// StagePaths stages one or more paths (used to stage a whole directory subtree).
func StagePaths(ctx context.Context, root string, paths ...string) error {
	return runGit(ctx, root, append([]string{"add", "--"}, paths...)...)
}

// UnstagePaths removes one or more paths from the index.
func UnstagePaths(ctx context.Context, root string, paths ...string) error {
	return runGit(ctx, root, append([]string{"restore", "--staged", "--"}, paths...)...)
}

// UnstageAll clears the index (keeps all working-tree changes).
func UnstageAll(ctx context.Context, root string) error {
	return runGit(ctx, root, "reset", "-q")
}

// CreateTag creates a lightweight tag at HEAD.
func CreateTag(ctx context.Context, root, name string) error {
	return runGit(ctx, root, "tag", name)
}

// CherryPick applies the given commit onto the current branch.
func CherryPick(ctx context.Context, root, ref string) error {
	return runGit(ctx, root, "cherry-pick", ref)
}

// ResetHard moves the current branch to ref and discards all changes. ref may
// be "HEAD" (discard uncommitted changes) or any commit-ish. DESTRUCTIVE.
func ResetHard(ctx context.Context, root, ref string) error {
	return runGit(ctx, root, "reset", "--hard", ref)
}

func itoa(n int) string {
	if n <= 0 {
		n = 100
	}
	// small positive int → string without importing strconv for one call
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

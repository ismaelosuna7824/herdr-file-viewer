package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ismaelosuna7824/herdr-file-viewer/internal/gitlog"
	"github.com/ismaelosuna7824/herdr-file-viewer/internal/gitstatus"
)

type logKind int

const (
	logChanges  logKind = iota // staging view (changed files with checkboxes)
	logBranches                // branch list (select to switch)
	logHistory                 // commit history
)

// logPanel is the right docked panel. It cycles between three views: the staging
// view (changed files), the branch list, and the commit history.
type logPanel struct {
	kind logKind

	changes []gitstatus.Change
	rows    []changeRow // changes flattened into a directory tree
	ccursor int

	commits []gitlog.Commit
	cursor  int

	branches  []gitlog.Branch
	bcursor   int
	branchErr string

	loading bool
	width   int
	height  int
}

// newLogPanel defaults to the staging (changes) view — the git-flow tab.
func newLogPanel() logPanel { return logPanel{kind: logChanges} }

func (p *logPanel) SetSize(w, h int) { p.width, p.height = w, h }

func (p *logPanel) setChanges(c []gitstatus.Change) {
	p.changes = c
	p.rows = buildChangeRows(c)
	if p.ccursor >= len(p.rows) {
		p.ccursor = len(p.rows) - 1
	}
	if p.ccursor < 0 {
		p.ccursor = 0
	}
}

func (p *logPanel) setCommits(c []gitlog.Commit) {
	p.commits = c
	p.loading = false
	if p.cursor >= len(c) {
		p.cursor = 0
	}
}

func (p *logPanel) setBranches(b []gitlog.Branch) {
	p.branches = b
	if p.bcursor >= len(b) {
		p.bcursor = 0
	}
}

// toggleKind cycles changes → branches → history → changes.
func (p *logPanel) toggleKind() {
	p.kind = (p.kind + 1) % 3
	p.branchErr = ""
}

func (p *logPanel) moveUp() {
	switch p.kind {
	case logChanges:
		if p.ccursor > 0 {
			p.ccursor--
		}
	case logBranches:
		if p.bcursor > 0 {
			p.bcursor--
		}
	default:
		if p.cursor > 0 {
			p.cursor--
		}
	}
}

func (p *logPanel) moveDown() {
	switch p.kind {
	case logChanges:
		if p.ccursor < len(p.rows)-1 {
			p.ccursor++
		}
	case logBranches:
		if p.bcursor < len(p.branches)-1 {
			p.bcursor++
		}
	default:
		if p.cursor < len(p.commits)-1 {
			p.cursor++
		}
	}
}

func (p *logPanel) selectedRow() (changeRow, bool) {
	if p.ccursor < 0 || p.ccursor >= len(p.rows) {
		return changeRow{}, false
	}
	return p.rows[p.ccursor], true
}

func (p *logPanel) selectedCommit() (gitlog.Commit, bool) {
	if p.cursor < 0 || p.cursor >= len(p.commits) {
		return gitlog.Commit{}, false
	}
	return p.commits[p.cursor], true
}

func (p *logPanel) selectedBranch() (gitlog.Branch, bool) {
	if p.bcursor < 0 || p.bcursor >= len(p.branches) {
		return gitlog.Branch{}, false
	}
	return p.branches[p.bcursor], true
}

// stagedCount returns how many changed files have staged content.
func (p *logPanel) stagedCount() int {
	n := 0
	for _, c := range p.changes {
		if c.Staged() {
			n++
		}
	}
	return n
}

// title returns the panel's label reflecting the current view.
func (p *logPanel) title() string {
	switch p.kind {
	case logChanges:
		return fmt.Sprintf("CHANGES (%d staged / %d)", p.stagedCount(), len(p.changes))
	case logBranches:
		return "BRANCHES"
	default:
		return "GIT LOG"
	}
}

func (p *logPanel) view(st styles, width, height int, focused bool) string {
	switch p.kind {
	case logChanges:
		return p.viewChanges(st, width, height, focused)
	case logBranches:
		return p.viewBranches(st, width, height, focused)
	default:
		return p.viewHistory(st, width, height, focused)
	}
}

// viewChanges renders the staging tree: nested directories and file leaves,
// each with a checkbox (staged / partial / none). Staged is green, unstaged
// amber; directory names are tinted like the file tree.
func (p *logPanel) viewChanges(st styles, width, height int, focused bool) string {
	if len(p.rows) == 0 {
		return st.muted.Render("  working tree clean — nothing to stage")
	}
	start := scrollStart(p.ccursor, len(p.rows), height)

	var b strings.Builder
	for i := start; i < start+height && i < len(p.rows); i++ {
		r := p.rows[i]
		indent := strings.Repeat("  ", r.depth)
		box := checkbox(r.staged)

		var content string
		if r.isDir {
			content = box + " " + st.dir.Render(r.name+"/")
		} else {
			content = box + " " + p.styleForState(r.staged).Render(string(r.change.Code().Letter())+" "+r.name)
		}

		if focused && i == p.ccursor {
			plain := "▸ " + indent + plainCheckbox(r.staged) + " " + r.name
			if r.isDir {
				plain += "/"
			} else {
				plain = "▸ " + indent + plainCheckbox(r.staged) + " " + string(r.change.Code().Letter()) + " " + r.name
			}
			b.WriteString(truncateLine(st.modalSel.Width(width).Render(plain), width))
		} else {
			b.WriteString(truncateLine("  "+indent+content, width))
		}
		if i < start+height-1 && i < len(p.rows)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (p *logPanel) styleForState(s stageState) lipgloss.Style {
	switch s {
	case stageAll:
		return lipgloss.NewStyle().Foreground(colGitNew)
	case stageSome:
		return lipgloss.NewStyle().Foreground(colGitModified)
	default:
		return lipgloss.NewStyle().Foreground(colGitModified)
	}
}

func checkbox(s stageState) string {
	switch s {
	case stageAll:
		return lipgloss.NewStyle().Foreground(colGitNew).Render("[✓]")
	case stageSome:
		return lipgloss.NewStyle().Foreground(colGitModified).Render("[~]")
	default:
		return lipgloss.NewStyle().Foreground(colMuted).Render("[ ]")
	}
}

func plainCheckbox(s stageState) string {
	switch s {
	case stageAll:
		return "[✓]"
	case stageSome:
		return "[~]"
	default:
		return "[ ]"
	}
}

// viewHistory renders the commit list.
func (p *logPanel) viewHistory(st styles, width, height int, focused bool) string {
	if p.loading {
		return st.muted.Render("  loading history…")
	}
	if len(p.commits) == 0 {
		return st.muted.Render("  no commits (or not a git repository)")
	}
	hashStyle := lipgloss.NewStyle().Foreground(colGitModified).Bold(true)
	start := scrollStart(p.cursor, len(p.commits), height)

	var b strings.Builder
	for i := start; i < start+height && i < len(p.commits); i++ {
		c := p.commits[i]
		meta := st.muted.Render("· " + c.Author + " · " + c.When)
		line := fmt.Sprintf("%s  %s  %s", hashStyle.Render(c.Short), c.Subject, meta)
		if focused && i == p.cursor {
			plain := fmt.Sprintf("%s  %s  · %s · %s", c.Short, c.Subject, c.Author, c.When)
			line = st.modalSel.Width(width).Render("▸ " + plain)
		} else {
			line = "  " + line
		}
		b.WriteString(truncateLine(line, width))
		if i < start+height-1 && i < len(p.commits)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// viewBranches renders the branch list, marking the current branch and showing
// any switch error.
func (p *logPanel) viewBranches(st styles, width, height int, focused bool) string {
	var head string
	if p.branchErr != "" {
		head = truncateLine(lipgloss.NewStyle().Foreground(colGitDeleted).Render("  "+p.branchErr), width) + "\n"
		height--
	}
	if len(p.branches) == 0 {
		return head + st.muted.Render("  no branches (or not a git repository)")
	}
	if height < 1 {
		height = 1
	}
	curStyle := lipgloss.NewStyle().Foreground(colGitNew).Bold(true)
	start := scrollStart(p.bcursor, len(p.branches), height)

	var b strings.Builder
	b.WriteString(head)
	for i := start; i < start+height && i < len(p.branches); i++ {
		br := p.branches[i]
		marker := "  "
		if br.Current {
			marker = "● "
		}
		name := br.Name
		if br.Current {
			name = curStyle.Render(br.Name + "  (current)")
		}
		line := marker + name
		if focused && i == p.bcursor {
			plainMarker := "  "
			if br.Current {
				plainMarker = "● "
			}
			suffix := ""
			if br.Current {
				suffix = "  (current)"
			}
			line = st.modalSel.Width(width).Render("▸ " + plainMarker + br.Name + suffix)
		}
		b.WriteString(truncateLine(line, width))
		if i < start+height-1 && i < len(p.branches)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

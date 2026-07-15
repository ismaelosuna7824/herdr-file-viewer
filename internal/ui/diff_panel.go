package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ismaelosuna7824/herdr-file-viewer/internal/gitdiff"
)

// diffPanel is the "review" view: a scrollable, syntax-neutral rendering of a
// file's diff against HEAD, with added lines in green, removed in red, and a
// two-column (old | new) line-number gutter.
type diffPanel struct {
	vp         viewport.Model
	diff       gitdiff.FileDiff
	loading    bool
	sideBySide bool // split old|new layout; auto-on for modified files
	width      int
	height     int

	hunkCursor   int   // index of the selected hunk (for hunk-level staging)
	hunkLineRows []int // rendered row index of each hunk header, current layout

	add     lipgloss.Style
	del     lipgloss.Style
	hunk    lipgloss.Style
	hunkSel lipgloss.Style
	gutter  lipgloss.Style
	ctx     lipgloss.Style
	fileHdr lipgloss.Style
}

// minSideBySideWidth is the narrowest pane where a split diff is legible; below
// it we always fall back to the inline layout.
const minSideBySideWidth = 90

func newDiffPanel() diffPanel {
	return diffPanel{
		vp:      viewport.New(0, 0),
		add:     lipgloss.NewStyle().Foreground(colGitNew),
		del:     lipgloss.NewStyle().Foreground(colGitDeleted),
		hunk:    lipgloss.NewStyle().Foreground(colAccent).Bold(true),
		hunkSel: lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(colAccent).Bold(true),
		gutter:  lipgloss.NewStyle().Foreground(colMuted),
		ctx:     lipgloss.NewStyle().Foreground(colText),
		fileHdr: lipgloss.NewStyle().Foreground(colDir).Bold(true).Underline(true),
	}
}

func (p *diffPanel) SetSize(w, h int) {
	p.width, p.height = w, h
	p.vp.Width = w
	p.vp.Height = h - 1 // reserve the title/stats bar
	if p.vp.Height < 0 {
		p.vp.Height = 0
	}
	p.render()
}

func (p *diffPanel) SetDiff(d gitdiff.FileDiff) {
	p.diff = d
	p.loading = false
	// A modified file (has removals) is far clearer side-by-side; a brand-new
	// file is all additions, so inline reads better. Auto-pick, user can toggle.
	p.sideBySide = d.Removed > 0
	// Keep the hunk cursor within the (possibly reduced) hunk set — e.g. after
	// staging a hunk the diff reloads with one fewer hunk.
	if n := p.hunkCount(); p.hunkCursor >= n {
		p.hunkCursor = n - 1
	}
	if p.hunkCursor < 0 {
		p.hunkCursor = 0
	}
	p.render()
	p.vp.GotoTop()
}

// hunkCount returns the number of hunks in the current diff.
func (p diffPanel) hunkCount() int {
	n := 0
	for _, ln := range p.diff.Lines {
		if ln.Kind == gitdiff.Hunk {
			n++
		}
	}
	return n
}

// currentHunk returns the 0-based index of the selected hunk.
func (p diffPanel) currentHunk() int { return p.hunkCursor }

// moveHunk advances the hunk cursor by delta (clamped) and scrolls it into view.
func (p *diffPanel) moveHunk(delta int) {
	n := p.hunkCount()
	if n == 0 {
		p.hunkCursor = 0
		return
	}
	p.hunkCursor += delta
	if p.hunkCursor < 0 {
		p.hunkCursor = 0
	}
	if p.hunkCursor >= n {
		p.hunkCursor = n - 1
	}
	p.render()
	p.scrollToHunk()
}

// scrollToHunk moves the viewport so the selected hunk header sits near the top.
func (p *diffPanel) scrollToHunk() {
	if p.hunkCursor < 0 || p.hunkCursor >= len(p.hunkLineRows) {
		return
	}
	target := p.hunkLineRows[p.hunkCursor] - 1
	if target < 0 {
		target = 0
	}
	p.vp.SetYOffset(target)
}

// toggleLayout flips between the inline and side-by-side diff layouts.
func (p *diffPanel) toggleLayout() {
	p.sideBySide = !p.sideBySide
	p.render()
}

// splitView reports whether the split layout is currently in effect (requested
// and the pane is wide enough).
func (p *diffPanel) splitView() bool {
	return p.sideBySide && p.width >= minSideBySideWidth
}

// beginLoad shows the loading placeholder for a file while its diff is fetched
// asynchronously, so the previous file's diff never lingers on screen.
func (p *diffPanel) beginLoad(rel string) {
	p.loading = true
	p.diff = gitdiff.FileDiff{Path: rel}
	p.hunkCursor = 0
	p.render()
}

// layoutName is the label shown in the stats bar for the current layout.
func (p diffPanel) layoutName() string {
	if p.splitView() {
		return "split"
	}
	return "inline"
}

func (p diffPanel) Update(msg tea.Msg) (diffPanel, tea.Cmd) {
	var cmd tea.Cmd
	p.vp, cmd = p.vp.Update(msg)
	return p, cmd
}

func (p diffPanel) View() string {
	stats := p.statsBar()
	return lipgloss.JoinVertical(lipgloss.Left, stats, p.vp.View())
}

// statsBar shows the file path, a "+added −removed" summary, and the layout.
func (p diffPanel) statsBar() string {
	summary := fmt.Sprintf("  %s %s  %s",
		p.add.Render(fmt.Sprintf("+%d", p.diff.Added)),
		p.del.Render(fmt.Sprintf("−%d", p.diff.Removed)),
		p.gutter.Render("["+p.layoutName()+"]"),
	)
	bar := lipgloss.NewStyle().Foreground(colAccent).Bold(true).
		Width(p.width).Render(truncateLine("  "+p.diff.Path, p.width-18) + summary)
	return bar
}

func (p *diffPanel) render() {
	p.hunkLineRows = p.hunkLineRows[:0]
	if p.loading {
		p.vp.SetContent(p.gutter.Render("\n  loading diff…"))
		return
	}
	if p.diff.Binary && len(p.diff.Lines) == 0 {
		p.vp.SetContent(p.gutter.Render("\n  (binary file — no textual diff)"))
		return
	}
	if p.diff.Empty || len(p.diff.Lines) == 0 {
		p.vp.SetContent(p.gutter.Render("\n  no changes in this review"))
		return
	}

	if p.splitView() {
		p.vp.SetContent(p.renderSideBySide())
		return
	}

	gw := p.gutterWidth()
	var rows []string
	hunkIdx := 0
	for _, ln := range p.diff.Lines {
		switch ln.Kind {
		case gitdiff.FileHeader:
			rows = append(rows, "")
			rows = append(rows, p.fileHdr.Render("▸ "+truncateLine(ln.Text, p.width-2)))
		case gitdiff.Hunk:
			p.hunkLineRows = append(p.hunkLineRows, len(rows))
			rows = append(rows, p.renderHunkHeader(ln.Text, hunkIdx == p.hunkCursor))
			hunkIdx++
		default:
			rows = append(rows, p.renderLine(ln, gw))
		}
	}
	p.vp.SetContent(strings.Join(rows, "\n"))
}

// renderHunkHeader draws a hunk's "@@ … @@" line, highlighting it when it is the
// staging cursor's current hunk so the user sees what space will stage.
func (p diffPanel) renderHunkHeader(text string, selected bool) string {
	if selected {
		return p.hunkSel.Render("▶ " + truncateLine(text, p.width-2))
	}
	return p.hunk.Render("  " + truncateLine(text, p.width-2))
}

func (p *diffPanel) renderLine(ln gitdiff.Line, gw int) string {
	text := strings.ReplaceAll(ln.Text, "\t", "    ")
	switch ln.Kind {
	case gitdiff.FileHeader:
		return "\n" + p.fileHdr.Render("▸ "+truncateLine(ln.Text, p.width-2))
	case gitdiff.Hunk:
		return p.hunk.Render(ln.Text)
	case gitdiff.Add:
		return p.gutter.Render(numCell("", ln.NewNum, gw)) + p.add.Render("+ "+text)
	case gitdiff.Del:
		return p.gutter.Render(numCell(numStr(ln.OldNum), "", gw)) + p.del.Render("- "+text)
	default: // Context
		return p.gutter.Render(numCell(ln.OldNum, ln.NewNum, gw)) + p.ctx.Render("  "+text)
	}
}

// gutterWidth returns the column width needed for the larger of the old/new
// line numbers present in the diff.
func (p *diffPanel) gutterWidth() int {
	max := 1
	for _, ln := range p.diff.Lines {
		if ln.OldNum > max {
			max = ln.OldNum
		}
		if ln.NewNum > max {
			max = ln.NewNum
		}
	}
	return len(fmt.Sprintf("%d", max))
}

// numCell formats the "old new " gutter, accepting either an int or "" (blank)
// for each side.
func numCell(old any, new any, w int) string {
	return fmt.Sprintf("%*s %*s ", w, cellStr(old), w, cellStr(new))
}

func cellStr(v any) string {
	switch n := v.(type) {
	case int:
		if n == 0 {
			return ""
		}
		return fmt.Sprintf("%d", n)
	case string:
		return n
	default:
		return ""
	}
}

func numStr(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%d", n)
}

// --- side-by-side layout ----------------------------------------------------

// sbsRow is one row of the split diff: an optional left (old) cell and an
// optional right (new) cell, or a full-width hunk header.
type sbsRow struct {
	hunk   string
	header string // per-file header in a multi-file (commit) diff
	lNum   int
	lText  string
	lDel   bool // left cell is a removed line (render red)
	rNum   int
	rText  string
	rAdd   bool // right cell is an added line (render green)
}

// pairRows turns the unified diff lines into aligned old|new rows: a run of
// removals is paired with the run of additions that follows it, so a changed
// line shows its before and after on the same row.
func pairRows(lines []gitdiff.Line) []sbsRow {
	var rows []sbsRow
	var dels, adds []gitdiff.Line

	flush := func() {
		n := len(dels)
		if len(adds) > n {
			n = len(adds)
		}
		for i := 0; i < n; i++ {
			var r sbsRow
			if i < len(dels) {
				r.lNum, r.lText, r.lDel = dels[i].OldNum, dels[i].Text, true
			}
			if i < len(adds) {
				r.rNum, r.rText, r.rAdd = adds[i].NewNum, adds[i].Text, true
			}
			rows = append(rows, r)
		}
		dels, adds = nil, nil
	}

	for _, ln := range lines {
		switch ln.Kind {
		case gitdiff.FileHeader:
			flush()
			rows = append(rows, sbsRow{header: ln.Text})
		case gitdiff.Hunk:
			flush()
			rows = append(rows, sbsRow{hunk: ln.Text})
		case gitdiff.Del:
			dels = append(dels, ln)
		case gitdiff.Add:
			adds = append(adds, ln)
		default: // Context appears identically on both sides
			flush()
			rows = append(rows, sbsRow{lNum: ln.OldNum, lText: ln.Text, rNum: ln.NewNum, rText: ln.Text})
		}
	}
	flush()
	return rows
}

func (p *diffPanel) renderSideBySide() string {
	rows := pairRows(p.diff.Lines)
	gw := p.gutterWidth()
	colW := (p.width - 3) / 2 // 3 cells for the " │ " divider
	if colW < 8 {
		colW = 8
	}
	sep := p.gutter.Render(" │ ")

	var out []string
	hunkIdx := 0
	for _, r := range rows {
		switch {
		case r.header != "":
			out = append(out, "")
			out = append(out, p.fileHdr.Render("▸ "+truncateLine(r.header, p.width-2)))
		case r.hunk != "":
			p.hunkLineRows = append(p.hunkLineRows, len(out))
			out = append(out, p.renderHunkHeader(r.hunk, hunkIdx == p.hunkCursor))
			hunkIdx++
		default:
			left := p.sideCell(r.lNum, r.lText, r.lDel, false, gw, colW)
			right := p.sideCell(r.rNum, r.rText, false, r.rAdd, gw, colW)
			out = append(out, left+sep+right)
		}
	}
	return strings.Join(out, "\n")
}

// sideCell renders one column of a split-diff row: a line-number gutter plus the
// text, tinted red for a removal or green for an addition, padded to colW.
func (p *diffPanel) sideCell(num int, text string, del, add bool, gw, colW int) string {
	if num == 0 && text == "" {
		return strings.Repeat(" ", colW) // blank counterpart
	}
	textStyle := p.ctx
	switch {
	case del:
		textStyle = p.del
	case add:
		textStyle = p.add
	}
	gutter := p.gutter.Render(fmt.Sprintf("%*s ", gw, numStr(num)))
	avail := colW - lipgloss.Width(gutter)
	if avail < 0 {
		avail = 0
	}
	body := textStyle.Render(truncateLine(strings.ReplaceAll(text, "\t", "    "), avail))
	return padRow(gutter+body, colW)
}

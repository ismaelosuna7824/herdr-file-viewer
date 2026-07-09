package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// helpEntry is one key/description row in the help card.
type helpEntry struct{ key, desc string }

type helpGroup struct {
	title   string
	entries []helpEntry
}

var helpGroups = []helpGroup{
	{"Browser", []helpEntry{
		{"↑ ↓ / k j", "move / preview"},
		{"→ / enter", "open / expand"},
		{"← / esc", "collapse / back"},
		{"space", "toggle dir"},
		{"e", "edit ($EDITOR)"},
		{"d", "review diff"},
		{"L", "locate file in tree"},
		{"o", "open location in OS"},
		{"m", "render markdown"},
		{"r", "refresh git"},
	}},
	{"Move between panels", []helpEntry{
		{"tab", "cycle panels"},
		{"alt+h/j/k/l", "move by direction"},
		{"alt+arrows", "move by direction"},
	}},
	{"Search panel", []helpEntry{
		{"^p", "focus · find files"},
		{"^f", "focus · search text"},
		{"↑ ↓", "move (previews)"},
		{"enter", "open in file view"},
		{"^t / ^w / ^r", "case / word / regex"},
		{"esc", "back to tree"},
	}},
	{"Review diff  (d)", []helpEntry{
		{"↑ ↓ pgup pgdn", "scroll"},
		{"s", "split ↔ inline"},
		{"d / esc / q", "back"},
	}},
	{"Git panel  (g)", []helpEntry{
		{"g", "focus · cycle 3 views"},
		{"↑ ↓", "move"},
		{"enter", "diff / switch / view commit"},
		{"esc", "back to tree"},
	}},
	{"Git · staging (changes)", []helpEntry{
		{"space", "stage / unstage file"},
		{"A / U", "stage all / unstage all"},
		{"c", "commit staged"},
		{"enter", "view file diff"},
	}},
	{"Git · local ops", []helpEntry{
		{"n", "new branch"},
		{"A", "stage all (add -A)"},
		{"c", "commit staged"},
		{"a", "amend last commit"},
		{"u", "undo last commit"},
		{"t", "tag at HEAD"},
		{"m", "merge → current"},
		{"r", "rebase → branch"},
		{"x", "delete branch"},
	}},
	{"Git · remote / stash / reset", []helpEntry{
		{"f", "fetch --all --prune"},
		{"p", "pull"},
		{"P", "push"},
		{"F", "force-push (lease)"},
		{"s / S", "stash / pop"},
		{"H", "reset --hard HEAD"},
		{"y", "cherry-pick (history)"},
		{"R", "reset --hard→commit"},
	}},
	{"Global", []helpEntry{
		{"?", "this help"},
		{"q / ^c", "quit"},
	}},
}

// helpContent renders the keybinding reference in two columns so the whole
// reference fits on screen without being clipped by short panes.
func helpContent(st styles) string {
	keyStyle := lipgloss.NewStyle().Foreground(colAccent).Bold(true).Width(15)

	renderColumn := func(groups []helpGroup) string {
		var lines []string
		for gi, g := range groups {
			if gi > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, st.dir.Render(g.title))
			for _, e := range g.entries {
				lines = append(lines, "  "+keyStyle.Render(e.key)+st.muted.Render(e.desc))
			}
		}
		return strings.Join(lines, "\n")
	}

	// Split the groups into two balanced columns.
	split := 5
	left := renderColumn(helpGroups[:split])
	right := renderColumn(helpGroups[split:])
	gap := lipgloss.NewStyle().Width(4).Render("")
	columns := lipgloss.JoinHorizontal(lipgloss.Top, left, gap, right)

	return st.modalTitle.Render("Keybindings") + "\n\n" + columns
}

// helpWidth is the rendered width of the help content, used to size the card.
func helpWidth(st styles) int {
	return lipgloss.Width(helpContent(st))
}

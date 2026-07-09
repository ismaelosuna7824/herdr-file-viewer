package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/ismaelosuna7824/herdr-file-viewer/internal/gitstatus"
)

// Palette. A small, calm set of colors that reads well on dark terminals and
// keeps the UI feeling like one coherent system rather than a rainbow.
var (
	colAccent   = lipgloss.Color("111") // soft blue — focus, selections
	colMuted    = lipgloss.Color("240") // dim gray — gutters, hints
	colText     = lipgloss.Color("252") // primary text
	colDir      = lipgloss.Color("110") // directories
	colMatch    = lipgloss.Color("215") // matched substring highlight
	colToggleOn = lipgloss.Color("114") // an enabled search toggle
	colBg       = lipgloss.Color("236") // selected-row background
	colBorder   = lipgloss.Color("238")

	// Git status colors, mirroring editor source-control decorations.
	colGitModified = lipgloss.Color("179") // amber
	colGitNew      = lipgloss.Color("114") // green
	colGitDeleted  = lipgloss.Color("203") // red
	colGitRenamed  = lipgloss.Color("110") // blue
	colGitConflict = lipgloss.Color("211") // pink/red
	colGitDirtyDir = lipgloss.Color("179") // dirty directory name tint
)

// gitDecor returns the color and single-letter badge for a git status code.
// The bool is false when there is nothing to decorate.
func gitDecor(code gitstatus.Code) (lipgloss.Color, string, bool) {
	switch code {
	case gitstatus.Modified:
		return colGitModified, code.Letter(), true
	case gitstatus.Untracked, gitstatus.Added:
		return colGitNew, code.Letter(), true
	case gitstatus.Deleted:
		return colGitDeleted, code.Letter(), true
	case gitstatus.Renamed:
		return colGitRenamed, code.Letter(), true
	case gitstatus.Conflicted:
		return colGitConflict, code.Letter(), true
	default:
		return colText, " ", false
	}
}

type styles struct {
	header      lipgloss.Style
	footer      lipgloss.Style
	paneFocused lipgloss.Style
	paneBlurred lipgloss.Style
	rowSelected lipgloss.Style
	dir         lipgloss.Style
	file        lipgloss.Style
	muted       lipgloss.Style
	match       lipgloss.Style
	toggleOn    lipgloss.Style
	toggleOff   lipgloss.Style
	prompt      lipgloss.Style
	countBadge  lipgloss.Style
	modalTitle  lipgloss.Style
	modalSel    lipgloss.Style
	divider     lipgloss.Style
}

func newStyles() styles {
	return styles{
		header: lipgloss.NewStyle().
			Foreground(lipgloss.Color("231")).Background(colAccent).Bold(true).Padding(0, 1),
		footer: lipgloss.NewStyle().Foreground(colMuted),
		paneFocused: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).BorderForeground(colAccent),
		paneBlurred: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).BorderForeground(colBorder),
		rowSelected: lipgloss.NewStyle().Background(colBg).Foreground(lipgloss.Color("231")),
		dir:         lipgloss.NewStyle().Foreground(colDir).Bold(true),
		file:        lipgloss.NewStyle().Foreground(colText),
		muted:       lipgloss.NewStyle().Foreground(colMuted),
		match:       lipgloss.NewStyle().Foreground(colMatch).Bold(true),
		toggleOn:    lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(colToggleOn).Padding(0, 1).Bold(true),
		toggleOff:   lipgloss.NewStyle().Foreground(colMuted).Background(lipgloss.Color("236")).Padding(0, 1),
		prompt:      lipgloss.NewStyle().Foreground(colAccent).Bold(true),
		countBadge:  lipgloss.NewStyle().Foreground(colMuted),
		modalTitle:  lipgloss.NewStyle().Foreground(colAccent).Bold(true),
		modalSel:    lipgloss.NewStyle().Background(lipgloss.Color("24")).Foreground(lipgloss.Color("231")),
		divider:     lipgloss.NewStyle().Foreground(colBorder),
	}
}

// modalBox is the floating-card style for the finder/search overlays: a rounded
// accent border with comfortable padding, at a fixed width.
func modalBox(width int) lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colAccent).
		Padding(1, 2).
		Width(width)
}

// centerModal places a rendered box in the middle of a w×h region.
func centerModal(w, h int, box string) string {
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
}

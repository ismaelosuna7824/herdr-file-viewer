package herdr

import (
	"errors"
	"fmt"
	"os/exec"
)

// PaneCWD returns the live foreground directory for a pane. Herdr's cwd field
// can briefly retain the pane's launch directory after a shell `cd`, so the
// foreground value wins whenever it points to a valid directory.
func PaneCWD(workspaceID, paneID string) (string, error) {
	if workspaceID == "" {
		return "", errors.New("Herdr workspace ID is unavailable")
	}
	if paneID == "" {
		return "", errors.New("Herdr pane ID is unavailable")
	}

	out, err := exec.Command(herdrBin(), "pane", "list", "--workspace", workspaceID).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("list Herdr panes for cwd refresh: %w: %s", err, out)
	}
	panes, err := parsePaneList(out)
	if err != nil {
		return "", fmt.Errorf("decode Herdr panes for cwd refresh: %w", err)
	}
	for _, pane := range panes {
		if pane.WorkspaceID != workspaceID || pane.PaneID != paneID {
			continue
		}
		for _, candidate := range []string{pane.ForegroundCwd, pane.Cwd} {
			if root := validRoot(candidate); root != "" {
				return root, nil
			}
		}
		return "", fmt.Errorf("followed Herdr pane %s has no valid directory", paneID)
	}
	return "", fmt.Errorf("followed Herdr pane %s not found", paneID)
}

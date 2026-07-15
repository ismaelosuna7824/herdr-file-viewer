package herdr

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ismaelosuna7824/herdr-file-viewer/internal/gitdiff"
)

// RestoreFocusedTab rehydrates plugin commands inside Herdr's persisted pane
// layout. A full Herdr server restart keeps tabs, panes, cwd, labels, layout and
// focus, but intentionally restores each pane as a fresh shell. Reusing those
// shells preserves the user's exact tab IDs and split geometry.
func RestoreFocusedTab(workspaceID, tabID, projectRoot string) error {
	if workspaceID == "" {
		return errors.New("Herdr workspace ID is unavailable")
	}
	if tabID == "" {
		return errors.New("Herdr tab ID is unavailable")
	}

	return withWorkspaceTreeLock(func() error {
		stateDir := os.Getenv("HERDR_PLUGIN_STATE_DIR")
		restore := func(state *fileTabState) error {
			return restoreFocusedTab(herdrBin(), workspaceID, tabID, projectRoot, state)
		}
		if stateDir == "" {
			state := newFileTabState()
			return restore(&state)
		}
		return withFileTabState(stateDir, restore)
	})
}

func restoreFocusedTab(bin, workspaceID, tabID, projectRoot string, state *fileTabState) error {
	out, err := exec.Command(bin, "pane", "list", "--workspace", workspaceID).CombinedOutput()
	if err != nil {
		return fmt.Errorf("list restored Herdr panes: %w: %s", err, out)
	}
	panes, err := parsePaneList(out)
	if err != nil {
		return fmt.Errorf("decode restored Herdr panes: %w", err)
	}

	tabPanes := make([]paneContext, 0, len(panes))
	for _, pane := range panes {
		if pane.WorkspaceID == workspaceID && pane.TabID == tabID {
			tabPanes = append(tabPanes, pane)
		}
	}
	if len(tabPanes) == 0 {
		return errors.New("focused Herdr tab has no panes to restore")
	}

	root := validRoot(state.root(workspaceID))
	if root == "" {
		root = rootFromExistingTreePanes(panes)
	}
	if root == "" {
		root = validRoot(projectRoot)
	}
	if root == "" {
		root = rootFromPanes(tabPanes)
	}
	if root == "" {
		return errors.New("project root is unavailable during Herdr tab restore")
	}
	state.setRoot(workspaceID, root)

	path := state.pathForTab(workspaceID, tabID)
	diffPath, diffMode := state.diffForTab(workspaceID, tabID)
	var filePane, diffPane, treePane paneContext
	for _, pane := range tabPanes {
		switch pane.Label {
		case "File":
			if filePane.PaneID == "" {
				filePane = pane
			}
		case "Diff":
			if diffPane.PaneID == "" {
				diffPane = pane
			}
		case "File Tree":
			if treePane.PaneID == "" {
				treePane = pane
			}
		}
	}
	if path == "" && filePane.PaneID != "" {
		path = recoverFilePath(bin, tabID, filePane)
		if path != "" {
			state.set(workspaceID, path, tabID)
		}
	}
	target := filePane
	if target.PaneID == "" {
		target = diffPane
	}
	if target.PaneID == "" {
		for _, pane := range tabPanes {
			if pane.Label != "File Tree" {
				target = pane
				break
			}
		}
	}
	followPaneID := ""
	if filePane.PaneID == "" && diffPane.PaneID == "" {
		followPaneID = target.PaneID
	}

	if path != "" && filePane.PaneID != "" {
		if err := rerunRestoredPane(bin, filePane, restoredPaneCommand("file", workspaceID, tabID, filePane.PaneID, root, path, "", "")); err != nil {
			return err
		}
	}
	if diffPath != "" && diffMode.Valid() && diffPane.PaneID != "" {
		if err := rerunRestoredPane(bin, diffPane, restoredPaneCommand("diff", workspaceID, tabID, diffPane.PaneID, root, diffPath, diffMode, "")); err != nil {
			return err
		}
	}
	if treePane.PaneID != "" {
		return rerunRestoredPane(bin, treePane, restoredPaneCommand("viewer", workspaceID, tabID, treePane.PaneID, root, "", "", followPaneID))
	}

	if target.PaneID == "" {
		return errors.New("focused Herdr tab has no pane available for the file tree")
	}
	args := openTreeArgs(target.PaneID, root)
	if followPaneID != "" {
		args = openFollowingTreeArgs(target.PaneID, root)
	}
	out, err = exec.Command(bin, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("restore default Herdr file tree: %w: %s", err, out)
	}
	return nil
}

func herdrBin() string {
	if bin := os.Getenv("HERDR_BIN_PATH"); bin != "" {
		return bin
	}
	return "herdr"
}

func validRoot(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return ""
	}
	return abs
}

func rootFromExistingTreePanes(panes []paneContext) string {
	for _, pane := range panes {
		if pane.Label == "File Tree" {
			if root := validRoot(pane.Cwd); root != "" {
				return root
			}
		}
	}
	return ""
}

func rootFromPanes(panes []paneContext) string {
	for _, pane := range panes {
		if root := validRoot(pane.Cwd); root != "" {
			return root
		}
	}
	return ""
}

func recoverFilePath(bin, tabID string, filePane paneContext) string {
	out, err := exec.Command(bin, "tab", "get", tabID).CombinedOutput()
	if err != nil {
		return ""
	}
	tab, err := parseTabContext(out)
	if err != nil || tab.Label == "" || filepath.Base(tab.Label) != tab.Label {
		return ""
	}
	candidate := filepath.Join(filePane.Cwd, tab.Label)
	info, err := os.Stat(candidate)
	if err != nil || info.IsDir() {
		return ""
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return ""
	}
	return abs
}

type restoredProcessInfo struct {
	ShellPID                 int `json:"shell_pid"`
	ForegroundProcessGroupID int `json:"foreground_process_group_id"`
	ForegroundProcesses      []struct {
		Name string   `json:"name"`
		Argv []string `json:"argv"`
	} `json:"foreground_processes"`
}

func rerunRestoredPane(bin string, pane paneContext, command string) error {
	out, err := exec.Command(bin, "pane", "process-info", "--pane", pane.PaneID).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect restored Herdr pane %s: %w: %s", pane.PaneID, err, out)
	}
	var response struct {
		Result struct {
			ProcessInfo restoredProcessInfo `json:"process_info"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &response); err != nil {
		return fmt.Errorf("decode restored Herdr pane %s: %w", pane.PaneID, err)
	}
	process := response.Result.ProcessInfo
	for _, foreground := range process.ForegroundProcesses {
		if foreground.Name == "file-viewer" {
			return nil
		}
		for _, arg := range foreground.Argv {
			if filepath.Base(arg) == "file-viewer" {
				return nil
			}
		}
	}
	// Never replace a user command that happens to be running in a restored
	// pane. A pristine restore shell is the only safe pane to rehydrate.
	if process.ShellPID == 0 || process.ForegroundProcessGroupID != process.ShellPID {
		return nil
	}
	out, err = exec.Command(bin, "pane", "run", pane.PaneID, command).CombinedOutput()
	if err != nil {
		return fmt.Errorf("rehydrate Herdr pane %s: %w: %s", pane.PaneID, err, out)
	}
	return nil
}

func restoredPaneCommand(entrypoint, workspaceID, tabID, paneID, root, path string, mode gitdiff.Mode, followPaneID string) string {
	pluginRoot := os.Getenv("HERDR_PLUGIN_ROOT")
	binary := filepath.Join(pluginRoot, "bin", "file-viewer")
	env := []string{
		"HERDR_BIN_PATH=" + herdrBin(),
		"HERDR_PLUGIN_ROOT=" + pluginRoot,
		"HERDR_PLUGIN_STATE_DIR=" + os.Getenv("HERDR_PLUGIN_STATE_DIR"),
		"HERDR_SOCKET_PATH=" + os.Getenv("HERDR_SOCKET_PATH"),
		"HERDR_WORKSPACE_ID=" + workspaceID,
		"HERDR_TAB_ID=" + tabID,
		"HERDR_PANE_ID=" + paneID,
		"HERDR_PLUGIN_ID=" + pluginID,
		"HERDR_PLUGIN_ENTRYPOINT_ID=" + entrypoint,
		"HERDR_TREE_ROOT=" + root,
	}
	if path != "" && entrypoint == "file" {
		env = append(env, "HERDR_FILE_PATH="+path)
	}
	if path != "" && entrypoint == "diff" {
		env = append(env,
			"HERDR_DIFF_PATH="+path,
			"HERDR_DIFF_ROOT="+root,
			"HERDR_DIFF_MODE="+string(mode),
		)
	}
	if followPaneID != "" {
		env = append(env, "HERDR_TREE_FOLLOW_PANE_ID="+followPaneID)
	}
	if contextJSON := os.Getenv("HERDR_PLUGIN_CONTEXT_JSON"); contextJSON != "" {
		env = append(env, "HERDR_PLUGIN_CONTEXT_JSON="+contextJSON)
	}
	parts := []string{"exec", "env"}
	parts = append(parts, env...)
	parts = append(parts, binary)
	if entrypoint == "viewer" {
		parts = append(parts, "--tree-only")
	}
	for i, part := range parts {
		parts[i] = shellQuote(part)
	}
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') &&
			!strings.ContainsRune("_@%+=:,./-", r)
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// Package herdr contains the small Herdr CLI bridge used by the file explorer.
package herdr

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ismaelosuna7824/herdr-file-viewer/internal/gitdiff"
)

const (
	pluginID         = "ismaelosuna.file-viewer"
	fileTabStateFile = "file-tabs.json"
	fileTabLockFile  = ".file-tabs.lock"
)

type openedPane struct {
	PaneID string
	TabID  string
}

type fileTabState struct {
	Version int                          `json:"version"`
	Tabs    map[string]map[string]string `json:"tabs"`
	Diffs   map[string]map[string]string `json:"diffs,omitempty"`
	Roots   map[string]string            `json:"roots,omitempty"`
}

var fileTabStateMu sync.Mutex

// OpenFileTab focuses the existing tab for path in workspaceID, or opens a new
// read-only file tab and attaches a tree split to its right. State is guarded
// by a process-wide lock everywhere and an advisory cross-process lock on the
// macOS/Linux platforms supported by the plugin, so clicks from multiple tree
// panes cannot create duplicate tabs for the same absolute path.
func OpenFileTab(workspaceID, path, projectRoot string) error {
	if workspaceID == "" {
		return errors.New("Herdr workspace ID is unavailable")
	}
	if path == "" {
		return errors.New("file path is empty")
	}
	if projectRoot == "" {
		return errors.New("project root is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return err
	}

	bin := os.Getenv("HERDR_BIN_PATH")
	if bin == "" {
		bin = "herdr"
	}
	stateDir := os.Getenv("HERDR_PLUGIN_STATE_DIR")
	var openedTabID string
	openOrReuse := func(state *fileTabState) error {
		if state != nil {
			state.setRoot(workspaceID, root)
			if tabID := state.tabID(workspaceID, abs); tabID != "" && tabIsReusable(bin, tabID, workspaceID, abs) {
				return nil
			}
		}
		var openErr error
		openedTabID, openErr = openNewFileTab(bin, workspaceID, abs, root, state)
		return openErr
	}
	if stateDir == "" {
		err = openOrReuse(nil)
	} else {
		err = withFileTabState(stateDir, openOrReuse)
	}
	// Cleanup stays outside withFileTabState: a successful pane open followed
	// by an atomic state-save failure must roll the new tab back too.
	if err != nil && openedTabID != "" {
		closeTabBestEffort(bin, openedTabID)
	}
	return err
}

// OpenDiffTab focuses the existing review for the same path and Git boundary,
// or opens a new diff tab with a project-root tree attached on the right.
func OpenDiffTab(workspaceID, path, projectRoot string, mode gitdiff.Mode) error {
	if workspaceID == "" {
		return errors.New("Herdr workspace ID is unavailable")
	}
	if path == "" {
		return errors.New("diff path is empty")
	}
	if projectRoot == "" {
		return errors.New("project root is empty")
	}
	if !mode.Valid() || mode == gitdiff.ModeHead {
		return fmt.Errorf("unsupported Source Control diff mode: %q", mode)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return err
	}

	bin := herdrBin()
	stateDir := os.Getenv("HERDR_PLUGIN_STATE_DIR")
	var openedTabID string
	openOrReuse := func(state *fileTabState) error {
		if state != nil {
			state.setRoot(workspaceID, root)
			if tabID := state.diffTabID(workspaceID, abs, mode); tabID != "" &&
				diffTabIsReusable(bin, tabID, workspaceID, abs, mode) {
				return nil
			}
		}
		var openErr error
		openedTabID, openErr = openNewDiffTab(bin, workspaceID, abs, root, mode, state)
		return openErr
	}
	if stateDir == "" {
		err = openOrReuse(nil)
	} else {
		err = withFileTabState(stateDir, openOrReuse)
	}
	if err != nil && openedTabID != "" {
		closeTabBestEffort(bin, openedTabID)
	}
	return err
}

func openNewFileTab(bin, workspaceID, path, projectRoot string, state *fileTabState) (string, error) {
	out, err := exec.Command(bin, openFileTabArgs(workspaceID, path)...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("open Herdr file tab: %w: %s", err, out)
	}
	opened, err := parseOpenedPane(out)
	if err != nil {
		return "", err
	}
	if out, err = exec.Command(bin, "tab", "rename", opened.TabID, filepath.Base(path)).CombinedOutput(); err != nil {
		return opened.TabID, fmt.Errorf("rename Herdr file tab: %w: %s", err, out)
	}
	// A file tab is a single read-only file pane. No tree is attached: the
	// tab.focused restore hook that re-attached one on every focus is gone, so
	// attaching here would only reintroduce runaway pane accumulation.
	if state != nil {
		state.set(workspaceID, path, opened.TabID)
	}
	return opened.TabID, nil
}

func openNewDiffTab(bin, workspaceID, path, projectRoot string, mode gitdiff.Mode, state *fileTabState) (string, error) {
	out, err := exec.Command(bin, openDiffTabArgs(workspaceID, path, projectRoot, mode)...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("open Herdr diff tab: %w: %s", err, out)
	}
	opened, err := parseOpenedPane(out)
	if err != nil {
		return "", err
	}
	if out, err = exec.Command(bin, "tab", "rename", opened.TabID, diffTabLabel(path, mode)).CombinedOutput(); err != nil {
		return opened.TabID, fmt.Errorf("rename Herdr diff tab: %w: %s", err, out)
	}
	// A diff tab is a single read-only review pane, mirroring file tabs: no tree
	// is attached (that behavior was removed in slice 1 to avoid runaway panes).
	if state != nil {
		state.setDiff(workspaceID, path, mode, opened.TabID)
	}
	return opened.TabID, nil
}

func closeTabBestEffort(bin, tabID string) {
	_ = exec.Command(bin, "tab", "close", tabID).Run()
}

func tabIsReusable(bin, tabID, workspaceID, path string) bool {
	out, err := exec.Command(bin, "tab", "get", tabID).CombinedOutput()
	if err != nil {
		return false
	}
	got, err := parseTabContext(out)
	// A file tab is now a single pane; require at least one pane.
	if err != nil || got.TabID != tabID || got.WorkspaceID != workspaceID ||
		got.Label != filepath.Base(path) || got.PaneCount < 1 {
		return false
	}
	if !tabHasOwnedFilePane(bin, workspaceID, tabID, path) {
		return false
	}
	return exec.Command(bin, "tab", "focus", tabID).Run() == nil
}

func diffTabIsReusable(bin, tabID, workspaceID, path string, mode gitdiff.Mode) bool {
	out, err := exec.Command(bin, "tab", "get", tabID).CombinedOutput()
	if err != nil {
		return false
	}
	got, err := parseTabContext(out)
	// A diff tab is now a single pane; require at least one pane.
	if err != nil || got.TabID != tabID || got.WorkspaceID != workspaceID ||
		got.Label != diffTabLabel(path, mode) || got.PaneCount < 1 {
		return false
	}
	if !tabHasOwnedPane(bin, workspaceID, tabID, path, "Diff") {
		return false
	}
	return exec.Command(bin, "tab", "focus", tabID).Run() == nil
}

func diffTabLabel(path string, mode gitdiff.Mode) string {
	prefix := "M"
	if mode == gitdiff.ModeStaged {
		prefix = "S"
	} else if mode == gitdiff.ModeUntracked {
		prefix = "U"
	}
	return prefix + " " + filepath.Base(path)
}

func openFileTabArgs(workspaceID, path string) []string {
	return []string{
		"plugin", "pane", "open",
		"--plugin", pluginID,
		"--entrypoint", "file",
		"--placement", "tab",
		"--workspace", workspaceID,
		"--cwd", filepath.Dir(path),
		"--env", "HERDR_FILE_PATH=" + path,
		"--focus",
	}
}

func openDiffTabArgs(workspaceID, path, projectRoot string, mode gitdiff.Mode) []string {
	return []string{
		"plugin", "pane", "open",
		"--plugin", pluginID,
		"--entrypoint", "diff",
		"--placement", "tab",
		"--workspace", workspaceID,
		"--cwd", filepath.Dir(path),
		"--env", "HERDR_DIFF_PATH=" + path,
		"--env", "HERDR_DIFF_ROOT=" + projectRoot,
		"--env", "HERDR_DIFF_MODE=" + string(mode),
		"--focus",
	}
}

func openTreeArgs(targetPaneID, projectRoot string) []string {
	return treeOpenArgs(targetPaneID, projectRoot, "", "")
}

func openTreeArgsForFileTab(targetPaneID, projectRoot, sourceTabID string) []string {
	return treeOpenArgs(targetPaneID, projectRoot, "", sourceTabID)
}

func openFollowingTreeArgs(targetPaneID, projectRoot string) []string {
	return treeOpenArgs(targetPaneID, projectRoot, targetPaneID, "")
}

func treeOpenArgs(targetPaneID, projectRoot, followPaneID, sourceTabID string) []string {
	args := []string{
		"plugin", "pane", "open",
		"--plugin", pluginID,
		"--entrypoint", "viewer",
		"--placement", "split",
		"--target-pane", targetPaneID,
		"--env", "HERDR_TREE_ROOT=" + projectRoot,
	}
	if followPaneID != "" {
		args = append(args, "--env", "HERDR_TREE_FOLLOW_PANE_ID="+followPaneID)
	}
	if sourceTabID != "" {
		args = append(args, "--env", "HERDR_TREE_STATE_SOURCE_TAB_ID="+sourceTabID)
	}
	return append(args,
		"--direction", "right",
		"--no-focus",
	)
}

func parseOpenedPane(raw []byte) (openedPane, error) {
	var response struct {
		Result struct {
			PluginPane struct {
				Pane struct {
					PaneID string `json:"pane_id"`
					TabID  string `json:"tab_id"`
				} `json:"pane"`
			} `json:"plugin_pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return openedPane{}, fmt.Errorf("decode Herdr pane response: %w", err)
	}
	opened := openedPane{
		PaneID: response.Result.PluginPane.Pane.PaneID,
		TabID:  response.Result.PluginPane.Pane.TabID,
	}
	if opened.TabID == "" {
		return openedPane{}, errors.New("Herdr pane response did not include a tab ID")
	}
	return opened, nil
}

func parseOpenedTabID(raw []byte) (string, error) {
	opened, err := parseOpenedPane(raw)
	return opened.TabID, err
}

type tabContext struct {
	TabID       string
	WorkspaceID string
	Label       string
	PaneCount   int
}

func parseTabContext(raw []byte) (tabContext, error) {
	var response struct {
		Result struct {
			Tab struct {
				TabID       string `json:"tab_id"`
				WorkspaceID string `json:"workspace_id"`
				Label       string `json:"label"`
				PaneCount   int    `json:"pane_count"`
			} `json:"tab"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return tabContext{}, err
	}
	got := tabContext{
		TabID:       response.Result.Tab.TabID,
		WorkspaceID: response.Result.Tab.WorkspaceID,
		Label:       response.Result.Tab.Label,
		PaneCount:   response.Result.Tab.PaneCount,
	}
	if got.TabID == "" || got.WorkspaceID == "" {
		return tabContext{}, errors.New("Herdr tab response is incomplete")
	}
	return got, nil
}

type paneContext struct {
	PaneID        string
	TabID         string
	WorkspaceID   string
	Label         string
	Cwd           string
	ForegroundCwd string
}

func tabHasOwnedFilePane(bin, workspaceID, tabID, path string) bool {
	return tabHasOwnedPane(bin, workspaceID, tabID, path, "File")
}

func tabHasOwnedPane(bin, workspaceID, tabID, path, label string) bool {
	out, err := exec.Command(bin, "pane", "list", "--workspace", workspaceID).CombinedOutput()
	if err != nil {
		return false
	}
	panes, err := parsePaneList(out)
	if err != nil {
		return false
	}
	wantDir := filepath.Clean(filepath.Dir(path))
	for _, pane := range panes {
		if pane.TabID == tabID && pane.WorkspaceID == workspaceID && pane.Label == label &&
			filepath.Clean(pane.Cwd) == wantDir {
			return true
		}
	}
	return false
}

func parsePaneList(raw []byte) ([]paneContext, error) {
	var response struct {
		Result struct {
			Panes []struct {
				PaneID        string `json:"pane_id"`
				TabID         string `json:"tab_id"`
				WorkspaceID   string `json:"workspace_id"`
				Label         string `json:"label"`
				Cwd           string `json:"cwd"`
				ForegroundCwd string `json:"foreground_cwd"`
			} `json:"panes"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, err
	}
	panes := make([]paneContext, 0, len(response.Result.Panes))
	for _, pane := range response.Result.Panes {
		panes = append(panes, paneContext{
			PaneID:        pane.PaneID,
			TabID:         pane.TabID,
			WorkspaceID:   pane.WorkspaceID,
			Label:         pane.Label,
			Cwd:           pane.Cwd,
			ForegroundCwd: pane.ForegroundCwd,
		})
	}
	return panes, nil
}

func newFileTabState() fileTabState {
	return fileTabState{
		Version: 1,
		Tabs:    make(map[string]map[string]string),
		Diffs:   make(map[string]map[string]string),
		Roots:   make(map[string]string),
	}
}

func (s *fileTabState) tabID(workspaceID, path string) string {
	if s.Tabs == nil || s.Tabs[workspaceID] == nil {
		return ""
	}
	return s.Tabs[workspaceID][path]
}

func (s *fileTabState) set(workspaceID, path, tabID string) {
	if s.Tabs == nil {
		s.Tabs = make(map[string]map[string]string)
	}
	if s.Tabs[workspaceID] == nil {
		s.Tabs[workspaceID] = make(map[string]string)
	}
	s.Version = 1
	s.Tabs[workspaceID][path] = tabID
}

func (s *fileTabState) pathForTab(workspaceID, tabID string) string {
	for path, savedTabID := range s.Tabs[workspaceID] {
		if savedTabID == tabID {
			return path
		}
	}
	return ""
}

func diffStateKey(path string, mode gitdiff.Mode) string {
	return string(mode) + ":" + path
}

func (s *fileTabState) diffTabID(workspaceID, path string, mode gitdiff.Mode) string {
	if s.Diffs == nil || s.Diffs[workspaceID] == nil {
		return ""
	}
	return s.Diffs[workspaceID][diffStateKey(path, mode)]
}

func (s *fileTabState) setDiff(workspaceID, path string, mode gitdiff.Mode, tabID string) {
	if s.Diffs == nil {
		s.Diffs = make(map[string]map[string]string)
	}
	if s.Diffs[workspaceID] == nil {
		s.Diffs[workspaceID] = make(map[string]string)
	}
	s.Version = 1
	s.Diffs[workspaceID][diffStateKey(path, mode)] = tabID
}

func (s *fileTabState) diffForTab(workspaceID, tabID string) (string, gitdiff.Mode) {
	for key, savedTabID := range s.Diffs[workspaceID] {
		if savedTabID != tabID {
			continue
		}
		modeText, path, ok := strings.Cut(key, ":")
		if !ok {
			return "", ""
		}
		mode := gitdiff.Mode(modeText)
		if !mode.Valid() {
			return "", ""
		}
		return path, mode
	}
	return "", ""
}

func (s *fileTabState) setRoot(workspaceID, root string) {
	if s.Roots == nil {
		s.Roots = make(map[string]string)
	}
	s.Version = 1
	s.Roots[workspaceID] = root
}

func (s *fileTabState) root(workspaceID string) string {
	if s.Roots == nil {
		return ""
	}
	return s.Roots[workspaceID]
}

func withFileTabState(dir string, fn func(*fileTabState) error) error {
	fileTabStateMu.Lock()
	defer fileTabStateMu.Unlock()

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create Herdr file-tab state directory: %w", err)
	}
	lock, err := os.OpenFile(filepath.Join(dir, fileTabLockFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open Herdr file-tab state lock: %w", err)
	}
	defer lock.Close()
	if err := lockFileExclusive(lock); err != nil {
		return fmt.Errorf("lock Herdr file-tab state: %w", err)
	}
	defer unlockFile(lock) //nolint:errcheck

	state, err := loadFileTabState(filepath.Join(dir, fileTabStateFile))
	if err != nil {
		// Corrupt or partially-written state is only a stale cache. Rebuild it
		// from successful opens instead of blocking file navigation.
		state = newFileTabState()
	}
	if err := fn(&state); err != nil {
		return err
	}
	if err := saveFileTabState(filepath.Join(dir, fileTabStateFile), state); err != nil {
		return fmt.Errorf("save Herdr file-tab state: %w", err)
	}
	return nil
}

func loadFileTabState(path string) (fileTabState, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return newFileTabState(), nil
	}
	if err != nil {
		return fileTabState{}, err
	}
	state := newFileTabState()
	if err := json.Unmarshal(raw, &state); err != nil {
		return fileTabState{}, err
	}
	if state.Version != 1 || state.Tabs == nil {
		return fileTabState{}, errors.New("unsupported Herdr file-tab state")
	}
	if state.Roots == nil {
		state.Roots = make(map[string]string)
	}
	if state.Diffs == nil {
		state.Diffs = make(map[string]map[string]string)
	}
	return state, nil
}

func saveFileTabState(path string, state fileTabState) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".file-tabs-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	enc := json.NewEncoder(tmp)
	if err := enc.Encode(state); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

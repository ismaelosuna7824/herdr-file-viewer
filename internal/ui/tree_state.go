package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/ismaelosuna7824/herdr-file-viewer/internal/explorer"
)

type persistedTreeState struct {
	Version  int      `json:"version"`
	Root     string   `json:"root"`
	Expanded []string `json:"expanded"`
	Selected string   `json:"selected"`
}

func treeStatePathFromEnv() string {
	dir := os.Getenv("HERDR_PLUGIN_STATE_DIR")
	workspaceID := os.Getenv("HERDR_WORKSPACE_ID")
	tabID := os.Getenv("HERDR_TAB_ID")
	if dir == "" || workspaceID == "" || tabID == "" {
		return ""
	}
	return treeStatePath(dir, workspaceID, tabID)
}

func treeStatePath(dir, workspaceID, tabID string) string {
	sum := sha256.Sum256([]byte(workspaceID + "\x00" + tabID))
	return filepath.Join(dir, "tree-state-"+hex.EncodeToString(sum[:])+".json")
}

func (m *Model) restoreTreeState() {
	if m.treeStatePath == "" {
		return
	}
	if m.restoreTreeStateFrom(m.treeStatePath) {
		return
	}
	dir := os.Getenv("HERDR_PLUGIN_STATE_DIR")
	workspaceID := os.Getenv("HERDR_WORKSPACE_ID")
	sourceTabID := os.Getenv("HERDR_TREE_STATE_SOURCE_TAB_ID")
	if dir == "" || workspaceID == "" || sourceTabID == "" || sourceTabID == os.Getenv("HERDR_TAB_ID") {
		return
	}
	if m.restoreTreeStateFrom(treeStatePath(dir, workspaceID, sourceTabID)) {
		m.persistTreeState()
	}
}

func (m *Model) restoreTreeStateFrom(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var saved persistedTreeState
	if json.Unmarshal(raw, &saved) != nil || saved.Version != 1 || filepath.Clean(saved.Root) != m.root {
		return false
	}
	m.tree.Restore(explorer.State{Expanded: saved.Expanded, Selected: saved.Selected})
	return true
}

func (m *Model) persistTreeState() {
	if m.treeStatePath == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(m.treeStatePath), 0o700); err != nil {
		m.statusNote = "tree state failed: " + err.Error()
		return
	}
	snapshot := m.tree.Snapshot()
	saved := persistedTreeState{
		Version:  1,
		Root:     m.root,
		Expanded: snapshot.Expanded,
		Selected: snapshot.Selected,
	}
	tmp, err := os.CreateTemp(filepath.Dir(m.treeStatePath), ".tree-state-*.tmp")
	if err != nil {
		m.statusNote = "tree state failed: " + err.Error()
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err == nil {
		err = json.NewEncoder(tmp).Encode(saved)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmpPath, m.treeStatePath)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		m.statusNote = "tree state failed: " + err.Error()
	}
}

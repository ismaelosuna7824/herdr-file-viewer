// Command file-viewer is the terminal UI launched inside a Herdr pane. It
// renders a file browser, fuzzy file finder and content search over a root
// directory.
//
// A Herdr plugin pane always runs with its working directory set to the plugin
// root, NOT the user's workspace. The workspace directory is delivered instead
// via HERDR_PLUGIN_CONTEXT_JSON (flat "workspace_cwd" key). So the root is
// resolved in priority order:
//  1. the first CLI argument, if given;
//  2. workspace_cwd (then focused_pane_cwd) from the Herdr context;
//  3. the current working directory.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ismaelosuna7824/herdr-file-viewer/internal/filetab"
	"github.com/ismaelosuna7824/herdr-file-viewer/internal/gitdiff"
	herdrbridge "github.com/ismaelosuna7824/herdr-file-viewer/internal/herdr"
	"github.com/ismaelosuna7824/herdr-file-viewer/internal/ui"
)

// version is injected at build time via -ldflags "-X main.version=vX.Y.Z".
var version = "dev"

func main() {
	if hasArg("--workspace-created") {
		if err := herdrbridge.EnsureWorkspaceTree(workspaceIDFromEvent()); err != nil {
			fmt.Fprintln(os.Stderr, "file-viewer:", err)
			os.Exit(1)
		}
		return
	}
	if hasArg("--restore-focused-tab") {
		workspaceID := workspaceIDFromEvent()
		tabID := tabIDFromEvent()
		if err := herdrbridge.RestoreFocusedTab(workspaceID, tabID, workspacePathFromContext()); err != nil {
			fmt.Fprintln(os.Stderr, "file-viewer:", err)
			os.Exit(1)
		}
		return
	}

	ui.SetVersion(version)

	model, err := newModel()
	if err != nil {
		fmt.Fprintln(os.Stderr, "file-viewer:", err)
		os.Exit(1)
	}

	p := tea.NewProgram(model, programOptions(model)...)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "file-viewer:", err)
		os.Exit(1)
	}
}

func programOptions(model tea.Model) []tea.ProgramOption {
	options := []tea.ProgramOption{tea.WithAltScreen()}
	if shouldCaptureMouse(model) {
		options = append(options, tea.WithMouseCellMotion())
	}
	return options
}

// shouldCaptureMouse leaves standalone file tabs in normal terminal mouse mode
// so Herdr core can highlight dragged text and copy it on release. Explorer and
// full-viewer panes still need Bubble Tea mouse events for clickable tree rows.
func shouldCaptureMouse(model tea.Model) bool {
	switch model.(type) {
	case filetab.Model, *filetab.Model, ui.DiffTabModel, *ui.DiffTabModel:
		return false
	default:
		return true
	}
}

// newModel selects the pane's UI by entrypoint. The diff entrypoint renders a
// single read-only diff review, the file entrypoint a single read-only file;
// everything else is the full explorer application.
func newModel() (tea.Model, error) {
	if path := os.Getenv("HERDR_DIFF_PATH"); path != "" {
		return ui.NewDiffTab(os.Getenv("HERDR_DIFF_ROOT"), path, gitdiff.Mode(os.Getenv("HERDR_DIFF_MODE")))
	}
	if path := os.Getenv("HERDR_FILE_PATH"); path != "" {
		return filetab.New(path)
	}
	return ui.New(resolveRoot())
}

func resolveRoot() string {
	for _, arg := range os.Args[1:] {
		if arg != "" && arg != "--tree-only" && arg != "--workspace-created" && arg != "--restore-focused-tab" {
			return arg
		}
	}
	if p := os.Getenv("HERDR_TREE_ROOT"); isDir(p) {
		return p
	}
	if p := workspacePathFromContext(); p != "" {
		return p
	}
	return "."
}

func tabIDFromEvent() string {
	if tabID := os.Getenv("HERDR_TAB_ID"); tabID != "" {
		return tabID
	}
	var payload struct {
		TabID string `json:"tab_id"`
		Tab   struct {
			TabID string `json:"tab_id"`
		} `json:"tab"`
		Data struct {
			TabID string `json:"tab_id"`
			Tab   struct {
				TabID string `json:"tab_id"`
			} `json:"tab"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(os.Getenv("HERDR_PLUGIN_EVENT_JSON")), &payload); err != nil {
		return ""
	}
	for _, tabID := range []string{payload.TabID, payload.Tab.TabID, payload.Data.TabID, payload.Data.Tab.TabID} {
		if tabID != "" {
			return tabID
		}
	}
	return ""
}

func workspaceIDFromEvent() string {
	if workspaceID := os.Getenv("HERDR_WORKSPACE_ID"); workspaceID != "" {
		return workspaceID
	}
	var payload struct {
		WorkspaceID string `json:"workspace_id"`
		Workspace   struct {
			WorkspaceID string `json:"workspace_id"`
		} `json:"workspace"`
		Data struct {
			WorkspaceID string `json:"workspace_id"`
			Workspace   struct {
				WorkspaceID string `json:"workspace_id"`
			} `json:"workspace"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(os.Getenv("HERDR_PLUGIN_EVENT_JSON")), &payload); err != nil {
		return ""
	}
	for _, workspaceID := range []string{
		payload.WorkspaceID,
		payload.Workspace.WorkspaceID,
		payload.Data.WorkspaceID,
		payload.Data.Workspace.WorkspaceID,
	} {
		if workspaceID != "" {
			return workspaceID
		}
	}
	return ""
}

func hasArg(want string) bool {
	for _, arg := range os.Args[1:] {
		if arg == want {
			return true
		}
	}
	return false
}

// workspacePathFromContext extracts the active workspace's directory from
// Herdr's injected context JSON. The keys are flat (confirmed empirically):
// "workspace_cwd" is the workspace's directory; "focused_pane_cwd" is a
// fallback for the pane the user was on when they launched the viewer.
func workspacePathFromContext() string {
	raw := os.Getenv("HERDR_PLUGIN_CONTEXT_JSON")
	if raw == "" {
		return ""
	}
	var ctx map[string]any
	if err := json.Unmarshal([]byte(raw), &ctx); err != nil {
		return ""
	}
	for _, key := range []string{"workspace_cwd", "focused_pane_cwd"} {
		if v, ok := ctx[key].(string); ok && isDir(v) {
			return v
		}
	}
	return ""
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

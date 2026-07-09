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

	"github.com/ismaelosuna7824/herdr-file-viewer/internal/ui"
)

func main() {
	root := resolveRoot()

	model, err := ui.New(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "file-viewer:", err)
		os.Exit(1)
	}

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "file-viewer:", err)
		os.Exit(1)
	}
}

func resolveRoot() string {
	if len(os.Args) > 1 && os.Args[1] != "" {
		return os.Args[1]
	}
	if p := workspacePathFromContext(); p != "" {
		return p
	}
	return "."
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

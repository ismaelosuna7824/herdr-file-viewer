// Package editor resolves the user's preferred text editor and builds a command
// to open a file in it. It is configuration-only — the plugin never bundles or
// depends on a specific editor. Resolution order:
//
//	FILE_VIEWER_EDITOR   (plugin-specific override)
//	VISUAL               (standard, for full-screen editors)
//	EDITOR               (standard fallback)
//
// The value is a command line, so "code --wait", "zed", "nvim", "hx" etc. all
// work — the target file is appended as the final argument.
package editor

import (
	"os"
	"os/exec"
	"strings"
)

// Resolve returns the configured editor command split into tokens, or nil if
// none is set.
func Resolve() []string {
	for _, key := range []string{"FILE_VIEWER_EDITOR", "VISUAL", "EDITOR"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return strings.Fields(v)
		}
	}
	return nil
}

// Command builds the exec.Cmd to open file in the resolved editor, or nil if no
// editor is configured.
func Command(file string) *exec.Cmd {
	parts := Resolve()
	if len(parts) == 0 {
		return nil
	}
	args := make([]string, 0, len(parts))
	args = append(args, parts[1:]...)
	args = append(args, file)
	return exec.Command(parts[0], args...)
}

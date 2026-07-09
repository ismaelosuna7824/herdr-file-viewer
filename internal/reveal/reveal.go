// Package reveal opens the host OS file manager at a given path, selecting the
// file where the platform supports it. It is cross-platform: macOS Finder,
// Windows Explorer, and freedesktop (Linux/BSD) file managers.
package reveal

import (
	"os/exec"
	"path/filepath"
	"runtime"
)

// command returns the OS command that reveals path in the system file manager.
// It is split out from Reveal so it can be unit-tested per-GOOS without actually
// launching a window.
//
//   - darwin:  open -R <path>            (selects the file in Finder)
//   - windows: explorer /select,<path>   (selects the file in Explorer)
//   - other:   xdg-open <dir>            (opens the containing folder)
func command(goos, path string) (name string, args []string) {
	switch goos {
	case "darwin":
		return "open", []string{"-R", path}
	case "windows":
		return "explorer", []string{"/select," + path}
	default:
		return "xdg-open", []string{filepath.Dir(path)}
	}
}

// Reveal launches the OS file manager for path. It returns as soon as the
// process is started (it does not wait for the file manager to exit).
func Reveal(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	name, args := command(runtime.GOOS, abs)
	return exec.Command(name, args...).Start()
}

//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly

package herdr

import "os"

// Herdr currently ships this plugin only on macOS and Linux. Keep unsupported
// platforms buildable; the package-level mutex still serializes calls made by
// one plugin process there.
func lockFileExclusive(_ *os.File) error { return nil }

func unlockFile(_ *os.File) error { return nil }

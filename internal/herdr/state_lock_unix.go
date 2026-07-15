//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package herdr

import (
	"os"
	"syscall"
)

func lockFileExclusive(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
}

func unlockFile(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}

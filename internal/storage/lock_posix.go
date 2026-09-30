//go:build !windows

package storage

import (
	"os"
	"syscall"
)

// isProcessAlive checks if a process with the given PID is currently active on POSIX systems.
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

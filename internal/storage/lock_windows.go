//go:build windows

package storage

import (
	"syscall"
)

// isProcessAlive checks if a process with the given PID is currently active on Windows.
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	const PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	h, err := syscall.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)

	var exitCode uint32
	err = syscall.GetExitCodeProcess(h, &exitCode)
	if err != nil {
		return false
	}
	// 259 (0x0103) is STILL_ACTIVE on Windows
	return exitCode == 259
}

package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ProcessLock manages an OS process-level lock to enforce single-instance execution.
type ProcessLock struct {
	lockPath string
	file     *os.File
	mu       sync.Mutex
}

// NewProcessLock attempts to acquire an OS-managed lock on lockPath.
// If another process currently holds the lock, it returns an error immediately.
func NewProcessLock(lockPath string) (*ProcessLock, error) {
	// Skip file locking for in-memory SQLite databases
	if strings.Contains(lockPath, ":memory:") {
		return &ProcessLock{}, nil
	}

	absPath, err := filepath.Abs(lockPath)
	if err != nil {
		return nil, fmt.Errorf("invalid lock path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create lock directory: %w", err)
	}

	file, err := openLockFile(absPath)
	if err != nil {
		return nil, err
	}

	pidStr := fmt.Sprintf("%d\n", os.Getpid())
	_, _ = file.WriteString(pidStr)
	_ = file.Sync()

	return &ProcessLock{
		lockPath: absPath,
		file:     file,
	}, nil
}

func openLockFile(absPath string) (*os.File, error) {
	file, err := os.OpenFile(absPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		if os.IsExist(err) {
			// Automatically clean up stale lockfiles if the owning PID is dead
			if isStaleLock(absPath) {
				_ = os.Remove(absPath)
				fileRetry, errRetry := os.OpenFile(absPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
				if errRetry == nil {
					return fileRetry, nil
				}
			}
			return nil, fmt.Errorf("failed to acquire database instance lock: another CodeGraph process is running against database lockfile '%s'", absPath)
		}
		return nil, fmt.Errorf("failed to create instance lockfile: %w", err)
	}
	return file, nil
}

func isStaleLock(absPath string) bool {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return true // If unreadable or empty, treat as stale
	}
	pidStr := strings.TrimSpace(string(data))
	if pidStr == "" {
		return true // Empty lockfile
	}
	var pid int
	if _, err := fmt.Sscanf(pidStr, "%d", &pid); err != nil {
		return true // Invalid format
	}
	return !isProcessAlive(pid)
}

// Release closes and removes the lock file.
func (l *ProcessLock) Release() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return nil
	}

	_ = l.file.Close()
	l.file = nil
	if l.lockPath != "" {
		_ = os.Remove(l.lockPath)
	}
	return nil
}

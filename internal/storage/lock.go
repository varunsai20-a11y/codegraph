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

	file, err := os.OpenFile(absPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("failed to acquire database instance lock: another CodeGraph process is running against database lockfile '%s'", absPath)
		}
		return nil, fmt.Errorf("failed to create instance lockfile: %w", err)
	}

	pidStr := fmt.Sprintf("%d\n", os.Getpid())
	_, _ = file.WriteString(pidStr)
	_ = file.Sync()

	return &ProcessLock{
		lockPath: absPath,
		file:     file,
	}, nil
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

package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestStaleLockAutoCleanup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lock_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	lockPath := filepath.Join(tmpDir, "test.db.lock")

	// 1. Create a dummy lockfile containing a non-existent PID (e.g. 999999)
	dummyPID := 999999
	if err := os.WriteFile(lockPath, []byte(fmt.Sprintf("%d\n", dummyPID)), 0600); err != nil {
		t.Fatalf("failed to write dummy lockfile: %v", err)
	}

	// 2. NewProcessLock should detect that PID 999999 is dead, remove the lock, and succeed
	lock, err := NewProcessLock(lockPath)
	if err != nil {
		t.Fatalf("expected NewProcessLock to clean stale lock and succeed, got error: %v", err)
	}
	defer lock.Release()

	// 3. Attempting to acquire a second lock while the current process lock is active MUST fail
	_, errSecond := NewProcessLock(lockPath)
	if errSecond == nil {
		t.Fatalf("expected second lock acquisition to fail while first process is active")
	}
}

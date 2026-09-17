package storage

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"codegraph/internal/models"
)

func TestSQLiteStorage_WALAndBusyTimeoutPragmas(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "pragma_test.db")

	store, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to init SQLite storage: %v", err)
	}
	defer store.Close()

	var jMode string
	if err := store.db.QueryRow("PRAGMA journal_mode;").Scan(&jMode); err != nil {
		t.Fatalf("failed to query journal_mode: %v", err)
	}

	var busyTimeout int
	if err := store.db.QueryRow("PRAGMA busy_timeout;").Scan(&busyTimeout); err != nil {
		t.Fatalf("failed to query busy_timeout: %v", err)
	}

	if busyTimeout < 5000 {
		t.Errorf("expected busy_timeout >= 5000ms, got %d ms", busyTimeout)
	}

	t.Logf("SQLite Storage initialized with journal_mode=%s, busy_timeout=%d ms", jMode, busyTimeout)
}

func TestSQLiteStorage_ConcurrentReadersAndWriters(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "concurrency_test.db")

	store, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to init SQLite storage: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	repo := &models.Repository{
		ID:         "repo-conc-1",
		Name:       "Conc Repo",
		SourceType: models.SourceTypeLocal,
		LocalPath:  "/tmp/conc",
		Status:     models.RepoStatusIndexed,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := store.CreateRepository(ctx, repo); err != nil {
		t.Fatalf("CreateRepository failed: %v", err)
	}

	// 1. Verify concurrent reader during active write transaction
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}

	_, err = tx.ExecContext(ctx, "UPDATE repositories SET status = ? WHERE id = ?", models.RepoStatusIndexing, repo.ID)
	if err != nil {
		t.Fatalf("tx ExecContext failed: %v", err)
	}

	readerErrChan := make(chan error, 1)
	go func() {
		fetched, err := store.GetRepository(context.Background(), repo.ID)
		if err != nil {
			readerErrChan <- err
			return
		}
		if fetched.Status != models.RepoStatusIndexed {
			t.Errorf("expected reader to see pre-tx status READY, got %s", fetched.Status)
		}
		readerErrChan <- nil
	}()

	select {
	case err := <-readerErrChan:
		if err != nil {
			t.Fatalf("concurrent reader failed during active write tx: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("concurrent reader blocked unexpectedly")
	}

	// 2. Verify concurrent writer timeout behavior
	writerResultChan := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tx2, err := store.db.BeginTx(context.Background(), nil)
		if err != nil {
			writerResultChan <- err
			return
		}
		_, err = tx2.Exec("UPDATE repositories SET status = ? WHERE id = ?", models.RepoStatusFailed, repo.ID)
		if err != nil {
			_ = tx2.Rollback()
			writerResultChan <- err
			return
		}
		writerResultChan <- tx2.Commit()
	}()

	time.Sleep(100 * time.Millisecond)
	if err := tx.Commit(); err != nil {
		t.Fatalf("tx commit failed: %v", err)
	}

	wg.Wait()
	close(writerResultChan)
	if err := <-writerResultChan; err != nil {
		t.Fatalf("concurrent writer failed despite busy_timeout: %v", err)
	}
}

func TestSingleInstanceStartupGuard(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "instance_lock_test.db")

	store1, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to init first storage instance: %v", err)
	}

	// Attempt second storage instance against same database path
	store2, err := NewSQLiteStorage(dbPath)
	if err == nil {
		store2.Close()
		store1.Close()
		t.Fatalf("expected second storage instance to fail with process lock conflict, got nil error")
	}

	t.Logf("Second instance correctly rejected: %v", err)

	// Close first instance and verify lock release
	if err := store1.Close(); err != nil {
		t.Fatalf("failed to close first storage instance: %v", err)
	}

	// Now third instance should succeed
	store3, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("expected third storage instance to succeed after lock release, got: %v", err)
	}
	defer store3.Close()
}

func TestSQLite_CanonicalURLAndHistoricalVariantPreservation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "canonical_test.db")

	store, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	rawURL := "https://GitHub.COM/Gin-Gonic/Gin.git/"
	canonicalURL := "https://github.com/gin-gonic/gin"

	repo := &models.Repository{
		ID:           "repo-hist-1",
		Name:         "Gin",
		SourceType:   models.SourceTypeGit,
		SourceURL:    rawURL,
		CanonicalURL: canonicalURL,
		LocalPath:    "/tmp/gin",
		Status:       models.RepoStatusIndexed,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := store.CreateRepository(ctx, repo); err != nil {
		t.Fatalf("CreateRepository failed: %v", err)
	}

	// 1. Query by canonical URL
	fetchedByCanon, err := store.GetRepositoryByCanonicalURL(ctx, canonicalURL)
	if err != nil {
		t.Fatalf("GetRepositoryByCanonicalURL failed: %v", err)
	}
	if fetchedByCanon.SourceURL != rawURL {
		t.Errorf("expected SourceURL to remain unchanged raw string '%s', got '%s'", rawURL, fetchedByCanon.SourceURL)
	}

	// 2. Query by variant source URL
	fetchedBySrc, err := store.GetRepositoryBySourceURL(ctx, "https://github.com/gin-gonic/gin")
	if err != nil {
		t.Fatalf("GetRepositoryBySourceURL failed: %v", err)
	}
	if fetchedBySrc.ID != repo.ID {
		t.Errorf("expected matched repo ID %s, got %s", repo.ID, fetchedBySrc.ID)
	}
}

func TestSQLite_OrphanJobRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "orphan_test.db")

	store1, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to init storage 1: %v", err)
	}

	ctx := context.Background()
	repo := &models.Repository{
		ID:         "repo-orphan",
		Name:       "Orphan Repo",
		SourceType: models.SourceTypeGit,
		LocalPath:  "/tmp/orphan",
		Status:     models.RepoStatusIndexing,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := store1.CreateRepository(ctx, repo); err != nil {
		t.Fatalf("CreateRepository failed: %v", err)
	}

	job := &models.IndexJob{
		ID:           "job-stale-1",
		RepositoryID: repo.ID,
		Status:       models.JobStatusRunning,
		StartedAt:    time.Now().Add(-5 * time.Minute),
	}
	if err := store1.CreateIndexJob(ctx, job); err != nil {
		t.Fatalf("CreateIndexJob failed: %v", err)
	}

	// Simulate server process exit
	store1.Close()

	// Re-open storage (simulating server restart)
	store2, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to init storage 2: %v", err)
	}
	defer store2.Close()

	recoveredJob, err := store2.GetIndexJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetIndexJob failed: %v", err)
	}

	if recoveredJob.Status != models.JobStatusFailed {
		t.Errorf("expected stale job status FAILED after restart, got %s", recoveredJob.Status)
	}
	if recoveredJob.Error != "Job marked failed due to server process restart" {
		t.Errorf("expected restart error message, got %s", recoveredJob.Error)
	}
}

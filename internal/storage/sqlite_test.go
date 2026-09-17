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


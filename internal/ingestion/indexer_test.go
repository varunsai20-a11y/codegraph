package ingestion

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"codegraph/internal/config"
	"codegraph/internal/graph"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/storage"
)

func setupTestIndexer(t *testing.T) (*Indexer, storage.Storage, string, *models.Repository) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	wsRoot := filepath.Join(tmpDir, "workspaces")

	cfg := &config.Config{
		MaxFileSize:       10 * 1024 * 1024,
		WorkspaceRoot:     wsRoot,
		DatabasePath:      dbPath,
		DefaultExclusions: []string{".git"},
	}

	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	wsMgr, err := repository.NewWorkspaceManager(wsRoot)
	if err != nil {
		t.Fatalf("failed to create wsMgr: %v", err)
	}

	indexer := NewIndexer(cfg, store, wsMgr)

	localDir := filepath.Join(tmpDir, "sample-repo")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatalf("failed to mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	repo := &models.Repository{
		ID:         "test-repo-stage1",
		Name:       "Test Repo Stage 1",
		SourceType: models.SourceTypeLocal,
		LocalPath:  localDir,
		Status:     models.RepoStatusRegistered,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := store.CreateRepository(context.Background(), repo); err != nil {
		t.Fatalf("failed to create repo in db: %v", err)
	}

	return indexer, store, localDir, repo
}

// Test 1: Failed re-indexing preserves previous valid graph in cache and SQLite
func TestIndexer_FailedReindexingPreservesPreviousGraph(t *testing.T) {
	indexer, store, _, repo := setupTestIndexer(t)

	// Initial successful index
	job1 := &models.IndexJob{ID: "job-1", RepositoryID: repo.ID, Status: models.JobStatusPending, StartedAt: time.Now()}
	_ = store.CreateIndexJob(context.Background(), job1)
	if err := indexer.RunIndex(context.Background(), job1, repo); err != nil {
		t.Fatalf("initial RunIndex failed: %v", err)
	}

	engine1, found := indexer.GetGraphEngine(repo.ID)
	if !found || engine1 == nil {
		t.Fatalf("expected initial graph engine in cache")
	}
	nodes1, _ := engine1.GetOverviewGraph(graph.QueryParams{NodeLimit: 10})
	if len(nodes1) == 0 {
		t.Fatalf("expected initial graph nodes > 0")
	}

	initialDbNodes, _, _ := store.GetGraphForRepository(context.Background(), repo.ID)
	if len(initialDbNodes) == 0 {
		t.Fatalf("expected initial DB nodes > 0")
	}

	// Trigger a failed re-indexing pass with canceled context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	job2 := &models.IndexJob{ID: "job-2", RepositoryID: repo.ID, Status: models.JobStatusPending, StartedAt: time.Now()}
	_ = store.CreateIndexJob(context.Background(), job2)

	err := indexer.RunIndex(canceledCtx, job2, repo)
	if err == nil {
		t.Fatalf("expected RunIndex to fail on canceled context")
	}

	// Verify previous valid graph is preserved in cache
	engineAfter, foundAfter := indexer.GetGraphEngine(repo.ID)
	if !foundAfter || engineAfter == nil {
		t.Fatalf("expected previous valid graph engine to be retained in cache after failed re-indexing")
	}

	nodesAfter, _ := engineAfter.GetOverviewGraph(graph.QueryParams{NodeLimit: 10})
	if len(nodesAfter) != len(nodes1) {
		t.Errorf("expected retained graph node count %d, got %d", len(nodes1), len(nodesAfter))
	}

	// Verify SQLite database was NOT mutated or corrupted
	dbNodes, _, dbErr := store.GetGraphForRepository(context.Background(), repo.ID)
	if dbErr != nil || len(dbNodes) != len(initialDbNodes) {
		t.Errorf("expected SQLite graph nodes to remain intact (%d nodes), got %d (err %v)", len(initialDbNodes), len(dbNodes), dbErr)
	}
}

// Test 2: Cache publication & fallback to SQLite
func TestIndexer_CachePublicationAndSQLiteFallback(t *testing.T) {
	indexer, store, _, repo := setupTestIndexer(t)

	job1 := &models.IndexJob{ID: "job-1", RepositoryID: repo.ID, Status: models.JobStatusPending, StartedAt: time.Now()}
	_ = store.CreateIndexJob(context.Background(), job1)
	if err := indexer.RunIndex(context.Background(), job1, repo); err != nil {
		t.Fatalf("initial RunIndex failed: %v", err)
	}

	// Verify publication to cache
	_, found := indexer.GetGraphEngine(repo.ID)
	if !found {
		t.Fatalf("expected engine cached after successful index")
	}

	// Invalidate cache manually
	indexer.InvalidateEngine(repo.ID)
	_, foundAfterInvalidate := indexer.GetGraphEngine(repo.ID)
	if foundAfterInvalidate {
		t.Fatalf("expected cache to be empty after InvalidateEngine")
	}

	// Trigger fallback by querying SQLite
	nodes, edges, err := store.GetGraphForRepository(context.Background(), repo.ID)
	if err != nil || len(nodes) == 0 {
		t.Fatalf("failed fallback SQLite query: %v", err)
	}
	fallbackEngine := graph.NewEngine(repo.ID)
	fallbackEngine.LoadGraph(nodes, edges)
	indexer.SetGraphEngine(repo.ID, fallbackEngine)

	repopulatedEngine, repopulated := indexer.GetGraphEngine(repo.ID)
	if !repopulated || repopulatedEngine == nil {
		t.Fatalf("expected cache to be repopulated after SQLite fallback")
	}
}

// Test 3: Cancellation during indexing/persistence rolls back atomically
func TestIndexer_CancellationDuringPersistenceRollsBack(t *testing.T) {
	indexer, store, _, repo := setupTestIndexer(t)

	// Pre-populate with job 1
	job1 := &models.IndexJob{ID: "job-1", RepositoryID: repo.ID, Status: models.JobStatusPending, StartedAt: time.Now()}
	_ = store.CreateIndexJob(context.Background(), job1)
	if err := indexer.RunIndex(context.Background(), job1, repo); err != nil {
		t.Fatalf("initial RunIndex failed: %v", err)
	}

	initialManifests, _ := store.GetManifestForRepository(context.Background(), repo.ID)
	initialSymbols, _ := store.GetSymbolsForRepository(context.Background(), repo.ID)

	// Cancel context before persistence
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	job2 := &models.IndexJob{ID: "job-2", RepositoryID: repo.ID, Status: models.JobStatusPending, StartedAt: time.Now()}
	_ = store.CreateIndexJob(context.Background(), job2)

	err := indexer.RunIndex(canceledCtx, job2, repo)
	if err == nil {
		t.Fatalf("expected RunIndex to fail on canceled context")
	}

	// Verify atomic rollback in SQLite
	afterManifests, _ := store.GetManifestForRepository(context.Background(), repo.ID)
	afterSymbols, _ := store.GetSymbolsForRepository(context.Background(), repo.ID)

	if len(afterManifests) != len(initialManifests) || len(afterSymbols) != len(initialSymbols) {
		t.Errorf("expected atomic rollback in SQLite: manifests %d vs %d, symbols %d vs %d",
			len(initialManifests), len(afterManifests), len(initialSymbols), len(afterSymbols))
	}
}

// Test 4: Concurrent readers during engine replacement
func TestIndexer_ConcurrentReadersDuringEngineReplacement(t *testing.T) {
	indexer, store, _, repo := setupTestIndexer(t)

	job1 := &models.IndexJob{ID: "job-1", RepositoryID: repo.ID, Status: models.JobStatusPending, StartedAt: time.Now()}
	_ = store.CreateIndexJob(context.Background(), job1)
	if err := indexer.RunIndex(context.Background(), job1, repo); err != nil {
		t.Fatalf("initial RunIndex failed: %v", err)
	}

	var wg sync.WaitGroup
	stopChan := make(chan struct{})

	// 10 concurrent readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			for {
				select {
				case <-stopChan:
					return
				default:
					engine, found := indexer.GetGraphEngine(repo.ID)
					if found && engine != nil {
						nodes, _ := engine.GetOverviewGraph(graph.QueryParams{NodeLimit: 20})
						_ = len(nodes)
					}
					time.Sleep(1 * time.Millisecond)
				}
			}
		}(i)
	}

	// Writer goroutine continuously re-indexing/replacing engine
	for cycle := 0; cycle < 5; cycle++ {
		job := &models.IndexJob{ID: fmt.Sprintf("job-concurrent-%d", cycle), RepositoryID: repo.ID, Status: models.JobStatusPending, StartedAt: time.Now()}
		_ = store.CreateIndexJob(context.Background(), job)
		_ = indexer.RunIndex(context.Background(), job, repo)
	}

	close(stopChan)
	wg.Wait()
}

// Test 5: Post-commit publication failure exercises production defer recovery path
func TestIndexer_PostCommitPublicationFailure(t *testing.T) {
	indexer, store, localDir, repo := setupTestIndexer(t)

	// Step 1: Initial successful index producing Graph V1
	job1 := &models.IndexJob{ID: "job-1", RepositoryID: repo.ID, Status: models.JobStatusPending, StartedAt: time.Now()}
	_ = store.CreateIndexJob(context.Background(), job1)
	if err := indexer.RunIndex(context.Background(), job1, repo); err != nil {
		t.Fatalf("initial RunIndex failed: %v", err)
	}

	engineV1, foundV1 := indexer.GetGraphEngine(repo.ID)
	if !foundV1 || engineV1 == nil {
		t.Fatalf("expected V1 graph engine in cache")
	}

	// Step 2: Add a new source file to trigger Graph V2 on re-indexing
	if err := os.WriteFile(filepath.Join(localDir, "service.go"), []byte("package main\nfunc ProcessData() {}\n"), 0644); err != nil {
		t.Fatalf("failed to write service.go: %v", err)
	}

	// Step 3: Inject test seam hook to simulate publication error immediately AFTER SaveIndexData commits V2 to SQLite
	indexer.onPostSaveCommit = func(repoID string) error {
		return fmt.Errorf("simulated post-commit publication failure")
	}

	job2 := &models.IndexJob{ID: "job-2", RepositoryID: repo.ID, Status: models.JobStatusPending, StartedAt: time.Now()}
	_ = store.CreateIndexJob(context.Background(), job2)

	// Execute RunIndex. SaveIndexData will commit V2 to SQLite, onPostSaveCommit will return error,
	// and the production defer block will execute InvalidateEngine(repo.ID).
	err := indexer.RunIndex(context.Background(), job2, repo)
	if err == nil {
		t.Fatalf("expected RunIndex to return error from post-commit hook")
	}

	// Reset hook
	indexer.onPostSaveCommit = nil

	// Step 4: Verify cache was evicted by production defer block (not stale V1)
	_, foundInCache := indexer.GetGraphEngine(repo.ID)
	if foundInCache {
		t.Fatalf("expected cache to be invalidated by production defer recovery block")
	}

	// Step 5: Execute normal fallback path (as API does) by querying SQLite and populating cache
	dbNodes, dbEdges, dbErr := store.GetGraphForRepository(context.Background(), repo.ID)
	if dbErr != nil || len(dbNodes) == 0 {
		t.Fatalf("expected SQLite to contain committed Graph V2, got error: %v", dbErr)
	}

	rebuiltEngine := graph.NewEngine(repo.ID)
	rebuiltEngine.LoadGraph(dbNodes, dbEdges)
	indexer.SetGraphEngine(repo.ID, rebuiltEngine)

	// Step 6: Verify normal retrieval returns complete Graph V2 and no partial/stale graph
	retrievedEngine, foundRetrieved := indexer.GetGraphEngine(repo.ID)
	if !foundRetrieved || retrievedEngine == nil {
		t.Fatalf("expected retrieved engine in cache after fallback repopulation")
	}

	allNodes := retrievedEngine.GetAllNodes()
	var foundProcessData bool
	for _, n := range allNodes {
		if n.Label == "sym:test-repo-stage1:service.go:ProcessData:1" || n.RelativePath == "service.go" {
			foundProcessData = true
			break
		}
	}
	if !foundProcessData {
		t.Errorf("expected retrieved graph to contain Graph V2 node from service.go, got nodes: %+v", allNodes)
	}
}

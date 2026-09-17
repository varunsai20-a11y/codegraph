package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codegraph/internal/api"
	"codegraph/internal/config"
	"codegraph/internal/graph"
	"codegraph/internal/ingestion"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/storage"
)

func TestE2E_IncrementalReindexingWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "reindex.db")
	wsRoot := filepath.Join(tmpDir, "workspaces")

	cfg := &config.Config{
		Port:               8080,
		MaxFileSize:        2 * 1024 * 1024,
		WorkspaceRoot:      wsRoot,
		DatabasePath:       dbPath,
		DefaultExclusions:  []string{".git"},
		SupportedLanguages: []string{"Go", "TypeScript"},
	}

	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	wsMgr, err := repository.NewWorkspaceManager(wsRoot)
	if err != nil {
		t.Fatalf("failed to init workspace manager: %v", err)
	}

	indexer := ingestion.NewIndexer(cfg, store, wsMgr)
	server := api.NewServer(cfg, store, wsMgr, indexer)

	// Create workspace repo source
	sourceDir := filepath.Join(tmpDir, "source-repo")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("failed to mkdir: %v", err)
	}

	file1Path := filepath.Join(sourceDir, "main.go")
	file2Path := filepath.Join(sourceDir, "old.go")
	_ = os.WriteFile(file1Path, []byte("package main\nfunc Main() {}\n"), 0644)
	_ = os.WriteFile(file2Path, []byte("package main\nfunc DeprecatedFunction() {}\n"), 0644)

	// Step 1: Register Repository
	regBody, _ := json.Marshal(api.RegisterRepoRequest{
		Name:       "E2E Reindex Repo",
		SourceType: models.SourceTypeLocal,
		LocalPath:  sourceDir,
	})
	req := httptest.NewRequest("POST", "/api/repositories", bytes.NewReader(regBody))
	rec := httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register API status = %d; want 201", rec.Code)
	}

	var repo models.Repository
	_ = json.Unmarshal(rec.Body.Bytes(), &repo)

	// Step 2: Initial Indexing Pass
	ctx := context.Background()
	job1 := &models.IndexJob{ID: "job-reindex-1", RepositoryID: repo.ID, Status: models.JobStatusPending, StartedAt: time.Now()}
	_ = store.CreateIndexJob(ctx, job1)
	if err := indexer.RunIndex(ctx, job1, &repo); err != nil {
		t.Fatalf("initial indexing failed: %v", err)
	}

	// Verify initial state
	symbolsV1, _ := store.GetSymbolsForRepository(ctx, repo.ID)
	nodesV1, _, _ := store.GetGraphForRepository(ctx, repo.ID)
	if len(symbolsV1) < 2 || len(nodesV1) < 2 {
		t.Fatalf("expected V1 symbols >= 2 and nodes >= 2, got symbols=%d nodes=%d", len(symbolsV1), len(nodesV1))
	}

	engineV1, foundV1 := indexer.GetGraphEngine(repo.ID)
	if !foundV1 || engineV1 == nil {
		t.Fatalf("expected V1 engine cached")
	}

	// Step 3: Incremental Workspace Changes (Add service.go, delete old.go, modify main.go)
	_ = os.Remove(file2Path)
	_ = os.WriteFile(file1Path, []byte("package main\nfunc MainV2() { ServiceHandler() }\n"), 0644)
	file3Path := filepath.Join(sourceDir, "service.go")
	_ = os.WriteFile(file3Path, []byte("package main\nfunc ServiceHandler() {}\n"), 0644)

	// Step 4: Re-Indexing Pass
	job2 := &models.IndexJob{ID: "job-reindex-2", RepositoryID: repo.ID, Status: models.JobStatusPending, StartedAt: time.Now()}
	_ = store.CreateIndexJob(ctx, job2)
	if err := indexer.RunIndex(ctx, job2, &repo); err != nil {
		t.Fatalf("re-indexing failed: %v", err)
	}

	// Step 5: Assert Stale Record Cleanup & Updated Graph State
	manifestsV2, _ := store.GetManifestForRepository(ctx, repo.ID)
	symbolsV2, _ := store.GetSymbolsForRepository(ctx, repo.ID)
	nodesV2, _, _ := store.GetGraphForRepository(ctx, repo.ID)

	var foundOldFile, foundServiceFile bool
	for _, m := range manifestsV2 {
		if m.RelativePath == "old.go" {
			foundOldFile = true
		}
		if m.RelativePath == "service.go" {
			foundServiceFile = true
		}
	}
	if foundOldFile {
		t.Errorf("expected deleted file old.go to be cleaned up from manifests")
	}
	if !foundServiceFile {
		t.Errorf("expected new file service.go to be present in manifests")
	}

	var foundDeprecatedSym, foundServiceSym bool
	for _, s := range symbolsV2 {
		if s.Name == "DeprecatedFunction" {
			foundDeprecatedSym = true
		}
		if s.Name == "ServiceHandler" {
			foundServiceSym = true
		}
	}
	if foundDeprecatedSym {
		t.Errorf("expected DeprecatedFunction to be removed from symbols table")
	}
	if !foundServiceSym {
		t.Errorf("expected ServiceHandler to be present in symbols table")
	}

	// Verify engine cache updated
	engineV2, foundV2 := indexer.GetGraphEngine(repo.ID)
	if !foundV2 || engineV2 == nil {
		t.Fatalf("expected V2 graph engine in cache after re-indexing")
	}

	overviewNodes, _ := engineV2.GetOverviewGraph(graph.QueryParams{NodeLimit: 20})
	if len(overviewNodes) == 0 {
		t.Fatalf("expected overview nodes in V2 engine")
	}
	_ = nodesV2
}

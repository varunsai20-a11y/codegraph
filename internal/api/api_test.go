package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"codegraph/internal/config"
	"codegraph/internal/ingestion"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/storage"
)

func setupTestServer(t *testing.T) (*Server, func()) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "api_test.db")
	wsPath := filepath.Join(tmpDir, "workspaces")

	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	wsMgr, err := repository.NewWorkspaceManager(wsPath)
	if err != nil {
		store.Close()
		t.Fatalf("failed to init workspace manager: %v", err)
	}

	cfg := config.Load()
	idx := ingestion.NewIndexer(cfg, store, wsMgr)
	srv := NewServer(cfg, store, wsMgr, idx)

	cleanup := func() {
		store.Close()
	}

	return srv, cleanup
}

func TestConcurrentRegistration_SemanticVariants(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	urls := []string{
		"https://github.com/gin-gonic/gin",
		"https://github.com/gin-gonic/gin/",
		"https://github.com/gin-gonic/gin.git",
		"https://GITHUB.COM/Gin-Gonic/Gin.git/",
		"https://github.com/GIN-GONIC/GIN",
	}

	numGoroutines := 10
	var wg sync.WaitGroup
	repoIDs := make([]string, numGoroutines)
	errors := make([]error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			urlToUse := urls[idx%len(urls)]
			payload := map[string]string{
				"source_type": "GIT",
				"source_url":  urlToUse,
			}
			bodyBytes, _ := json.Marshal(payload)

			req := httptest.NewRequest("POST", "/api/repositories", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			srv.router.ServeHTTP(w, req)

			if w.Code != http.StatusOK && w.Code != http.StatusCreated {
				errors[idx] = fmt.Errorf("unexpected status %d: %s", w.Code, w.Body.String())
				return
			}

			var repo models.Repository
			if err := json.Unmarshal(w.Body.Bytes(), &repo); err != nil {
				errors[idx] = err
				return
			}
			repoIDs[idx] = repo.ID
		}(i)
	}

	wg.Wait()

	for i, err := range errors {
		if err != nil {
			t.Fatalf("goroutine %d failed: %v", i, err)
		}
	}

	firstID := repoIDs[0]
	if firstID == "" {
		t.Fatalf("expected non-empty repository ID")
	}

	for i, id := range repoIDs {
		if id != firstID {
			t.Errorf("goroutine %d got repository ID %s, expected canonical %s", i, id, firstID)
		}
	}

	// Verify database contains exactly 1 repository row
	repos, err := srv.store.ListRepositories(context.Background())
	if err != nil {
		t.Fatalf("ListRepositories failed: %v", err)
	}
	if len(repos) != 1 {
		t.Errorf("expected exactly 1 repository in database, got %d", len(repos))
	}
}

func TestConcurrentIndexJobCreation(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Register test repository
	payload := map[string]string{
		"source_type": "GIT",
		"source_url":  "https://github.com/gin-gonic/gin",
	}
	bodyBytes, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/repositories", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	var repo models.Repository
	_ = json.Unmarshal(w.Body.Bytes(), &repo)

	// Trigger concurrent job starts for the same repo
	numGoroutines := 5
	var wg sync.WaitGroup
	jobIDs := make([]string, numGoroutines)
	errors := make([]error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			jobReq := httptest.NewRequest("POST", fmt.Sprintf("/api/repositories/%s/index", repo.ID), nil)
			jobW := httptest.NewRecorder()

			srv.router.ServeHTTP(jobW, jobReq)

			if jobW.Code != http.StatusOK && jobW.Code != http.StatusAccepted {
				errors[idx] = fmt.Errorf("unexpected status %d: %s", jobW.Code, jobW.Body.String())
				return
			}

			var job models.IndexJob
			if err := json.Unmarshal(jobW.Body.Bytes(), &job); err != nil {
				errors[idx] = err
				return
			}
			jobIDs[idx] = job.ID
		}(i)
	}

	wg.Wait()

	for i, err := range errors {
		if err != nil {
			t.Fatalf("job goroutine %d failed: %v", i, err)
		}
	}

	firstJobID := jobIDs[0]
	for i, id := range jobIDs {
		if id != firstJobID {
			t.Errorf("job goroutine %d got job ID %s, expected %s", i, id, firstJobID)
		}
	}
}

func TestExplainRepository_GroundingAndIsolation(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create test repository
	repo := &models.Repository{
		ID:           "repo-ai-test",
		Name:         "AI Test Repo",
		SourceType:   models.SourceTypeGit,
		SourceURL:    "https://github.com/test/ai-repo",
		CanonicalURL: "https://github.com/test/ai-repo",
		LocalPath:    "/tmp/ai-repo",
		Status:       models.RepoStatusIndexed,
	}
	if err := srv.store.CreateRepository(ctx, repo); err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	// 2. Insert test symbol into storage
	sym := &models.Symbol{
		ID:            "sym-server-init",
		RepositoryID:  repo.ID,
		FileID:        "file-main-go",
		Name:          "InitializeServer",
		QualifiedName: "main.InitializeServer",
		Kind:          models.SymbolKindFunction,
		RelativePath:  "cmd/server/main.go",
		Location:      models.Location{StartLine: 10, EndLine: 25},
	}
	if err := srv.store.SaveSymbols(ctx, repo.ID, []*models.Symbol{sym}); err != nil {
		t.Fatalf("failed to insert symbols: %v", err)
	}

	manifestItem := &models.FileManifestItem{
		ID:           "file-main-go",
		RepositoryID: repo.ID,
		RelativePath: "cmd/server/main.go",
		Language:     "GO",
		Status:       models.FileStatusIndexed,
	}
	if err := srv.store.SaveManifestItems(ctx, repo.ID, []*models.FileManifestItem{manifestItem}); err != nil {
		t.Fatalf("failed to insert manifest: %v", err)
	}

	// Test A: Relevant Query for indexed symbol
	queryPayload := map[string]string{
		"query": "How does InitializeServer work?",
	}
	bodyBytes, _ := json.Marshal(queryPayload)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/repositories/%s/explain", repo.ID), bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 for relevant query, got %d: %s", w.Code, w.Body.String())
	}

	var resp models.ExplanationResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal explanation response: %v", err)
	}

	if resp.IsInsufficientEvidence {
		t.Errorf("expected relevant query to have sufficient evidence, got insufficient")
	}
	if len(resp.Evidence) == 0 {
		t.Errorf("expected evidence candidates for relevant query, got 0")
	}
	if resp.Grounding.EvidenceCount == 0 {
		t.Errorf("expected positive evidence count in grounding details")
	}

	// Test B: Irrelevant Query returning INSUFFICIENT_EVIDENCE
	irrelevantPayload := map[string]string{
		"query": "recipe for chocolate cake with frosting",
	}
	irrBytes, _ := json.Marshal(irrelevantPayload)
	irrReq := httptest.NewRequest("POST", fmt.Sprintf("/api/repositories/%s/explain", repo.ID), bytes.NewReader(irrBytes))
	irrReq.Header.Set("Content-Type", "application/json")
	irrW := httptest.NewRecorder()
	srv.router.ServeHTTP(irrW, irrReq)

	if irrW.Code != http.StatusOK {
		t.Fatalf("expected status 200 for irrelevant query, got %d: %s", irrW.Code, irrW.Body.String())
	}

	var irrResp models.ExplanationResponse
	if err := json.Unmarshal(irrW.Body.Bytes(), &irrResp); err != nil {
		t.Fatalf("failed to unmarshal irrelevant response: %v", err)
	}

	if !irrResp.IsInsufficientEvidence {
		t.Errorf("expected irrelevant query to return IsInsufficientEvidence = true")
	}

	// Test C: Non-existent Repository
	badRepoReq := httptest.NewRequest("POST", "/api/repositories/non-existent-repo-id/explain", bytes.NewReader(bodyBytes))
	badRepoReq.Header.Set("Content-Type", "application/json")
	badW := httptest.NewRecorder()
	srv.router.ServeHTTP(badW, badRepoReq)

	if badW.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for non-existent repository, got %d", badW.Code)
	}
}

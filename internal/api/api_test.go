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

func TestMultiRepositoryDynamicStats(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create Repo Tiny (3 files, 1 folder)
	repoTiny := &models.Repository{
		ID:         "repo-tiny-id",
		Name:       "repo-tiny",
		SourceType: models.SourceTypeLocal,
		LocalPath:  "/tmp/tiny",
		Status:     models.RepoStatusIndexed,
	}
	if err := srv.store.CreateRepository(ctx, repoTiny); err != nil {
		t.Fatalf("failed to create repoTiny: %v", err)
	}

	manifestsTiny := []*models.FileManifestItem{
		{ID: "t1", RepositoryID: repoTiny.ID, RelativePath: "src/a.go", Language: "Go", Status: models.FileStatusIndexed},
		{ID: "t2", RepositoryID: repoTiny.ID, RelativePath: "src/b.go", Language: "Go", Status: models.FileStatusIndexed},
		{ID: "t3", RepositoryID: repoTiny.ID, RelativePath: "src/c.go", Language: "Go", Status: models.FileStatusIndexed},
	}
	symbolsTiny := []*models.Symbol{
		{ID: "sym-t1", RepositoryID: repoTiny.ID, FileID: "t1", Name: "FuncA", Kind: models.SymbolKindFunction},
		{ID: "sym-t2", RepositoryID: repoTiny.ID, FileID: "t2", Name: "FuncB", Kind: models.SymbolKindFunction},
	}
	_ = srv.store.SaveIndexData(ctx, repoTiny.ID, manifestsTiny, symbolsTiny, nil, nil, nil)

	// 2. Create Repo Medium (25 files across 4 folders)
	repoMed := &models.Repository{
		ID:         "repo-medium-id",
		Name:       "repo-medium",
		SourceType: models.SourceTypeLocal,
		LocalPath:  "/tmp/medium",
		Status:     models.RepoStatusIndexed,
	}
	if err := srv.store.CreateRepository(ctx, repoMed); err != nil {
		t.Fatalf("failed to create repoMed: %v", err)
	}

	var manifestsMed []*models.FileManifestItem
	folders := []string{"pkg/core", "pkg/util", "cmd/app", "docs"}
	for i := 0; i < 25; i++ {
		folder := folders[i%len(folders)]
		manifestsMed = append(manifestsMed, &models.FileManifestItem{
			ID:           fmt.Sprintf("m%d", i),
			RepositoryID: repoMed.ID,
			RelativePath: fmt.Sprintf("%s/file_%d.go", folder, i),
			Language:     "Go",
			Status:       models.FileStatusIndexed,
		})
	}
	var symbolsMed []*models.Symbol
	for i := 0; i < 40; i++ {
		symbolsMed = append(symbolsMed, &models.Symbol{
			ID:           fmt.Sprintf("sym-m%d", i),
			RepositoryID: repoMed.ID,
			FileID:       "m0",
			Name:         fmt.Sprintf("SymMed_%d", i),
			Kind:         models.SymbolKindFunction,
		})
	}
	_ = srv.store.SaveIndexData(ctx, repoMed.ID, manifestsMed, symbolsMed, nil, nil, nil)

	// 3. Create Repo Large (120 files across 12 folders)
	repoLarge := &models.Repository{
		ID:         "repo-large-id",
		Name:       "repo-large",
		SourceType: models.SourceTypeLocal,
		LocalPath:  "/tmp/large",
		Status:     models.RepoStatusIndexed,
	}
	if err := srv.store.CreateRepository(ctx, repoLarge); err != nil {
		t.Fatalf("failed to create repoLarge: %v", err)
	}

	var manifestsLarge []*models.FileManifestItem
	for i := 0; i < 120; i++ {
		folderNum := (i % 12) + 1
		manifestsLarge = append(manifestsLarge, &models.FileManifestItem{
			ID:           fmt.Sprintf("l%d", i),
			RepositoryID: repoLarge.ID,
			RelativePath: fmt.Sprintf("module_%d/submodule/file_%d.ts", folderNum, i),
			Language:     "TypeScript",
			Status:       models.FileStatusIndexed,
		})
	}
	var symbolsLarge []*models.Symbol
	for i := 0; i < 200; i++ {
		symbolsLarge = append(symbolsLarge, &models.Symbol{
			ID:           fmt.Sprintf("sym-l%d", i),
			RepositoryID: repoLarge.ID,
			FileID:       "l0",
			Name:         fmt.Sprintf("SymLarge_%d", i),
			Kind:         models.SymbolKindClass,
		})
	}
	_ = srv.store.SaveIndexData(ctx, repoLarge.ID, manifestsLarge, symbolsLarge, nil, nil, nil)

	// Function to fetch stats via API
	getStats := func(repoID string) *models.RepositoryStats {
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/repositories/%s/stats", repoID), nil)
		w := httptest.NewRecorder()
		srv.router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for repo %s, got %d: %s", repoID, w.Code, w.Body.String())
		}
		var stats models.RepositoryStats
		if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
			t.Fatalf("failed to parse stats: %v", err)
		}
		return &stats
	}

	// Step 4. Validate Tiny Repo Stats
	sTiny := getStats(repoTiny.ID)
	if sTiny.FilesDiscovered != 3 || sTiny.FilesIndexed != 3 {
		t.Errorf("Tiny repo expected 3 files discovered/indexed, got %d discovered, %d indexed", sTiny.FilesDiscovered, sTiny.FilesIndexed)
	}
	if sTiny.FoldersDiscovered != 1 {
		t.Errorf("Tiny repo expected 1 folder (src), got %d", sTiny.FoldersDiscovered)
	}
	if sTiny.TotalSymbols != 2 {
		t.Errorf("Tiny repo expected 2 symbols, got %d", sTiny.TotalSymbols)
	}

	// Step 5. Validate Medium Repo Stats
	sMed := getStats(repoMed.ID)
	if sMed.FilesDiscovered != 25 || sMed.FilesIndexed != 25 {
		t.Errorf("Medium repo expected 25 files, got %d discovered, %d indexed", sMed.FilesDiscovered, sMed.FilesIndexed)
	}
	if sMed.FoldersDiscovered < 4 {
		t.Errorf("Medium repo expected at least 4 folders, got %d", sMed.FoldersDiscovered)
	}
	if sMed.TotalSymbols != 40 {
		t.Errorf("Medium repo expected 40 symbols, got %d", sMed.TotalSymbols)
	}

	// Step 6. Validate Large Repo Stats
	sLarge := getStats(repoLarge.ID)
	if sLarge.FilesDiscovered != 120 || sLarge.FilesIndexed != 120 {
		t.Errorf("Large repo expected 120 files, got %d discovered, %d indexed", sLarge.FilesDiscovered, sLarge.FilesIndexed)
	}
	if sLarge.FoldersDiscovered < 12 {
		t.Errorf("Large repo expected at least 12 folders, got %d", sLarge.FoldersDiscovered)
	}
	if sLarge.TotalSymbols != 200 {
		t.Errorf("Large repo expected 200 symbols, got %d", sLarge.TotalSymbols)
	}

	// Step 7. Repository Switch Test Sequence: Tiny -> Medium -> Large -> Tiny
	switchSequence := []struct {
		id            string
		expectedFiles int
	}{
		{repoTiny.ID, 3},
		{repoMed.ID, 25},
		{repoLarge.ID, 120},
		{repoTiny.ID, 3},
	}

	for idx, step := range switchSequence {
		st := getStats(step.id)
		if st.FilesIndexed != step.expectedFiles {
			t.Errorf("Switch step %d (repo %s): expected %d indexed files, got %d", idx, step.id, step.expectedFiles, st.FilesIndexed)
		}
		if st.RepositoryID != step.id {
			t.Errorf("Switch step %d: repository ID mismatch, expected %s got %s", idx, step.id, st.RepositoryID)
		}
	}
}

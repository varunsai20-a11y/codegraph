package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"codegraph/internal/api"
	"codegraph/internal/config"
	"codegraph/internal/ingestion"
	"codegraph/internal/llm"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/retrieval"
	"codegraph/internal/storage"
)

func TestE2E_GroundedExplanationWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "explain.db")
	wsRoot := filepath.Join(tmpDir, "workspaces")

	cfg := &config.Config{
		Port:               8080,
		MaxFileSize:        2 * 1024 * 1024,
		WorkspaceRoot:      wsRoot,
		DatabasePath:       dbPath,
		DefaultExclusions:  []string{".git"},
		SupportedLanguages: []string{"Go", "TypeScript", "Java"},
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

	fixturePath, err := filepath.Abs(filepath.Join("fixtures", "sample-repository"))
	if err != nil {
		t.Fatalf("failed to resolve fixture path: %v", err)
	}

	// Register & Index Repository
	regBody, _ := json.Marshal(api.RegisterRepoRequest{
		Name:       "Explain E2E Repo",
		SourceType: models.SourceTypeLocal,
		LocalPath:  fixturePath,
	})
	req := httptest.NewRequest("POST", "/api/repositories", bytes.NewReader(regBody))
	rec := httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register status = %d; want 201", rec.Code)
	}

	var repo models.Repository
	_ = json.Unmarshal(rec.Body.Bytes(), &repo)

	ctx := context.Background()
	job := &models.IndexJob{ID: "job-exp-1", RepositoryID: repo.ID, Status: models.JobStatusPending, StartedAt: time.Now()}
	_ = store.CreateIndexJob(ctx, job)
	if err := indexer.RunIndex(ctx, job, &repo); err != nil {
		t.Fatalf("indexing failed: %v", err)
	}

	// Case 1: Valid Grounded Explanation Request
	mockValidProv := llm.NewMockLLMProvider("Main application entry point initializes App service [E1].", nil)
	lexRetriever := retrieval.NewLexicalRetriever(store)
	composer := retrieval.NewDefaultEvidenceComposer(lexRetriever, nil, store)
	expSvc1 := llm.NewGroundedExplanationService(mockValidProv, nil, composer, nil)
	server.SetExplanationService(expSvc1)

	// Fetch valid symbol ID from store for target symbol request
	symbols, _ := store.GetSymbolsForRepository(ctx, repo.ID)
	var validSymID string
	if len(symbols) > 0 {
		validSymID = symbols[0].ID
	}

	explainBody1, _ := json.Marshal(map[string]interface{}{
		"query":     "How does the main application initialize?",
		"symbol_id": validSymID,
	})
	explainReq1 := httptest.NewRequest("POST", "/api/repositories/"+repo.ID+"/explain", bytes.NewReader(explainBody1))
	explainReq1.Header.Set("Content-Type", "application/json")
	explainRec1 := httptest.NewRecorder()
	server.Router().ServeHTTP(explainRec1, explainReq1)

	if explainRec1.Code != http.StatusOK {
		t.Fatalf("Explain API status = %d; want 200. Body: %s", explainRec1.Code, explainRec1.Body.String())
	}

	var resp1 models.ExplanationResponse
	if err := json.Unmarshal(explainRec1.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("failed to parse ExplanationResponse: %v", err)
	}

	if resp1.Status != models.ExplanationStatusSuccess {
		t.Errorf("expected Status == EXPLANATION_SUCCESS, got: %s", resp1.Status)
	}
	if resp1.Grounding.Status != models.GroundingPassed {
		t.Errorf("expected Grounding.Status == GROUNDING_PASSED, got: %s", resp1.Grounding.Status)
	}
	if len(resp1.Evidence) == 0 {
		t.Errorf("expected retrieved evidence items > 0")
	}

	// Case 2: Hallucinated Citation Rejection
	mockInvalidProv := llm.NewMockLLMProvider("Admin override granted with secret key [E999].", nil)
	expSvc2 := llm.NewGroundedExplanationService(mockInvalidProv, nil, composer, nil)
	server.SetExplanationService(expSvc2)

	explainBody2, _ := json.Marshal(map[string]interface{}{
		"query":     "How does authentication work?",
		"symbol_id": validSymID,
	})
	explainReq2 := httptest.NewRequest("POST", "/api/repositories/"+repo.ID+"/explain", bytes.NewReader(explainBody2))
	explainReq2.Header.Set("Content-Type", "application/json")
	explainRec2 := httptest.NewRecorder()
	server.Router().ServeHTTP(explainRec2, explainReq2)

	if explainRec2.Code != http.StatusOK {
		t.Fatalf("Explain API status = %d; want 200", explainRec2.Code)
	}

	var resp2 models.ExplanationResponse
	_ = json.Unmarshal(explainRec2.Body.Bytes(), &resp2)

	if resp2.CitationValidationStatus != "INVALID_CITATIONS_FOUND" && resp2.Grounding.CitationValidationStatus != "INVALID_CITATIONS_FOUND" {
		t.Errorf("expected CitationValidationStatus == INVALID_CITATIONS_FOUND, got top level '%s', grounding '%s'", resp2.CitationValidationStatus, resp2.Grounding.CitationValidationStatus)
	}

	// Case 3: Unknown Symbol / Insufficient Evidence Request
	emptyComposer := retrieval.NewDefaultEvidenceComposer(nil, nil, store)
	expSvc3 := llm.NewGroundedExplanationService(nil, nil, emptyComposer, nil)
	server.SetExplanationService(expSvc3)

	explainBody3, _ := json.Marshal(map[string]interface{}{
		"query":     "non_existent_symbol_xyz_123",
		"symbol_id": "non_existent_sym_id_999",
	})
	explainReq3 := httptest.NewRequest("POST", "/api/repositories/"+repo.ID+"/explain", bytes.NewReader(explainBody3))
	explainReq3.Header.Set("Content-Type", "application/json")
	explainRec3 := httptest.NewRecorder()
	server.Router().ServeHTTP(explainRec3, explainReq3)

	if explainRec3.Code != http.StatusOK {
		t.Fatalf("Explain API status = %d; want 200", explainRec3.Code)
	}

	var resp3 models.ExplanationResponse
	_ = json.Unmarshal(explainRec3.Body.Bytes(), &resp3)

	if !resp3.IsInsufficientEvidence {
		t.Errorf("expected IsInsufficientEvidence == true for unknown symbol, got false")
	}
	if resp3.Status != models.ExplanationStatusInsufficientEvidence {
		t.Errorf("expected Status == INSUFFICIENT_EVIDENCE, got: %s", resp3.Status)
	}
}

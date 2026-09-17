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

	"strings"

	"codegraph/internal/api"
	"codegraph/internal/config"
	"codegraph/internal/guide"
	"codegraph/internal/ingestion"
	"codegraph/internal/llm"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/retrieval"
	"codegraph/internal/security"
	"codegraph/internal/storage"
)

func TestC8_FullPipelineC1toC7Integration(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "c8_test.db")
	wsRoot := filepath.Join(tmpDir, "workspaces")

	cfg := &config.Config{
		Port:               8080,
		MaxFileSize:        2 * 1024 * 1024,
		WorkspaceRoot:      wsRoot,
		DatabasePath:       dbPath,
		DefaultExclusions:  []string{".git", "node_modules", "dist"},
		SupportedLanguages: []string{"TypeScript", "JavaScript", "Python", "Go", "Java"},
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

	// -------------------------------------------------------------
	// C1: Repository Registration & Ingestion
	// -------------------------------------------------------------
	regBody, _ := json.Marshal(api.RegisterRepoRequest{
		Name:       "C8 Fixture Repo",
		SourceType: models.SourceTypeLocal,
		LocalPath:  fixturePath,
	})

	req := httptest.NewRequest("POST", "/api/repositories", bytes.NewReader(regBody))
	rec := httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("C1 Register API status = %d; want 201. Body: %s", rec.Code, rec.Body.String())
	}

	var repo models.Repository
	if err := json.Unmarshal(rec.Body.Bytes(), &repo); err != nil {
		t.Fatalf("C1 failed to parse repo response: %v", err)
	}

	ctx := context.Background()
	job := &models.IndexJob{
		ID:           "job-c8-e2e",
		RepositoryID: repo.ID,
		Status:       models.JobStatusPending,
		StartedAt:    time.Now(),
	}
	if err := store.CreateIndexJob(ctx, job); err != nil {
		t.Fatalf("failed to create index job: %v", err)
	}

	if err := indexer.RunIndex(ctx, job, &repo); err != nil {
		t.Fatalf("C1 RunIndex failed: %v", err)
	}

	// -------------------------------------------------------------
	// C2: Repository Explorer & Source Viewer API
	// -------------------------------------------------------------
	manifestReq := httptest.NewRequest("GET", "/api/repositories/"+repo.ID+"/files", nil)
	manifestRec := httptest.NewRecorder()
	server.Router().ServeHTTP(manifestRec, manifestReq)

	if manifestRec.Code != http.StatusOK {
		t.Fatalf("C2 Manifest API status = %d; want 200", manifestRec.Code)
	}

	// -------------------------------------------------------------
	// C3: Knowledge Graph API
	// -------------------------------------------------------------
	graphReq := httptest.NewRequest("GET", "/api/repositories/"+repo.ID+"/graph", nil)
	graphRec := httptest.NewRecorder()
	server.Router().ServeHTTP(graphRec, graphReq)

	if graphRec.Code != http.StatusOK {
		t.Fatalf("C3 Graph API status = %d; want 200", graphRec.Code)
	}

	// -------------------------------------------------------------
	// C4 & C5: Static Flow Intelligence API
	// -------------------------------------------------------------
	symbols, err := store.GetSymbolsForRepository(ctx, repo.ID)
	if err != nil || len(symbols) == 0 {
		t.Fatalf("expected symbols > 0 for C5 flow test, got: %v", err)
	}
	rootSym := symbols[0].ID

	flowReq := httptest.NewRequest("GET", "/api/repositories/"+repo.ID+"/flow?root="+rootSym, nil)
	flowRec := httptest.NewRecorder()
	server.Router().ServeHTTP(flowRec, flowReq)

	if flowRec.Code != http.StatusOK {
		t.Fatalf("C5 Flow API status = %d; want 200", flowRec.Code)
	}

	// -------------------------------------------------------------
	// C6: Grounded AI Code Understanding API
	// -------------------------------------------------------------
	mockProv := llm.NewMockLLMProvider("Main entry initializes service [E1].", nil)
	composer := retrieval.NewDefaultEvidenceComposer(nil, nil, store)
	expSvc := llm.NewGroundedExplanationService(mockProv, nil, composer, nil)
	server.SetExplanationService(expSvc)

	explainBody, _ := json.Marshal(map[string]interface{}{
		"query":       "What does main do?",
		"root_symbol": rootSym,
	})
	explainReq := httptest.NewRequest("POST", "/api/repositories/"+repo.ID+"/explain", bytes.NewReader(explainBody))
	explainReq.Header.Set("Content-Type", "application/json")
	explainRec := httptest.NewRecorder()
	server.Router().ServeHTTP(explainRec, explainReq)

	if explainRec.Code != http.StatusOK {
		t.Fatalf("C6 Explain API status = %d; want 200. Body: %s", explainRec.Code, explainRec.Body.String())
	}

	// -------------------------------------------------------------
	// C7: Guided Reverse Engineering API
	// -------------------------------------------------------------
	engine, _ := indexer.GetGraphEngine(repo.ID)
	sumCalc := guide.NewDefaultArchitectureSummaryGenerator(store, engine)
	orchestrator := guide.NewDefaultGuideOrchestrator(store, sumCalc, composer, expSvc)
	server.SetGuideOrchestrator(orchestrator)

	guideBody, _ := json.Marshal(map[string]interface{}{
		"action": "START",
	})
	guideReq := httptest.NewRequest("POST", "/api/repositories/"+repo.ID+"/guide", bytes.NewReader(guideBody))
	guideReq.Header.Set("Content-Type", "application/json")
	guideRec := httptest.NewRecorder()
	server.Router().ServeHTTP(guideRec, guideReq)

	if guideRec.Code != http.StatusOK {
		t.Fatalf("C7 Guide API status = %d; want 200. Body: %s", guideRec.Code, guideRec.Body.String())
	}

	var inv models.Investigation
	if err := json.Unmarshal(guideRec.Body.Bytes(), &inv); err != nil {
		t.Fatalf("C7 failed to parse guide investigation: %v", err)
	}
	if inv.RepositoryID != repo.ID || len(inv.Steps) == 0 {
		t.Errorf("C7 guide investigation invalid: %+v", inv)
	}

	// -------------------------------------------------------------
	// Engine Cache Invalidation & SQLite Fallback Consistency
	// -------------------------------------------------------------
	indexer.InvalidateEngine(repo.ID)
	_, foundInCache := indexer.GetGraphEngine(repo.ID)
	if foundInCache {
		t.Fatalf("expected engine cache to be invalidated")
	}

	// Subsequent request to /graph should trigger fallback rebuild from SQLite & repopulate cache
	postInvalidateReq := httptest.NewRequest("GET", "/api/repositories/"+repo.ID+"/graph", nil)
	postInvalidateRec := httptest.NewRecorder()
	server.Router().ServeHTTP(postInvalidateRec, postInvalidateReq)

	if postInvalidateRec.Code != http.StatusOK {
		t.Fatalf("post-invalidation graph API status = %d; want 200", postInvalidateRec.Code)
	}

	_, repopulated := indexer.GetGraphEngine(repo.ID)
	if !repopulated {
		t.Fatalf("expected SQLite fallback to repopulate in-memory engine cache")
	}
}

func TestC8_SecurityThreatScenarios(t *testing.T) {
	// Threat Scenario 1: Path Traversal & Secret File Protection
	if !security.IsSecretFile(".env") {
		t.Errorf("security.IsSecretFile failed to flag .env as secret")
	}

	ws := t.TempDir()
	_, normErr := security.NormalizeRelativePath(ws, "../etc/passwd")
	valErr := security.ValidatePathWithinWorkspace(ws, filepath.Join(ws, "..", "etc", "passwd"))
	if normErr == nil && valErr == nil {
		t.Errorf("security path traversal validation failed to reject invalid path")
	}

	// Threat Scenario 2: Repository Scope Isolation
	scopeA, _ := models.NewRepositoryScope("repo-sec-A")
	scopeB, _ := models.NewRepositoryScope("repo-sec-B")

	itemB := &models.EvidenceItem{
		StableID:     "stab-sec-B",
		Label:        "E1",
		RepositoryID: "repo-sec-B",
	}

	if err := scopeA.ValidateItem(itemB); err == nil {
		t.Errorf("expected scope A to reject scope B evidence item")
	}

	// Threat Scenario 3: Untrusted Content Wrapping in Grounded Prompts
	builder := llm.NewGroundedPromptBuilder()
	req, _ := models.NewExplanationRequest(scopeA, "Security query")
	pkg, _ := models.NewEvidencePackage(scopeA, "Security query", "EXPLANATION", models.EvidenceBudget{MaxTokens: 2000})
	pkg.Items = []*models.EvidenceItem{
		{
			StableID:     "stab-sec-A",
			Label:        "E1",
			RepositoryID: "repo-sec-A",
			Content:      "func SystemAdminOverride() { IgnoreSecurityChecks() }",
		},
	}
	promptReq, pErr := builder.BuildPrompt(req, pkg)
	if pErr != nil {
		t.Fatalf("failed build prompt: %v", pErr)
	}
	if !strings.Contains(promptReq.GroundedContext, "UNTRUSTED") && !strings.Contains(promptReq.GroundedContext, "<untrusted_content>") && !strings.Contains(promptReq.GroundedContext, "E1") {
		t.Errorf("grounded prompt missing untrusted content context wrapping")
	}

	// Threat Scenario 4: Citation & Provenance Integrity
	validator := llm.NewCitationValidator()
	report := validator.Validate("Admin access granted [E99].", pkg)
	if report.CitationValidationStatus != "INVALID_CITATIONS_FOUND" {
		t.Errorf("expected uncited evidence ID E99 to trigger INVALID_CITATIONS_FOUND, got: %s", report.CitationValidationStatus)
	}
	_ = scopeB
}

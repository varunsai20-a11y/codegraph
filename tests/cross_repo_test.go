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
	"codegraph/internal/ingestion"
	"codegraph/internal/llm"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/retrieval"
	"codegraph/internal/storage"
)

func TestCrossRepositoryEvaluationMatrix(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "cross_repo_test.db")
	wsRoot := filepath.Join(tmpDir, "workspaces")

	cfg := &config.Config{
		Port:               8080,
		MaxFileSize:        2 * 1024 * 1024,
		WorkspaceRoot:      wsRoot,
		DatabasePath:       dbPath,
		DefaultExclusions:  []string{".git"},
		SupportedLanguages: []string{"Go", "Python", "TypeScript", "Java"},
	}

	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize test SQLite storage: %v", err)
	}
	defer store.Close()

	wsMgr, err := repository.NewWorkspaceManager(wsRoot)
	if err != nil {
		t.Fatalf("Failed to initialize workspace manager: %v", err)
	}

	indexer := ingestion.NewIndexer(cfg, store, wsMgr)
	server := api.NewServer(cfg, store, wsMgr, indexer)

	lexRetriever := retrieval.NewLexicalRetriever(store, wsMgr)
	composer := retrieval.NewDefaultEvidenceComposer(lexRetriever, nil, store)
	synthProvider := llm.NewGroundedSynthesisProvider()
	expSvc := llm.NewGroundedExplanationService(synthProvider, nil, composer, nil)
	server.SetExplanationService(expSvc)

	ctx := context.Background()

	// 1. Setup Repo A: Go Repository (CodeGraph subset)
	repoAPath := filepath.Join(tmpDir, "repo_a_go")
	if err := os.MkdirAll(filepath.Join(repoAPath, "cmd"), 0755); err != nil {
		t.Fatalf("Failed to create repo A dir: %v", err)
	}
	_ = os.WriteFile(filepath.Join(repoAPath, "README.md"), []byte("# CodeGraph Engine\nCodeGraph is a code intelligence and graph retrieval engine written in Go.\nIt indexes local repositories and constructs dependency graphs."), 0644)
	_ = os.WriteFile(filepath.Join(repoAPath, "cmd", "main.go"), []byte("package main\nimport \"fmt\"\nfunc main() {\n\tfmt.Println(\"Starting CodeGraph server\")\n\tRunServer()\n}\nfunc RunServer() {\n\tfmt.Println(\"Server running\")\n}"), 0644)

	regBodyA, _ := json.Marshal(api.RegisterRepoRequest{
		Name:       "CodeGraphEngine",
		SourceType: models.SourceTypeLocal,
		LocalPath:  repoAPath,
	})
	reqRegA := httptest.NewRequest("POST", "/api/repositories", bytes.NewReader(regBodyA))
	recRegA := httptest.NewRecorder()
	server.Router().ServeHTTP(recRegA, reqRegA)
	if recRegA.Code != http.StatusCreated {
		t.Fatalf("Register Repo A status = %d", recRegA.Code)
	}
	var repoA models.Repository
	_ = json.Unmarshal(recRegA.Body.Bytes(), &repoA)

	jobA := &models.IndexJob{ID: "job-cross-a", RepositoryID: repoA.ID, Status: models.JobStatusPending, StartedAt: time.Now()}
	_ = store.CreateIndexJob(ctx, jobA)
	if err := indexer.RunIndex(ctx, jobA, &repoA); err != nil {
		t.Fatalf("Indexing Repo A failed: %v", err)
	}

	// 2. Setup Repo B: Python Repository (Multi-Agent subset)
	repoBPath := filepath.Join(tmpDir, "repo_b_python")
	if err := os.MkdirAll(repoBPath, 0755); err != nil {
		t.Fatalf("Failed to create repo B dir: %v", err)
	}
	_ = os.WriteFile(filepath.Join(repoBPath, "README.md"), []byte("# Multi Agent Researcher\nMulti Agent Researcher is an AI agent research assistant using CrewAI and Python.\nIt orchestrates automated web research using LLM worker agents."), 0644)
	_ = os.WriteFile(filepath.Join(repoBPath, "run.py"), []byte("import os\ndef ensure_environment():\n    check_ollama()\ndef check_ollama():\n    print(\"Ollama checked\")\ndef main():\n    ensure_environment()\nif __name__ == '__main__':\n    main()"), 0644)

	regBodyB, _ := json.Marshal(api.RegisterRepoRequest{
		Name:       "MultiAgentResearcher",
		SourceType: models.SourceTypeLocal,
		LocalPath:  repoBPath,
	})
	reqRegB := httptest.NewRequest("POST", "/api/repositories", bytes.NewReader(regBodyB))
	recRegB := httptest.NewRecorder()
	server.Router().ServeHTTP(recRegB, reqRegB)
	if recRegB.Code != http.StatusCreated {
		t.Fatalf("Register Repo B status = %d", recRegB.Code)
	}
	var repoB models.Repository
	_ = json.Unmarshal(recRegB.Body.Bytes(), &repoB)

	jobB := &models.IndexJob{ID: "job-cross-b", RepositoryID: repoB.ID, Status: models.JobStatusPending, StartedAt: time.Now()}
	_ = store.CreateIndexJob(ctx, jobB)
	if err := indexer.RunIndex(ctx, jobB, &repoB); err != nil {
		t.Fatalf("Indexing Repo B failed: %v", err)
	}

	// 3. Setup Repo C: SampleRepository (Java / JS Fixture)
	sampleFixturePath, err := filepath.Abs("fixtures/sample-repository")
	if err != nil {
		t.Fatalf("Failed to resolve sample fixture path: %v", err)
	}

	regBodyC, _ := json.Marshal(api.RegisterRepoRequest{
		Name:       "SampleRepositoryFixture",
		SourceType: models.SourceTypeLocal,
		LocalPath:  sampleFixturePath,
	})
	reqRegC := httptest.NewRequest("POST", "/api/repositories", bytes.NewReader(regBodyC))
	recRegC := httptest.NewRecorder()
	server.Router().ServeHTTP(recRegC, reqRegC)
	if recRegC.Code != http.StatusCreated {
		t.Fatalf("Register Repo C status = %d", recRegC.Code)
	}
	var repoC models.Repository
	_ = json.Unmarshal(recRegC.Body.Bytes(), &repoC)

	jobC := &models.IndexJob{ID: "job-cross-c", RepositoryID: repoC.ID, Status: models.JobStatusPending, StartedAt: time.Now()}
	_ = store.CreateIndexJob(ctx, jobC)
	if err := indexer.RunIndex(ctx, jobC, &repoC); err != nil {
		t.Fatalf("Indexing Repo C failed: %v", err)
	}

	// Helper function to query explain API
	explainQuery := func(repoID string, q string) *models.ExplanationResponse {
		body, _ := json.Marshal(map[string]interface{}{"query": q})
		httpReq := httptest.NewRequest("POST", "/api/repositories/"+repoID+"/explain", bytes.NewReader(body))
		httpReq.Header.Set("Content-Type", "application/json")
		httpRec := httptest.NewRecorder()
		server.Router().ServeHTTP(httpRec, httpReq)
		var resp models.ExplanationResponse
		_ = json.Unmarshal(httpRec.Body.Bytes(), &resp)
		return &resp
	}

	// -------------------------------------------------------------
	// MATRIX TEST: REPOSITORY A (Go Repo - CodeGraph Engine)
	// -------------------------------------------------------------
	respA1 := explainQuery(repoA.ID, "What does this project do?")
	if respA1.Status != models.ExplanationStatusSuccess || respA1.IsInsufficientEvidence {
		t.Errorf("Repo A Q1 failed: expected SUCCESS, got status=%s", respA1.Status)
	}

	respA2 := explainQuery(repoA.ID, "Explain the RunServer component based on its source")
	if respA2.Status != models.ExplanationStatusSuccess || len(respA2.Evidence) == 0 {
		t.Errorf("Repo A Q2 failed: expected component evidence")
	}

	respA3 := explainQuery(repoA.ID, "Explain the execution flow in main")
	if respA3.Status != models.ExplanationStatusSuccess || len(respA3.Evidence) == 0 {
		t.Errorf("Repo A Q3 failed: expected execution flow evidence")
	}

	respA4 := explainQuery(repoA.ID, "Where is the implementation of a chocolate cake recipe?")
	if !respA4.IsInsufficientEvidence || respA4.Status != models.ExplanationStatusInsufficientEvidence {
		t.Errorf("Repo A Q4 failed: expected INSUFFICIENT_EVIDENCE, got status=%s", respA4.Status)
	}

	// -------------------------------------------------------------
	// MATRIX TEST: REPOSITORY B (Python Repo - MultiAgentResearcher)
	// -------------------------------------------------------------
	respB1 := explainQuery(repoB.ID, "What does this project do?")
	if respB1.Status != models.ExplanationStatusSuccess || respB1.IsInsufficientEvidence {
		t.Errorf("Repo B Q1 failed: expected SUCCESS, got status=%s", respB1.Status)
	}

	respB2 := explainQuery(repoB.ID, "Explain the ensure_environment component based on its source")
	if respB2.Status != models.ExplanationStatusSuccess || len(respB2.Evidence) == 0 {
		t.Errorf("Repo B Q2 failed: expected component evidence")
	}

	respB3 := explainQuery(repoB.ID, "Explain the execution flow in main")
	if respB3.Status != models.ExplanationStatusSuccess || len(respB3.Evidence) == 0 {
		t.Errorf("Repo B Q3 failed: expected execution flow evidence")
	}

	respB4 := explainQuery(repoB.ID, "Where is the quantum physics simulator?")
	if !respB4.IsInsufficientEvidence || respB4.Status != models.ExplanationStatusInsufficientEvidence {
		t.Errorf("Repo B Q4 failed: expected INSUFFICIENT_EVIDENCE, got status=%s", respB4.Status)
	}

	// -------------------------------------------------------------
	// MATRIX TEST: REPOSITORY C (Java/JS Fixture - SampleRepository)
	// -------------------------------------------------------------
	respC1 := explainQuery(repoC.ID, "What does this project do?")
	if respC1.Status != models.ExplanationStatusSuccess || respC1.IsInsufficientEvidence {
		t.Errorf("Repo C Q1 failed: expected SUCCESS, got status=%s", respC1.Status)
	}

	respC2 := explainQuery(repoC.ID, "Explain the App class based on its source")
	if respC2.Status != models.ExplanationStatusSuccess || len(respC2.Evidence) == 0 {
		t.Errorf("Repo C Q2 failed: expected component evidence")
	}

	respC3 := explainQuery(repoC.ID, "Explain the execution flow in main")
	if respC3.Status != models.ExplanationStatusSuccess || len(respC3.Evidence) == 0 {
		t.Errorf("Repo C Q3 failed: expected execution flow evidence")
	}

	respC4 := explainQuery(repoC.ID, "Where is the database migration script for user sessions?")
	if !respC4.IsInsufficientEvidence || respC4.Status != models.ExplanationStatusInsufficientEvidence {
		t.Errorf("Repo C Q4 failed: expected INSUFFICIENT_EVIDENCE, got status=%s", respC4.Status)
	}

	// -------------------------------------------------------------
	// REPOSITORY ISOLATION CHECK
	// -------------------------------------------------------------
	for _, ev := range respA1.Evidence {
		if ev.RepositoryID != repoA.ID {
			t.Errorf("Cross-repository isolation leak! Repo A evidence has repository_id = %s", ev.RepositoryID)
		}
	}
	for _, ev := range respB1.Evidence {
		if ev.RepositoryID != repoB.ID {
			t.Errorf("Cross-repository isolation leak! Repo B evidence has repository_id = %s", ev.RepositoryID)
		}
	}
	for _, ev := range respC1.Evidence {
		if ev.RepositoryID != repoC.ID {
			t.Errorf("Cross-repository isolation leak! Repo C evidence has repository_id = %s", ev.RepositoryID)
		}
	}
}

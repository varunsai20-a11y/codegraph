package evaluation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"codegraph/internal/api"
	"codegraph/internal/config"
	"codegraph/internal/ingestion"
	"codegraph/internal/llm"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/retrieval"
	"codegraph/internal/storage"
)

type mockProductionLLM struct {
	invoked bool
	lastReq llm.LLMRequest
}

func (m *mockProductionLLM) Name() string  { return "ollama" }
func (m *mockProductionLLM) Model() string { return "qwen2.5:3b" }
func (m *mockProductionLLM) Generate(ctx context.Context, req llm.LLMRequest) (*llm.LLMResponse, error) {
	m.invoked = true
	m.lastReq = req
	return &llm.LLMResponse{
		Content:  "The project is a Web Application designed to process user requests via API entry points [E1]. Primary components manage graph indexing and data storage [E2].\n\nEvidence:\n[E1] src/index.ts: Application entry point.\n[E2] package.json: Project configuration.",
		Provider: "ollama",
		Model:    "qwen2.5:3b",
	}, nil
}

func TestProductionUIExplainPipeline(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}
	defer store.Close()

	// Create sample project workspace structure
	sampleRepoPath := filepath.Join(tmpDir, "sample-app")
	_ = os.MkdirAll(filepath.Join(sampleRepoPath, "src"), 0755)
	_ = os.MkdirAll(filepath.Join(sampleRepoPath, "docs", "archive"), 0755)

	_ = os.WriteFile(filepath.Join(sampleRepoPath, "README.md"), []byte("# Sample Web App\nThis app provides API endpoints and graph storage routines."), 0644)
	_ = os.WriteFile(filepath.Join(sampleRepoPath, "package.json"), []byte(`{"name":"sample-app","version":"1.0.0","dependencies":{"express":"^4.18.2"}}`), 0644)
	_ = os.WriteFile(filepath.Join(sampleRepoPath, "src", "index.ts"), []byte("import express from 'express';\nconst app = express();\napp.listen(3000);"), 0644)
	_ = os.WriteFile(filepath.Join(sampleRepoPath, "docs", "archive", "old-plan.md"), []byte("# Old Planning Document\nArchived notes from 2023."), 0644)

	cfg := &config.Config{
		Port:          8080,
		WorkspaceRoot: tmpDir,
		DatabasePath:  dbPath,
	}

	wsMgr, err := repository.NewWorkspaceManager(tmpDir)
	if err != nil {
		t.Fatalf("failed to init wsMgr: %v", err)
	}

	indexer := ingestion.NewIndexer(cfg, store, wsMgr)
	server := api.NewServer(cfg, store, wsMgr, indexer)

	mockLLM := &mockProductionLLM{}

	lexRetriever := retrieval.NewLexicalRetriever(store, wsMgr)
	graphRetriever := retrieval.NewGraphRetriever(store)
	hybridEngine := retrieval.NewHybridRetrieverEngine(nil, lexRetriever, nil, graphRetriever, retrieval.DefaultHybridRetrievalConfig())
	hybridAdapter := retrieval.NewHybridRetrieverAdapter(hybridEngine)

	composer := retrieval.NewDefaultEvidenceComposer(hybridAdapter, nil, store)
	expSvc := llm.NewGroundedExplanationService(mockLLM, nil, composer, nil)
	server.SetExplanationService(expSvc)

	ctx := context.Background()

	// 1. Register Repo
	repo := &models.Repository{
		ID:         "prod-test-repo-1",
		Name:       "sample-app",
		SourceType: models.SourceTypeLocal,
		LocalPath:  sampleRepoPath,
		Status:     models.RepoStatusRegistered,
	}
	if err := store.CreateRepository(ctx, repo); err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	// 2. Index Repo
	job := &models.IndexJob{
		ID:           "job-1",
		RepositoryID: repo.ID,
		Status:       models.JobStatusPending,
	}
	_ = store.CreateIndexJob(ctx, job)
	err = indexer.RunIndex(ctx, job, repo)
	if err != nil {
		t.Fatalf("indexing failed: %v", err)
	}

	testCases := []struct {
		query          string
		expectedIntent string
		expectSuff     models.SufficiencyStatus
	}{
		{
			query:          "What does this project do?",
			expectedIntent: retrieval.IntentRepoOverview,
			expectSuff:     models.SufficiencySufficient,
		},
		{
			query:          "Explain the architecture.",
			expectedIntent: retrieval.IntentRepoOverview,
			expectSuff:     models.SufficiencySufficient,
		},
		{
			query:          "Explain this project to a beginner.",
			expectedIntent: retrieval.IntentRepoOverview,
			expectSuff:     models.SufficiencySufficient,
		},
		{
			query:          "Trace the request flow.",
			expectedIntent: retrieval.IntentTrace,
			expectSuff:     models.SufficiencySufficient,
		},
		{
			query:          "Explain package.json.",
			expectedIntent: retrieval.IntentFileQuery,
			expectSuff:     models.SufficiencySufficient,
		},
	}

	router := server.Router()

	for i, tc := range testCases {
		mockLLM.invoked = false
		bodyMap := map[string]interface{}{
			"query": tc.query,
		}
		bodyBytes, _ := json.Marshal(bodyMap)

		req := httptest.NewRequest("POST", fmt.Sprintf("/api/repositories/%s/explain", repo.ID), bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("case %d (%s): HTTP status = %d, response = %s", i+1, tc.query, rec.Code, rec.Body.String())
		}

		var resp models.ExplanationResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("case %d (%s): json decode failed: %v", i+1, tc.query, err)
		}

		t.Logf("Query [%d]: '%s' | Sufficiency: %s | LLM Invoked: %v | Evidence Items: %d", i+1, tc.query, resp.Sufficiency.Status, mockLLM.invoked, len(resp.Evidence))

		if resp.Sufficiency.Status != tc.expectSuff {
			t.Errorf("case %d (%s): expected sufficiency %s, got %s (reasons: %v)", i+1, tc.query, tc.expectSuff, resp.Sufficiency.Status, resp.Sufficiency.Reasons)
		}

		if !mockLLM.invoked {
			t.Errorf("case %d (%s): expected LLM provider to be invoked, but it was NOT", i+1, tc.query)
		}

		// Verify archived docs are NOT returned as primary evidence
		for _, ev := range resp.Evidence {
			if ev.RelativePath == "docs/archive/old-plan.md" {
				t.Errorf("case %d (%s): unexpected archived doc 'docs/archive/old-plan.md' in evidence", i+1, tc.query)
			}
		}
	}
}

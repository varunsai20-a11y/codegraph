package evaluation

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"codegraph/internal/config"
	"codegraph/internal/ingestion"
	"codegraph/internal/llm"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/retrieval"
	"codegraph/internal/storage"
)

func TestRealLocalOllamaSmokeAndComparison(t *testing.T) {
	// Check if local Ollama is reachable
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil || resp.StatusCode != 200 {
		t.Skip("Local Ollama endpoint http://localhost:11434 is not running. Skipping live smoke test.")
		return
	}
	resp.Body.Close()

	// 1. Setup workspace & index sample repository
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "ollama_smoke.db")
	wsPath := filepath.Join(tmpDir, "workspaces")

	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}
	defer store.Close()

	wsMgr, err := repository.NewWorkspaceManager(wsPath)
	if err != nil {
		t.Fatalf("failed to init wsMgr: %v", err)
	}

	cfg := config.Load()
	idx := ingestion.NewIndexer(cfg, store, wsMgr)

	repoPath, _ := filepath.Abs("../..")
	repo := &models.Repository{
		ID:         "real-ollama-smoke-repo",
		Name:       "codegraph",
		SourceType: models.SourceTypeLocal,
		LocalPath:  repoPath,
		Status:     models.RepoStatusRegistered,
	}
	if err := store.CreateRepository(context.Background(), repo); err != nil {
		t.Fatalf("failed to register repo: %v", err)
	}

	job := &models.IndexJob{
		ID:           "job-ollama-smoke",
		RepositoryID: repo.ID,
		Status:       models.JobStatusPending,
	}
	if err := store.CreateIndexJob(context.Background(), job); err != nil {
		t.Fatalf("failed to create index job: %v", err)
	}

	if err := idx.RunIndex(context.Background(), job, repo); err != nil {
		t.Fatalf("RunIndex failed: %v", err)
	}

	manifest, _ := store.GetManifestForRepository(context.Background(), repo.ID)
	t.Logf("Indexed manifest count for repo %s: %d files", repo.ID, len(manifest))
	for _, m := range manifest {
		t.Logf(" - File: %s (status: %s)", m.RelativePath, m.Status)
	}

	// 2. Setup Ollama Provider
	ollamaProv, err := llm.NewHTTPLLMProvider(llm.LLMConfig{
		Provider: "ollama",
		Model:    "qwen2.5:3b",
		Endpoint: "http://localhost:11434/api/chat",
		Timeout:  45 * time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("failed to create Ollama provider: %v", err)
	}

	lex := retrieval.NewLexicalRetriever(store, wsMgr)
	graphR := retrieval.NewGraphRetriever(store)
	hybridEngine := retrieval.NewHybridRetrieverEngine(nil, lex, nil, graphR, retrieval.DefaultHybridRetrievalConfig())
	hybridAdapter := retrieval.NewHybridRetrieverAdapter(hybridEngine)
	composer := retrieval.NewDefaultEvidenceComposer(hybridAdapter, nil, store)

	expSvc := llm.NewGroundedExplanationService(ollamaProv, nil, composer, nil)
	scope := models.RepositoryScope{RepositoryID: repo.ID}

	queries := []string{
		"What does this project do?",
		"Explain this project like I'm a beginner.",
		"Explain package.json.",
		"Explain src/index.ts.",
		"Trace the request flow.",
		"Why does this project use SQLite?",
	}

	t.Logf("=== RUNNING REAL OLLAMA (qwen2.5:3b) SMOKE TEST ON 6 QUERIES ===")

	for idx, q := range queries {
		start := time.Now()
		req := &models.ExplanationRequest{
			RepositoryScope: scope,
			Question:        q,
		}

		res, err := expSvc.ExplainRequest(context.Background(), req)
		dur := time.Since(start)

		if err != nil {
			t.Errorf("Query %d (%s) failed: %v", idx+1, q, err)
			continue
		}

		t.Logf("\n--- Query %d: \"%s\" ---", idx+1, q)
		t.Logf("Provider: %s | Model: %s | Mode: %s | Latency: %v", res.Provider, res.Model, res.ProviderMode, dur)
		t.Logf("Grounding Status: %s | Valid Citations: %d | Invalid: %d", res.Grounding.Status, res.Grounding.CitedEvidenceCount, res.Grounding.UnsupportedClaimCount)
		t.Logf("Answer Preview (first 250 chars):\n%s", truncateStr(res.Answer, 250))

		if res.ProviderMode != "LOCAL_LLM" {
			t.Errorf("expected provider_mode = LOCAL_LLM, got %s", res.ProviderMode)
		}
		if res.Provider != "ollama" {
			t.Errorf("expected provider = ollama, got %s", res.Provider)
		}
	}
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

package evaluation_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
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

func TestMultiRepoRequestFlowAndQuality(t *testing.T) {
	// Check if Ollama is running
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil || resp.StatusCode != 200 {
		t.Skip("Local Ollama server is not running on http://localhost:11434")
		return
	}
	resp.Body.Close()

	targetRepos := []struct {
		ID            string
		Name          string
		Path          string
		HasWebFlow    bool
	}{
		{
			ID:         "worldmonitor",
			Name:       "WorldMonitor",
			Path:       `c:\Users\varun\codegraph\_workspaces\112684e1-3c95-43cd-aed2-163a4df0de1b`,
			HasWebFlow: true,
		},
		{
			ID:         "varuns-portfolio",
			Name:       "varuns-portfolio",
			Path:       `c:\Users\varun\codegraph\_workspaces\78ebba81-1d1d-47e3-bfd6-3c482c809112`,
			HasWebFlow: true,
		},
		{
			ID:         "fastapi",
			Name:       "FastAPI",
			Path:       `c:\Users\varun\codegraph\_workspaces\fastapi`,
			HasWebFlow: true,
		},
		{
			ID:         "gin",
			Name:       "Gin",
			Path:       `c:\Users\varun\codegraph\_workspaces\gin`,
			HasWebFlow: true,
		},
		{
			ID:         "ripgrep",
			Name:       "ripgrep",
			Path:       `c:\Users\varun\codegraph\_workspaces\ripgrep`,
			HasWebFlow: false, // Pure CLI tool, no web HTTP request flow
		},
	}

	for _, repo := range targetRepos {
		if _, err := os.Stat(repo.Path); err != nil {
			t.Fatalf("Workspace path for %s not found: %s", repo.Name, repo.Path)
		}
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "flow_eval.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to init SQLite storage: %v", err)
	}
	defer store.Close()

	wsMgr, err := repository.NewWorkspaceManager(filepath.Join(tmpDir, "workspaces"))
	if err != nil {
		t.Fatalf("failed to init workspace manager: %v", err)
	}

	cfg := &config.Config{
		DefaultExclusions: []string{".git", "node_modules", "dist", "build", "coverage", ".cache", ".tmp", "vendor"},
		MaxFileSize:       1 * 1024 * 1024,
	}

	idx := ingestion.NewIndexer(cfg, store, wsMgr)
	ctx := context.Background()

	// Index all 5 repositories
	for _, r := range targetRepos {
		repoModel := &models.Repository{
			ID:         r.ID,
			Name:       r.Name,
			SourceType: models.SourceTypeLocal,
			LocalPath:  r.Path,
			Status:     models.RepoStatusRegistered,
		}
		if err := store.CreateRepository(ctx, repoModel); err != nil {
			t.Fatalf("failed to register %s: %v", r.Name, err)
		}
		job := &models.IndexJob{ID: "job-" + r.ID, RepositoryID: r.ID, Status: models.JobStatusPending}
		if err := store.CreateIndexJob(ctx, job); err != nil {
			t.Fatalf("failed to create index job for %s: %v", r.Name, err)
		}
		if err := idx.RunIndex(ctx, job, repoModel); err != nil {
			t.Fatalf("RunIndex %s failed: %v", r.Name, err)
		}
	}

	// Setup Ollama provider with 120s timeout
	ollamaProv, err := llm.NewHTTPLLMProvider(llm.LLMConfig{
		Provider: "ollama",
		Model:    "qwen2.5:3b",
		Endpoint: "http://localhost:11434/api/chat",
		Timeout:  120 * time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("failed to init Ollama provider: %v", err)
	}

	lexRetriever := retrieval.NewLexicalRetriever(store, wsMgr)
	graphRetriever := retrieval.NewGraphRetriever(store)
	hybridEngine := retrieval.NewHybridRetrieverEngine(nil, lexRetriever, nil, graphRetriever, retrieval.DefaultHybridRetrievalConfig())
	hybridAdapter := retrieval.NewHybridRetrieverAdapter(hybridEngine)
	composer := retrieval.NewDefaultEvidenceComposer(hybridAdapter, nil, store)

	promptBuilder := llm.NewGroundedPromptBuilder()
	validator := llm.NewCitationValidator()
	svc := llm.NewGroundedExplanationService(ollamaProv, validator, composer, promptBuilder)

	queries := []struct {
		Category string
		Query    string
	}{
		{"A. Request Flow", "How does a request flow through this repository?"},
		{"B. Architecture", "Explain the architecture of this repository."},
		{"C. Function", "Explain the main function or entry point."},
		{"D. Unsupported", "Why does this project use AWS DynamoDB?"},
	}

	for _, r := range targetRepos {
		scope, _ := models.NewRepositoryScope(r.ID)
		t.Logf("\n==================================================")
		t.Logf("=== TESTING REPOSITORY: %s (HasWebFlow: %v) ===", r.Name, r.HasWebFlow)
		t.Logf("==================================================")

		for qIdx, qItem := range queries {
			start := time.Now()
			expReq, _ := models.NewExplanationRequest(scope, qItem.Query)
			resp, err := svc.ExplainRequest(ctx, expReq)
			dur := time.Since(start)

			if err != nil {
				t.Errorf("[%s] Query [%s] '%s' failed: %v", r.Name, qItem.Category, qItem.Query, err)
				continue
			}

			fmt.Printf("\n--------------------------------------------------\n")
			fmt.Printf("[%s] %s: \"%s\"\n", r.Name, qItem.Category, qItem.Query)
			fmt.Printf("Latency: %v | ProviderMode: '%s' | Sufficiency: %s | Grounding: %s | Citations Valid: %d | Invalid: %d\n",
				dur, resp.ProviderMode, resp.Sufficiency.Status, resp.Grounding.Status, resp.Grounding.CitedEvidenceCount, resp.Grounding.UnsupportedClaimCount)
			fmt.Printf("--- FULL ANSWER ---\n%s\n--- END ANSWER ---\n", resp.Answer)

			if qIdx == 0 { // A. Request Flow
				if r.HasWebFlow {
					if resp.Sufficiency.Status != models.SufficiencySufficient {
						t.Errorf("[%s] Expected SufficiencySufficient for Request Flow query, got %s", r.Name, resp.Sufficiency.Status)
					}
					if resp.ProviderMode != "LOCAL_LLM" {
						t.Errorf("[%s] Expected ProviderMode LOCAL_LLM, got %s", r.Name, resp.ProviderMode)
					}
				} else {
					// ripgrep has no web HTTP request flow, so sufficiency must return SUFFICIENCY_INSUFFICIENT
					if resp.Sufficiency.Status != models.SufficiencyInsufficient && resp.Grounding.Status != models.GroundingInsufficientEvidence {
						t.Errorf("[%s] Expected SufficiencyInsufficient for ripgrep request flow query, got sufficiency %s, grounding %s",
							r.Name, resp.Sufficiency.Status, resp.Grounding.Status)
					}
				}
			} else if qIdx == 3 { // D. Unsupported query
				if resp.Sufficiency.Status != models.SufficiencyInsufficient && resp.Grounding.Status != models.GroundingInsufficientEvidence {
					t.Errorf("[%s] Expected SufficiencyInsufficient for unsupported query, got %s", r.Name, resp.Sufficiency.Status)
				}
			}
		}
	}
}

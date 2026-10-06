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

func TestQwenExplanationQualityBothRepos(t *testing.T) {
	// Check if Ollama is running
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil || resp.StatusCode != 200 {
		t.Skip("Local Ollama server is not running on http://localhost:11434")
		return
	}
	resp.Body.Close()

	worldMonitorPath := `c:\Users\varun\codegraph\_workspaces\112684e1-3c95-43cd-aed2-163a4df0de1b`
	portfolioPath := `c:\Users\varun\codegraph\_workspaces\78ebba81-1d1d-47e3-bfd6-3c482c809112`

	if _, err := os.Stat(worldMonitorPath); err != nil {
		t.Fatalf("WorldMonitor workspace path not found: %s", worldMonitorPath)
	}
	if _, err := os.Stat(portfolioPath); err != nil {
		t.Fatalf("varuns-portfolio workspace path not found: %s", portfolioPath)
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "qwen_eval.db")
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

	// Index WorldMonitor
	wmRepo := &models.Repository{
		ID:         "worldmonitor",
		Name:       "WorldMonitor",
		SourceType: models.SourceTypeLocal,
		LocalPath:  worldMonitorPath,
		Status:     models.RepoStatusRegistered,
	}
	if err := store.CreateRepository(ctx, wmRepo); err != nil {
		t.Fatalf("failed to register WorldMonitor: %v", err)
	}
	wmJob := &models.IndexJob{ID: "job-wm", RepositoryID: wmRepo.ID, Status: models.JobStatusPending}
	if err := store.CreateIndexJob(ctx, wmJob); err != nil {
		t.Fatalf("failed to create job wm: %v", err)
	}
	if err := idx.RunIndex(ctx, wmJob, wmRepo); err != nil {
		t.Fatalf("RunIndex WorldMonitor failed: %v", err)
	}

	// Index varuns-portfolio
	pfRepo := &models.Repository{
		ID:         "varuns-portfolio",
		Name:       "varuns-portfolio",
		SourceType: models.SourceTypeLocal,
		LocalPath:  portfolioPath,
		Status:     models.RepoStatusRegistered,
	}
	if err := store.CreateRepository(ctx, pfRepo); err != nil {
		t.Fatalf("failed to register portfolio: %v", err)
	}
	pfJob := &models.IndexJob{ID: "job-pf", RepositoryID: pfRepo.ID, Status: models.JobStatusPending}
	if err := store.CreateIndexJob(ctx, pfJob); err != nil {
		t.Fatalf("failed to create job pf: %v", err)
	}
	if err := idx.RunIndex(ctx, pfJob, pfRepo); err != nil {
		t.Fatalf("RunIndex portfolio failed: %v", err)
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

	targetQueries := []string{
		"Explain the main function or entry point.", // A. Function
		"What does package.json do?",                 // B. File
		"How does a request flow through this repository?", // C. Request flow
		"Explain the architecture of this repository.",     // D. Architecture
		"Why does this project use AWS DynamoDB?",          // E. Unsupported
	}

	reposToTest := []struct {
		ID   string
		Name string
	}{
		{"worldmonitor", "WorldMonitor"},
		{"varuns-portfolio", "varuns-portfolio"},
	}

	for _, r := range reposToTest {
		scope, _ := models.NewRepositoryScope(r.ID)
		t.Logf("\n==================================================")
		t.Logf("=== TESTING REPOSITORY: %s ===", r.Name)
		t.Logf("==================================================")

		for idx, q := range targetQueries {
			start := time.Now()
			expReq, _ := models.NewExplanationRequest(scope, q)
			resp, err := svc.ExplainRequest(ctx, expReq)
			dur := time.Since(start)

			if err != nil {
				t.Errorf("[%s] Query #%d '%s' failed: %v", r.Name, idx+1, q, err)
				continue
			}

			fmt.Printf("\n--------------------------------------------------\n")
			fmt.Printf("[%s] Q%d: \"%s\"\n", r.Name, idx+1, q)
			fmt.Printf("Latency: %v | ProviderMode: %s | Citations Valid: %d | Invalid: %d | Status: %s\n",
				dur, resp.ProviderMode, resp.Grounding.CitedEvidenceCount, resp.Grounding.UnsupportedClaimCount, resp.Grounding.Status)
			fmt.Printf("--- FULL ANSWER ---\n%s\n--- END ANSWER ---\n", resp.Answer)

			if idx == 4 { // E. Unsupported query
				if resp.Sufficiency.Status != models.SufficiencyInsufficient && resp.Grounding.Status != models.GroundingInsufficientEvidence {
					t.Errorf("[%s] Q%d expected SUFFICIENCY_INSUFFICIENT, got sufficiency %s, grounding %s", r.Name, idx+1, resp.Sufficiency.Status, resp.Grounding.Status)
				}
			} else if resp.Sufficiency.Status == models.SufficiencyInsufficient {
				t.Logf("[%s] Q%d '%s' returned SUFFICIENCY_INSUFFICIENT (no Ollama invocation needed)", r.Name, idx+1, q)
			} else {
				if resp.ProviderMode != "LOCAL_LLM" {
					t.Errorf("[%s] expected LOCAL_LLM, got %s", r.Name, resp.ProviderMode)
				}
			}
		}
	}
}

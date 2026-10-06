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

type TestQueryResult struct {
	RepoID          string
	QueryIndex      int
	Question        string
	Latency         time.Duration
	Provider        string
	Model           string
	ProviderMode    string
	GroundingStatus string
	ValidCitations  int
	InvalidCitations int
	Answer          string
}

func TestFinalEndToEndAcceptanceMatrix(t *testing.T) {
	// Check if local Ollama server is running
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil || resp.StatusCode != 200 {
		t.Skip("Local Ollama endpoint http://localhost:11434 is not reachable. Skipping live acceptance matrix.")
		return
	}
	resp.Body.Close()

	worldMonitorPath := `c:\Users\varun\codegraph\_workspaces\112684e1-3c95-43cd-aed2-163a4df0de1b`
	portfolioPath := `c:\Users\varun\codegraph\_workspaces\78ebba81-1d1d-47e3-bfd6-3c482c809112`

	if _, err := os.Stat(worldMonitorPath); err != nil {
		t.Fatalf("WorldMonitor workspace path missing: %s", worldMonitorPath)
	}
	if _, err := os.Stat(portfolioPath); err != nil {
		t.Fatalf("varuns-portfolio workspace path missing: %s", portfolioPath)
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "final_acceptance.db")

	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
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

	// 1. Register and Index WorldMonitor
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
		t.Fatalf("failed to create wm job: %v", err)
	}
	startWMIndex := time.Now()
	if err := idx.RunIndex(ctx, wmJob, wmRepo); err != nil {
		t.Fatalf("RunIndex WorldMonitor failed: %v", err)
	}
	wmIndexDur := time.Since(startWMIndex)

	// 2. Register and Index varuns-portfolio
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
		t.Fatalf("failed to create pf job: %v", err)
	}
	startPFIndex := time.Now()
	if err := idx.RunIndex(ctx, pfJob, pfRepo); err != nil {
		t.Fatalf("RunIndex portfolio failed: %v", err)
	}
	pfIndexDur := time.Since(startPFIndex)

	// Fetch Stats for verification
	wmManifest, _ := store.GetManifestForRepository(ctx, wmRepo.ID)
	wmSymbols, _ := store.GetSymbolsForRepository(ctx, wmRepo.ID)
	wmRels, _ := store.GetRelationshipsForRepository(ctx, wmRepo.ID)

	pfManifest, _ := store.GetManifestForRepository(ctx, pfRepo.ID)
	pfSymbols, _ := store.GetSymbolsForRepository(ctx, pfRepo.ID)
	pfRels, _ := store.GetRelationshipsForRepository(ctx, pfRepo.ID)

	t.Logf("=== REPOSITORY INDEXING STATS ===")
	t.Logf("WorldMonitor: Files=%d, Symbols=%d, Rels=%d, IndexDuration=%v", len(wmManifest), len(wmSymbols), len(wmRels), wmIndexDur)
	t.Logf("varuns-portfolio: Files=%d, Symbols=%d, Rels=%d, IndexDuration=%v", len(pfManifest), len(pfSymbols), len(pfRels), pfIndexDur)

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

	wmQueries := []string{
		"What does this project do?",
		"Explain this project to me like I'm a beginner.",
		"Explain the architecture of this project and how the major components communicate.",
		"Explain package.json.",
		"Trace the request flow.",
		"Explain workers/railway-reconcile-control/src/index.ts.",
		"Why does this project use Cloudflare Workers and Durable Objects?",
		"Does this project use AWS DynamoDB for multi-region database replication?",
	}

	pfQueries := []string{
		"What does this project do?",
		"Explain this project to me like I'm a beginner.",
		"Explain the architecture of this project and how the major components communicate.",
		"Explain package.json.",
		"Trace the request flow.",
		"Explain src/app/page.tsx.",
		"Why does this project use Next.js and Three.js?",
		"Does this project use Apache Kafka for event streaming?",
	}

	// Test Matrix execution order: WorldMonitor -> varuns-portfolio -> WorldMonitor (switch back to test isolation)
	runs := []struct {
		RepoID  string
		Name    string
		Queries []string
	}{
		{"worldmonitor", "WorldMonitor (Run 1)", wmQueries},
		{"varuns-portfolio", "varuns-portfolio (Run 1)", pfQueries},
		{"worldmonitor", "WorldMonitor (Run 2 Isolation Check)", wmQueries[:2]},
	}

	for _, run := range runs {
		scope, _ := models.NewRepositoryScope(run.RepoID)
		t.Logf("\n==================================================")
		t.Logf("=== EXECUTING MATRIX FOR REPOSITORY: %s ===", run.Name)
		t.Logf("==================================================")

		for qIdx, q := range run.Queries {
			start := time.Now()
			expReq, _ := models.NewExplanationRequest(scope, q)
			resp, err := svc.ExplainRequest(ctx, expReq)
			dur := time.Since(start)

			if err != nil {
				t.Errorf("[%s] Q%d '%s' failed: %v", run.Name, qIdx+1, q, err)
				continue
			}

			fmt.Printf("\n--------------------------------------------------\n")
			fmt.Printf("[%s] Q%d: \"%s\"\n", run.Name, qIdx+1, q)
			fmt.Printf("Latency: %v | Mode: %s | Provider: %s | Citations Valid: %d | Invalid: %d | Status: %s\n",
				dur, resp.ProviderMode, resp.Provider, resp.Grounding.CitedEvidenceCount, resp.Grounding.UnsupportedClaimCount, resp.Grounding.Status)
			fmt.Printf("--- FULL ANSWER ---\n%s\n--- END ANSWER ---\n", resp.Answer)

			// Assertions
			if qIdx == 7 {
				if resp.Sufficiency.Status != models.SufficiencyInsufficient && resp.Grounding.Status != models.GroundingInsufficientEvidence {
					t.Errorf("[%s] Q%d expected TRUE_INSUFFICIENT, got sufficiency status %s, grounding status %s", run.Name, qIdx+1, resp.Sufficiency.Status, resp.Grounding.Status)
				}
			} else {
				if resp.ProviderMode != "LOCAL_LLM" {
					t.Errorf("[%s] Q%d expected LOCAL_LLM mode, got %s", run.Name, qIdx+1, resp.ProviderMode)
				}
				if resp.Provider != "ollama" {
					t.Errorf("[%s] Q%d expected provider = ollama, got %s", run.Name, qIdx+1, resp.Provider)
				}
				if resp.Grounding.UnsupportedClaimCount > 0 {
					t.Errorf("[%s] Q%d contained %d invalid citations", run.Name, qIdx+1, resp.Grounding.UnsupportedClaimCount)
				}
			}
		}
	}
}

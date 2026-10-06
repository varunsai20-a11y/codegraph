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

func TestFeatureLevelCodeUnderstanding(t *testing.T) {
	// Check if Ollama is running
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil || resp.StatusCode != 200 {
		t.Skip("Local Ollama server is not running on http://localhost:11434")
		return
	}
	resp.Body.Close()

	featureTests := []struct {
		RepoID           string
		RepoName         string
		RepoPath         string
		RealFeatureQuery string
		UnsupportedQuery string
	}{
		{
			RepoID:           "fastapi",
			RepoName:         "FastAPI",
			RepoPath:         `c:\Users\varun\codegraph\_workspaces\fastapi`,
			RealFeatureQuery: "How does FastAPI process a registered route from the incoming request to the response?",
			UnsupportedQuery: "How does FastAPI manage Redis cluster sharding?",
		},
		{
			RepoID:           "gin",
			RepoName:         "Gin",
			RepoPath:         `c:\Users\varun\codegraph\_workspaces\gin`,
			RealFeatureQuery: "How does Gin route an incoming HTTP request to a handler?",
			UnsupportedQuery: "How does Gin handle Kafka topic partitioning?",
		},
		{
			RepoID:           "worldmonitor",
			RepoName:         "WorldMonitor",
			RepoPath:         `c:\Users\varun\codegraph\_workspaces\112684e1-3c95-43cd-aed2-163a4df0de1b`,
			RealFeatureQuery: "How does a request reach one of the API/worker endpoints and produce its result?",
			UnsupportedQuery: "How does WorldMonitor perform GraphQL schema stitching?",
		},
		{
			RepoID:           "varuns-portfolio",
			RepoName:         "varuns-portfolio",
			RepoPath:         `c:\Users\varun\codegraph\_workspaces\78ebba81-1d1d-47e3-bfd6-3c482c809112`,
			RealFeatureQuery: "How does the LeetCode API route get called and return its response?",
			UnsupportedQuery: "How does varuns-portfolio handle Payment Gateway webhooks?",
		},
	}

	for _, ft := range featureTests {
		if _, err := os.Stat(ft.RepoPath); err != nil {
			t.Fatalf("Workspace path for %s not found: %s", ft.RepoName, ft.RepoPath)
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

	for _, ft := range featureTests {
		t.Logf("\n==================================================")
		t.Logf("=== TESTING FEATURE UNDERSTANDING: %s ===", ft.RepoName)
		t.Logf("==================================================")

		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, ft.RepoID+"_feature.db")
		store, err := storage.NewSQLiteStorage(dbPath)
		if err != nil {
			t.Fatalf("failed to init SQLite storage for %s: %v", ft.RepoName, err)
		}

		wsMgr, err := repository.NewWorkspaceManager(filepath.Join(tmpDir, "workspaces"))
		if err != nil {
			store.Close()
			t.Fatalf("failed to init workspace manager for %s: %v", ft.RepoName, err)
		}

		cfg := &config.Config{
			DefaultExclusions: []string{".git", "node_modules", "dist", "build", "coverage", ".cache", ".tmp", "vendor"},
			MaxFileSize:       512 * 1024,
		}

		idx := ingestion.NewIndexer(cfg, store, wsMgr)
		ctx := context.Background()

		repoModel := &models.Repository{
			ID:         ft.RepoID,
			Name:       ft.RepoName,
			SourceType: models.SourceTypeLocal,
			LocalPath:  ft.RepoPath,
			Status:     models.RepoStatusRegistered,
		}
		if err := store.CreateRepository(ctx, repoModel); err != nil {
			store.Close()
			t.Fatalf("failed to register %s: %v", ft.RepoName, err)
		}
		job := &models.IndexJob{ID: "job-" + ft.RepoID, RepositoryID: ft.RepoID, Status: models.JobStatusPending}
		if err := store.CreateIndexJob(ctx, job); err != nil {
			store.Close()
			t.Fatalf("failed to create index job for %s: %v", ft.RepoName, err)
		}
		if err := idx.RunIndex(ctx, job, repoModel); err != nil {
			store.Close()
			t.Fatalf("RunIndex %s failed: %v", ft.RepoName, err)
		}

		lexRetriever := retrieval.NewLexicalRetriever(store, wsMgr)
		graphRetriever := retrieval.NewGraphRetriever(store)
		hybridEngine := retrieval.NewHybridRetrieverEngine(nil, lexRetriever, nil, graphRetriever, retrieval.DefaultHybridRetrievalConfig())
		hybridAdapter := retrieval.NewHybridRetrieverAdapter(hybridEngine)
		composer := retrieval.NewDefaultEvidenceComposer(hybridAdapter, nil, store)

		promptBuilder := llm.NewGroundedPromptBuilder()
		validator := llm.NewCitationValidator()
		svc := llm.NewGroundedExplanationService(ollamaProv, validator, composer, promptBuilder)

		scope, _ := models.NewRepositoryScope(ft.RepoID)

		// 1. TEST REAL FEATURE FLOW
		startReal := time.Now()
		reqReal, _ := models.NewExplanationRequest(scope, ft.RealFeatureQuery)
		respReal, errReal := svc.ExplainRequest(ctx, reqReal)
		durReal := time.Since(startReal)

		if errReal != nil {
			store.Close()
			t.Errorf("[%s] Real feature query failed: %v", ft.RepoName, errReal)
			continue
		}

		fmt.Printf("\n--------------------------------------------------\n")
		fmt.Printf("[%s] REAL FEATURE QUERY: \"%s\"\n", ft.RepoName, ft.RealFeatureQuery)
		fmt.Printf("Latency: %v | ProviderMode: '%s' | Sufficiency: %s | Grounding: %s | Cited Count: %d | Unsupported: %d\n",
			durReal, respReal.ProviderMode, respReal.Sufficiency.Status, respReal.Grounding.Status, respReal.Grounding.CitedEvidenceCount, respReal.Grounding.UnsupportedClaimCount)
		fmt.Printf("--- FULL ANSWER ---\n%s\n--- END ANSWER ---\n", respReal.Answer)

		// Assertions for Real Feature Flow
		if respReal.Sufficiency.Status != models.SufficiencySufficient {
			t.Errorf("[%s] Expected SufficiencySufficient for real feature query, got %s", ft.RepoName, respReal.Sufficiency.Status)
		}
		if respReal.ProviderMode != "LOCAL_LLM" {
			t.Errorf("[%s] Expected ProviderMode LOCAL_LLM, got %s", ft.RepoName, respReal.ProviderMode)
		}

		// 2. TEST UNSUPPORTED / NONEXISTENT FEATURE
		startUnsup := time.Now()
		reqUnsup, _ := models.NewExplanationRequest(scope, ft.UnsupportedQuery)
		respUnsup, errUnsup := svc.ExplainRequest(ctx, reqUnsup)
		durUnsup := time.Since(startUnsup)

		if errUnsup != nil {
			store.Close()
			t.Errorf("[%s] Unsupported query error: %v", ft.RepoName, errUnsup)
			continue
		}

		fmt.Printf("\n--------------------------------------------------\n")
		fmt.Printf("[%s] UNSUPPORTED FEATURE QUERY: \"%s\"\n", ft.RepoName, ft.UnsupportedQuery)
		fmt.Printf("Latency: %v | ProviderMode: '%s' | Sufficiency: %s | Grounding: %s | Cited Count: %d\n",
			durUnsup, respUnsup.ProviderMode, respUnsup.Sufficiency.Status, respUnsup.Grounding.Status, respUnsup.Grounding.CitedEvidenceCount)
		fmt.Printf("--- FULL ANSWER ---\n%s\n--- END ANSWER ---\n", respUnsup.Answer)

		// Assertions for Unsupported Feature
		if respUnsup.Sufficiency.Status != models.SufficiencyInsufficient && respUnsup.Grounding.Status != models.GroundingInsufficientEvidence {
			t.Errorf("[%s] Expected SufficiencyInsufficient for unsupported query '%s', got sufficiency %s, grounding %s",
				ft.RepoName, ft.UnsupportedQuery, respUnsup.Sufficiency.Status, respUnsup.Grounding.Status)
		}
		if respUnsup.ProviderMode != "" {
			t.Errorf("[%s] Expected empty ProviderMode (no LLM call) for unsupported query, got %s", ft.RepoName, respUnsup.ProviderMode)
		}

		store.Close()
	}
}

package evaluation_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codegraph/internal/config"
	"codegraph/internal/ingestion"
	"codegraph/internal/llm"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/retrieval"
	"codegraph/internal/storage"
	"codegraph/internal/vector"
)

func TestEndToEndWorldMonitorAcceptance(t *testing.T) {
	wsPath := `c:\Users\varun\codegraph\_workspaces\112684e1-3c95-43cd-aed2-163a4df0de1b`
	if _, err := os.Stat(wsPath); err != nil {
		t.Skip("WorldMonitor workspace not found")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "e2e_worldmonitor.db")

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
		MaxFileSize:       10 * 1024 * 1024,
	}

	idx := ingestion.NewIndexer(cfg, store, wsMgr)

	ctx := context.Background()
	repoID := "worldmonitor-e2e"
	repo := &models.Repository{
		ID:         repoID,
		Name:       "WorldMonitor",
		SourceType: models.SourceTypeLocal,
		LocalPath:  wsPath,
		Status:     models.RepoStatusRegistered,
	}
	if err := store.CreateRepository(ctx, repo); err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}

	job := &models.IndexJob{
		ID:           "job-e2e-1",
		RepositoryID: repoID,
		Status:       models.JobStatusPending,
	}
	if err := store.CreateIndexJob(ctx, job); err != nil {
		t.Fatalf("failed to create index job: %v", err)
	}

	startIdx := time.Now()
	if err := idx.RunIndex(ctx, job, repo); err != nil {
		t.Fatalf("RunIndex failed: %v", err)
	}
	t.Logf("Indexing Duration: %v", time.Since(startIdx))

	// 1. Record Ingestion & Indexing Stats
	manifest, err := store.GetManifestForRepository(ctx, repoID)
	if err != nil {
		t.Fatalf("failed to get manifest: %v", err)
	}
	symbols, err := store.GetSymbolsForRepository(ctx, repoID)
	if err != nil {
		t.Fatalf("failed to get symbols: %v", err)
	}
	rels, err := store.GetRelationshipsForRepository(ctx, repoID)
	if err != nil {
		t.Fatalf("failed to get relationships: %v", err)
	}

	langDist := make(map[string]int)
	extDist := make(map[string]int)
	statusDist := make(map[string]int)
	for _, m := range manifest {
		statusDist[string(m.Status)]++
		if m.Status == models.FileStatusIndexed {
			langDist[m.Language]++
			extDist[m.Extension]++
		}
	}

	t.Logf("==================================================")
	t.Logf("1. WORLDMONITOR INDEXING STATISTICS")
	t.Logf("   - Discovered Files: %d", job.FilesDiscovered)
	t.Logf("   - Indexed Files:    %d", job.FilesIndexed)
	t.Logf("   - Skipped Files:    %d", job.FilesSkipped)
	t.Logf("   - Failed Files:     %d", job.FilesFailed)
	t.Logf("   - Symbols Count:    %d", len(symbols))
	t.Logf("   - Relationships:    %d", len(rels))
	t.Logf("   - Status Breakdown: %+v", statusDist)
	t.Logf("   - Language Breakdown: %+v", langDist)
	t.Logf("   - Key Extensions Breakdown: .mts=%d, .ts=%d, .mjs=%d, .json=%d, .md=%d, .yaml=%d, .yml=%d",
		extDist[".mts"], extDist[".ts"], extDist[".mjs"], extDist[".json"], extDist[".md"], extDist[".yaml"], extDist[".yml"])
	t.Logf("==================================================")

	// Set up explanation service components
	lexRetriever := retrieval.NewLexicalRetriever(store, wsMgr)
	graphRetriever := retrieval.NewGraphRetriever(store)

	vStore, _ := vector.NewSQLiteSemanticStore(store.DB())
	embedProv := vector.NewMockEmbeddingProvider()
	semRetriever := retrieval.NewSemanticRetriever(vStore, embedProv)

	hybridEngine := retrieval.NewHybridRetrieverEngine(nil, lexRetriever, semRetriever, graphRetriever, retrieval.DefaultHybridRetrievalConfig())
	hybridAdapter := retrieval.NewHybridRetrieverAdapter(hybridEngine)
	composer := retrieval.NewDefaultEvidenceComposer(hybridAdapter, nil, store)

	// Check environment variable for real LLM credentials test
	apiKey := os.Getenv("GEMINI_API_KEY")
	var llmProvider llm.LLMProvider
	var realLLMActive bool

	if apiKey != "" {
		geminiProv, errProv := llm.NewHTTPLLMProvider(llm.LLMConfig{
			Provider: "gemini",
			Model:    "gemini-1.5-flash",
			APIKey:   apiKey,
		}, nil)
		if errProv == nil {
			llmProvider = llm.NewCascadeLLMProvider(geminiProv)
			realLLMActive = true
			t.Logf("REAL_LLM Active (Gemini API Key detected)")
		} else {
			llmProvider = llm.NewGroundedSynthesisProvider()
			realLLMActive = false
			t.Logf("REAL_LLM_UNAVAILABLE_FOR_THIS_TEST (Using GroundedSynthesisProvider)")
		}
	} else {
		llmProvider = llm.NewGroundedSynthesisProvider()
		realLLMActive = false
		t.Logf("REAL_LLM_UNAVAILABLE_FOR_THIS_TEST (Using GroundedSynthesisProvider)")
	}

	promptBuilder := llm.NewGroundedPromptBuilder()
	validator := llm.NewCitationValidator()
	svc := llm.NewGroundedExplanationService(llmProvider, validator, composer, promptBuilder)

	scope, _ := models.NewRepositoryScope(repoID)
	classifier := retrieval.NewRuleBasedIntentClassifier()
	evaluator := retrieval.NewDefaultSufficiencyEvaluator()

	// 2. Test Repository-Wide Queries
	repoQueries := []string{
		"What does this project do?",
		"Summarise this full repository for me",
		"Explain this project to a beginner",
		"Explain the architecture",
		"Trace the request flow",
	}

	t.Logf("\n2. REPOSITORY-WIDE QUERIES TEST RESULTS")
	for idx, q := range repoQueries {
		detectedIntent := classifier.Classify(q)
		expReq, _ := models.NewExplanationRequest(scope, q)
		pkg, errComp := composer.Compose(ctx, expReq)
		if errComp != nil {
			t.Fatalf("composer failed for query '%s': %v", q, errComp)
		}

		suffRes, _ := evaluator.Evaluate(ctx, scope, q, "EXPLANATION", pkg.Items)
		resp, errExp := svc.ExplainRequest(ctx, expReq)
		if errExp != nil {
			t.Fatalf("explain service failed for query '%s': %v", q, errExp)
		}

		catMap := make(map[string]int)
		for _, item := range pkg.Items {
			catMap[item.RetrieverType]++
		}

		// Verify no fabricated path claims exist in Answer
		hasFabricatedPath := strings.Contains(resp.Answer, "src/components/") || strings.Contains(resp.Answer, "src/services/") || strings.Contains(resp.Answer, "src/api/") || strings.Contains(resp.Answer, "src/App.tsx")

		t.Logf("--------------------------------------------------")
		t.Logf("Query #%d: '%s'", idx+1, q)
		t.Logf("  Detected Intent:     %s", detectedIntent)
		t.Logf("  Evidence Count:      %d", len(pkg.Items))
		t.Logf("  Evidence Categories: %+v", catMap)
		t.Logf("  Sufficiency Result:  Status=%s, TargetFound=%t", suffRes.Status, suffRes.TargetFound)
		t.Logf("  Provider & Model:    Provider=%s, Model=%s, Mode=%s", resp.Provider, resp.Model, resp.ProviderMode)
		t.Logf("  Citation Validation: Status=%s, Valid=%d, Invalid=%d", resp.CitationValidationStatus, resp.Grounding.CitedEvidenceCount, resp.Grounding.UnsupportedClaimCount)
		t.Logf("  Fabricated Paths:    %t", hasFabricatedPath)

		if hasFabricatedPath {
			t.Errorf("Query '%s' produced fabricated path in answer", q)
		}
		if suffRes.Status != models.SufficiencySufficient {
			t.Errorf("Query '%s' returned insufficient status: %s", q, suffRes.Status)
		}
	}

	// 3. Test Targeted Queries
	targetedQueries := []struct {
		query        string
		expectedPath string
	}{
		{"Explain vite.config.ts", "vite.config.ts"},
		{"Explain package.json", "package.json"},
	}

	t.Logf("\n3. TARGETED QUERIES TEST RESULTS")
	for idx, tq := range targetedQueries {
		detectedIntent := classifier.Classify(tq.query)
		expReq, _ := models.NewExplanationRequest(scope, tq.query)
		pkg, errComp := composer.Compose(ctx, expReq)
		if errComp != nil {
			t.Fatalf("composer failed for targeted query '%s': %v", tq.query, errComp)
		}

		suffRes, _ := evaluator.Evaluate(ctx, scope, tq.query, "EXPLANATION", pkg.Items)
		resp, errExp := svc.ExplainRequest(ctx, expReq)
		if errExp != nil {
			t.Fatalf("explain service failed for targeted query '%s': %v", tq.query, errExp)
		}

		// Verify target evidence item is present and system context overview package is NOT overwhelming targeted retrieval
		hasTargetFile := false
		for _, item := range pkg.Items {
			if strings.EqualFold(filepath.Base(item.RelativePath), filepath.Base(tq.expectedPath)) {
				hasTargetFile = true
				break
			}
		}

		t.Logf("--------------------------------------------------")
		t.Logf("Targeted Query #%d: '%s'", idx+1, tq.query)
		t.Logf("  Detected Intent:     %s", detectedIntent)
		t.Logf("  Evidence Count:      %d", len(pkg.Items))
		t.Logf("  Target File Present: %t", hasTargetFile)
		t.Logf("  Sufficiency Result:  Status=%s", suffRes.Status)
		t.Logf("  Provider & Model:    Provider=%s, Mode=%s", resp.Provider, resp.ProviderMode)

		if !hasTargetFile {
			t.Errorf("Targeted query '%s' missing expected target file evidence", tq.query)
		}
		if suffRes.Status != models.SufficiencySufficient {
			t.Errorf("Targeted query '%s' expected SUFFICIENCY_SUFFICIENT, got %s", tq.query, suffRes.Status)
		}
	}

	t.Logf("\nWorldMonitor End-to-End Acceptance Test Completed Successfully.")
	_ = realLLMActive
}

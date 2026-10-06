package evaluation_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codegraph/internal/config"
	"codegraph/internal/ingestion"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/retrieval"
	"codegraph/internal/storage"
)

type DiagnosticResult struct {
	RepoName           string
	QueryIndex         int
	Question           string
	SufficiencyStatus  string
	TargetFound        bool
	EvidenceCount      int
	SufficiencyReasons []string
}

func TestMultiRepoDiagnosticPass(t *testing.T) {
	repos := []struct {
		ID   string
		Name string
		Path string
	}{
		{"worldmonitor", "WorldMonitor", `c:\Users\varun\codegraph\_workspaces\112684e1-3c95-43cd-aed2-163a4df0de1b`},
		{"varuns-portfolio", "varuns-portfolio", `c:\Users\varun\codegraph\_workspaces\78ebba81-1d1d-47e3-bfd6-3c482c809112`},
		{"fastapi", "FastAPI", `c:\Users\varun\codegraph\_workspaces\fastapi`},
		{"gin", "Gin", `c:\Users\varun\codegraph\_workspaces\gin`},
		{"ripgrep", "ripgrep", `c:\Users\varun\codegraph\_workspaces\ripgrep`},
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "diag.db")
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
		MaxFileSize:       10 * 1024 * 1024,
	}

	idx := ingestion.NewIndexer(cfg, store, wsMgr)
	ctx := context.Background()

	lexRetriever := retrieval.NewLexicalRetriever(store, wsMgr)
	graphRetriever := retrieval.NewGraphRetriever(store)
	hybridEngine := retrieval.NewHybridRetrieverEngine(nil, lexRetriever, nil, graphRetriever, retrieval.DefaultHybridRetrievalConfig())
	hybridAdapter := retrieval.NewHybridRetrieverAdapter(hybridEngine)
	composer := retrieval.NewDefaultEvidenceComposer(hybridAdapter, nil, store)
	evaluator := retrieval.NewDefaultSufficiencyEvaluator()

	for _, r := range repos {
		if _, err := os.Stat(r.Path); err != nil {
			t.Logf("Skipping repo %s (path %s not found)", r.Name, r.Path)
			continue
		}

		repo := &models.Repository{
			ID:         r.ID,
			Name:       r.Name,
			SourceType: models.SourceTypeLocal,
			LocalPath:  r.Path,
			Status:     models.RepoStatusRegistered,
		}
		if err := store.CreateRepository(ctx, repo); err != nil {
			t.Fatalf("failed to register repo %s: %v", r.Name, err)
		}
		job := &models.IndexJob{ID: "job-" + r.ID, RepositoryID: repo.ID, Status: models.JobStatusPending}
		if err := store.CreateIndexJob(ctx, job); err != nil {
			t.Fatalf("failed to create job %s: %v", r.Name, err)
		}
		start := time.Now()
		if err := idx.RunIndex(ctx, job, repo); err != nil {
			t.Fatalf("RunIndex %s failed: %v", r.Name, err)
		}
		t.Logf("Indexed %s: %d files in %v", r.Name, job.FilesIndexed, time.Since(start))

		scope, _ := models.NewRepositoryScope(r.ID)
		queries := []string{
			"What does this project do? Explain it to me like a beginner.",
			"Explain the architecture of this project.",
			"Where does this project start executing?",
			"What are the main dependencies/build configuration?",
			"Trace an important request or execution flow through the project.",
			"Explain one important source file/module.",
			"Why does this project use one of its major technologies?",
			"Does this project use AWS DynamoDB for multi-region database replication?",
		}

		fmt.Printf("\n==================================================\n")
		fmt.Printf("=== DIAGNOSTIC RETRIEVAL & SUFFICIENCY: %s ===\n", r.Name)
		fmt.Printf("==================================================\n")

		for qIdx, q := range queries {
			expReq, _ := models.NewExplanationRequest(scope, q)
			pkg, err := composer.Compose(ctx, expReq)
			if err != nil {
				t.Errorf("[%s] Composer failed for '%s': %v", r.Name, q, err)
				continue
			}

			suffRes, err := evaluator.Evaluate(ctx, scope, q, "EXPLANATION", pkg.Items)
			if err != nil {
				t.Errorf("[%s] Evaluator failed for '%s': %v", r.Name, q, err)
				continue
			}

			fmt.Printf("[%s] Q%d: \"%s\"\n", r.Name, qIdx+1, q)
			fmt.Printf("   Items Retrieved: %d | Sufficiency: %s | Target Found: %t\n", len(pkg.Items), suffRes.Status, suffRes.TargetFound)
			if len(suffRes.Reasons) > 0 {
				fmt.Printf("   Reasons: %v\n", suffRes.Reasons)
			}
		}
	}
}

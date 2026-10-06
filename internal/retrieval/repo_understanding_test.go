package retrieval_test

import (
	"context"
	"path/filepath"
	"testing"

	"codegraph/internal/models"
	"codegraph/internal/retrieval"
	"codegraph/internal/storage"
)

func TestRepoUnderstandingRetrievalAndSufficiency(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "repo_test.db")

	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	scope, _ := models.NewRepositoryScope("repo-understanding-1")
	repo := &models.Repository{
		ID:         scope.RepositoryID,
		Name:       "understanding-demo",
		SourceType: models.SourceTypeLocal,
		LocalPath:  tmpDir,
		Status:     models.RepoStatusIndexed,
	}
	_ = store.CreateRepository(ctx, repo)

	// Save file manifest items representing a real project setup
	manifestItems := []*models.FileManifestItem{
		{
			ID:           scope.RepositoryID + ":README.md",
			RepositoryID: scope.RepositoryID,
			RelativePath: "README.md",
			Language:     "Markdown",
			Status:       models.FileStatusIndexed,
		},
		{
			ID:           scope.RepositoryID + ":package.json",
			RepositoryID: scope.RepositoryID,
			RelativePath: "package.json",
			Language:     "JSON",
			Status:       models.FileStatusIndexed,
		},
		{
			ID:           scope.RepositoryID + ":src/index.ts",
			RepositoryID: scope.RepositoryID,
			RelativePath: "src/index.ts",
			Language:     "TypeScript",
			Status:       models.FileStatusIndexed,
		},
	}
	_ = store.SaveManifestItems(ctx, scope.RepositoryID, manifestItems)

	symbols := []*models.Symbol{
		{
			ID:           scope.RepositoryID + ":sym1",
			RepositoryID: scope.RepositoryID,
			FileID:       scope.RepositoryID + ":src/index.ts",
			RelativePath: "src/index.ts",
			Name:         "bootstrapApp",
			Kind:         models.SymbolKindFunction,
		},
	}
	_ = store.SaveSymbols(ctx, scope.RepositoryID, symbols)

	composer := retrieval.NewDefaultEvidenceComposer(nil, nil, store)
	evaluator := retrieval.NewDefaultSufficiencyEvaluator()

	repoWideQueries := []string{
		"What does this project do?",
		"Summarise this repository",
		"Summarise this full repository for me",
		"Explain the architecture",
		"How does this project work?",
		"Explain this project to a beginner",
	}

	for _, q := range repoWideQueries {
		req, err := models.NewExplanationRequest(scope, q)
		if err != nil {
			t.Fatalf("failed to create request for '%s': %v", q, err)
		}

		pkg, err := composer.Compose(ctx, req)
		if err != nil {
			t.Fatalf("failed to compose evidence package for '%s': %v", q, err)
		}

		if len(pkg.Items) == 0 {
			t.Errorf("query '%s': expected system context evidence items, got 0", q)
		}

		// Verify retrieval collects README, package.json, entry points, or workspace tree
		foundDocOrSetup := false
		for _, item := range pkg.Items {
			if item.RetrieverType == "SYSTEM_CONTEXT_README" || item.RetrieverType == "SYSTEM_CONTEXT_SETUP" ||
				item.RetrieverType == "SYSTEM_CONTEXT_ENTRYPOINT" || item.RetrieverType == "SYSTEM_CONTEXT_TREE" ||
				item.RetrieverType == "SYSTEM_CONTEXT_SYMBOL" {
				foundDocOrSetup = true
				break
			}
		}
		if !foundDocOrSetup {
			t.Errorf("query '%s': expected system context items (README/setup/entrypoint/tree/symbol)", q)
		}

		// Verify sufficiency evaluation
		suffRes, err := evaluator.Evaluate(ctx, scope, q, "EXPLANATION", pkg.Items)
		if err != nil {
			t.Fatalf("sufficiency evaluation failed for '%s': %v", q, err)
		}
		if suffRes.Status != models.SufficiencySufficient {
			t.Errorf("query '%s': expected SUFFICIENCY_SUFFICIENT, got %s (reasons: %v)", q, suffRes.Status, suffRes.Reasons)
		}
	}
}

func TestRepoUnderstandingInsufficientEvidence(t *testing.T) {
	evaluator := retrieval.NewDefaultSufficiencyEvaluator()
	scope, _ := models.NewRepositoryScope("repo-empty")

	// Empty evidence items list
	items := []*models.EvidenceItem{}

	queries := []string{
		"What does this project do?",
		"Summarise this repository",
		"Explain the architecture",
	}

	for _, q := range queries {
		res, err := evaluator.Evaluate(context.Background(), scope, q, "EXPLANATION", items)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Status != models.SufficiencyInsufficient {
			t.Errorf("query '%s': expected SUFFICIENCY_INSUFFICIENT, got %s", q, res.Status)
		}
		if len(res.MissingInfo) == 0 {
			t.Errorf("query '%s': expected explicit MissingInfo explanations", q)
		}
	}
}

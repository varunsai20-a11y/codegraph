package evaluation_test

import (
	"context"
	"path/filepath"
	"testing"

	"codegraph/internal/evaluation"
	"codegraph/internal/graph"
	"codegraph/internal/models"
	"codegraph/internal/retrieval"
	"codegraph/internal/storage"
)

func BenchmarkEvaluation_Metrics_PrecisionAt5(b *testing.B) {
	retrieved := []string{"sym1", "sym2", "sym3", "sym4", "sym5"}
	groundTruth := []string{"sym2", "sym5", "sym9"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = evaluation.CalculatePrecisionAtK(retrieved, groundTruth, 5)
	}
}

func BenchmarkEvaluation_Metrics_MRR(b *testing.B) {
	retrieved := []string{"sym1", "sym2", "sym3", "sym4", "sym5"}
	groundTruth := []string{"sym2", "sym5", "sym9"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = evaluation.CalculateMRR(retrieved, groundTruth)
	}
}

func BenchmarkEvaluation_ColdVsWarmRetrieval(b *testing.B) {
	tempDir := b.TempDir()
	dbPath := filepath.Join(tempDir, "bench_eval.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		b.Fatalf("failed to create sqlite storage: %v", err)
	}
	defer store.Close()

	repo := &models.Repository{
		ID:        "repo-bench-eval",
		Name:      "Bench Repo",
		LocalPath: tempDir,
		Status:    models.RepoStatusIndexed,
	}
	_ = store.CreateRepository(context.Background(), repo)

	// Create 100 graph nodes and 99 call edges
	nodes := make([]*models.Node, 0, 100)
	edges := make([]*models.Edge, 0, 99)

	for i := 1; i <= 100; i++ {
		n := &models.Node{
			ID:            graph.FormatNodeID(models.NodeKindSymbol, "repo-bench-eval", string(rune('A'+i%26))),
			RepositoryID:  "repo-bench-eval",
			Kind:          models.NodeKindSymbol,
			Label:         string(rune('A' + i%26)),
			QualifiedName: "pkg." + string(rune('A'+i%26)),
		}
		nodes = append(nodes, n)
	}
	_ = store.SaveGraph(context.Background(), "repo-bench-eval", nodes, edges)

	scope, _ := models.NewRepositoryScope("repo-bench-eval")
	graphRet := retrieval.NewGraphRetriever(store)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = graphRet.Retrieve(context.Background(), scope, "A", retrieval.IntentCallerQuery, 5)
	}
}

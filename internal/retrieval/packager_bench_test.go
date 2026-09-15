package retrieval_test

import (
	"context"
	"fmt"
	"testing"

	"codegraph/internal/models"
	"codegraph/internal/retrieval"
)

func generateBenchmarkItems(repoID string, count int) []*models.EvidenceItem {
	items := make([]*models.EvidenceItem, 0, count)
	for i := 1; i <= count; i++ {
		item := &models.EvidenceItem{
			StableID:     fmt.Sprintf("item-bench-%05d", i),
			RepositoryID: repoID,
			Type:         models.EvidenceTypeCodeSnippet,
			RelativePath: fmt.Sprintf("pkg/file_%d.go", i%50),
			Location:     models.Location{StartLine: 1, EndLine: 20},
			Content:      fmt.Sprintf("func BenchmarkFunction_%d() {\n\t// Benchmark evidence item content line\n\treturn %d\n}", i, i),
			RRFScore:     1.0 / float64(i),
			Rank:         i,
		}
		items = append(items, item)
	}
	return items
}

func BenchmarkPackager_BuildPackage_100Items(b *testing.B) {
	scope, _ := models.NewRepositoryScope("repo-bench")
	packager := retrieval.NewDefaultEvidencePackager(nil)
	items := generateBenchmarkItems("repo-bench", 100)
	budget := models.DefaultEvidenceBudget()
	budget.MaxTokens = 50000 // Allow all items for benchmark

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := packager.BuildPackage(context.Background(), scope, "query", retrieval.IntentSymbolLookup, items, budget)
		if err != nil {
			b.Fatalf("benchmark error: %v", err)
		}
	}
}

func BenchmarkPackager_BuildPackage_1000Items(b *testing.B) {
	scope, _ := models.NewRepositoryScope("repo-bench")
	packager := retrieval.NewDefaultEvidencePackager(nil)
	items := generateBenchmarkItems("repo-bench", 1000)
	budget := models.DefaultEvidenceBudget()
	budget.MaxTokens = 500000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := packager.BuildPackage(context.Background(), scope, "query", retrieval.IntentSymbolLookup, items, budget)
		if err != nil {
			b.Fatalf("benchmark error: %v", err)
		}
	}
}

func BenchmarkPackager_BuildPackage_10000Items(b *testing.B) {
	scope, _ := models.NewRepositoryScope("repo-bench")
	packager := retrieval.NewDefaultEvidencePackager(nil)
	items := generateBenchmarkItems("repo-bench", 10000)
	budget := models.DefaultEvidenceBudget()
	budget.MaxTokens = 5000000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := packager.BuildPackage(context.Background(), scope, "query", retrieval.IntentSymbolLookup, items, budget)
		if err != nil {
			b.Fatalf("benchmark error: %v", err)
		}
	}
}

func BenchmarkContextBuilder_BuildGroundedContext_100Items(b *testing.B) {
	scope, _ := models.NewRepositoryScope("repo-bench")
	packager := retrieval.NewDefaultEvidencePackager(nil)
	items := generateBenchmarkItems("repo-bench", 100)
	budget := models.DefaultEvidenceBudget()
	budget.MaxTokens = 50000

	pkg, _ := packager.BuildPackage(context.Background(), scope, "query", retrieval.IntentSymbolLookup, items, budget)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = retrieval.BuildGroundedContext(pkg)
	}
}

func BenchmarkContextBuilder_BuildGroundedContext_1000Items(b *testing.B) {
	scope, _ := models.NewRepositoryScope("repo-bench")
	packager := retrieval.NewDefaultEvidencePackager(nil)
	items := generateBenchmarkItems("repo-bench", 1000)
	budget := models.DefaultEvidenceBudget()
	budget.MaxTokens = 500000

	pkg, _ := packager.BuildPackage(context.Background(), scope, "query", retrieval.IntentSymbolLookup, items, budget)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = retrieval.BuildGroundedContext(pkg)
	}
}

func BenchmarkContextBuilder_BuildGroundedContext_10000Items(b *testing.B) {
	scope, _ := models.NewRepositoryScope("repo-bench")
	packager := retrieval.NewDefaultEvidencePackager(nil)
	items := generateBenchmarkItems("repo-bench", 10000)
	budget := models.DefaultEvidenceBudget()
	budget.MaxTokens = 5000000

	pkg, _ := packager.BuildPackage(context.Background(), scope, "query", retrieval.IntentSymbolLookup, items, budget)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = retrieval.BuildGroundedContext(pkg)
	}
}

package llm_test

import (
	"fmt"
	"strings"
	"testing"

	"codegraph/internal/llm"
	"codegraph/internal/models"
)

func createBenchmarkPackage(count int) (*models.EvidencePackage, models.RepositoryScope) {
	scope, _ := models.NewRepositoryScope("repo-bench")
	budget := models.DefaultEvidenceBudget()
	budget.MaxTokens = 500000

	pkg, _ := models.NewEvidencePackage(scope, "Benchmark Query?", "SYMBOL_LOOKUP", budget)
	for i := 1; i <= count; i++ {
		item := &models.EvidenceItem{
			StableID:     fmt.Sprintf("stable-%05d", i),
			RepositoryID: "repo-bench",
			Type:         models.EvidenceTypeSymbol,
			RelativePath: fmt.Sprintf("pkg/file_%d.go", i),
			Location:     models.Location{StartLine: i, EndLine: i + 10},
			Content:      fmt.Sprintf("func Symbol_%d()", i),
			RRFScore:     1.0 / float64(i),
		}
		_ = pkg.AddItem(scope, item)
	}
	pkg.FinalizePackage()
	return pkg, scope
}

func BenchmarkCitationValidator_Validate_100Citations(b *testing.B) {
	pkg, _ := createBenchmarkPackage(100)
	validator := llm.NewCitationValidator()

	var sb strings.Builder
	sb.WriteString("Explanations with citations: ")
	for i := 1; i <= 100; i++ {
		sb.WriteString(fmt.Sprintf("Symbol_%d is referenced in [E%d]. ", i, i))
	}
	text := sb.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.Validate(text, pkg)
	}
}

func BenchmarkCitationValidator_Validate_1000Citations(b *testing.B) {
	pkg, _ := createBenchmarkPackage(1000)
	validator := llm.NewCitationValidator()

	var sb strings.Builder
	sb.WriteString("Explanations with citations: ")
	for i := 1; i <= 1000; i++ {
		sb.WriteString(fmt.Sprintf("Symbol_%d is referenced in [E%d]. ", i, i))
	}
	text := sb.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.Validate(text, pkg)
	}
}

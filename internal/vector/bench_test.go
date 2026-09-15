package vector_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"codegraph/internal/models"
	"codegraph/internal/vector"
)

// BENCHMARK SCOPE DOCUMENTATION:
// BenchmarkCosineSimilarity_PureGo measures pure CPU cosine similarity calculation per vector pair (D=384).
// BenchmarkSQLiteSemanticStore_Search_* measures the full end-to-end vector search path including:
// 1. SQLite database query execution for repository_id
// 2. JSON vector unmarshalling into float32 slices
// 3. Cosine similarity calculation per candidate vector
// 4. Score-based sorting and candidate ranking
func BenchmarkCosineSimilarity_PureGo(b *testing.B) {
	provider := vector.NewMockEmbeddingProvider()
	v1, _ := provider.Embed(context.Background(), "func LoginController(w, r)")
	v2, _ := provider.Embed(context.Background(), "func AuthenticateUser(ctx, creds)")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = vector.CosineSimilarity(v1, v2)
	}
}

func BenchmarkSQLiteSemanticStore_Save_1000(b *testing.B) {
	tempDir := b.TempDir()
	store, err := vector.NewSQLiteSemanticStorePath(filepath.Join(tempDir, "bench_save.db"))
	if err != nil {
		b.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	provider := vector.NewMockEmbeddingProvider()
	scope, _ := models.NewRepositoryScope("repo-bench")

	records := make([]*vector.VectorRecord, 1000)
	for i := 0; i < 1000; i++ {
		vec, _ := provider.Embed(context.Background(), fmt.Sprintf("func Function_%d()", i))
		records[i] = &vector.VectorRecord{
			RepositoryID:       "repo-bench",
			ChunkID:            fmt.Sprintf("chunk-%d", i),
			ContentHash:        fmt.Sprintf("hash-%d", i),
			Content:            fmt.Sprintf("func Function_%d()", i),
			EmbeddingModel:     provider.ModelName(),
			EmbeddingDimension: provider.Dimension(),
			EmbeddingVersion:   provider.Version(),
			Vector:             vec,
			UpdatedAt:          time.Now(),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = store.SaveEmbeddings(context.Background(), scope, records)
	}
}

func BenchmarkSQLiteSemanticStore_Search_1000(b *testing.B) {
	benchmarkSearchScale(b, 1000)
}

func BenchmarkSQLiteSemanticStore_Search_10000(b *testing.B) {
	benchmarkSearchScale(b, 10000)
}

func benchmarkSearchScale(b *testing.B, vectorCount int) {
	tempDir := b.TempDir()
	store, err := vector.NewSQLiteSemanticStorePath(filepath.Join(tempDir, fmt.Sprintf("bench_search_%d.db", vectorCount)))
	if err != nil {
		b.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	provider := vector.NewMockEmbeddingProvider()
	scope, _ := models.NewRepositoryScope("repo-bench")

	batchSize := 1000
	for i := 0; i < vectorCount; i += batchSize {
		records := make([]*vector.VectorRecord, 0, batchSize)
		for j := 0; j < batchSize && (i+j) < vectorCount; j++ {
			idx := i + j
			vec, _ := provider.Embed(context.Background(), fmt.Sprintf("func Function_%d()", idx))
			records = append(records, &vector.VectorRecord{
				RepositoryID:       "repo-bench",
				ChunkID:            fmt.Sprintf("chunk-%d", idx),
				ContentHash:        fmt.Sprintf("hash-%d", idx),
				Content:            fmt.Sprintf("func Function_%d()", idx),
				EmbeddingModel:     provider.ModelName(),
				EmbeddingDimension: provider.Dimension(),
				EmbeddingVersion:   provider.Version(),
				Vector:             vec,
				UpdatedAt:          time.Now(),
			})
		}
		_ = store.SaveEmbeddings(context.Background(), scope, records)
	}

	queryVec, _ := provider.Embed(context.Background(), "func Function_500()")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = store.Search(context.Background(), scope, queryVec, provider.ModelName(), provider.Version(), 10, -1.0)
	}
}

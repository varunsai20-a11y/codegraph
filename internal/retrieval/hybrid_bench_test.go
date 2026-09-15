package retrieval_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"codegraph/internal/graph"
	"codegraph/internal/models"
	"codegraph/internal/retrieval"
	"codegraph/internal/storage"
	"codegraph/internal/vector"
)

func BenchmarkRRFFusion_1000Candidates(b *testing.B) {
	benchmarkRRFFusionScale(b, 1000)
}

func BenchmarkRRFFusion_10000Candidates(b *testing.B) {
	benchmarkRRFFusionScale(b, 10000)
}

func benchmarkRRFFusionScale(b *testing.B, count int) {
	scope, _ := models.NewRepositoryScope("repo-bench")

	var itemsLex []*models.EvidenceItem
	var itemsSem []*models.EvidenceItem

	for i := 0; i < count; i++ {
		stID := fmt.Sprintf("stable-id-%d", i)
		itemsLex = append(itemsLex, &models.EvidenceItem{
			StableID:     stID,
			RepositoryID: "repo-bench",
			Content:      fmt.Sprintf("Lexical Content %d", i),
			Rank:         i + 1,
			RawScore:     float64(count - i),
		})
		itemsSem = append(itemsSem, &models.EvidenceItem{
			StableID:     stID,
			RepositoryID: "repo-bench",
			Content:      fmt.Sprintf("Semantic Content %d", i),
			Rank:         count - i,
			RawScore:     float64(i),
		})
	}

	lexMock := &MockRetriever{items: itemsLex}
	semMock := &MockRetriever{items: itemsSem}

	config := retrieval.DefaultHybridRetrievalConfig()
	config.CandidateLimit = count
	config.TopK = 50

	engine := retrieval.NewHybridRetrieverEngine(nil, lexMock, semMock, nil, config)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = engine.Retrieve(context.Background(), scope, "Login")
	}
}

func BenchmarkHybridRetriever_ExecutionBreakdown(b *testing.B) {
	tempDir := b.TempDir()
	dbPath := filepath.Join(tempDir, "bench_hybrid.db")

	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		b.Fatalf("failed to create storage: %v", err)
	}
	defer store.Close()

	vecStore, err := vector.NewSQLiteSemanticStorePath(filepath.Join(tempDir, "bench_hybrid_vec.db"))
	if err != nil {
		b.Fatalf("failed to create vector store: %v", err)
	}
	defer vecStore.Close()

	provider := vector.NewMockEmbeddingProvider()

	// Seed repository metadata, symbols, graph nodes, and vectors
	repo := &models.Repository{
		ID:        "repo-hybrid",
		Name:      "Hybrid Bench Repo",
		LocalPath: tempDir,
		Status:    models.RepoStatusIndexed,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_ = store.CreateRepository(context.Background(), repo)

	scope, _ := models.NewRepositoryScope("repo-hybrid")

	var symbols []*models.Symbol
	var vecRecords []*vector.VectorRecord

	for i := 0; i < 100; i++ {
		sym := &models.Symbol{
			ID:            fmt.Sprintf("sym-%d", i),
			RepositoryID:  "repo-hybrid",
			FileID:        "file-1",
			RelativePath:  "auth/login.go",
			Name:          fmt.Sprintf("LoginHandler_%d", i),
			QualifiedName: fmt.Sprintf("auth.LoginHandler_%d", i),
			Kind:          models.SymbolKindFunction,
			Location:      models.Location{StartLine: i + 1, EndLine: i + 5},
		}
		symbols = append(symbols, sym)

		vec, _ := provider.Embed(context.Background(), sym.QualifiedName)
		vecRecords = append(vecRecords, &vector.VectorRecord{
			RepositoryID:       "repo-hybrid",
			ChunkID:            sym.FileID,
			SymbolID:           sym.ID,
			RelativePath:       sym.RelativePath,
			Location:           sym.Location,
			ContentHash:        fmt.Sprintf("hash-%d", i),
			Content:            sym.QualifiedName,
			EmbeddingModel:     provider.ModelName(),
			EmbeddingDimension: provider.Dimension(),
			EmbeddingVersion:   provider.Version(),
			Vector:             vec,
		})
	}

	_ = store.SaveSymbols(context.Background(), "repo-hybrid", symbols)
	_ = vecStore.SaveEmbeddings(context.Background(), scope, vecRecords)

	// Create graph nodes and edges
	node1 := &models.Node{
		ID:            graph.FormatNodeID(models.NodeKindSymbol, "repo-hybrid", "sym-1"),
		RepositoryID:  "repo-hybrid",
		Kind:          models.NodeKindSymbol,
		Label:         "LoginHandler_1",
		QualifiedName: "auth.LoginHandler_1",
	}
	node2 := &models.Node{
		ID:            graph.FormatNodeID(models.NodeKindSymbol, "repo-hybrid", "sym-2"),
		RepositoryID:  "repo-hybrid",
		Kind:          models.NodeKindSymbol,
		Label:         "LoginHandler_2",
		QualifiedName: "auth.LoginHandler_2",
	}
	edge := &models.Edge{
		ID:           "edge-1",
		RepositoryID: "repo-hybrid",
		SourceID:     node1.ID,
		TargetID:     node2.ID,
		TargetKind:   models.TargetKindInternal,
		Kind:         models.EdgeKindCalls,
		Status:       models.RelStatusResolved,
	}
	_ = store.SaveGraph(context.Background(), "repo-hybrid", []*models.Node{node1, node2}, []*models.Edge{edge})

	lexicalRetriever := retrieval.NewLexicalRetriever(store)
	semanticRetriever := retrieval.NewSemanticRetriever(vecStore, provider)
	graphRetriever := retrieval.NewGraphRetriever(store)
	classifier := retrieval.NewRuleBasedIntentClassifier()

	config := retrieval.DefaultHybridRetrievalConfig()
	engine := retrieval.NewHybridRetrieverEngine(classifier, lexicalRetriever, semanticRetriever, graphRetriever, config)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = engine.Retrieve(context.Background(), scope, "LoginHandler_1")
	}
}

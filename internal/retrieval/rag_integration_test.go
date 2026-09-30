package retrieval

import (
	"context"
	"testing"
	"time"

	"codegraph/internal/models"
	"codegraph/internal/vector"
)

func TestRAGSemanticRetrieval_EndToEnd(t *testing.T) {
	// 1. Setup in-memory vector store & mock provider
	vStore, err := vector.NewSQLiteSemanticStorePath(":memory:")
	if err != nil {
		t.Fatalf("failed to create sqlite vector store: %v", err)
	}
	defer vStore.Close()

	embedProvider := vector.NewMockEmbeddingProvider()
	semRetriever := NewSemanticRetriever(vStore, embedProvider)

	repoID := "repo-rag-test"
	scope, err := models.NewRepositoryScope(repoID)
	if err != nil {
		t.Fatalf("failed to create repo scope: %v", err)
	}

	// 2. Store sample vector records for repo-rag-test
	text1 := "func ResearchAgentProcess(query string) (*Result, error) {\n    // Handles deep research task\n}"
	vec1, _ := embedProvider.Embed(context.Background(), text1)

	rec1 := &vector.VectorRecord{
		RepositoryID:       repoID,
		ChunkID:            "chunk-1",
		SymbolID:           "sym-research-agent",
		RelativePath:       "agents/research.go",
		Location:           models.Location{StartLine: 10, EndLine: 25},
		ContentHash:        "hash1",
		Content:            text1,
		EmbeddingModel:     embedProvider.ModelName(),
		EmbeddingDimension: embedProvider.Dimension(),
		EmbeddingVersion:   embedProvider.Version(),
		Vector:             vec1,
		UpdatedAt:          time.Now(),
	}
	rec1.ID = vector.ComputeVectorID(repoID, rec1.ChunkID, rec1.EmbeddingModel, rec1.EmbeddingVersion)

	if err := vStore.SaveEmbeddings(context.Background(), scope, []*vector.VectorRecord{rec1}); err != nil {
		t.Fatalf("failed to save embeddings: %v", err)
	}

	// 3. Test Repository Isolation: Query different repo ID -> 0 items
	otherScope, _ := models.NewRepositoryScope("repo-other")
	otherItems, err := semRetriever.Retrieve(context.Background(), otherScope, "research agent", "EXPLANATION", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(otherItems) != 0 {
		t.Fatalf("expected 0 items for isolated repo, got %d", len(otherItems))
	}

	// 4. Test Semantic Retrieval -> Returns relevant chunk
	items, err := semRetriever.Retrieve(context.Background(), scope, "research agent process", "EXPLANATION", 10)
	if err != nil {
		t.Fatalf("failed to retrieve semantic items: %v", err)
	}
	if len(items) == 0 {
		t.Fatalf("expected semantic retrieval to return items, got 0")
	}

	if items[0].RelativePath != "agents/research.go" {
		t.Errorf("expected relative path 'agents/research.go', got '%s'", items[0].RelativePath)
	}
	if items[0].RetrieverType != "VECTOR" {
		t.Errorf("expected retriever type 'VECTOR', got '%s'", items[0].RetrieverType)
	}

	// 5. Test HybridRetrieverEngine receives semantic results
	hybridEngine := NewHybridRetrieverEngine(nil, nil, semRetriever, nil, DefaultHybridRetrievalConfig())
	hybridRes, err := hybridEngine.Retrieve(context.Background(), scope, "research agent process")
	if err != nil {
		t.Fatalf("hybrid retrieval failed: %v", err)
	}
	if len(hybridRes.Items) == 0 {
		t.Fatalf("expected hybrid engine to contain fused semantic items, got 0")
	}
}

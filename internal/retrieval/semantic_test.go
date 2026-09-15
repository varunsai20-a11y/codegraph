package retrieval_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codegraph/internal/models"
	"codegraph/internal/retrieval"
	"codegraph/internal/vector"
)

func TestSemanticRetriever_ContractAndProvenance(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_semantic_retriever.db")
	vectorStore, err := vector.NewSQLiteSemanticStorePath(dbPath)
	if err != nil {
		t.Fatalf("failed to create vector store: %v", err)
	}
	defer vectorStore.Close()

	provider := vector.NewMockEmbeddingProvider()
	scope, _ := models.NewRepositoryScope("repo-1")

	// Embed & index a test code chunk
	chunkText := "func AuthenticateUser(ctx, creds)"
	vec, err := provider.Embed(context.Background(), chunkText)
	if err != nil {
		t.Fatalf("failed to generate embedding: %v", err)
	}

	rec := &vector.VectorRecord{
		RepositoryID:       "repo-1",
		ChunkID:            "file-auth",
		SymbolID:           "sym-auth",
		RelativePath:       "auth/login.go",
		Location:           models.Location{StartLine: 12, EndLine: 30},
		ContentHash:        "hash-auth-123",
		Content:            chunkText,
		EmbeddingModel:     provider.ModelName(),
		EmbeddingDimension: provider.Dimension(),
		EmbeddingVersion:   provider.Version(),
		Vector:             vec,
		UpdatedAt:          time.Now(),
	}

	if err := vectorStore.SaveEmbeddings(context.Background(), scope, []*vector.VectorRecord{rec}); err != nil {
		t.Fatalf("failed to save embeddings: %v", err)
	}

	semanticRetriever := retrieval.NewSemanticRetriever(vectorStore, provider)

	// Retrieve semantic evidence for query identical to chunk text
	items, err := semanticRetriever.Retrieve(context.Background(), scope, chunkText, "FEATURE_SEARCH", 10)
	if err != nil {
		t.Fatalf("unexpected error during semantic retrieval: %v", err)
	}

	if len(items) == 0 {
		t.Fatalf("expected at least 1 semantic evidence item")
	}

	top := items[0]
	if top.RetrieverType != "VECTOR" {
		t.Errorf("expected RetrieverType = 'VECTOR', got %s", top.RetrieverType)
	}
	if top.RepositoryID != "repo-1" {
		t.Errorf("expected RepositoryID = 'repo-1', got %s", top.RepositoryID)
	}
	if top.RelativePath != "auth/login.go" {
		t.Errorf("expected RelativePath = 'auth/login.go', got %s", top.RelativePath)
	}
	if top.Location.StartLine != 12 {
		t.Errorf("expected StartLine = 12, got %d", top.Location.StartLine)
	}
	if top.Rank != 1 {
		t.Errorf("expected Rank = 1, got %d", top.Rank)
	}
	if top.RawScore < 0.99 {
		t.Errorf("expected RawScore >= 0.99 for identical text, got %f", top.RawScore)
	}
	if top.Metadata["embedding_model"] != provider.ModelName() {
		t.Errorf("expected embedding_model metadata = %s, got %s", provider.ModelName(), top.Metadata["embedding_model"])
	}
	if top.Metadata["content_hash"] != "hash-auth-123" {
		t.Errorf("expected content_hash metadata = 'hash-auth-123', got %s", top.Metadata["content_hash"])
	}
}

func TestSemanticRetriever_InterfaceContract(t *testing.T) {
	// Verifies that SemanticRetriever relies strictly on EmbeddingProvider and SemanticStore interfaces
	dbPath := filepath.Join(t.TempDir(), "test_semantic_interface.db")
	var store vector.SemanticStore
	var provider vector.EmbeddingProvider

	var err error
	store, err = vector.NewSQLiteSemanticStorePath(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	provider = vector.NewMockEmbeddingProvider()

	// Verify contract instance construction using interface types
	retriever := retrieval.NewSemanticRetriever(store, provider)
	scope, _ := models.NewRepositoryScope("repo-contract")

	items, err := retriever.Retrieve(context.Background(), scope, "test query", "FEATURE_SEARCH", 5)
	if err != nil {
		t.Fatalf("unexpected error from interface contract test: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items for empty index in contract test, got %d", len(items))
	}
}

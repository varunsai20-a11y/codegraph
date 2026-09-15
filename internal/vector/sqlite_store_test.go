package vector_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codegraph/internal/models"
	"codegraph/internal/vector"
)

func setupTestVectorStore(t *testing.T) vector.SemanticStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test_vector_store.db")
	store, err := vector.NewSQLiteSemanticStorePath(dbPath)
	if err != nil {
		t.Fatalf("failed to create vector store: %v", err)
	}
	return store
}

func TestSQLiteSemanticStore_IdempotencyAndDeterministicIdentity(t *testing.T) {
	store := setupTestVectorStore(t)
	defer store.Close()

	scope, _ := models.NewRepositoryScope("repo-A")
	provider := vector.NewMockEmbeddingProvider()
	vec, _ := provider.Embed(context.Background(), "func AuthenticateUser()")

	rec1 := &vector.VectorRecord{
		RepositoryID:       "repo-A",
		ChunkID:            "chunk-1",
		SymbolID:           "sym-auth",
		RelativePath:       "auth/login.go",
		Location:           models.Location{StartLine: 10, EndLine: 20},
		ContentHash:        "hash-v1",
		Content:            "func AuthenticateUser()",
		EmbeddingModel:     provider.ModelName(),
		EmbeddingDimension: provider.Dimension(),
		EmbeddingVersion:   provider.Version(),
		Vector:             vec,
		UpdatedAt:          time.Now(),
	}

	// First save
	if err := store.SaveEmbeddings(context.Background(), scope, []*vector.VectorRecord{rec1}); err != nil {
		t.Fatalf("failed to save embeddings first run: %v", err)
	}

	id1 := vector.ComputeVectorID(rec1.RepositoryID, rec1.ChunkID, rec1.EmbeddingModel, rec1.EmbeddingVersion)

	// Second save with identical record (idempotency test)
	if err := store.SaveEmbeddings(context.Background(), scope, []*vector.VectorRecord{rec1}); err != nil {
		t.Fatalf("failed to save embeddings second run: %v", err)
	}

	results, err := store.Search(context.Background(), scope, vec, provider.ModelName(), provider.Version(), 10, -1.0)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected exactly 1 record after idempotent insert, got %d", len(results))
	}
	if results[0].Record.ID != id1 {
		t.Errorf("expected vector ID %s, got %s", id1, results[0].Record.ID)
	}
}

func TestSQLiteSemanticStore_StaleVectorReplacement(t *testing.T) {
	store := setupTestVectorStore(t)
	defer store.Close()

	scope, _ := models.NewRepositoryScope("repo-A")
	provider := vector.NewMockEmbeddingProvider()
	vec, _ := provider.Embed(context.Background(), "func Login()")

	// Step 1: Save chunk C1 with hash H1
	recH1 := &vector.VectorRecord{
		RepositoryID:       "repo-A",
		ChunkID:            "chunk-C1",
		ContentHash:        "hash-H1",
		Content:            "func Login()",
		EmbeddingModel:     provider.ModelName(),
		EmbeddingDimension: provider.Dimension(),
		EmbeddingVersion:   provider.Version(),
		Vector:             vec,
	}
	if err := store.SaveEmbeddings(context.Background(), scope, []*vector.VectorRecord{recH1}); err != nil {
		t.Fatalf("failed to save initial embedding H1: %v", err)
	}

	// Step 2: Verify 1 embedding record exists with hash H1
	res1, err := store.Search(context.Background(), scope, vec, provider.ModelName(), provider.Version(), 10, -1.0)
	if err != nil || len(res1) != 1 {
		t.Fatalf("expected 1 record H1, got len=%d, err=%v", len(res1), err)
	}
	if res1[0].Record.ContentHash != "hash-H1" {
		t.Errorf("expected ContentHash = 'hash-H1', got %s", res1[0].Record.ContentHash)
	}

	// Step 3: Re-index same chunk C1 with hash H2 (content updated)
	recH2 := &vector.VectorRecord{
		RepositoryID:       "repo-A",
		ChunkID:            "chunk-C1",
		ContentHash:        "hash-H2",
		Content:            "func Login(ctx context.Context)",
		EmbeddingModel:     provider.ModelName(),
		EmbeddingDimension: provider.Dimension(),
		EmbeddingVersion:   provider.Version(),
		Vector:             vec,
	}
	if err := store.SaveEmbeddings(context.Background(), scope, []*vector.VectorRecord{recH2}); err != nil {
		t.Fatalf("failed to save updated embedding H2: %v", err)
	}

	// Step 4 & 5 & 6: Verify current embedding is H2, stale H1 is NOT returned, and count = 1
	res2, err := store.Search(context.Background(), scope, vec, provider.ModelName(), provider.Version(), 10, -1.0)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(res2) != 1 {
		t.Fatalf("expected exactly 1 vector record after update (no stale accumulation), got %d", len(res2))
	}
	if res2[0].Record.ContentHash != "hash-H2" {
		t.Errorf("expected current vector content hash = 'hash-H2', got %s", res2[0].Record.ContentHash)
	}
}

func TestSQLiteSemanticStore_ModelAndVersionChecks(t *testing.T) {
	store := setupTestVectorStore(t)
	defer store.Close()

	scope, _ := models.NewRepositoryScope("repo-A")
	provider := vector.NewMockEmbeddingProvider()
	vec, _ := provider.Embed(context.Background(), "func Search()")

	rec := &vector.VectorRecord{
		RepositoryID:       "repo-A",
		ChunkID:            "chunk-search",
		ContentHash:        "hash-s",
		Content:            "func Search()",
		EmbeddingModel:     provider.ModelName(),
		EmbeddingDimension: provider.Dimension(),
		EmbeddingVersion:   provider.Version(),
		Vector:             vec,
	}
	_ = store.SaveEmbeddings(context.Background(), scope, []*vector.VectorRecord{rec})

	// Mismatched model name -> 0 results
	resWrongModel, _ := store.Search(context.Background(), scope, vec, "different-model", provider.Version(), 10, -1.0)
	if len(resWrongModel) != 0 {
		t.Errorf("expected 0 results for mismatched model name, got %d", len(resWrongModel))
	}

	// Mismatched version -> 0 results
	resWrongVersion, _ := store.Search(context.Background(), scope, vec, provider.ModelName(), "9.9.9", 10, -1.0)
	if len(resWrongVersion) != 0 {
		t.Errorf("expected 0 results for mismatched embedding version, got %d", len(resWrongVersion))
	}
}

func TestSQLiteSemanticStore_MinSimilarityFilter(t *testing.T) {
	store := setupTestVectorStore(t)
	defer store.Close()

	scope, _ := models.NewRepositoryScope("repo-A")
	provider := vector.NewMockEmbeddingProvider()
	vec1, _ := provider.Embed(context.Background(), "exact match text")
	vec2, _ := provider.Embed(context.Background(), "completely different string xyz 123")

	rec := &vector.VectorRecord{
		RepositoryID:       "repo-A",
		ChunkID:            "chunk-1",
		ContentHash:        "hash-1",
		Content:            "exact match text",
		EmbeddingModel:     provider.ModelName(),
		EmbeddingDimension: provider.Dimension(),
		EmbeddingVersion:   provider.Version(),
		Vector:             vec1,
	}
	_ = store.SaveEmbeddings(context.Background(), scope, []*vector.VectorRecord{rec})

	// Search with vec2 and high minSimilarity (0.95) -> should filter out low similarity result
	results, err := store.Search(context.Background(), scope, vec2, provider.ModelName(), provider.Version(), 10, 0.95)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results with high minSimilarity cutoff 0.95, got %d", len(results))
	}
}

func TestSQLiteSemanticStore_RepositoryIsolation(t *testing.T) {
	store := setupTestVectorStore(t)
	defer store.Close()

	scopeA, _ := models.NewRepositoryScope("repo-A")
	scopeB, _ := models.NewRepositoryScope("repo-B")
	provider := vector.NewMockEmbeddingProvider()
	vec, _ := provider.Embed(context.Background(), "func Secret()")

	recB := &vector.VectorRecord{
		RepositoryID:       "repo-B",
		ChunkID:            "chunk-secret-b",
		ContentHash:        "hash-b",
		Content:            "func SecretB()",
		EmbeddingModel:     provider.ModelName(),
		EmbeddingDimension: provider.Dimension(),
		EmbeddingVersion:   provider.Version(),
		Vector:             vec,
	}
	_ = store.SaveEmbeddings(context.Background(), scopeB, []*vector.VectorRecord{recB})

	// Search repo-A scope MUST return 0 items from repo-B
	results, err := store.Search(context.Background(), scopeA, vec, provider.ModelName(), provider.Version(), 10, -1.0)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("repository isolation violated! expected 0 items for repo-A search when vector belongs to repo-B, got %d", len(results))
	}
}

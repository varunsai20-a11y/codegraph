package vector

import (
	"context"
	"errors"
	"fmt"
	"time"

	"codegraph/internal/hashing"
	"codegraph/internal/models"
)

var (
	ErrEmptyVector       = errors.New("empty vector provided")
	ErrDimensionMismatch = errors.New("vector dimension mismatch")
	ErrModelMismatch     = errors.New("embedding model mismatch: stored vectors derived from different model")
	ErrVersionMismatch   = errors.New("embedding version mismatch: stored vectors derived from different version")
)

// EmbeddingProvider defines the contract for generating vector embeddings from text.
type EmbeddingProvider interface {
	ModelName() string
	Dimension() int
	Version() string
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}

// VectorRecord represents a code-aware embedding record stored in the SemanticStore.
type VectorRecord struct {
	ID                 string          `json:"id"` // Deterministic vector ID (repo_id|chunk_id|model|version)
	RepositoryID       string          `json:"repository_id"`
	ChunkID            string          `json:"chunk_id"`
	SymbolID           string          `json:"symbol_id,omitempty"`
	RelativePath       string          `json:"relative_path"`
	Location           models.Location `json:"location"`
	ContentHash        string          `json:"content_hash"`
	Content            string          `json:"content"`
	EmbeddingModel     string          `json:"embedding_model"`
	EmbeddingDimension int             `json:"embedding_dimension"`
	EmbeddingVersion   string          `json:"embedding_version"`
	Vector             []float32       `json:"vector"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

// VectorSearchResult holds a retrieved record and its calculated cosine similarity score.
type VectorSearchResult struct {
	Record *VectorRecord `json:"record"`
	Score  float64       `json:"score"` // Cosine similarity [-1.0, 1.0]
}

// SemanticStore defines the decoupled storage interface for repository embeddings.
type SemanticStore interface {
	SaveEmbeddings(ctx context.Context, scope models.RepositoryScope, records []*VectorRecord) error
	Search(ctx context.Context, scope models.RepositoryScope, queryVector []float32, modelName string, version string, limit int, minSimilarity float64) ([]*VectorSearchResult, error)
	DeleteRepositoryEmbeddings(ctx context.Context, scope models.RepositoryScope) error
	Close() error
}

// ComputeVectorID generates a deterministic logical identity hash for a stored embedding record.
// The identity is derived from repository_id + chunk_id + embedding_model + embedding_version.
// This guarantees that re-indexing a chunk with updated content_hash replaces the existing record
// rather than accumulating stale vectors in the database.
func ComputeVectorID(repoID, chunkID, modelName, version string) string {
	raw := fmt.Sprintf("%s|%s|%s|%s", repoID, chunkID, modelName, version)
	return hashing.HashBytes([]byte(raw))
}

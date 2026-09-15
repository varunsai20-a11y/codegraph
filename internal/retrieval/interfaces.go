package retrieval

import (
	"context"

	"codegraph/internal/models"
)

// Retriever defines the contract for all retrieval components (Graph, Lexical, Vector, Hybrid).
type Retriever interface {
	Retrieve(ctx context.Context, scope models.RepositoryScope, query string, intent string, limit int) ([]*models.EvidenceItem, error)
}

// SufficiencyEvaluator evaluates evidence quality/confidence independently of retrieval RRF rank scores.
type SufficiencyEvaluator interface {
	Evaluate(ctx context.Context, scope models.RepositoryScope, query string, intent string, items []*models.EvidenceItem) (models.EvidenceSufficiencyResult, error)
}

// EvidencePackager compiles raw evidence items into a structured prompt package adhering to an EvidenceBudget.
type EvidencePackager interface {
	BuildPackage(ctx context.Context, scope models.RepositoryScope, question string, intent string, items []*models.EvidenceItem, budget models.EvidenceBudget) (*models.EvidencePackage, error)
}

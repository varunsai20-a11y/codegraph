package retrieval

import (
	"context"
	"sort"
	"strings"

	"codegraph/internal/models"
	"codegraph/internal/vector"
)

// SemanticRetriever implements vector semantic search conforming strictly to the Retriever contract.
//
// SCOPE BOUNDARY:
// SemanticRetriever is a clean, single-purpose vector retrieval component (Query -> Embed -> Store Search -> Evidence).
// It does NOT perform graph reasoning or query intent classification. Intent-driven weighting, graph retrieval,
// and multi-retriever orchestration are owned by Checkpoint 5 (Hybrid Retriever Fusion).
type SemanticRetriever struct {
	store         vector.SemanticStore
	provider      vector.EmbeddingProvider
	minSimilarity float64
}

func NewSemanticRetriever(store vector.SemanticStore, provider vector.EmbeddingProvider) *SemanticRetriever {
	return &SemanticRetriever{
		store:         store,
		provider:      provider,
		minSimilarity: -1.0, // Default: no similarity cutoff
	}
}

func (r *SemanticRetriever) WithMinSimilarity(minSim float64) *SemanticRetriever {
	r.minSimilarity = minSim
	return r
}

func (r *SemanticRetriever) Retrieve(
	ctx context.Context,
	scope models.RepositoryScope,
	query string,
	intent string,
	limit int,
) ([]*models.EvidenceItem, error) {
	if scope.RepositoryID == "" {
		return nil, models.ErrInvalidRepositoryScope
	}
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return []*models.EvidenceItem{}, nil
	}
	if limit <= 0 {
		limit = 10
	}

	// 1. Generate query vector using EmbeddingProvider
	queryVector, err := r.provider.Embed(ctx, trimmedQuery)
	if err != nil {
		return nil, err
	}

	// 2. Query SemanticStore scoped strictly by RepositoryID
	searchResults, err := r.store.Search(ctx, scope, queryVector, r.provider.ModelName(), r.provider.Version(), limit, r.minSimilarity)
	if err != nil {
		return nil, err
	}

	var candidates []*models.EvidenceItem

	// 3. Convert VectorSearchResult to models.EvidenceItem
	for _, res := range searchResults {
		rec := res.Record
		evidenceType := models.EvidenceTypeCodeSnippet
		if rec.SymbolID != "" {
			evidenceType = models.EvidenceTypeSymbol
		}

		item := &models.EvidenceItem{
			RepositoryID:  scope.RepositoryID,
			Type:          evidenceType,
			FileID:        rec.ChunkID,
			RelativePath:  rec.RelativePath,
			Location:      rec.Location,
			Content:       rec.Content,
			RetrieverType: "VECTOR",
			RawScore:      res.Score,
			Metadata: map[string]string{
				"content_hash":      rec.ContentHash,
				"embedding_model":   rec.EmbeddingModel,
				"embedding_version": rec.EmbeddingVersion,
				"symbol_id":         rec.SymbolID,
			},
		}
		item.StableID = item.ComputeStableID()
		candidates = append(candidates, item)
	}

	// 4. Deterministic Sort: Primary = RawScore desc, Secondary = StableID asc
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].RawScore != candidates[j].RawScore {
			return candidates[i].RawScore > candidates[j].RawScore
		}
		return candidates[i].StableID < candidates[j].StableID
	})

	// 5. Assign sequential Rank and limit output
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	for i, item := range candidates {
		item.Rank = i + 1
	}

	return candidates, nil
}

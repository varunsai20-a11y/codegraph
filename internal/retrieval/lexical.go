package retrieval

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"codegraph/internal/models"
	"codegraph/internal/storage"
)

// LexicalRetriever implements code-aware lexical search using Phase 2 static analysis intelligence.
//
// SCALABILITY BOUNDARY:
// LexicalRetriever is currently an in-memory & SQLite baseline scanner operating directly over Phase 2
// static symbols and file manifests. It implements the Retriever interface so an indexed engine
// (e.g. SQLite FTS5 or trigram index) can replace the baseline without modifying higher-level contracts.
//
// RANKING SEMANTICS:
// RawScore values are non-probabilistic heuristic ranking scores used strictly to order candidate lexical
// items relative to each other. Raw scores are preserved for diagnostic provenance and are kept distinct
// from semantic vector cosine scores, fused RRF scores (Checkpoint 5), and evidence sufficiency scores.
type LexicalRetriever struct {
	store   storage.Storage
	chunker *ASTSymbolChunker
}

func NewLexicalRetriever(store storage.Storage) *LexicalRetriever {
	return &LexicalRetriever{
		store:   store,
		chunker: NewASTSymbolChunker(),
	}
}

func (r *LexicalRetriever) Retrieve(
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

	// 1. Fetch static intelligence symbols for target repository ONLY
	symbols, err := r.store.GetSymbolsForRepository(ctx, scope.RepositoryID)
	if err != nil {
		return nil, err
	}

	// Fetch repository details to get local disk path if available
	repo, _ := r.store.GetRepository(ctx, scope.RepositoryID)
	repoLocalPath := ""
	if repo != nil {
		repoLocalPath = repo.LocalPath
	}

	lowerQuery := strings.ToLower(trimmedQuery)
	var candidates []*models.EvidenceItem

	// 2. Score symbols against query and query intent
	for _, sym := range symbols {
		rawScore := r.calculateSymbolScore(sym, trimmedQuery, lowerQuery, intent)
		if rawScore <= 0 {
			continue
		}

		item, err := r.chunker.CreateSymbolEvidence(scope, sym, repoLocalPath, rawScore)
		if err != nil {
			continue
		}
		candidates = append(candidates, item)
	}

	// 3. Score file manifest paths against query and query intent
	manifestItems, _ := r.store.GetManifestForRepository(ctx, scope.RepositoryID)
	for _, f := range manifestItems {
		rawScore := r.calculateFileScore(f, trimmedQuery, lowerQuery, intent)
		if rawScore <= 0 {
			continue
		}

		item := &models.EvidenceItem{
			RepositoryID:  scope.RepositoryID,
			Type:          models.EvidenceTypeDependency,
			FileID:        f.ID,
			RelativePath:  f.RelativePath,
			Location:      models.Location{StartLine: 1, EndLine: 1},
			Content:       "file " + f.RelativePath + " (" + f.Language + ")",
			RetrieverType: "LEXICAL",
			RawScore:      rawScore,
			Metadata: map[string]string{
				"file_id":       f.ID,
				"language":      f.Language,
				"relative_path": f.RelativePath,
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

// calculateSymbolScore evaluates a static symbol against query terms and intent heuristics.
// Lexical Ranking Strategy:
// - Exact Qualified Name Match: 100.0
// - Exact Symbol Name Match: 80.0
// - Prefix Symbol Name Match: 60.0
// - Identifier Substring Match: 40.0
// Intent Boosts:
// - SYMBOL_LOOKUP: +30.0 for exact/qualified matches
// - CALLER_QUERY / CALLEE_QUERY: +25.0 for FUNCTION / METHOD kinds
func (r *LexicalRetriever) calculateSymbolScore(sym *models.Symbol, query, lowerQuery, intent string) float64 {
	lowerName := strings.ToLower(sym.Name)
	lowerQual := strings.ToLower(sym.QualifiedName)
	score := 0.0

	if query == sym.QualifiedName || lowerQuery == lowerQual {
		score = 100.0
	} else if query == sym.Name || lowerQuery == lowerName {
		score = 80.0
	} else if strings.HasPrefix(lowerName, lowerQuery) {
		score = 60.0
	} else if strings.Contains(lowerQual, lowerQuery) || strings.Contains(lowerName, lowerQuery) {
		score = 40.0
	}

	if score == 0.0 {
		words := strings.FieldsFunc(lowerQuery, func(r rune) bool {
			return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-')
		})
		for _, w := range words {
			if len(w) < 3 {
				continue
			}
			if w == lowerName || w == lowerQual {
				score = 70.0
				break
			} else if strings.Contains(lowerName, w) || strings.Contains(lowerQual, w) {
				score = 40.0
				break
			}
		}
	}

	if score == 0.0 {
		return 0.0
	}

	// Intent-based rank adjustments
	switch intent {
	case IntentSymbolLookup:
		if score >= 80.0 {
			score += 30.0
		}
	case IntentCallerQuery, IntentCalleeQuery:
		if sym.Kind == models.SymbolKindFunction || sym.Kind == models.SymbolKindMethod {
			score += 25.0
		}
	case IntentFeatureSearch:
		if score <= 60.0 {
			score += 15.0
		}
	}

	return score
}

// calculateFileScore evaluates a file path against query terms and intent heuristics.
// Lexical Ranking Strategy:
// - Exact Path Match: 50.0
// - Path Substring / Package Match: 30.0
// Intent Boosts:
// - DEPENDENCY_QUERY / ARCHITECTURE_QUERY: +35.0 for file/module matches
func (r *LexicalRetriever) calculateFileScore(f *models.FileManifestItem, query, lowerQuery, intent string) float64 {
	lowerPath := strings.ToLower(f.RelativePath)
	baseName := strings.ToLower(filepath.Base(f.RelativePath))
	score := 0.0

	if query == f.RelativePath || lowerQuery == lowerPath || lowerQuery == baseName {
		score = 50.0
	} else if strings.Contains(lowerPath, lowerQuery) {
		score = 30.0
	}

	if score == 0.0 {
		words := strings.FieldsFunc(lowerQuery, func(r rune) bool {
			return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '/' || r == '.')
		})
		for _, w := range words {
			if len(w) < 3 {
				continue
			}
			if strings.Contains(lowerPath, w) || strings.Contains(baseName, w) {
				score = 30.0
				break
			}
		}
	}

	if score == 0.0 {
		return 0.0
	}

	switch intent {
	case IntentDependencyQuery, IntentArchitectureQuery:
		score += 35.0
	}

	return score
}

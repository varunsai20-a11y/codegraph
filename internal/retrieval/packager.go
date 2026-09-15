package retrieval

import (
	"context"
	"sort"
	"strings"

	"codegraph/internal/models"
)

// DefaultEvidencePackager builds a bounded, deterministic EvidencePackage adhering to EvidenceBudget.
type DefaultEvidencePackager struct {
	evaluator SufficiencyEvaluator
}

func NewDefaultEvidencePackager(evaluator SufficiencyEvaluator) *DefaultEvidencePackager {
	if evaluator == nil {
		evaluator = NewDefaultSufficiencyEvaluator()
	}
	return &DefaultEvidencePackager{
		evaluator: evaluator,
	}
}

func (p *DefaultEvidencePackager) BuildPackage(
	ctx context.Context,
	scope models.RepositoryScope,
	question string,
	intent string,
	items []*models.EvidenceItem,
	budget models.EvidenceBudget,
) (*models.EvidencePackage, error) {
	if scope.RepositoryID == "" {
		return nil, models.ErrInvalidRepositoryScope
	}

	// Apply default budget limits if zero or negative
	defaultBudget := models.DefaultEvidenceBudget()
	if budget.MaxTokens <= 0 {
		budget.MaxTokens = defaultBudget.MaxTokens
	}
	if budget.MaxFiles <= 0 {
		budget.MaxFiles = defaultBudget.MaxFiles
	}
	if budget.MaxSymbols <= 0 {
		budget.MaxSymbols = defaultBudget.MaxSymbols
	}
	if budget.MaxSnippetLines <= 0 {
		budget.MaxSnippetLines = defaultBudget.MaxSnippetLines
	}
	if budget.MaxGraphHops <= 0 {
		budget.MaxGraphHops = defaultBudget.MaxGraphHops
	}

	// 1. Enforce repository isolation & clone items to guarantee input immutability
	clonedCandidates := make([]*models.EvidenceItem, 0, len(items))
	targetAmbiguousFound := false

	for _, item := range items {
		if item == nil {
			continue
		}
		if err := scope.ValidateItem(item); err != nil {
			return nil, err
		}
		cloned := item.Clone()
		if cloned.StableID == "" {
			cloned.StableID = cloned.ComputeStableID()
		}
		if cloned.TargetResolution == models.TargetAmbiguous {
			targetAmbiguousFound = true
		}
		clonedCandidates = append(clonedCandidates, cloned)
	}

	// 2. Deterministic Ranking: Primary RRFScore desc, Secondary Rank asc, Tertiary StableID asc
	sort.Slice(clonedCandidates, func(i, j int) bool {
		if clonedCandidates[i].RRFScore != clonedCandidates[j].RRFScore {
			return clonedCandidates[i].RRFScore > clonedCandidates[j].RRFScore
		}
		if clonedCandidates[i].Rank != clonedCandidates[j].Rank {
			return clonedCandidates[i].Rank < clonedCandidates[j].Rank
		}
		return clonedCandidates[i].StableID < clonedCandidates[j].StableID
	})

	// 3. Selection Algorithm & Budget Enforcement
	seenIDs := make(map[string]bool)
	seenFiles := make(map[string]bool)
	symbolCount := 0
	accumulatedTokens := 0

	var selected []*models.EvidenceItem

	for _, candidate := range clonedCandidates {
		if seenIDs[candidate.StableID] {
			continue
		}

		// Line-level snippet truncation
		if budget.MaxSnippetLines > 0 && candidate.Type == models.EvidenceTypeCodeSnippet {
			lines := strings.Split(candidate.Content, "\n")
			if len(lines) > budget.MaxSnippetLines {
				candidate.Content = strings.Join(lines[:budget.MaxSnippetLines], "\n")
				if candidate.Location.StartLine > 0 {
					candidate.Location.EndLine = candidate.Location.StartLine + budget.MaxSnippetLines - 1
				}
				if candidate.Metadata == nil {
					candidate.Metadata = make(map[string]string)
				}
				candidate.Metadata["truncated"] = "true"
			}
		}

		itemTokens := models.EstimateTokens(candidate.Content)

		// Check file budget
		isNewFile := candidate.RelativePath != "" && !seenFiles[candidate.RelativePath]
		if isNewFile && len(seenFiles) >= budget.MaxFiles {
			continue
		}

		// Check symbol budget
		isSymbol := candidate.Type == models.EvidenceTypeSymbol
		if isSymbol && symbolCount >= budget.MaxSymbols {
			continue
		}

		// Check token budget
		if accumulatedTokens+itemTokens > budget.MaxTokens {
			if len(selected) > 0 {
				break
			}
			// If first item exceeds token budget, skip or break
			break
		}

		seenIDs[candidate.StableID] = true
		if candidate.RelativePath != "" {
			seenFiles[candidate.RelativePath] = true
		}
		if isSymbol {
			symbolCount++
		}
		accumulatedTokens += itemTokens
		selected = append(selected, candidate)
	}

	// 4. Evaluate evidence sufficiency
	sufficiency, err := p.evaluator.Evaluate(ctx, scope, question, intent, selected)
	if err != nil {
		return nil, err
	}
	if targetAmbiguousFound {
		sufficiency.TargetResolution = models.TargetAmbiguous
		sufficiency.Reasons = append(sufficiency.Reasons, "Query matched multiple ambiguous target entities in knowledge graph")
	}

	// 5. Construct & finalize package
	pkg, err := models.NewEvidencePackage(scope, question, intent, budget)
	if err != nil {
		return nil, err
	}
	for _, sel := range selected {
		_ = pkg.AddItem(scope, sel)
	}

	pkg.FinalizePackage()
	pkg.Sufficiency = sufficiency

	return pkg, nil
}

package retrieval

import (
	"context"

	"codegraph/internal/models"
)

type DefaultSufficiencyEvaluator struct{}

func NewDefaultSufficiencyEvaluator() *DefaultSufficiencyEvaluator {
	return &DefaultSufficiencyEvaluator{}
}

func (e *DefaultSufficiencyEvaluator) Evaluate(
	ctx context.Context,
	scope models.RepositoryScope,
	query string,
	intent string,
	items []*models.EvidenceItem,
) (models.EvidenceSufficiencyResult, error) {
	// Enforce repository isolation
	for _, item := range items {
		if err := scope.ValidateItem(item); err != nil {
			return models.EvidenceSufficiencyResult{}, err
		}
	}

	if len(items) == 0 {
		return models.EvidenceSufficiencyResult{
			Status:             models.SufficiencyInsufficient,
			TargetFound:        false,
			SourceAvailable:    false,
			ResolutionCoverage: 0.0,
			EvidenceCoverage:   0.0,
			HeuristicScore:     0.0,
			Reasons:            []string{"No evidence items retrieved from repository index"},
			MissingInfo:        []string{"No matching symbols, graph edges, or source code snippets found"},
		}, nil
	}

	var symbolCount, edgeCount, snippetCount int
	var resolvedCount, unresolvedCount int

	for _, item := range items {
		switch item.Type {
		case models.EvidenceTypeSymbol:
			symbolCount++
		case models.EvidenceTypeGraphEdge:
			edgeCount++
		case models.EvidenceTypeCodeSnippet:
			snippetCount++
		}

		if item.ResolutionStatus == models.RelStatusResolved {
			resolvedCount++
		} else if item.ResolutionStatus == models.RelStatusUnresolved {
			unresolvedCount++
		}
	}

	totalRelations := resolvedCount + unresolvedCount
	resolvedRatio := 1.0
	if totalRelations > 0 {
		resolvedRatio = float64(resolvedCount) / float64(totalRelations)
	}

	targetFound := (symbolCount > 0 || snippetCount > 0)
	sourceAvailable := (snippetCount > 0)

	varietyTypes := 0
	if symbolCount > 0 {
		varietyTypes++
	}
	if edgeCount > 0 {
		varietyTypes++
	}
	if snippetCount > 0 {
		varietyTypes++
	}
	evidenceCoverage := float64(varietyTypes) / 3.0

	// Non-probabilistic heuristic score
	heuristicScore := 0.0
	if targetFound {
		heuristicScore += 0.4
	}
	heuristicScore += 0.3 * resolvedRatio
	heuristicScore += 0.3 * evidenceCoverage

	var status models.SufficiencyStatus
	var reasons []string
	var missingInfo []string
	var conflicts []string

	if !targetFound || heuristicScore < 0.3 {
		status = models.SufficiencyInsufficient
		reasons = append(reasons, "Retrieved evidence does not contain direct symbol or snippet matches for target entity")
		missingInfo = append(missingInfo, "Target entity declaration or definition missing from index")
	} else if heuristicScore >= 0.7 && resolvedRatio >= 0.5 {
		status = models.SufficiencySufficient
		reasons = append(reasons, "Retrieved evidence contains verified symbol declarations, source code, and resolved graph relationships")
	} else {
		status = models.SufficiencyPartial
		reasons = append(reasons, "Evidence contains partial symbol or snippet context, but some relationships remain unresolved")
		if unresolvedCount > 0 {
			missingInfo = append(missingInfo, "Static analysis contains unresolved dynamic call targets")
		}
	}

	return models.EvidenceSufficiencyResult{
		Status:             status,
		TargetFound:        targetFound,
		SourceAvailable:    sourceAvailable,
		ResolutionCoverage: resolvedRatio,
		EvidenceCoverage:   evidenceCoverage,
		HeuristicScore:     heuristicScore,
		Conflicts:          conflicts,
		Reasons:            reasons,
		MissingInfo:        missingInfo,
	}, nil
}

package retrieval

import (
	"context"
	"strings"

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

	effectiveIntent := intent
	if effectiveIntent == "" || effectiveIntent == "EXPLANATION" || effectiveIntent == IntentExplanation {
		classifier := NewRuleBasedIntentClassifier()
		effectiveIntent = classifier.Classify(query)
	}

	// Bypass sufficiency gates for INTENT_GREETING & INTENT_REPO_OVERVIEW
	if effectiveIntent == IntentGreeting || intent == IntentGreeting {
		return models.EvidenceSufficiencyResult{
			Status:             models.SufficiencySufficient,
			TargetFound:        true,
			SourceAvailable:    true,
			ResolutionCoverage: 1.0,
			EvidenceCoverage:   1.0,
			HeuristicScore:     1.0,
			Reasons:            []string{"Conversational input (greeting) requires no code evidence"},
		}, nil
	}

	if effectiveIntent == IntentRepoOverview || intent == IntentRepoOverview {
		return models.EvidenceSufficiencyResult{
			Status:             models.SufficiencySufficient,
			TargetFound:        true,
			SourceAvailable:    true,
			ResolutionCoverage: 1.0,
			EvidenceCoverage:   1.0,
			HeuristicScore:     1.0,
			Reasons:            []string{"Repository overview intent uses System Context Package"},
		}, nil
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
	var queryRelevantItems int

	lowerQuery := strings.ToLower(query)
	queryWords := strings.FieldsFunc(lowerQuery, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-')
	})

	effectiveIntent = intent
	if effectiveIntent == "" || effectiveIntent == "EXPLANATION" || effectiveIntent == IntentExplanation {
		classifier := NewRuleBasedIntentClassifier()
		effectiveIntent = classifier.Classify(query)
	}

	for _, item := range items {
		// Verify item content has actual substance beyond metadata header
		hasSubstance := len(strings.TrimSpace(item.Content)) > 0 && !strings.HasPrefix(item.Content, "file ")

		// Check term-level query relevance
		isRelevant := false
		lowerContent := strings.ToLower(item.Content)
		lowerPath := strings.ToLower(item.RelativePath)

		for _, qw := range queryWords {
			if len(qw) >= 3 && !isStopWord(qw) {
				stem := qw
				if len(qw) > 5 {
					stem = qw[:5]
				}
				if strings.Contains(lowerContent, qw) || strings.Contains(lowerPath, qw) || strings.Contains(lowerContent, stem) {
					isRelevant = true
					break
				}
			}
		}

		isOverviewQuery := effectiveIntent == IntentArchitectureQuery || effectiveIntent == IntentRepoOverview || effectiveIntent == IntentTrace ||
			strings.Contains(lowerQuery, "purpose") ||
			strings.Contains(lowerQuery, "what does") ||
			strings.Contains(lowerQuery, "overview") ||
			strings.Contains(lowerQuery, "about") ||
			strings.Contains(lowerQuery, "architecture") ||
			strings.Contains(lowerQuery, "start") ||
			strings.Contains(lowerQuery, "execution") ||
			strings.Contains(lowerQuery, "entry point") ||
			strings.Contains(lowerQuery, "entrypoint")

		if isOverviewQuery {
			if item.Type == models.EvidenceTypeDocumentation || item.Type == models.EvidenceTypeStaticFlow || strings.HasSuffix(lowerPath, "readme.md") || strings.Contains(lowerPath, "main") || strings.Contains(lowerPath, "cmd/") || strings.Contains(lowerPath, "run.py") || strings.Contains(lowerPath, "app.py") {
				isRelevant = true
			}
		}

		if item.RetrieverType == "SELECTED_SYMBOL" || item.Type == models.EvidenceTypeStaticFlow {
			isRelevant = true
		}

		if isRelevant {
			queryRelevantItems++
		}

		switch item.Type {
		case models.EvidenceTypeSymbol:
			symbolCount++
		case models.EvidenceTypeGraphEdge:
			edgeCount++
		case models.EvidenceTypeCodeSnippet, models.EvidenceTypeDocumentation:
			if hasSubstance {
				snippetCount++
			}
		case models.EvidenceTypeStaticFlow:
			symbolCount++
			edgeCount++
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

	// Calculate distinct query topic term coverage across retrieved evidence
	matchedTermsMap := make(map[string]bool)
	totalNonStopWords := 0

	for _, qw := range queryWords {
		if len(qw) >= 3 && !isStopWord(qw) {
			totalNonStopWords++
			for _, item := range items {
				lowerContent := strings.ToLower(item.Content)
				lowerPath := strings.ToLower(item.RelativePath)
				stem := qw
				if len(qw) > 5 {
					stem = qw[:5]
				}
				if strings.Contains(lowerContent, qw) || strings.Contains(lowerPath, qw) || strings.Contains(lowerContent, stem) {
					matchedTermsMap[qw] = true
					break
				}
			}
		}
	}

	termCoverage := 1.0
	if totalNonStopWords > 0 {
		termCoverage = float64(len(matchedTermsMap)) / float64(totalNonStopWords)
	}

	isOverview := effectiveIntent == IntentArchitectureQuery || effectiveIntent == IntentRepoOverview || effectiveIntent == IntentTrace ||
		strings.Contains(lowerQuery, "purpose") ||
		strings.Contains(lowerQuery, "what does") ||
		strings.Contains(lowerQuery, "overview") ||
		strings.Contains(lowerQuery, "about") ||
		strings.Contains(lowerQuery, "architecture") ||
		strings.Contains(lowerQuery, "start") ||
		strings.Contains(lowerQuery, "execution") ||
		strings.Contains(lowerQuery, "entry point") ||
		strings.Contains(lowerQuery, "entrypoint")

	targetFound := (symbolCount > 0 || snippetCount > 0 || edgeCount > 0) && (queryRelevantItems > 0)
	if !isOverview && totalNonStopWords >= 3 && termCoverage < 0.4 && edgeCount == 0 {
		targetFound = false
	}

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
		reasons = append(reasons, "Retrieved evidence does not contain query-relevant symbol or snippet matches for target entity")
		missingInfo = append(missingInfo, "Target entity declaration, source snippet, or graph edge missing from index")
	} else if heuristicScore >= 0.6 && (snippetCount > 0 || symbolCount > 0) {
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

func isStopWord(w string) bool {
	switch w {
	case "for", "the", "where", "what", "with", "from", "this", "that", "have", "has", "had", "are", "was", "were", "been", "being", "does", "did", "how", "why", "who", "which", "and", "but", "not", "all", "any", "can", "could", "would", "should", "your", "its", "explain", "describe", "summary", "overview", "component", "function", "class", "method", "source", "based", "execution", "flow", "code", "implementation", "details", "application", "initialize", "system", "setup", "service", "services", "start", "work", "works":
		return true
	default:
		return false
	}
}

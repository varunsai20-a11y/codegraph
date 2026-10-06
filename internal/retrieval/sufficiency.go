package retrieval

import (
	"context"
	"fmt"
	"path/filepath"
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

	// Greeting bypass
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

	lowerQuery := strings.ToLower(strings.TrimSpace(query))
	targetFile := ExtractTargetFile(query)
	targetSymbol := ExtractTargetSymbol(query)

	isFileQuery := effectiveIntent == IntentFileQuery || targetFile != ""
	isSymbolQuery := effectiveIntent == IntentSymbolLookup || effectiveIntent == IntentCallerQuery || effectiveIntent == IntentCalleeQuery || (targetSymbol != "" && !isFileQuery)
	isArchQuery := effectiveIntent == IntentArchitectureQuery || effectiveIntent == IntentRepoOverview ||
		strings.Contains(lowerQuery, "architecture") || strings.Contains(lowerQuery, "what does this project do") ||
		strings.Contains(lowerQuery, "what does this repo do") || strings.Contains(lowerQuery, "repository overview") ||
		strings.Contains(lowerQuery, "project overview") || strings.Contains(lowerQuery, "how is this repository structured") ||
		strings.Contains(lowerQuery, "major components") || strings.Contains(lowerQuery, "summaris") ||
		strings.Contains(lowerQuery, "summariz") || strings.Contains(lowerQuery, "summary") ||
		strings.Contains(lowerQuery, "beginner") || lowerQuery == "explain the code"

	hasStaticFlowItem := false
	for _, item := range items {
		if item.Type == models.EvidenceTypeStaticFlow || item.RetrieverType == "STATIC_FLOW" {
			hasStaticFlowItem = true
			break
		}
	}

	isFlowQuery := effectiveIntent == IntentTrace || isExecutionFlowQuery(lowerQuery) || strings.Contains(lowerQuery, "flow") || hasStaticFlowItem
	isDependencyQuery := effectiveIntent == IntentDependencyQuery || strings.Contains(lowerQuery, "depend") || strings.Contains(lowerQuery, "build config") || strings.Contains(lowerQuery, "manifest")

	// 1. ABSENT SPECIFIC TECHNOLOGY / FEATURE CHECK (TRUE_INSUFFICIENT)
	// If user query asks about a specific non-existent technology or feature (e.g. "dynamodb", "redis", "kafka", "graphql", "webhooks"),
	// check if those specific feature terms are absent from all retrieved evidence.
	specificTerms := extractSpecificTechnicalTerms(lowerQuery)
	if !isArchQuery && len(specificTerms) > 0 && isSpecificTechnologyQuery(lowerQuery, specificTerms) {
		var nonRepoTerms []string
		repoNameLower := strings.ToLower(scope.RepositoryID)
		for _, st := range specificTerms {
			if !strings.Contains(repoNameLower, st) && !strings.Contains(st, "fastapi") && !strings.Contains(st, "gin") && !strings.Contains(st, "worldmonitor") && !strings.Contains(st, "portfolio") && !strings.Contains(st, "ripgrep") && !strings.Contains(st, "varuns") {
				nonRepoTerms = append(nonRepoTerms, st)
			}
		}

		if len(nonRepoTerms) > 0 {
			var missingTerms []string
			for _, st := range nonRepoTerms {
				matched := false
				for _, item := range items {
					lowerContent := strings.ToLower(item.Content)
					lowerPath := strings.ToLower(item.RelativePath)
					if strings.Contains(lowerContent, st) || strings.Contains(lowerPath, st) {
						matched = true
						break
					}
				}
				if !matched {
					missingTerms = append(missingTerms, st)
				}
			}

			if len(missingTerms) > 0 {
				return models.EvidenceSufficiencyResult{
					Status:             models.SufficiencyInsufficient,
					TargetFound:        false,
					SourceAvailable:    false,
					ResolutionCoverage: 0.0,
					EvidenceCoverage:   0.0,
					HeuristicScore:     0.0,
					Reasons:            []string{fmt.Sprintf("Requested technology or topic '%s' is not present in repository evidence", strings.Join(missingTerms, ", "))},
					MissingInfo:        []string{"No matching dependency, configuration, or source code declaration found for requested feature"},
				}, nil
			}
		}
	}

	// 2. FILE-SPECIFIC QUERY EVALUATION
	if isFileQuery && targetFile != "" {
		targetFileLower := strings.ToLower(targetFile)
		targetFileBase := filepath.Base(targetFileLower)

		var matchedFileItem *models.EvidenceItem
		for _, item := range items {
			itemPathLower := strings.ToLower(item.RelativePath)
			itemBase := filepath.Base(itemPathLower)
			if itemPathLower == targetFileLower || itemBase == targetFileBase || strings.HasSuffix(itemPathLower, "/"+targetFileLower) {
				matchedFileItem = item
				break
			}
			if strings.Contains(strings.ToLower(item.Content), targetFileLower) || strings.Contains(strings.ToLower(item.Content), targetFileBase) {
				matchedFileItem = item
				break
			}
		}

		if matchedFileItem != nil {
			return models.EvidenceSufficiencyResult{
				Status:             models.SufficiencySufficient,
				TargetFound:        true,
				SourceAvailable:    true,
				ResolutionCoverage: 1.0,
				EvidenceCoverage:   1.0,
				HeuristicScore:     1.0,
				Reasons:            []string{"Target file contents and context present in retrieved evidence"},
			}, nil
		}

		return models.EvidenceSufficiencyResult{
			Status:             models.SufficiencyInsufficient,
			TargetFound:        false,
			SourceAvailable:    false,
			ResolutionCoverage: 0.0,
			EvidenceCoverage:   0.0,
			HeuristicScore:     0.0,
			Reasons:            []string{"Target file not found in repository index or evidence"},
			MissingInfo:        []string{targetFile + " source snippet or declaration missing"},
		}, nil
	}

	// 3. ARCHITECTURE / PROJECT-PURPOSE QUERY EVALUATION
	if isArchQuery {
		hasOverviewEvidence := false
		for _, item := range items {
			lowerPath := strings.ToLower(item.RelativePath)
			base := filepath.Base(lowerPath)
			if item.RetrieverType == "SYSTEM_CONTEXT_README" || item.RetrieverType == "SYSTEM_CONTEXT_SETUP" ||
				item.RetrieverType == "SYSTEM_CONTEXT_ENTRYPOINT" || item.RetrieverType == "SYSTEM_CONTEXT_TREE" ||
				item.Type == models.EvidenceTypeDocumentation || base == "readme.md" || base == "package.json" ||
				base == "go.mod" || base == "cargo.toml" || base == "pyproject.toml" || base == "setup.py" || base == "requirements.txt" ||
				strings.HasPrefix(base, "main.") || strings.HasPrefix(base, "app.") || strings.HasPrefix(base, "lib.") || base == "gin.go" || base == "run.py" || base == "index.ts" {
				hasOverviewEvidence = true
				break
			}
		}

		if hasOverviewEvidence {
			return models.EvidenceSufficiencyResult{
				Status:             models.SufficiencySufficient,
				TargetFound:        true,
				SourceAvailable:    true,
				ResolutionCoverage: 1.0,
				EvidenceCoverage:   1.0,
				HeuristicScore:     1.0,
				Reasons:            []string{"System overview, entry point, and repository manifest evidence present"},
			}, nil
		}

		return models.EvidenceSufficiencyResult{
			Status:             models.SufficiencyInsufficient,
			TargetFound:        false,
			SourceAvailable:    false,
			ResolutionCoverage: 0.0,
			EvidenceCoverage:   0.0,
			HeuristicScore:     0.2,
			Reasons:            []string{"Insufficient repository-wide architecture evidence retrieved for project-level query"},
			MissingInfo:        []string{"README documentation, package manifest, or application entry point evidence missing"},
		}, nil
	}

	// 4. DEPENDENCY / BUILD CONFIGURATION QUERY EVALUATION
	if isDependencyQuery {
		hasDependencyEvidence := false
		for _, item := range items {
			lowerPath := strings.ToLower(item.RelativePath)
			base := filepath.Base(lowerPath)
			if item.RetrieverType == "SYSTEM_CONTEXT_SETUP" || item.Type == models.EvidenceTypeDependency ||
				base == "package.json" || base == "go.mod" || base == "cargo.toml" || base == "pyproject.toml" ||
				base == "requirements.txt" || base == "setup.py" || base == "setup.cfg" || base == "pipfile" ||
				base == "pom.xml" || base == "build.gradle" || base == "composer.json" || base == "gemfile" ||
				base == "docker-compose.yml" || base == "dockerfile" || base == "makefile" || base == "cmakelists.txt" ||
				base == "tsconfig.json" || base == "vite.config.ts" || base == "next.config.js" || base == "wrangler.json" {
				hasDependencyEvidence = true
				break
			}
		}

		if hasDependencyEvidence {
			return models.EvidenceSufficiencyResult{
				Status:             models.SufficiencySufficient,
				TargetFound:        true,
				SourceAvailable:    true,
				ResolutionCoverage: 1.0,
				EvidenceCoverage:   1.0,
				HeuristicScore:     1.0,
				Reasons:            []string{"Package manifest, dependency declarations, or build configuration present"},
			}, nil
		}
	}

	// 5. SYMBOL / FUNCTION QUERY EVALUATION
	if isSymbolQuery {
		hasSymbolEvidence := false
		var resolvedCount, unresolvedCount int
		for _, item := range items {
			if item.Type == models.EvidenceTypeSymbol || item.Type == models.EvidenceTypeCodeSnippet || item.RetrieverType == "SELECTED_SYMBOL" {
				if targetSymbol != "" {
					if strings.Contains(strings.ToLower(item.Content), strings.ToLower(targetSymbol)) || strings.Contains(strings.ToLower(item.RelativePath), strings.ToLower(targetSymbol)) {
						hasSymbolEvidence = true
					}
				} else {
					hasSymbolEvidence = true
				}
			}
			if item.ResolutionStatus == models.RelStatusResolved {
				resolvedCount++
			} else if item.ResolutionStatus == models.RelStatusUnresolved {
				unresolvedCount++
			}
		}

		if hasSymbolEvidence {
			resRatio := 1.0
			if resolvedCount+unresolvedCount > 0 {
				resRatio = float64(resolvedCount) / float64(resolvedCount+unresolvedCount)
			}
			status := models.SufficiencySufficient
			if unresolvedCount > 0 && resolvedCount == 0 {
				status = models.SufficiencyPartial
			}
			return models.EvidenceSufficiencyResult{
				Status:             status,
				TargetFound:        true,
				SourceAvailable:    true,
				ResolutionCoverage: resRatio,
				EvidenceCoverage:   1.0,
				HeuristicScore:     1.0,
				Reasons:            []string{"Target symbol definition or code context present in retrieved evidence"},
			}, nil
		}

		return models.EvidenceSufficiencyResult{
			Status:             models.SufficiencyInsufficient,
			TargetFound:        false,
			SourceAvailable:    false,
			ResolutionCoverage: 0.0,
			EvidenceCoverage:   0.0,
			HeuristicScore:     0.0,
			Reasons:            []string{"Target symbol definition not found in repository index or evidence"},
		}, nil
	}

	// 6. FLOW / CALL-PATH QUERY EVALUATION
	if isFlowQuery {
		hasFlowEvidence := false
		isHTTPRequestQuery := strings.Contains(lowerQuery, "request") || strings.Contains(lowerQuery, "http") || strings.Contains(lowerQuery, "api")

		for _, item := range items {
			lowerContent := strings.ToLower(item.Content)
			lowerPath := strings.ToLower(item.RelativePath)

			if isHTTPRequestQuery {
				// Require request, router, endpoint, handler, server, worker, or API evidence for HTTP request flow queries
				if item.RetrieverType == "FLOW_CONTEXT" || item.Type == models.EvidenceTypeStaticFlow || item.Type == models.EvidenceTypeGraphEdge ||
					strings.Contains(lowerPath, "route") || strings.Contains(lowerPath, "router") ||
					strings.Contains(lowerPath, "api") || strings.Contains(lowerPath, "handler") ||
					strings.Contains(lowerPath, "controller") || strings.Contains(lowerPath, "server") ||
					strings.Contains(lowerPath, "worker") || strings.Contains(lowerPath, "endpoint") ||
					strings.Contains(lowerPath, "gin") || strings.Contains(lowerPath, "app.py") ||
					strings.Contains(lowerContent, "servehttp") || strings.Contains(lowerContent, "handlehttprequest") ||
					strings.Contains(lowerContent, "apirouter") || strings.Contains(lowerContent, "fastapi") {
					hasFlowEvidence = true
					break
				}
			} else {
				// General execution flow query
				if item.Type == models.EvidenceTypeStaticFlow || item.Type == models.EvidenceTypeGraphEdge ||
					item.RetrieverType == "STATIC_FLOW" || item.RetrieverType == "FLOW_CONTEXT" ||
					item.RetrieverType == "SYSTEM_CONTEXT_ENTRYPOINT" || item.RetrieverType == "SYSTEM_CONTEXT_README" ||
					item.Type == models.EvidenceTypeDocumentation ||
					strings.Contains(lowerContent, "main") || strings.Contains(lowerPath, "main") ||
					strings.Contains(lowerContent, "flow") || strings.Contains(lowerContent, "call") ||
					strings.Contains(lowerPath, "server") || strings.Contains(lowerPath, "worker") ||
					strings.Contains(lowerPath, "api") || strings.Contains(lowerPath, "index") ||
					strings.Contains(lowerPath, "app") || strings.Contains(lowerPath, "route") {
					hasFlowEvidence = true
					break
				}
			}
		}

		if hasFlowEvidence {
			return models.EvidenceSufficiencyResult{
				Status:             models.SufficiencySufficient,
				TargetFound:        true,
				SourceAvailable:    true,
				ResolutionCoverage: 1.0,
				EvidenceCoverage:   1.0,
				HeuristicScore:     1.0,
				Reasons:            []string{"Connected call path, static flow steps, or entry point evidence present"},
			}, nil
		}

		return models.EvidenceSufficiencyResult{
			Status:             models.SufficiencyInsufficient,
			TargetFound:        false,
			SourceAvailable:    false,
			ResolutionCoverage: 0.0,
			EvidenceCoverage:   0.0,
			HeuristicScore:     0.0,
			Reasons:            []string{"Execution flow path or call edges not found in evidence"},
		}, nil
	}

	// 7. GENERAL / FALLBACK EVALUATION (with cleaned query tokens)
	queryWords := strings.Fields(lowerQuery)
	matchedTermsMap := make(map[string]bool)
	var nonStopTerms []string
	for _, qw := range queryWords {
		clean := CleanToken(qw)
		if len(clean) >= 3 && !isStopWord(clean) {
			nonStopTerms = append(nonStopTerms, clean)
		}
	}

	queryRelevantItems := 0
	for _, item := range items {
		lowerContent := strings.ToLower(item.Content)
		lowerPath := strings.ToLower(item.RelativePath)
		for _, qw := range nonStopTerms {
			stem := qw
			if len(qw) > 5 {
				stem = qw[:5]
			}
			if strings.Contains(lowerContent, qw) || strings.Contains(lowerPath, qw) || strings.Contains(lowerContent, stem) || strings.Contains(lowerPath, stem) {
				queryRelevantItems++
				matchedTermsMap[qw] = true
			}
		}
	}

	termCoverage := 1.0
	if len(nonStopTerms) > 0 {
		termCoverage = float64(len(matchedTermsMap)) / float64(len(nonStopTerms))
	}

	if queryRelevantItems > 0 && (len(nonStopTerms) < 2 || termCoverage >= 0.4) {
		return models.EvidenceSufficiencyResult{
			Status:             models.SufficiencySufficient,
			TargetFound:        true,
			SourceAvailable:    true,
			ResolutionCoverage: 1.0,
			EvidenceCoverage:   1.0,
			HeuristicScore:     1.0,
			Reasons:            []string{"Retrieved evidence contains query-relevant code snippets or symbols"},
		}, nil
	}

	return models.EvidenceSufficiencyResult{
		Status:             models.SufficiencyInsufficient,
		TargetFound:        false,
		SourceAvailable:    false,
		ResolutionCoverage: 0.0,
		EvidenceCoverage:   0.0,
		HeuristicScore:     0.0,
		Reasons:            []string{"Retrieved evidence does not contain query-relevant symbol or snippet matches"},
	}, nil
}

func extractSpecificTechnicalTerms(lowerQuery string) []string {
	words := strings.FieldsFunc(lowerQuery, func(r rune) bool {
		return r == ' ' || r == '/' || r == '-' || r == '_' || r == ',' || r == '.' || r == '?' || r == '!' || r == ':' || r == ';' || r == '(' || r == ')' || r == '"' || r == '\''
	})
	var specific []string
	for _, w := range words {
		clean := CleanToken(w)
		if len(clean) >= 3 && !isStopWord(clean) && !isGenericConceptWord(clean) {
			specific = append(specific, clean)
		}
	}
	return specific
}

func isGenericConceptWord(w string) bool {
	switch w {
	case "architecture", "overview", "project", "repository", "codebase", "execution", "flow", "start", "starting", "starts", "entry", "point", "dependency", "dependencies", "build", "configuration", "config", "technology", "technologies", "framework", "library", "module", "file", "source", "code", "structure", "design", "pattern", "patterns", "component", "components", "function", "class", "method", "type", "interface", "struct", "one", "major", "authentication", "auth", "login", "security", "user", "users", "request", "response", "api", "database", "db", "server", "client", "controller", "handler", "service", "services", "model", "view", "route", "router", "test", "tests", "work", "works", "working", "how", "what", "where", "why", "manage", "managing", "management", "manager", "handle", "handling", "process", "processing", "processor", "perform", "performing", "support", "supporting", "execute", "executing", "run", "running", "do", "doing", "get", "getting", "set", "setting", "fetch", "fetching", "send", "sending", "receive", "receiving", "call", "calling", "called", "validate", "validating", "validation", "check", "checking", "checker", "verify", "verifying", "verification", "logic", "rule", "rules", "data", "input", "output", "param", "params", "parameter", "parameters", "incoming", "outgoing", "registered", "register", "registering", "registration", "received", "default", "custom", "internal", "external", "simple", "complex", "new", "old", "specific", "given", "return", "returned", "returning", "returns", "reach", "reaches", "reaching", "produce", "produces", "producing", "result", "results", "endpoint", "endpoints", "routes", "routers", "routing", "path", "paths", "url", "urls":
		return true
	default:
		return false
	}
}

func isSpecificTechnologyQuery(lowerQuery string, specificTerms []string) bool {
	if strings.Contains(lowerQuery, "why") || strings.Contains(lowerQuery, "what") || strings.Contains(lowerQuery, "how") || strings.Contains(lowerQuery, "does") || strings.Contains(lowerQuery, "is") || strings.Contains(lowerQuery, "can") {
		for _, term := range specificTerms {
			if !isGenericConceptWord(term) {
				return true
			}
		}
	}
	return false
}

func isStopWord(w string) bool {
	switch w {
	case "for", "the", "where", "what", "with", "from", "this", "that", "have", "has", "had", "are", "was", "were", "been", "being", "does", "did", "how", "why", "who", "which", "and", "but", "not", "all", "any", "can", "could", "would", "should", "your", "its", "explain", "describe", "summary", "overview", "component", "function", "class", "method", "source", "based", "execution", "flow", "code", "implementation", "details", "application", "initialize", "system", "setup", "service", "services", "start", "work", "works", "project", "repo", "repository", "codebase", "use", "uses", "using", "used", "one", "main", "major", "important", "like", "beginner", "someone", "tell", "show", "me", "to", "through", "across", "into", "within", "between", "during":
		return true
	default:
		return false
	}
}

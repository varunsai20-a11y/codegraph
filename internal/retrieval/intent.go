package retrieval

import (
	"regexp"
	"strings"
)

const (
	IntentSymbolLookup           = "SYMBOL_LOOKUP"
	IntentCallerQuery            = "CALLER_QUERY"
	IntentCalleeQuery            = "CALLEE_QUERY"
	IntentDependencyQuery        = "DEPENDENCY_QUERY"
	IntentImpactQuery            = "IMPACT_QUERY"
	IntentFeatureSearch          = "FEATURE_SEARCH"
	IntentArchitectureQuery      = "ARCHITECTURE_QUERY"
	IntentTrace                  = "TRACE"
	IntentExplanation            = "EXPLANATION"
	IntentGeneralRepositoryQuery = "GENERAL_REPOSITORY_QUESTION"
)

// QueryIntentClassifier defines the interface for intent classification.
type QueryIntentClassifier interface {
	Classify(query string) string
}

// RuleBasedIntentClassifier classifies developer query strings into discrete query intents
// using deterministic regex and keyword heuristics.
//
// DETERMINISTIC INTENT PRECEDENCE:
// 1. High-Precision Structural Relationship Queries (CALLER, CALLEE, IMPACT, DEPENDENCY, TRACE, ARCHITECTURE)
//   - Specific structural intents override generic verbs (e.g. "Explain who calls Login" -> CALLER_QUERY).
//
// 2. Explicit Symbol Definition & Declaration Queries (SYMBOL_LOOKUP)
//   - e.g. "Where is AuthService defined?", "func LoginController", "auth.Login"
//
// 3. Explanation Queries (EXPLANATION)
//   - e.g. "Explain the authentication flow", "overview of indexer"
//
// 4. Feature Search Fallback (FEATURE_SEARCH)
//   - Multi-word feature/concept queries without structural keywords.
//
// 5. Default Fallback (GENERAL_REPOSITORY_QUESTION)
type RuleBasedIntentClassifier struct{}

func NewRuleBasedIntentClassifier() *RuleBasedIntentClassifier {
	return &RuleBasedIntentClassifier{}
}

var (
	callerRegex       = regexp.MustCompile(`(?i)\b(who calls|callers of|calls to|where is .* called|called by)\b`)
	calleeRegex       = regexp.MustCompile(`(?i)\b(what does .* call|callees of|functions called by|calls from)\b`)
	impactRegex       = regexp.MustCompile(`(?i)\b(what breaks|impact of|affected by|if i modify|what happens if i modify|change impact|ripple effect)\b`)
	dependencyRegex   = regexp.MustCompile(`(?i)\b(depend on|depends on|dependency|dependencies|imports of|imported by|package dependencies|modules used by)\b`)
	traceRegex        = regexp.MustCompile(`(?i)\b(trace|call stack|execution path|flow from|request path)\b`)
	archRegex         = regexp.MustCompile(`(?i)\b(entry point|architecture|system structure|package structure|how is .* organized|project layout)\b`)
	symbolWhereRegex  = regexp.MustCompile(`(?i)\bwhere is\s+([A-Za-z0-9_.]+)\s+defined\b`)
	symbolLookupRegex = regexp.MustCompile(`(?i)\b(func|function|class|struct|type|interface|enum|method|var|const)\s+([A-Za-z0-9_]+)`)
	explainRegex      = regexp.MustCompile(`(?i)\b(explain|overview of|describe|summary of)\b`)
)

func (c *RuleBasedIntentClassifier) Classify(query string) string {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return IntentGeneralRepositoryQuery
	}

	lower := strings.ToLower(trimmed)

	// Step 1: Specific structural relationship queries (overrides generic explain verbs)
	if callerRegex.MatchString(lower) {
		return IntentCallerQuery
	}
	if calleeRegex.MatchString(lower) {
		return IntentCalleeQuery
	}
	if impactRegex.MatchString(lower) {
		return IntentImpactQuery
	}
	if dependencyRegex.MatchString(lower) {
		return IntentDependencyQuery
	}
	if traceRegex.MatchString(lower) {
		return IntentTrace
	}
	if archRegex.MatchString(lower) {
		return IntentArchitectureQuery
	}

	// Step 2: Symbol lookup (e.g. "Where is AuthService defined?", "func Login", "auth.Login")
	if symbolWhereRegex.MatchString(lower) || symbolLookupRegex.MatchString(trimmed) || isSingleIdentifierOrQualifiedSymbol(trimmed) {
		return IntentSymbolLookup
	}

	// Step 3: Explanation queries
	if explainRegex.MatchString(lower) {
		return IntentExplanation
	}

	// Step 4: Feature search fallback (multi-word natural language query)
	if strings.Contains(lower, "how to") || strings.Contains(lower, "how does") || len(strings.Fields(lower)) >= 3 {
		return IntentFeatureSearch
	}

	return IntentGeneralRepositoryQuery
}

func isSingleIdentifierOrQualifiedSymbol(s string) bool {
	fields := strings.Fields(s)
	if len(fields) != 1 {
		return false
	}
	token := fields[0]
	for _, r := range token {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '.') {
			return false
		}
	}
	return len(token) > 1
}

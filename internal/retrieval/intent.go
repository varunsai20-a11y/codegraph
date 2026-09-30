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

	IntentGreeting     = "INTENT_GREETING"
	IntentRepoOverview = "INTENT_REPO_OVERVIEW"
	IntentCodeQuery    = "INTENT_CODE_QUERY"
)

// QueryIntentClassifier defines the interface for intent classification.
type QueryIntentClassifier interface {
	Classify(query string) string
}

// RuleBasedIntentClassifier classifies developer query strings into discrete query intents
// using deterministic regex and keyword heuristics.
type RuleBasedIntentClassifier struct{}

func NewRuleBasedIntentClassifier() *RuleBasedIntentClassifier {
	return &RuleBasedIntentClassifier{}
}

var (
	greetingRegex     = regexp.MustCompile(`(?i)^\s*(hi|hello|hey|greetings|good morning|good afternoon|good evening|who are you|what can you do|help me|howdy)\s*[\.!\?]?$`)
	greetingSubRegex  = regexp.MustCompile(`(?i)\b(who are you|what can you do)\b`)
	repoOverviewRegex = regexp.MustCompile(`(?i)\b(explain the code|explain codebase|explain repository|explain repo|explain project|how does this project work|how do i run|how to run|overview|what does this repo do|what does this project do|what is this repository for|what is this repo for|what is this project for|explain the architecture|repository overview|project overview|architecture overview|getting started|run this)\b`)
	purposeRegex      = regexp.MustCompile(`(?i)\b(what does .* do|project purpose|purpose of|what is this project|about this project|about this repo)\b`)
	callerRegex       = regexp.MustCompile(`(?i)\b(who calls|callers of|calls to|where is .* called|called by)\b`)
	calleeRegex       = regexp.MustCompile(`(?i)\b(what does .* call|callees of|functions called by|calls from)\b`)
	impactRegex       = regexp.MustCompile(`(?i)\b(what breaks|impact of|affected by|if i modify|what happens if i modify|change impact|ripple effect)\b`)
	dependencyRegex   = regexp.MustCompile(`(?i)\b(depend on|depends on|dependency|dependencies|imports of|imported by|package dependencies|modules used by)\b`)
	traceRegex        = regexp.MustCompile(`(?i)\b(trace|call stack|execution path|flow from|request path)\b`)
	archRegex         = regexp.MustCompile(`(?i)\b(entry point|architecture|system structure|package structure|how is .* organized|project layout)\b`)
	symbolWhereRegex  = regexp.MustCompile(`(?i)\bwhere is\s+([A-Za-z0-9_.]+)\s+defined\b`)
	symbolLookupRegex = regexp.MustCompile(`(?i)\b(func|function|class|struct|type|interface|enum|method|var|const)\s+([A-Za-z0-9_]+)`)
	explainRegex       = regexp.MustCompile(`(?i)\b(explain|overview of|describe|summary of)\b`)
	featureSearchRegex = regexp.MustCompile(`(?i)\b(how to|implement|handling|logic for|where is the code for)\b`)
	codeQueryRegex     = regexp.MustCompile(`(?i)\b(function|func|class|method|symbol|struct|interface|variable|type|handler|controller)\b`)
)

func (c *RuleBasedIntentClassifier) Classify(query string) string {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return IntentGeneralRepositoryQuery
	}

	lower := strings.ToLower(trimmed)

	// Step 1: Greeting & Conversational Inputs
	if greetingRegex.MatchString(lower) || greetingSubRegex.MatchString(lower) || lower == "hi" || lower == "hello" || lower == "hey" || lower == "help" {
		return IntentGreeting
	}

	// Step 2: High-Precision Structural Relationship Queries
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
	if traceRegex.MatchString(lower) || isExecutionFlowQuery(lower) {
		return IntentTrace
	}

	// Step 3: Symbol Lookup
	if symbolWhereRegex.MatchString(lower) || symbolLookupRegex.MatchString(trimmed) || isSingleIdentifierOrQualifiedSymbol(trimmed) {
		return IntentSymbolLookup
	}

	// Step 4: Repository Overview & Startup Queries
	if repoOverviewRegex.MatchString(lower) || isStartupQuery(lower) || lower == "overview" {
		return IntentRepoOverview
	}

	if purposeRegex.MatchString(lower) || archRegex.MatchString(lower) {
		return IntentArchitectureQuery
	}

	// Step 5: Explanation Queries
	if explainRegex.MatchString(lower) {
		return IntentExplanation
	}

	// Step 6: Feature Search vs Code Query
	if featureSearchRegex.MatchString(lower) {
		return IntentFeatureSearch
	}

	if codeQueryRegex.MatchString(lower) || strings.Contains(lower, "how does") || len(strings.Fields(lower)) >= 3 {
		return IntentCodeQuery
	}

	return IntentGeneralRepositoryQuery
}

func isStartupQuery(lower string) bool {
	return strings.Contains(lower, "start") || strings.Contains(lower, "starts") ||
		strings.Contains(lower, "startup") || strings.Contains(lower, "initialize") ||
		strings.Contains(lower, "initialization") || strings.Contains(lower, "after i run") ||
		strings.Contains(lower, "getting started")
}

func isExecutionFlowQuery(lower string) bool {
	return strings.Contains(lower, "execution flow") || strings.Contains(lower, "execution path") ||
		strings.Contains(lower, "execution begin") || strings.Contains(lower, "where does execution") ||
		strings.Contains(lower, "walk me through")
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

package retrieval

import (
	"path/filepath"
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
	IntentFileQuery    = "FILE_QUERY"
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
	repoOverviewRegex = regexp.MustCompile(`(?i)\b(summaris[ea]|summary|overview|explain the code|explain codebase|explain repository|explain repo|explain project|explain this project|explain this repository|explain this repo|how does this project work|how does this repo work|how does this codebase work|how do i run|how to run|what does this repo do|what does this project do|what is this repository for|what is this repo for|what is this project for|explain the architecture|repository overview|project overview|architecture overview|getting started|run this|beginner)\b`)
	purposeRegex      = regexp.MustCompile(`(?i)\b(what does .* do|project purpose|purpose of|what is this project|about this project|about this repo|summarise|summarize|summary|high level|overview)\b`)
	callerRegex       = regexp.MustCompile(`(?i)\b(who calls|callers of|calls to|where is .* called|called by)\b`)
	calleeRegex       = regexp.MustCompile(`(?i)\b(what does .* call|callees of|functions called by|calls from)\b`)
	impactRegex       = regexp.MustCompile(`(?i)\b(what breaks|impact of|affected by|if i modify|what happens if i modify|change impact|ripple effect)\b`)
	dependencyRegex   = regexp.MustCompile(`(?i)\b(depend on|depends on|dependency|dependencies|imports of|imported by|package dependencies|modules used by|build config|build configuration|package manifest)\b`)
	traceRegex        = regexp.MustCompile(`(?i)\b(trace|call stack|execution path|flow from|request path|request flow|flow of request|flow through|request lifecycle|data flow|execution flow)\b`)
	archRegex         = regexp.MustCompile(`(?i)\b(entry point|architecture|system structure|package structure|how is .* organized|project layout)\b`)
	symbolWhereRegex  = regexp.MustCompile(`(?i)\bwhere is\s+(?:func|function|class|struct|method|type|interface)?\s*([A-Za-z0-9_.]+)\s+defined\b`)
	symbolLookupRegex = regexp.MustCompile(`(?i)\b(func|function|class|struct|type|interface|enum|method|var|const)\s+([A-Za-z0-9_]+)`)
	symbolPostRegex   = regexp.MustCompile(`(?i)\b([A-Za-z0-9_]+)\s+(class|function|func|struct|type|interface|method|handler|controller)\b`)
	explainRegex       = regexp.MustCompile(`(?i)\b(explain|overview of|describe|summary of)\b`)
	featureSearchRegex = regexp.MustCompile(`(?i)\b(how to|implement|handling|logic for|where is the code for)\b`)
	codeQueryRegex     = regexp.MustCompile(`(?i)\b(function|func|class|method|symbol|struct|interface|variable|type|handler|controller)\b`)

	knownFileExtensions = map[string]bool{
		".js": true, ".jsx": true, ".ts": true, ".tsx": true, ".go": true,
		".py": true, ".java": true, ".c": true, ".cpp": true, ".h": true,
		".hpp": true, ".cs": true, ".rs": true, ".json": true, ".yaml": true,
		".yml": true, ".toml": true, ".md": true, ".xml": true, ".css": true,
		".scss": true, ".html": true, ".sh": true, ".bat": true, ".sql": true,
		".mod": true, ".sum": true, ".env": true, ".config": true,
	}
)

// CleanToken removes leading and trailing punctuation from a token string.
func CleanToken(s string) string {
	s = strings.TrimSpace(s)
	cutset := ".,?!:;\"'()[]{}<>"
	return strings.Trim(s, cutset)
}

// ExtractTargetFile returns the target file path or file name referenced in a query.
func ExtractTargetFile(query string) string {
	fields := strings.Fields(query)
	for i, f := range fields {
		clean := CleanToken(f)
		if clean == "" {
			continue
		}
		lowerClean := strings.ToLower(clean)
		// Explicit "file <target>" or "in <target>" pattern
		if (lowerClean == "file" || lowerClean == "in" || lowerClean == "of") && i+1 < len(fields) {
			nextClean := CleanToken(fields[i+1])
			if nextClean != "" {
				ext := strings.ToLower(filepath.Ext(nextClean))
				if knownFileExtensions[ext] || strings.Contains(nextClean, "/") || strings.Contains(nextClean, "\\") {
					return nextClean
				}
			}
		}
		ext := strings.ToLower(filepath.Ext(clean))
		if knownFileExtensions[ext] {
			return clean
		}
		if (strings.Contains(clean, "/") || strings.Contains(clean, "\\")) && ext != "" {
			return clean
		}
	}
	return ""
}

// ExtractTargetSymbol returns the target symbol referenced in a query.
func ExtractTargetSymbol(query string) string {
	trimmed := strings.TrimSpace(query)
	if isSingleIdentifierOrQualifiedSymbol(trimmed) {
		return CleanToken(trimmed)
	}
	matches := symbolWhereRegex.FindStringSubmatch(query)
	if len(matches) > 1 {
		return CleanToken(matches[1])
	}
	matches = symbolPostRegex.FindStringSubmatch(query)
	if len(matches) > 1 {
		sym := CleanToken(matches[1])
		if !isStopWord(strings.ToLower(sym)) && len(sym) >= 2 {
			return sym
		}
	}
	matches = symbolLookupRegex.FindStringSubmatch(query)
	if len(matches) > 2 {
		sym := CleanToken(matches[2])
		lowerSym := strings.ToLower(sym)
		if !isStopWord(lowerSym) && lowerSym != "based" && lowerSym != "definition" && lowerSym != "implementation" && lowerSym != "overview" && lowerSym != "details" && lowerSym != "source" && lowerSym != "code" && lowerSym != "logic" && lowerSym != "file" && lowerSym != "files" {
			return sym
		}
	}
	return ""
}

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

	// Step 3.5: File-Specific Queries
	if targetFile := ExtractTargetFile(query); targetFile != "" {
		return IntentFileQuery
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
		strings.Contains(lower, "walk me through") || strings.Contains(lower, "request flow") ||
		strings.Contains(lower, "request lifecycle") || strings.Contains(lower, "how does a request flow") ||
		strings.Contains(lower, "how requests are handled") || strings.Contains(lower, "data flow") ||
		strings.Contains(lower, "flow through") || strings.Contains(lower, "request to the response") ||
		strings.Contains(lower, "request reach") || strings.Contains(lower, "get called and return") ||
		strings.Contains(lower, "route an incoming") || strings.Contains(lower, "process a registered route")
}

func isSingleIdentifierOrQualifiedSymbol(s string) bool {
	fields := strings.Fields(s)
	if len(fields) != 1 {
		return false
	}
	token := CleanToken(fields[0])
	for _, r := range token {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '.') {
			return false
		}
	}
	return len(token) > 1
}

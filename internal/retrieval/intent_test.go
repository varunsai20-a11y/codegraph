package retrieval_test

import (
	"testing"

	"codegraph/internal/retrieval"
)

func TestRuleBasedIntentClassifier_PrecedenceAndAmbiguousQueries(t *testing.T) {
	classifier := retrieval.NewRuleBasedIntentClassifier()

	tests := []struct {
		name     string
		query    string
		expected string
	}{
		{
			name:     "User Example 1: Who calls auth.Login?",
			query:    "Who calls auth.Login?",
			expected: retrieval.IntentCallerQuery,
		},
		{
			name:     "User Example 2: Explain who calls Login (Structural intent overrides Explain)",
			query:    "Explain who calls Login",
			expected: retrieval.IntentCallerQuery,
		},
		{
			name:     "User Example 3: What dependencies does AuthService have?",
			query:    "What dependencies does AuthService have?",
			expected: retrieval.IntentDependencyQuery,
		},
		{
			name:     "User Example 4: What happens if I modify UserService?",
			query:    "What happens if I modify UserService?",
			expected: retrieval.IntentImpactQuery,
		},
		{
			name:     "User Example 5: Where is the authentication entry point?",
			query:    "Where is the authentication entry point?",
			expected: retrieval.IntentArchitectureQuery,
		},
		{
			name:     "User Example 6: Explain the authentication flow",
			query:    "Explain the authentication flow",
			expected: retrieval.IntentExplanation,
		},
		{
			name:     "User Example 7: Where is AuthService defined?",
			query:    "Where is AuthService defined?",
			expected: retrieval.IntentSymbolLookup,
		},
		{
			name:     "Callee Query",
			query:    "what does RunIndex call?",
			expected: retrieval.IntentCalleeQuery,
		},
		{
			name:     "Trace Query",
			query:    "trace HTTP request call stack",
			expected: retrieval.IntentTrace,
		},
		{
			name:     "Symbol Lookup Keyword",
			query:    "func LoginController",
			expected: retrieval.IntentSymbolLookup,
		},
		{
			name:     "Symbol Lookup Identifier",
			query:    "LoginController",
			expected: retrieval.IntentSymbolLookup,
		},
		{
			name:     "Symbol Lookup Qualified",
			query:    "auth.LoginController.Login",
			expected: retrieval.IntentSymbolLookup,
		},
		{
			name:     "Feature Search Phrase Fallback",
			query:    "how to handle password hashing logic",
			expected: retrieval.IntentFeatureSearch,
		},
		{
			name:     "Empty Query Fallback",
			query:    "   ",
			expected: retrieval.IntentGeneralRepositoryQuery,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := classifier.Classify(tc.query)
			if actual != tc.expected {
				t.Errorf("query '%s': expected intent '%s', got '%s'", tc.query, tc.expected, actual)
			}
		})
	}
}

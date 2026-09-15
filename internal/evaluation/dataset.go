package evaluation

// LoadGoldenDataset constructs the deterministic golden query evaluation dataset.
func LoadGoldenDataset(repoID string) GoldenDataset {
	if repoID == "" {
		repoID = "codegraph"
	}

	cases := []EvaluationCase{
		// 1. SYMBOL_LOOKUP
		{
			ID:             "case-01",
			Query:          "Where is EvidencePackage defined?",
			ExpectedIntent: "SYMBOL_LOOKUP",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"EvidencePackage", "internal/models/retrieval.go"},
			Description:    "Lookup definition location of EvidencePackage symbol",
		},
		{
			ID:             "case-02",
			Query:          "Find definition of HybridRetrieverEngine",
			ExpectedIntent: "SYMBOL_LOOKUP",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"HybridRetrieverEngine", "internal/retrieval/hybrid.go"},
			Description:    "Lookup definition of HybridRetrieverEngine struct",
		},
		{
			ID:             "case-03",
			Query:          "Show declaration of Location struct",
			ExpectedIntent: "SYMBOL_LOOKUP",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"Location", "internal/models/models.go"},
			Description:    "Lookup Location struct declaration",
		},

		// 2. CALLER_QUERY
		{
			ID:             "case-04",
			Query:          "Who calls BuildGroundedContext?",
			ExpectedIntent: "CALLER_QUERY",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"BuildGroundedContext", "GroundedExplanationService", "internal/llm/service.go"},
			Description:    "Find all callers invoking BuildGroundedContext",
		},
		{
			ID:             "case-05",
			Query:          "What functions call ValidateItem?",
			ExpectedIntent: "CALLER_QUERY",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"ValidateItem", "AddItem", "DefaultEvidencePackager"},
			Description:    "Find callers of RepositoryScope.ValidateItem",
		},

		// 3. CALLEE_QUERY
		{
			ID:             "case-06",
			Query:          "What does FinalizePackage call?",
			ExpectedIntent: "CALLEE_QUERY",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"FinalizePackage", "EstimateTokens", "Citation"},
			Description:    "Find outgoing function calls from FinalizePackage",
		},
		{
			ID:             "case-07",
			Query:          "What functions does NewHybridRetrieverEngine invoke?",
			ExpectedIntent: "CALLEE_QUERY",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"NewRuleBasedIntentClassifier", "HybridRetrieverEngine"},
			Description:    "Find callees of NewHybridRetrieverEngine constructor",
		},

		// 4. DEPENDENCY_QUERY
		{
			ID:             "case-08",
			Query:          "What packages does internal/retrieval depend on?",
			ExpectedIntent: "DEPENDENCY_QUERY",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"internal/models", "internal/graph", "internal/storage"},
			Description:    "Find module import dependencies of retrieval package",
		},
		{
			ID:             "case-09",
			Query:          "What dependencies does internal/llm have?",
			ExpectedIntent: "DEPENDENCY_QUERY",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"internal/models", "internal/retrieval"},
			Description:    "Find module imports for llm package",
		},

		// 5. IMPACT_QUERY
		{
			ID:             "case-10",
			Query:          "What components are affected if I modify internal/models/retrieval.go?",
			ExpectedIntent: "IMPACT_QUERY",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"internal/retrieval", "internal/llm", "internal/analysis"},
			Description:    "Transitive impact analysis for retrieval models modification",
		},
		{
			ID:             "case-11",
			Query:          "What files depend on internal/graph/engine.go?",
			ExpectedIntent: "IMPACT_QUERY",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"internal/retrieval", "graph_retriever.go"},
			Description:    "Impact analysis for graph engine changes",
		},

		// 6. FEATURE_SEARCH
		{
			ID:             "case-12",
			Query:          "How is Reciprocal Rank Fusion calculated?",
			ExpectedIntent: "FEATURE_SEARCH",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"RRFScore", "rrfContribution", "internal/retrieval/hybrid.go"},
			Description:    "Find implementation of RRF mathematical scoring",
		},
		{
			ID:             "case-13",
			Query:          "Find line-level snippet truncation logic",
			ExpectedIntent: "FEATURE_SEARCH",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"MaxSnippetLines", "internal/retrieval/packager.go"},
			Description:    "Search snippet line truncation implementation",
		},

		// 7. ARCHITECTURE_QUERY
		{
			ID:             "case-14",
			Query:          "Explain the architecture of the hybrid retriever engine",
			ExpectedIntent: "ARCHITECTURE_QUERY",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"HybridRetrieverEngine", "QueryIntentClassifier", "internal/retrieval/hybrid.go"},
			Description:    "Architectural overview of hybrid retrieval orchestration",
		},
		{
			ID:             "case-15",
			Query:          "How does citation validation work in CodeGraph?",
			ExpectedIntent: "ARCHITECTURE_QUERY",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"CitationValidator", "Validate", "internal/llm/validator.go"},
			Description:    "Architectural query on citation validation design",
		},

		// 8. TRACE
		{
			ID:             "case-16",
			Query:          "Trace execution flow from user query to grounded explanation",
			ExpectedIntent: "TRACE",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"Retrieve", "BuildPackage", "BuildGroundedContext", "Explain"},
			Description:    "Trace end-to-end execution flow through retrieval, packaging, and LLM explanation",
		},
		{
			ID:             "case-17",
			Query:          "Trace how evidence sufficiency is evaluated",
			ExpectedIntent: "TRACE",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"SufficiencyEvaluator", "Evaluate", "internal/retrieval/sufficiency.go"},
			Description:    "Trace sufficiency score calculation flow",
		},

		// 9. EXPLANATION
		{
			ID:             "case-18",
			Query:          "Why does EvidencePackage clone items before adding?",
			ExpectedIntent: "EXPLANATION",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"Clone", "immutability", "internal/retrieval/packager.go"},
			Description:    "Explanation query regarding input immutability guarantee",
		},
		{
			ID:             "case-19",
			Query:          "Why are source code strings wrapped in untrusted content boundaries?",
			ExpectedIntent: "EXPLANATION",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"UNTRUSTED REPOSITORY CONTENT", "internal/retrieval/context_builder.go"},
			Description:    "Explanation query regarding prompt injection structural defense",
		},

		// 10. GENERAL_REPOSITORY_QUESTION
		{
			ID:             "case-20",
			Query:          "Overview of CodeGraph codebase structure",
			ExpectedIntent: "GENERAL_REPOSITORY_QUESTION",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"internal/models", "internal/retrieval", "internal/graph", "internal/llm"},
			Description:    "General overview query of CodeGraph architecture",
		},
		{
			ID:             "case-21",
			Query:          "What vector distance metric is used by SQLite store?",
			ExpectedIntent: "GENERAL_REPOSITORY_QUESTION",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"CosineDistance", "internal/vector"},
			Description:    "General repository query regarding vector distance metric",
		},
		{
			ID:             "case-22",
			Query:          "How is repository isolation enforced in scope?",
			ExpectedIntent: "SYMBOL_LOOKUP",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"RepositoryScope", "ValidateItem", "internal/models/retrieval.go"},
			Description:    "Lookup RepositoryScope validation contract",
		},
		{
			ID:             "case-23",
			Query:          "Who calls LoadGraph?",
			ExpectedIntent: "CALLER_QUERY",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"LoadGraph", "GraphRetriever", "internal/retrieval/graph_retriever.go"},
			Description:    "Callers of graph Engine.LoadGraph",
		},
		{
			ID:             "case-24",
			Query:          "What does SanitizeRetrieverError return for timeout?",
			ExpectedIntent: "SYMBOL_LOOKUP",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"sanitizeRetrieverError", "EXECUTION_TIMEOUT", "internal/retrieval/hybrid.go"},
			Description:    "Lookup error code for deadline exceeded in hybrid retriever",
		},
		{
			ID:             "case-25",
			Query:          "Where is DefaultEvidenceBudget defined?",
			ExpectedIntent: "SYMBOL_LOOKUP",
			RepositoryID:   repoID,
			RelevantIDs:    []string{"DefaultEvidenceBudget", "internal/models/retrieval.go"},
			Description:    "Lookup DefaultEvidenceBudget constructor",
		},
	}

	return GoldenDataset{
		Version:    "1.0.0",
		Cases:      cases,
		TotalCases: len(cases),
	}
}

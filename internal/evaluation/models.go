package evaluation

import (
	"time"

	"codegraph/internal/models"
)

// EvaluationCase represents a single golden query test case with ground-truth relevance.
type EvaluationCase struct {
	ID             string   `json:"id"`
	Query          string   `json:"query"`
	ExpectedIntent string   `json:"expected_intent"`
	RepositoryID   string   `json:"repository_id"`
	RelevantIDs    []string `json:"relevant_ids"` // Ground-truth symbol/file/node IDs or paths
	Description    string   `json:"description"`
}

// GoldenDataset represents a collection of golden query test cases.
type GoldenDataset struct {
	Version    string           `json:"version"`
	Cases      []EvaluationCase `json:"cases"`
	TotalCases int              `json:"total_cases"`
}

// RetrievalMetrics holds standard evaluation metrics at rank K.
type RetrievalMetrics struct {
	PrecisionAt1 float64 `json:"precision_at_1"`
	PrecisionAt5 float64 `json:"precision_at_5"`
	RecallAt5    float64 `json:"recall_at_5"`
	MRR          float64 `json:"mrr"`
	HitRateAt5   float64 `json:"hit_rate_at_5"`
	K            int     `json:"k"`
}

// RetrieverComparisonResult compares retrieval quality across individual and hybrid retrievers.
type RetrieverComparisonResult struct {
	RetrieverType string           `json:"retriever_type"` // LEXICAL, SEMANTIC, HYBRID
	Metrics       RetrievalMetrics `json:"metrics"`
}

// IntentCaseDetail records expected vs predicted intent for each golden case.
type IntentCaseDetail struct {
	CaseID          string `json:"case_id"`
	Query           string `json:"query"`
	ExpectedIntent  string `json:"expected_intent"`
	PredictedIntent string `json:"predicted_intent"`
	Passed          bool   `json:"passed"`
}

// IntentEvalResult holds evaluation metrics and 10x10 confusion matrix for intent classification.
type IntentEvalResult struct {
	TotalQueries       int                       `json:"total_queries"`
	CorrectQueries     int                       `json:"correct_queries"`
	Accuracy           float64                   `json:"accuracy"`
	PerIntentPrecision map[string]float64        `json:"per_intent_precision"`
	PerIntentRecall    map[string]float64        `json:"per_intent_recall"`
	ConfusionMatrix    map[string]map[string]int `json:"confusion_matrix"`
	CaseDetails        []IntentCaseDetail        `json:"case_details,omitempty"`
}

// StructuralDiagnostic provides detail for a failed structural graph evaluation case.
type StructuralDiagnostic struct {
	Query            string   `json:"query"`
	Category         string   `json:"category"`
	ExpectedNodeIDs  []string `json:"expected_node_ids"`
	ActualNodeIDs    []string `json:"actual_node_ids"`
	ResolutionState  string   `json:"resolution_state"`
	CandidateSymbols []string `json:"candidate_symbols,omitempty"`
	ExpectedEdges    int      `json:"expected_edges"`
	ActualEdges      int      `json:"actual_edges"`
	MismatchReason   string   `json:"mismatch_reason"`
}

// GraphCorrectnessResult holds evaluation results for graph structural retrieval operations.
type GraphCorrectnessResult struct {
	CallerAccuracy           float64                `json:"caller_accuracy"`
	CalleeAccuracy           float64                `json:"callee_accuracy"`
	DependencyAccuracy       float64                `json:"dependency_accuracy"`
	DependentsAccuracy       float64                `json:"dependents_accuracy"`
	ImpactAccuracy           float64                `json:"impact_accuracy"`
	NeighborhoodAccuracy     float64                `json:"neighborhood_accuracy"`
	TargetResolutionAccuracy float64                `json:"target_resolution_accuracy"`
	NodeSetPrecision         float64                `json:"node_set_precision"`
	NodeSetRecall            float64                `json:"node_set_recall"`
	ExactMatchRate           float64                `json:"exact_match_rate"`
	EdgeCorrectnessRate      float64                `json:"edge_correctness_rate"`
	OverallGraphScore        float64                `json:"overall_graph_score"`
	Diagnostics              []StructuralDiagnostic `json:"diagnostics,omitempty"`
}

// PerIntentRetrievalMetric records per-intent MRR and HitRate@5 comparisons across Lexical, Semantic, and Hybrid retrievers.
type PerIntentRetrievalMetric struct {
	Intent       string  `json:"intent"`
	CaseCount    int     `json:"case_count"`
	LexicalMRR   float64 `json:"lexical_mrr"`
	SemanticMRR  float64 `json:"semantic_mrr"`
	HybridMRR    float64 `json:"hybrid_mrr"`
	LexicalHit5  float64 `json:"lexical_hit5"`
	SemanticHit5 float64 `json:"semantic_hit5"`
	HybridHit5   float64 `json:"hybrid_hit5"`
}

// HybridCaseDiagnostic traces fusion mechanics and rank displacement for a golden case.
type HybridCaseDiagnostic struct {
	CaseID           string   `json:"case_id"`
	Query            string   `json:"query"`
	Intent           string   `json:"intent"`
	ExpectedIDs      []string `json:"expected_ids"`
	LexicalRank      int      `json:"lexical_rank"`  // 0 if unranked
	SemanticRank     int      `json:"semantic_rank"` // 0 if unranked
	GraphRank        int      `json:"graph_rank"`    // 0 if unranked
	AppliedWeights   string   `json:"applied_weights"`
	FinalRRFScore    float64  `json:"final_rrf_score"`
	FinalHybridRank  int      `json:"final_hybrid_rank"`
	DisplacedFromTop bool     `json:"displaced_from_top"`
	RegressionReason string   `json:"regression_reason"`
}

// StageAResult represents the outcome of Stage A (Retrieval & Ranking Evaluation).
type StageAResult struct {
	Comparisons        []RetrieverComparisonResult `json:"comparisons"`
	PerIntentRetrieval []PerIntentRetrievalMetric  `json:"per_intent_retrieval,omitempty"`
	HybridDiagnostics  []HybridCaseDiagnostic      `json:"hybrid_diagnostics,omitempty"`
	IntentEval         IntentEvalResult            `json:"intent_eval"`
	GraphEval          GraphCorrectnessResult      `json:"graph_eval"`
	EvaluatedCases     int                         `json:"evaluated_cases"`
}

// StageBResult represents the outcome of Stage B (Grounding & Citation Evaluation).
type StageBResult struct {
	TotalScenarios            int                       `json:"total_scenarios"`
	ValidCitationPrecision    float64                   `json:"valid_citation_precision"`
	ValidCitationRecall       float64                   `json:"valid_citation_recall"`
	ValidCitationF1           float64                   `json:"valid_citation_f1"`
	InvalidDetectionRecall    float64                   `json:"invalid_detection_recall"`
	InvalidDetectionPrecision float64                   `json:"invalid_detection_precision"`
	CitationCoverage          float64                   `json:"citation_coverage"`
	BudgetComplianceRate      float64                   `json:"budget_compliance_rate"`
	AmbiguityPreservedRate    float64                   `json:"ambiguity_preserved_rate"`
	InsufficiencyHandled      bool                      `json:"insufficiency_handled"`
	CitationConfusionMatrix   map[string]map[string]int `json:"citation_confusion_matrix"`
}

// RealRepoMetrics contains indexing, retrieval latency, and memory allocations for a real repository benchmark.
type RealRepoMetrics struct {
	RepositoryName    string        `json:"repository_name"`
	TotalFiles        int           `json:"total_files"`
	IndexedFiles      int           `json:"indexed_files"`
	Languages         []string      `json:"languages,omitempty"`
	TotalSymbols      int           `json:"total_symbols"`
	Relationships     int           `json:"relationships"`
	GraphNodes        int           `json:"graph_nodes"`
	GraphEdges        int           `json:"graph_edges"`
	IndexDuration     time.Duration `json:"index_duration"`
	ColdRetrievalMs   float64       `json:"cold_retrieval_ms"`
	WarmRetrievalMs   float64       `json:"warm_retrieval_ms"`
	PackagingMs       float64       `json:"packaging_ms"`
	ValidationMs      float64       `json:"validation_ms"`
	MemoryAllocatedMB float64       `json:"memory_allocated_mb"`
	TotalAllocations  uint64        `json:"total_allocations"`
}

// EvaluationReport represents the complete master evaluation report.
type EvaluationReport struct {
	Timestamp             time.Time         `json:"timestamp"`
	StageA                StageAResult      `json:"stage_a"`
	StageB                StageBResult      `json:"stage_b"`
	RealRepo              RealRepoMetrics   `json:"real_repo"`
	RealRepos             []RealRepoMetrics `json:"real_repos"`
	Strengths             []string          `json:"strengths"`
	Weaknesses            []string          `json:"weaknesses"`
	Bottlenecks           []string          `json:"bottlenecks"`
	Limitations           []string          `json:"limitations"`
	Phase5Recommendations []string          `json:"phase5_recommendations"`
}

var _ = models.DefaultEvidenceBudget

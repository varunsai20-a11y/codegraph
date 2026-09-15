package retrieval_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"codegraph/internal/models"
	"codegraph/internal/retrieval"
)

// MockRetriever for deterministic unit testing
type MockRetriever struct {
	items []*models.EvidenceItem
	err   error
}

func (m *MockRetriever) Retrieve(ctx context.Context, scope models.RepositoryScope, query string, intent string, limit int) ([]*models.EvidenceItem, error) {
	if m.err != nil {
		return nil, m.err
	}
	// Filter by repo scope for safety in mock
	var res []*models.EvidenceItem
	for _, it := range m.items {
		if it.RepositoryID == scope.RepositoryID {
			res = append(res, it)
		}
	}
	if limit > 0 && len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

func TestHybridRetriever_RRFMathematics(t *testing.T) {
	scope, _ := models.NewRepositoryScope("repo-1")

	// Lexical rank 1, Semantic rank 3
	lexicalMock := &MockRetriever{items: []*models.EvidenceItem{
		{StableID: "item-A", RepositoryID: "repo-1", Content: "func ItemA()", RawScore: 100.0, Rank: 1},
	}}
	semanticMock := &MockRetriever{items: []*models.EvidenceItem{
		{StableID: "other", RepositoryID: "repo-1", Content: "func Other()", RawScore: 90.0, Rank: 1},
		{StableID: "other2", RepositoryID: "repo-1", Content: "func Other2()", RawScore: 80.0, Rank: 2},
		{StableID: "item-A", RepositoryID: "repo-1", Content: "func ItemA()", RawScore: 70.0, Rank: 3},
	}}

	config := retrieval.DefaultHybridRetrievalConfig()
	config.RRFK = 60.0
	// Equal weights 1.0 for lexical and semantic, 0 for graph
	config.IntentWeights["SYMBOL_LOOKUP"] = retrieval.RetrieverWeights{Lexical: 1.0, Semantic: 1.0, Graph: 0.0}

	engine := retrieval.NewHybridRetrieverEngine(nil, lexicalMock, semanticMock, nil, config)

	res, err := engine.Retrieve(context.Background(), scope, "func ItemA")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expected RRF(itemA) = 1.0/(60 + 1) + 1.0/(60 + 3) = 1/61 + 1/63 = 0.01639344 + 0.01587301 = 0.03226645
	expectedRRF := 1.0/61.0 + 1.0/63.0

	var foundItem *models.EvidenceItem
	for _, it := range res.Items {
		if it.StableID == "item-A" {
			foundItem = it
			break
		}
	}

	if foundItem == nil {
		t.Fatalf("item-A not found in fused results")
	}

	if math.Abs(foundItem.RRFScore-expectedRRF) > 1e-6 {
		t.Errorf("expected RRFScore %f, got %f", expectedRRF, foundItem.RRFScore)
	}
}

func TestHybridRetriever_DeduplicationAndIndependentSurvival(t *testing.T) {
	scope, _ := models.NewRepositoryScope("repo-1")

	// C1 appears in both Lexical and Semantic; C2 is Lexical-only; C3 is Semantic-only
	itemC1Lex := &models.EvidenceItem{StableID: "C1", RepositoryID: "repo-1", Content: "C1 Lexical", Rank: 1, RawScore: 100}
	itemC2Lex := &models.EvidenceItem{StableID: "C2", RepositoryID: "repo-1", Content: "C2 Lexical", Rank: 2, RawScore: 80}

	itemC1Sem := &models.EvidenceItem{StableID: "C1", RepositoryID: "repo-1", Content: "C1 Semantic", Rank: 2, RawScore: 0.9}
	itemC3Sem := &models.EvidenceItem{StableID: "C3", RepositoryID: "repo-1", Content: "C3 Semantic", Rank: 1, RawScore: 0.95}

	lexMock := &MockRetriever{items: []*models.EvidenceItem{itemC1Lex, itemC2Lex}}
	semMock := &MockRetriever{items: []*models.EvidenceItem{itemC3Sem, itemC1Sem}}

	config := retrieval.DefaultHybridRetrievalConfig()
	engine := retrieval.NewHybridRetrieverEngine(nil, lexMock, semMock, nil, config)

	res, err := engine.Retrieve(context.Background(), scope, "Login")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Total unique items must be 3 (C1, C2, C3)
	if res.TotalFused != 3 {
		t.Errorf("expected TotalFused = 3, got %d", res.TotalFused)
	}

	// C1 must appear ONCE as HYBRID
	c1Count := 0
	for _, it := range res.Items {
		if it.StableID == "C1" {
			c1Count++
			if it.RetrieverType != "HYBRID" {
				t.Errorf("expected C1 RetrieverType = HYBRID, got %s", it.RetrieverType)
			}
		}
	}
	if c1Count != 1 {
		t.Errorf("expected C1 to appear exactly once, got %d times", c1Count)
	}
}

func TestHybridRetriever_DeterministicOrdering(t *testing.T) {
	scope, _ := models.NewRepositoryScope("repo-1")

	items := []*models.EvidenceItem{
		{StableID: "stable-1", RepositoryID: "repo-1", Content: "Content 1", Rank: 1, RawScore: 50},
		{StableID: "stable-2", RepositoryID: "repo-1", Content: "Content 2", Rank: 2, RawScore: 50},
		{StableID: "stable-3", RepositoryID: "repo-1", Content: "Content 3", Rank: 3, RawScore: 50},
	}
	lexMock := &MockRetriever{items: items}

	config := retrieval.DefaultHybridRetrievalConfig()
	engine := retrieval.NewHybridRetrieverEngine(nil, lexMock, nil, nil, config)

	run1, _ := engine.Retrieve(context.Background(), scope, "func Test")
	run2, _ := engine.Retrieve(context.Background(), scope, "func Test")

	if len(run1.Items) != len(run2.Items) {
		t.Fatalf("run length mismatch: %d vs %d", len(run1.Items), len(run2.Items))
	}

	for i := range run1.Items {
		if run1.Items[i].StableID != run2.Items[i].StableID {
			t.Errorf("StableID mismatch at index %d: %s vs %s", i, run1.Items[i].StableID, run2.Items[i].StableID)
		}
		if run1.Items[i].RRFScore != run2.Items[i].RRFScore {
			t.Errorf("RRFScore mismatch at index %d: %f vs %f", i, run1.Items[i].RRFScore, run2.Items[i].RRFScore)
		}
	}
}

func TestHybridRetriever_IntentRouting(t *testing.T) {
	classifier := retrieval.NewRuleBasedIntentClassifier()

	tests := []struct {
		query    string
		expected string
	}{
		{"Where is AuthService defined?", retrieval.IntentSymbolLookup},
		{"Who calls auth.Login?", retrieval.IntentCallerQuery},
		{"What dependencies does AuthService have?", retrieval.IntentDependencyQuery},
		{"What happens if I modify UserService?", retrieval.IntentImpactQuery},
		{"Explain the authentication flow", retrieval.IntentExplanation},
	}

	for _, tc := range tests {
		actual := classifier.Classify(tc.query)
		if actual != tc.expected {
			t.Errorf("query '%s': expected intent '%s', got '%s'", tc.query, tc.expected, actual)
		}
	}
}

func TestHybridRetriever_RepositoryIsolation(t *testing.T) {
	scopeA, _ := models.NewRepositoryScope("repo-A")

	itemsA := []*models.EvidenceItem{{StableID: "item-A", RepositoryID: "repo-A", Content: "Repo A"}}
	itemsB := []*models.EvidenceItem{{StableID: "item-B", RepositoryID: "repo-B", Content: "Repo B"}}

	lexMock := &MockRetriever{items: append(itemsA, itemsB...)}

	config := retrieval.DefaultHybridRetrievalConfig()
	engine := retrieval.NewHybridRetrieverEngine(nil, lexMock, nil, nil, config)

	res, err := engine.Retrieve(context.Background(), scopeA, "Login")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, item := range res.Items {
		if item.RepositoryID != "repo-A" {
			t.Errorf("repository isolation violated! expected repo-A, got %s", item.RepositoryID)
		}
	}
}

func TestHybridRetriever_FailureIsolation(t *testing.T) {
	scope, _ := models.NewRepositoryScope("repo-1")

	lexMock := &MockRetriever{items: []*models.EvidenceItem{
		{StableID: "lex-1", RepositoryID: "repo-1", Content: "Lexical Item", Rank: 1},
	}}
	// Mock semantic failure with raw internal error string
	semMock := &MockRetriever{err: errors.New("vector database connection error: /var/db/secret_vector_store.db failed")}

	config := retrieval.DefaultHybridRetrievalConfig()
	engine := retrieval.NewHybridRetrieverEngine(nil, lexMock, semMock, nil, config)

	res, err := engine.Retrieve(context.Background(), scope, "Login")
	if err != nil {
		t.Fatalf("expected hybrid retrieval to succeed gracefully despite semantic failure, got error: %v", err)
	}

	failure, found := res.Failed["VECTOR"]
	if !found {
		t.Fatalf("expected Failed diagnostics to record VECTOR failure, got %+v", res.Failed)
	}
	if failure.RetrieverType != "VECTOR" || failure.Status != "FAILED" || failure.ErrorCode != retrieval.ErrCodeRetrieverUnavailable {
		t.Errorf("unexpected failure diagnostic: %+v", failure)
	}
	if len(res.Succeeded) != 1 || res.Succeeded[0] != "LEXICAL" {
		t.Errorf("expected Succeeded diagnostics = ['LEXICAL'], got %v", res.Succeeded)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 lexical item, got %d", len(res.Items))
	}
}

func TestHybridRetriever_SanitizeRetrieverFailureDiagnostics(t *testing.T) {
	scope, _ := models.NewRepositoryScope("repo-sensitive")

	rawSensitiveErrStr := "open /var/secret/private_repo_keys.pem: permission denied"
	semMock := &MockRetriever{err: errors.New(rawSensitiveErrStr)}

	config := retrieval.DefaultHybridRetrievalConfig()
	config.IntentWeights["GENERAL_QUERY"] = retrieval.RetrieverWeights{Semantic: 1.0}

	engine := retrieval.NewHybridRetrieverEngine(nil, nil, semMock, nil, config)

	res, err := engine.Retrieve(context.Background(), scope, "Login")
	if err == nil {
		t.Fatalf("expected error when all retrievers fail, got nil")
	}

	// Verify that raw internal error string is NOT present in error output
	errStr := err.Error()
	if strings.Contains(errStr, rawSensitiveErrStr) || strings.Contains(errStr, "/var/secret") {
		t.Errorf("raw sensitive error string leaked in error output: %s", errStr)
	}

	// Verify structured error code is present
	if !strings.Contains(errStr, retrieval.ErrCodeRetrieverUnavailable) {
		t.Errorf("expected error string to contain classified error code '%s', got '%s'", retrieval.ErrCodeRetrieverUnavailable, errStr)
	}
	_ = res
}

func TestHybridRetriever_ConfigurationValidation(t *testing.T) {
	cfg1 := retrieval.DefaultHybridRetrievalConfig()
	cfg1.RRFK = 0
	if err := cfg1.Validate(); err != retrieval.ErrInvalidRRFK {
		t.Errorf("expected ErrInvalidRRFK for RRFK=0, got %v", err)
	}

	cfg2 := retrieval.DefaultHybridRetrievalConfig()
	cfg2.TopK = 0
	if err := cfg2.Validate(); err != retrieval.ErrInvalidTopK {
		t.Errorf("expected ErrInvalidTopK for TopK=0, got %v", err)
	}

	cfg3 := retrieval.DefaultHybridRetrievalConfig()
	cfg3.CandidateLimit = -5
	if err := cfg3.Validate(); err != retrieval.ErrInvalidCandidateLimit {
		t.Errorf("expected ErrInvalidCandidateLimit for negative limit, got %v", err)
	}

	cfg4 := retrieval.DefaultHybridRetrievalConfig()
	cfg4.IntentWeights["SYMBOL_LOOKUP"] = retrieval.RetrieverWeights{Lexical: -1.0, Semantic: 0.5, Graph: 0.5}
	if err := cfg4.Validate(); err == nil {
		t.Errorf("expected error for negative weight, got nil")
	}
}

func TestHybridRetriever_TopKTruncationAfterFusion(t *testing.T) {
	scope, _ := models.NewRepositoryScope("repo-1")

	// Generate 15 items
	var items []*models.EvidenceItem
	for i := 1; i <= 15; i++ {
		items = append(items, &models.EvidenceItem{
			StableID:     fmt.Sprintf("item-%d", i),
			RepositoryID: "repo-1",
			Content:      "Item",
			Rank:         i,
			RawScore:     float64(100 - i),
		})
	}

	lexMock := &MockRetriever{items: items}
	config := retrieval.DefaultHybridRetrievalConfig()
	config.TopK = 5
	config.CandidateLimit = 20

	engine := retrieval.NewHybridRetrieverEngine(nil, lexMock, nil, nil, config)
	res, err := engine.Retrieve(context.Background(), scope, "Login")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.TotalFused != 15 {
		t.Errorf("expected TotalFused = 15 before truncation, got %d", res.TotalFused)
	}
	if len(res.Items) != 5 {
		t.Errorf("expected final TopK items = 5 after truncation, got %d", len(res.Items))
	}
}

func TestHybridRetriever_AllRRFInvariants(t *testing.T) {
	scope, _ := models.NewRepositoryScope("repo-inv")

	// 1. Lexical candidate limit=50, top candidate at rank=1
	lexItems := []*models.EvidenceItem{
		{StableID: "sym-1", RepositoryID: "repo-inv", Content: "func Target()", Rank: 1, RawScore: 100},
		{StableID: "sym-2", RepositoryID: "repo-inv", Content: "func Other()", Rank: 2, RawScore: 90},
	}
	// 2. Semantic candidate at rank=1 (noisy) and rank=2 (duplicate sym-1)
	semItems := []*models.EvidenceItem{
		{StableID: "noisy-sem", RepositoryID: "repo-inv", Content: "noisy embedding match", Rank: 1, RawScore: 0.99},
		{StableID: "sym-1", RepositoryID: "repo-inv", Content: "func Target()", Rank: 2, RawScore: 0.95},
	}

	lexMock := &MockRetriever{items: lexItems}
	semMock := &MockRetriever{items: semItems}

	config := retrieval.DefaultHybridRetrievalConfig()
	config.TopK = 5
	config.CandidateLimit = 50
	config.RRFK = 60.0
	config.IntentWeights["SYMBOL_LOOKUP"] = retrieval.RetrieverWeights{Lexical: 1.0, Semantic: 0.5, Graph: 0.0}

	engine := retrieval.NewHybridRetrieverEngine(nil, lexMock, semMock, nil, config)
	res, err := engine.Retrieve(context.Background(), scope, "Where is Target defined?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Invariant 1 & 2: Rank values start at 1
	// Invariant 3: RRF formula = weight / (60 + rank)
	// RRF(sym-1) = 1.0/(60+1) + 0.5/(60+2) = 1/61 + 0.5/62 = 0.01639344 + 0.00806451 = 0.02445795
	expRRF_sym1 := 1.0/61.0 + 0.5/62.0

	// Invariant 7: Deduplication uses StableID
	if res.TotalFused != 3 {
		t.Errorf("Invariant 7 (Deduplication) failed: expected 3 fused items, got %d", res.TotalFused)
	}

	// Invariant 8: Duplicate evidence merged correctly into sym-1
	if res.Items[0].StableID != "sym-1" {
		t.Errorf("Invariant 9 (Deterministic ordering) failed: expected top item sym-1, got %s", res.Items[0].StableID)
	}
	if math.Abs(res.Items[0].RRFScore-expRRF_sym1) > 1e-6 {
		t.Errorf("Invariant 3 (RRF formula) & Invariant 4 (Weights) failed: expected RRF %f, got %f", expRRF_sym1, res.Items[0].RRFScore)
	}

	// Invariant 6: TopK applied only after fusion
	if len(res.Items) > 5 {
		t.Errorf("Invariant 6 (TopK after fusion) failed: got %d items", len(res.Items))
	}
}

package retrieval_test

import (
	"context"
	"testing"

	"codegraph/internal/models"
	"codegraph/internal/retrieval"
)

func TestSufficiencyEvaluator_EmptyEvidence(t *testing.T) {
	evaluator := retrieval.NewDefaultSufficiencyEvaluator()
	scope, _ := models.NewRepositoryScope("repo-1")

	res, err := evaluator.Evaluate(context.Background(), scope, "Where is Login handled?", "SYMBOL_LOOKUP", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != models.SufficiencyInsufficient {
		t.Errorf("expected SUFFICIENCY_INSUFFICIENT, got %s", res.Status)
	}
	if res.TargetFound {
		t.Errorf("expected TargetFound = false for empty items")
	}
	if res.HeuristicScore != 0.0 {
		t.Errorf("expected HeuristicScore 0.0, got %f", res.HeuristicScore)
	}
}

func TestSufficiencyEvaluator_RepositoryIsolation(t *testing.T) {
	evaluator := retrieval.NewDefaultSufficiencyEvaluator()
	scopeA, _ := models.NewRepositoryScope("repo-A")

	items := []*models.EvidenceItem{
		{
			RepositoryID: "repo-A",
			Type:         models.EvidenceTypeSymbol,
			Content:      "func Auth()",
		},
		{
			RepositoryID: "repo-B", // Mismatched repo!
			Type:         models.EvidenceTypeSymbol,
			Content:      "func Leak()",
		},
	}

	_, err := evaluator.Evaluate(context.Background(), scopeA, "Auth function", "SYMBOL_LOOKUP", items)
	if err == nil {
		t.Fatalf("expected ErrRepositoryMismatch when items contain repo-B in repo-A scope")
	}
}

func TestSufficiencyEvaluator_HighSufficiency(t *testing.T) {
	evaluator := retrieval.NewDefaultSufficiencyEvaluator()
	scope, _ := models.NewRepositoryScope("repo-1")

	items := []*models.EvidenceItem{
		{
			RepositoryID:     "repo-1",
			Type:             models.EvidenceTypeSymbol,
			Content:          "func AuthenticateUser(ctx, creds)",
			ResolutionStatus: models.RelStatusResolved,
		},
		{
			RepositoryID:     "repo-1",
			Type:             models.EvidenceTypeGraphEdge,
			Content:          "LoginController CALLS AuthenticateUser",
			ResolutionStatus: models.RelStatusResolved,
		},
		{
			RepositoryID:     "repo-1",
			Type:             models.EvidenceTypeCodeSnippet,
			Content:          "func AuthenticateUser(...) { return db.Verify(...) }",
			ResolutionStatus: models.RelStatusResolved,
		},
	}

	res, err := evaluator.Evaluate(context.Background(), scope, "How does authentication work?", "EXPLANATION", items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != models.SufficiencySufficient {
		t.Errorf("expected SUFFICIENCY_SUFFICIENT, got %s", res.Status)
	}
	if !res.TargetFound {
		t.Errorf("expected TargetFound = true")
	}
	if res.ResolutionCoverage != 1.0 {
		t.Errorf("expected ResolutionCoverage = 1.0, got %f", res.ResolutionCoverage)
	}
	if res.HeuristicScore < 0.7 {
		t.Errorf("expected HeuristicScore >= 0.7, got %f", res.HeuristicScore)
	}
}

func TestSufficiencyEvaluator_RankingVsSufficiencyIndependence(t *testing.T) {
	evaluator := retrieval.NewDefaultSufficiencyEvaluator()
	scope, _ := models.NewRepositoryScope("repo-1")

	// High RRF rank score (0.99) but unresolved relationships and no code snippet
	items := []*models.EvidenceItem{
		{
			RepositoryID:     "repo-1",
			Type:             models.EvidenceTypeGraphEdge,
			Content:          "UnknownCaller CALLS DynamicTarget",
			RawScore:         99.0,
			Rank:             1,
			RRFScore:         0.99, // High RRF rank score
			ResolutionStatus: models.RelStatusUnresolved,
		},
	}

	res, err := evaluator.Evaluate(context.Background(), scope, "What calls DynamicTarget?", "CALLER_QUERY", items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should NOT be SUFFICIENCY_SUFFICIENT despite high RRF score because target snippet is missing and relation is unresolved
	if res.Status == models.SufficiencySufficient {
		t.Errorf("expected insufficient or partial status despite high RRF rank score, got %s", res.Status)
	}
	if res.ResolutionCoverage != 0.0 {
		t.Errorf("expected ResolutionCoverage = 0.0 for unresolved relation, got %f", res.ResolutionCoverage)
	}
}

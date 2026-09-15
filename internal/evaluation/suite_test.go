package evaluation_test

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"codegraph/internal/evaluation"
	"codegraph/internal/models"
)

func TestMetrics_Calculations(t *testing.T) {
	retrieved := []string{"item1", "item2", "item3", "item4", "item5"}
	groundTruth := []string{"item2", "item5", "item9"}

	// Precision@5: 2 hits out of 5 = 0.4
	prec := evaluation.CalculatePrecisionAtK(retrieved, groundTruth, 5)
	if math.Abs(prec-0.4) > 1e-6 {
		t.Errorf("expected Precision@5 = 0.4, got %f", prec)
	}

	// Recall@5: 2 hits out of 3 ground truth = 0.666667
	rec := evaluation.CalculateRecallAtK(retrieved, groundTruth, 5)
	if math.Abs(rec-0.666667) > 1e-4 {
		t.Errorf("expected Recall@5 = 0.6667, got %f", rec)
	}

	// MRR: First hit at rank 2 (item2) -> 1/2 = 0.5
	mrr := evaluation.CalculateMRR(retrieved, groundTruth)
	if math.Abs(mrr-0.5) > 1e-6 {
		t.Errorf("expected MRR = 0.5, got %f", mrr)
	}

	// HitRate@5: At least 1 hit in top 5 -> 1.0
	hit := evaluation.CalculateHitRateAtK(retrieved, groundTruth, 5)
	if math.Abs(hit-1.0) > 1e-6 {
		t.Errorf("expected HitRate@5 = 1.0, got %f", hit)
	}
}

func TestGoldenDataset_Validation(t *testing.T) {
	dataset := evaluation.LoadGoldenDataset("codegraph")

	if dataset.TotalCases < 20 {
		t.Fatalf("expected at least 20 evaluation cases in golden dataset, got %d", dataset.TotalCases)
	}

	seenIDs := make(map[string]bool)
	intentsSeen := make(map[string]bool)

	for _, c := range dataset.Cases {
		if c.ID == "" {
			t.Errorf("evaluation case has empty ID")
		}
		if seenIDs[c.ID] {
			t.Errorf("duplicate evaluation case ID: %s", c.ID)
		}
		seenIDs[c.ID] = true
		intentsSeen[c.ExpectedIntent] = true

		if c.Query == "" {
			t.Errorf("case %s has empty query", c.ID)
		}
		if len(c.RelevantIDs) == 0 {
			t.Errorf("case %s has empty ground-truth relevant IDs", c.ID)
		}
	}

	// Verify all 10 intent categories are represented
	expectedIntents := []string{
		"SYMBOL_LOOKUP", "CALLER_QUERY", "CALLEE_QUERY", "DEPENDENCY_QUERY",
		"IMPACT_QUERY", "FEATURE_SEARCH", "ARCHITECTURE_QUERY", "TRACE",
		"EXPLANATION", "GENERAL_REPOSITORY_QUESTION",
	}

	for _, intent := range expectedIntents {
		if !intentsSeen[intent] {
			t.Errorf("golden dataset missing evaluation cases for intent: %s", intent)
		}
	}
}

func TestEvaluationSuite_FullExecutionAndReportGeneration(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "eval_out")
	suite := evaluation.NewEvaluationSuite()

	// Run evaluation suite on current workspace repository (codegraph)
	report, err := suite.Run(context.Background(), ".", outDir)
	if err != nil {
		t.Fatalf("evaluation suite run failed: %v", err)
	}

	if report == nil {
		t.Fatalf("expected non-nil evaluation report")
	}

	// Verify generated artifact files
	goldenPath := filepath.Join(outDir, "golden_cases.json")
	if _, err := os.Stat(goldenPath); os.IsNotExist(err) {
		t.Errorf("golden_cases.json file was not created at %s", goldenPath)
	}

	resultsPath := filepath.Join(outDir, "retrieval_results.json")
	if _, err := os.Stat(resultsPath); os.IsNotExist(err) {
		t.Errorf("retrieval_results.json file was not created at %s", resultsPath)
	}

	reportPath := filepath.Join(outDir, "benchmark_report.md")
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		t.Errorf("benchmark_report.md file was not created at %s", reportPath)
	}

	reportContent, _ := os.ReadFile(reportPath)
	contentStr := string(reportContent)

	if len(contentStr) == 0 {
		t.Errorf("benchmark_report.md is empty")
	}

	if models.DefaultEvidenceBudget().MaxTokens <= 0 {
		t.Errorf("invalid default evidence budget max tokens")
	}
}

func TestEvaluationSuite_Reproducibility(t *testing.T) {
	suite := evaluation.NewEvaluationSuite()
	ctx := context.Background()

	dir1 := filepath.Join(t.TempDir(), "eval_run1")
	dir2 := filepath.Join(t.TempDir(), "eval_run2")

	rep1, err1 := suite.Run(ctx, ".", dir1)
	if err1 != nil {
		t.Fatalf("Run 1 failed: %v", err1)
	}

	rep2, err2 := suite.Run(ctx, ".", dir2)
	if err2 != nil {
		t.Fatalf("Run 2 failed: %v", err2)
	}

	g1, _ := os.ReadFile(filepath.Join(dir1, "golden_cases.json"))
	g2, _ := os.ReadFile(filepath.Join(dir2, "golden_cases.json"))
	if string(g1) != string(g2) {
		t.Errorf("golden_cases.json mismatch across runs")
	}

	if rep1.StageA.IntentEval.Accuracy != rep2.StageA.IntentEval.Accuracy {
		t.Errorf("Stage A Intent Accuracy mismatch: %f vs %f", rep1.StageA.IntentEval.Accuracy, rep2.StageA.IntentEval.Accuracy)
	}

	if rep1.StageA.GraphEval.OverallGraphScore != rep2.StageA.GraphEval.OverallGraphScore {
		t.Errorf("Stage A Graph Score mismatch: %f vs %f", rep1.StageA.GraphEval.OverallGraphScore, rep2.StageA.GraphEval.OverallGraphScore)
	}

	if rep1.StageB.ValidCitationPrecision != rep2.StageB.ValidCitationPrecision {
		t.Errorf("Stage B ValidCitationPrecision mismatch: %f vs %f", rep1.StageB.ValidCitationPrecision, rep2.StageB.ValidCitationPrecision)
	}
}

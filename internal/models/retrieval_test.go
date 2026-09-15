package models_test

import (
	"testing"

	"codegraph/internal/models"
)

func TestRepositoryScope_Validation(t *testing.T) {
	_, err := models.NewRepositoryScope("")
	if err != models.ErrInvalidRepositoryScope {
		t.Fatalf("expected ErrInvalidRepositoryScope for empty repo ID, got %v", err)
	}

	scopeA, err := models.NewRepositoryScope("repo-A")
	if err != nil {
		t.Fatalf("unexpected error creating scope: %v", err)
	}

	itemA := &models.EvidenceItem{
		RepositoryID: "repo-A",
		Type:         models.EvidenceTypeSymbol,
		Content:      "func Login()",
	}
	itemB := &models.EvidenceItem{
		RepositoryID: "repo-B",
		Type:         models.EvidenceTypeSymbol,
		Content:      "func Logout()",
	}

	if err := scopeA.ValidateItem(itemA); err != nil {
		t.Errorf("expected itemA to be valid for scopeA, got: %v", err)
	}

	if err := scopeA.ValidateItem(itemB); err == nil {
		t.Errorf("expected error when validating itemB against scopeA, got nil")
	}
}

func TestEvidencePackage_RepositoryIsolationAndIDs(t *testing.T) {
	scopeA, _ := models.NewRepositoryScope("repo-A")
	budget := models.DefaultEvidenceBudget()

	pkg, err := models.NewEvidencePackage(scopeA, "How does auth work?", "FEATURE_SEARCH", budget)
	if err != nil {
		t.Fatalf("failed to create package: %v", err)
	}

	item1 := &models.EvidenceItem{
		RepositoryID: "repo-A",
		RelativePath: "auth/service.go",
		Type:         models.EvidenceTypeSymbol,
		Content:      "type AuthService struct{}",
	}
	item2 := &models.EvidenceItem{
		RepositoryID: "repo-A",
		RelativePath: "auth/login.go",
		Type:         models.EvidenceTypeCodeSnippet,
		Content:      "func Authenticate() error",
	}

	if err := pkg.AddItem(scopeA, item1); err != nil {
		t.Fatalf("failed to add item1: %v", err)
	}
	if err := pkg.AddItem(scopeA, item2); err != nil {
		t.Fatalf("failed to add item2: %v", err)
	}

	pkg.FinalizePackage()

	if len(pkg.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(pkg.Items))
	}

	// Verify items have non-empty StableIDs and labels assigned
	for _, item := range pkg.Items {
		if item.StableID == "" {
			t.Errorf("expected non-empty StableID for item %s", item.Content)
		}
		if item.Label == "" {
			t.Errorf("expected non-empty Label for item %s", item.Content)
		}
	}

	// Test repository isolation rejection
	itemForeign := &models.EvidenceItem{
		RepositoryID: "repo-B",
		Type:         models.EvidenceTypeSymbol,
		Content:      "func UntrustedForeignCode()",
	}
	err = pkg.AddItem(scopeA, itemForeign)
	if err == nil {
		t.Errorf("expected error when adding repo-B item to repo-A package, got nil")
	}
}

func TestEvidencePackage_StableIdentityAndLabelAssignment(t *testing.T) {
	scope, _ := models.NewRepositoryScope("repo-A")
	budget := models.DefaultEvidenceBudget()

	itemAlpha := &models.EvidenceItem{
		RepositoryID:  "repo-A",
		RelativePath:  "alpha.go",
		Type:          models.EvidenceTypeSymbol,
		Location:      models.Location{StartLine: 1, EndLine: 10},
		Content:       "func Alpha()",
		RetrieverType: "LEXICAL",
	}

	itemBeta := &models.EvidenceItem{
		RepositoryID:  "repo-A",
		RelativePath:  "beta.go",
		Type:          models.EvidenceTypeCodeSnippet,
		Location:      models.Location{StartLine: 20, EndLine: 35},
		Content:       "func Beta()",
		RetrieverType: "GRAPH",
	}

	itemGamma := &models.EvidenceItem{
		RepositoryID:  "repo-A",
		RelativePath:  "gamma.go",
		Type:          models.EvidenceTypeGraphEdge,
		Location:      models.Location{StartLine: 5, EndLine: 5},
		Content:       "Alpha CALLS Beta",
		RetrieverType: "GRAPH",
	}

	pkg, _ := models.NewEvidencePackage(scope, "Q", "INTENT", budget)
	_ = pkg.AddItem(scope, itemAlpha)
	_ = pkg.AddItem(scope, itemBeta)
	_ = pkg.AddItem(scope, itemGamma)
	pkg.FinalizePackage()

	if len(pkg.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(pkg.Items))
	}

	// Order must be preserved as added (itemAlpha, itemBeta, itemGamma)
	if pkg.Items[0].RelativePath != "alpha.go" || pkg.Items[0].Label != "E1" {
		t.Errorf("expected E1 -> alpha.go, got %s -> %s", pkg.Items[0].Label, pkg.Items[0].RelativePath)
	}
	if pkg.Items[1].RelativePath != "beta.go" || pkg.Items[1].Label != "E2" {
		t.Errorf("expected E2 -> beta.go, got %s -> %s", pkg.Items[1].Label, pkg.Items[1].RelativePath)
	}
	if pkg.Items[2].RelativePath != "gamma.go" || pkg.Items[2].Label != "E3" {
		t.Errorf("expected E3 -> gamma.go, got %s -> %s", pkg.Items[2].Label, pkg.Items[2].RelativePath)
	}

	if len(pkg.Citations) != 3 {
		t.Fatalf("expected 3 citations, got %d", len(pkg.Citations))
	}
	if pkg.Citations[0].EvidenceID != "E1" || pkg.Citations[0].RelativePath != "alpha.go" {
		t.Errorf("citation 0 mismatch: %+v", pkg.Citations[0])
	}
}

func TestEvidenceItem_ScoreSeparation(t *testing.T) {
	item := models.EvidenceItem{
		RepositoryID:  "repo-1",
		RelativePath:  "main.go",
		Type:          models.EvidenceTypeSymbol,
		RetrieverType: "LEXICAL",
		RawScore:      15.5,    // Raw BM25 or lexical score
		Rank:          1,       // 1st place in lexical retriever
		RRFScore:      0.01639, // Fused RRF score (Checkpoint 5)
	}

	if item.RawScore != 15.5 {
		t.Errorf("expected RawScore 15.5, got %f", item.RawScore)
	}
	if item.Rank != 1 {
		t.Errorf("expected Rank 1, got %d", item.Rank)
	}
	if item.RRFScore != 0.01639 {
		t.Errorf("expected RRFScore 0.01639, got %f", item.RRFScore)
	}
}

func TestEvidenceBudget_Configurable(t *testing.T) {
	defaultBudget := models.DefaultEvidenceBudget()
	if defaultBudget.MaxTokens != 4000 {
		t.Errorf("expected default MaxTokens to be 4000, got %d", defaultBudget.MaxTokens)
	}

	customBudget := models.EvidenceBudget{
		MaxTokens:       8000,
		MaxFiles:        50,
		MaxSymbols:      100,
		MaxGraphHops:    4,
		MaxSnippetLines: 250,
	}
	if customBudget.MaxTokens != 8000 || customBudget.MaxFiles != 50 {
		t.Errorf("custom budget failed to retain configured values: %+v", customBudget)
	}
}

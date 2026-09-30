package retrieval_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codegraph/internal/models"
	"codegraph/internal/retrieval"
	"codegraph/internal/storage"
)

func setupTestStorage(t *testing.T) storage.Storage {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test_retrieval.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create sqlite test storage: %v", err)
	}

	repoA := &models.Repository{
		ID:        "repo-A",
		Name:      "Repository A",
		LocalPath: t.TempDir(),
		Status:    models.RepoStatusIndexed,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	repoB := &models.Repository{
		ID:        "repo-B",
		Name:      "Repository B",
		LocalPath: t.TempDir(),
		Status:    models.RepoStatusIndexed,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_ = store.CreateRepository(context.Background(), repoA)
	_ = store.CreateRepository(context.Background(), repoB)

	// Repo A symbols
	symbolsA := []*models.Symbol{
		{
			ID:            "sym-1",
			RepositoryID:  "repo-A",
			FileID:        "file-1",
			RelativePath:  "auth/login.go",
			Name:          "Login",
			QualifiedName: "auth.LoginController.Login",
			Kind:          models.SymbolKindFunction,
			Location:      models.Location{StartLine: 10, EndLine: 25},
			UpdatedAt:     time.Now(),
		},
		{
			ID:            "sym-2",
			RepositoryID:  "repo-A",
			FileID:        "file-1",
			RelativePath:  "auth/login.go",
			Name:          "LoginController",
			QualifiedName: "auth.LoginController",
			Kind:          models.SymbolKindStruct,
			Location:      models.Location{StartLine: 5, EndLine: 30},
			UpdatedAt:     time.Now(),
		},
		{
			ID:            "sym-3",
			RepositoryID:  "repo-A",
			FileID:        "file-2",
			RelativePath:  "storage/db.go",
			Name:          "ConnectDB",
			QualifiedName: "storage.ConnectDB",
			Kind:          models.SymbolKindFunction,
			Location:      models.Location{StartLine: 1, EndLine: 15},
			UpdatedAt:     time.Now(),
		},
	}
	_ = store.SaveSymbols(context.Background(), "repo-A", symbolsA)

	// Repo A files
	filesA := []*models.FileManifestItem{
		{
			ID:           "file-1",
			RepositoryID: "repo-A",
			RelativePath: "auth/login.go",
			Language:     "Go",
			Extension:    ".go",
			Size:         1024,
			Status:       models.FileStatusIndexed,
			UpdatedAt:    time.Now(),
		},
		{
			ID:           "file-2",
			RepositoryID: "repo-A",
			RelativePath: "storage/db.go",
			Language:     "Go",
			Extension:    ".go",
			Size:         512,
			Status:       models.FileStatusIndexed,
			UpdatedAt:    time.Now(),
		},
	}
	_ = store.SaveManifestItems(context.Background(), "repo-A", filesA)

	// Repo B symbols (for repo isolation test)
	symbolsB := []*models.Symbol{
		{
			ID:            "sym-b1",
			RepositoryID:  "repo-B",
			FileID:        "file-b1",
			RelativePath:  "secret/login.go",
			Name:          "Login",
			QualifiedName: "secret.Login",
			Kind:          models.SymbolKindFunction,
			Location:      models.Location{StartLine: 1, EndLine: 5},
			UpdatedAt:     time.Now(),
		},
	}
	_ = store.SaveSymbols(context.Background(), "repo-B", symbolsB)

	return store
}

func TestLexicalRetriever_ExactSymbolLookup(t *testing.T) {
	store := setupTestStorage(t)
	defer store.Close()

	retriever := retrieval.NewLexicalRetriever(store, nil)
	scope, _ := models.NewRepositoryScope("repo-A")

	items, err := retriever.Retrieve(context.Background(), scope, "Login", retrieval.IntentSymbolLookup, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) == 0 {
		t.Fatalf("expected at least 1 item for exact symbol 'Login'")
	}

	// First item should be exact symbol match 'Login' with highest raw score
	first := items[0]
	if first.Metadata["symbol_name"] != "Login" {
		t.Errorf("expected top result symbol name to be 'Login', got %s", first.Metadata["symbol_name"])
	}
	if first.RawScore <= 0 {
		t.Errorf("expected positive RawScore, got %f", first.RawScore)
	}
	if first.Rank != 1 {
		t.Errorf("expected top result rank to be 1, got %d", first.Rank)
	}
}

func TestLexicalRetriever_QualifiedSymbolLookup(t *testing.T) {
	store := setupTestStorage(t)
	defer store.Close()

	retriever := retrieval.NewLexicalRetriever(store, nil)
	scope, _ := models.NewRepositoryScope("repo-A")

	items, err := retriever.Retrieve(context.Background(), scope, "auth.LoginController.Login", retrieval.IntentSymbolLookup, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) == 0 {
		t.Fatalf("expected at least 1 item for qualified symbol 'auth.LoginController.Login'")
	}

	top := items[0]
	if top.Metadata["qualified_name"] != "auth.LoginController.Login" {
		t.Errorf("expected top item to match qualified name 'auth.LoginController.Login', got %s", top.Metadata["qualified_name"])
	}
	// Exact qualified match score (100) + SYMBOL_LOOKUP intent boost (30) = 130
	if top.RawScore != 130.0 {
		t.Errorf("expected score 130.0 for exact qualified symbol match with SYMBOL_LOOKUP boost, got %f", top.RawScore)
	}
}

func TestLexicalRetriever_PartialIdentifierMatching(t *testing.T) {
	store := setupTestStorage(t)
	defer store.Close()

	retriever := retrieval.NewLexicalRetriever(store, nil)
	scope, _ := models.NewRepositoryScope("repo-A")

	items, err := retriever.Retrieve(context.Background(), scope, "Connect", retrieval.IntentFeatureSearch, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) == 0 {
		t.Fatalf("expected at least 1 item for partial identifier 'Connect'")
	}

	top := items[0]
	if top.Metadata["symbol_name"] != "ConnectDB" {
		t.Errorf("expected top result symbol to be 'ConnectDB', got %s", top.Metadata["symbol_name"])
	}
}

func TestLexicalRetriever_PathMatching(t *testing.T) {
	store := setupTestStorage(t)
	defer store.Close()

	retriever := retrieval.NewLexicalRetriever(store, nil)
	scope, _ := models.NewRepositoryScope("repo-A")

	items, err := retriever.Retrieve(context.Background(), scope, "storage/db.go", retrieval.IntentDependencyQuery, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) == 0 {
		t.Fatalf("expected at least 1 item for file path 'storage/db.go'")
	}

	foundFile := false
	for _, item := range items {
		if item.RelativePath == "storage/db.go" {
			foundFile = true
			break
		}
	}
	if !foundFile {
		t.Errorf("expected file 'storage/db.go' in retrieval results")
	}
}

func TestLexicalRetriever_DeterministicRanking(t *testing.T) {
	store := setupTestStorage(t)
	defer store.Close()

	retriever := retrieval.NewLexicalRetriever(store, nil)
	scope, _ := models.NewRepositoryScope("repo-A")

	// Run retrieval twice with identical inputs
	run1, err1 := retriever.Retrieve(context.Background(), scope, "Login", retrieval.IntentSymbolLookup, 10)
	run2, err2 := retriever.Retrieve(context.Background(), scope, "Login", retrieval.IntentSymbolLookup, 10)

	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected error in deterministic test: %v, %v", err1, err2)
	}

	if len(run1) != len(run2) {
		t.Fatalf("result length mismatch: %d vs %d", len(run1), len(run2))
	}

	for i := range run1 {
		if run1[i].StableID != run2[i].StableID {
			t.Errorf("StableID mismatch at index %d: %s vs %s", i, run1[i].StableID, run2[i].StableID)
		}
		if run1[i].RawScore != run2[i].RawScore {
			t.Errorf("RawScore mismatch at index %d: %f vs %f", i, run1[i].RawScore, run2[i].RawScore)
		}
		if run1[i].Rank != run2[i].Rank {
			t.Errorf("Rank mismatch at index %d: %d vs %d", i, run1[i].Rank, run2[i].Rank)
		}
	}
}

func TestLexicalRetriever_RepositoryIsolation(t *testing.T) {
	store := setupTestStorage(t)
	defer store.Close()

	retriever := retrieval.NewLexicalRetriever(store, nil)
	scopeA, _ := models.NewRepositoryScope("repo-A")

	// Query for 'Login' in repo-A scope
	items, err := retriever.Retrieve(context.Background(), scopeA, "Login", retrieval.IntentSymbolLookup, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, item := range items {
		if item.RepositoryID != "repo-A" {
			t.Errorf("repository isolation violated! expected repo-A, got item from %s", item.RepositoryID)
		}
		if item.RelativePath == "secret/login.go" {
			t.Errorf("repository isolation violated! item from repo-B (secret/login.go) leaked into repo-A search")
		}
	}
}

func TestLexicalRetriever_SourceProvenance(t *testing.T) {
	store := setupTestStorage(t)
	defer store.Close()

	retriever := retrieval.NewLexicalRetriever(store, nil)
	scope, _ := models.NewRepositoryScope("repo-A")

	items, err := retriever.Retrieve(context.Background(), scope, "LoginController", retrieval.IntentSymbolLookup, 1)
	if err != nil || len(items) == 0 {
		t.Fatalf("expected result for LoginController: err=%v, len=%d", err, len(items))
	}

	item := items[0]
	if item.StableID == "" {
		t.Errorf("expected non-empty StableID")
	}
	if item.RetrieverType != "LEXICAL" {
		t.Errorf("expected RetrieverType = LEXICAL, got %s", item.RetrieverType)
	}
	if item.RepositoryID != "repo-A" {
		t.Errorf("expected RepositoryID = repo-A, got %s", item.RepositoryID)
	}
	if item.FileID != "file-1" {
		t.Errorf("expected FileID = file-1, got %s", item.FileID)
	}
	if item.RelativePath != "auth/login.go" {
		t.Errorf("expected RelativePath = auth/login.go, got %s", item.RelativePath)
	}
	if item.Location.StartLine != 5 {
		t.Errorf("expected StartLine = 5, got %d", item.Location.StartLine)
	}
}

func TestLexicalRetriever_EmptyAndNoMatchQueries(t *testing.T) {
	store := setupTestStorage(t)
	defer store.Close()

	retriever := retrieval.NewLexicalRetriever(store, nil)
	scope, _ := models.NewRepositoryScope("repo-A")

	// Empty query
	emptyItems, err := retriever.Retrieve(context.Background(), scope, "   ", retrieval.IntentSymbolLookup, 10)
	if err != nil {
		t.Errorf("unexpected error for empty query: %v", err)
	}
	if len(emptyItems) != 0 {
		t.Errorf("expected 0 items for empty query, got %d", len(emptyItems))
	}

	// No-match query
	noMatchItems, err := retriever.Retrieve(context.Background(), scope, "NonExistentXYZ123", retrieval.IntentSymbolLookup, 10)
	if err != nil {
		t.Errorf("unexpected error for no-match query: %v", err)
	}
	if len(noMatchItems) != 0 {
		t.Errorf("expected 0 items for no-match query, got %d", len(noMatchItems))
	}
}

func TestLexicalRetriever_EdgeCases(t *testing.T) {
	store := setupTestStorage(t)
	defer store.Close()

	retriever := retrieval.NewLexicalRetriever(store, nil)

	// Invalid repository scope
	emptyScope := models.RepositoryScope{}
	_, err := retriever.Retrieve(context.Background(), emptyScope, "Login", retrieval.IntentSymbolLookup, 10)
	if err != models.ErrInvalidRepositoryScope {
		t.Errorf("expected ErrInvalidRepositoryScope for empty scope, got %v", err)
	}

	// Limit <= 0 should default to 10
	scope, _ := models.NewRepositoryScope("repo-A")
	items, err := retriever.Retrieve(context.Background(), scope, "Login", retrieval.IntentSymbolLookup, 0)
	if err != nil {
		t.Fatalf("unexpected error for limit 0: %v", err)
	}
	if len(items) == 0 {
		t.Errorf("expected results with default limit, got 0")
	}
}

func TestLexicalRetriever_ConceptualSourceContentRetrieval(t *testing.T) {
	tempWorkspace := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "conceptual_retrieval.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create sqlite test storage: %v", err)
	}
	defer store.Close()

	repo := &models.Repository{
		ID:        "repo-conceptual",
		Name:      "Conceptual Test Repo",
		LocalPath: tempWorkspace,
		Status:    models.RepoStatusIndexed,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_ = store.CreateRepository(context.Background(), repo)

	// File 1: "pkg/guard.go" (File path and symbols do NOT contain "traversal" or "boundary")
	// Body contains actual path traversal protection logic
	guardAbsPath := filepath.Join(tempWorkspace, "pkg", "guard.go")
	_ = os.MkdirAll(filepath.Dir(guardAbsPath), 0755)
	guardContent := `package guard

import (
	"errors"
	"path/filepath"
	"strings"
)

var ErrTraversal = errors.New("path traversal detected")

// VerifySecurityBoundary validates that target path stays within base directory.
func VerifySecurityBoundary(baseDir, userPath string) error {
	cleanBase := filepath.Clean(baseDir)
	cleanTarget := filepath.Clean(userPath)
	rel, err := filepath.Rel(cleanBase, cleanTarget)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ErrTraversal
	}
	return nil
}
`
	if err := os.WriteFile(guardAbsPath, []byte(guardContent), 0644); err != nil {
		t.Fatalf("failed to write test guard file: %v", err)
	}

	// File 2: "pkg/unrelated.go" (completely irrelevant file)
	unrelatedAbsPath := filepath.Join(tempWorkspace, "pkg", "unrelated.go")
	_ = os.WriteFile(unrelatedAbsPath, []byte("package guard\n\nfunc CalculateSum(a, b int) int { return a + b }\n"), 0644)

	manifestItems := []*models.FileManifestItem{
		{
			ID:           "file-guard",
			RepositoryID: "repo-conceptual",
			RelativePath: "pkg/guard.go",
			Language:     "Go",
			Extension:    ".go",
			Size:         int64(len(guardContent)),
			Status:       models.FileStatusIndexed,
			UpdatedAt:    time.Now(),
		},
		{
			ID:           "file-unrelated",
			RepositoryID: "repo-conceptual",
			RelativePath: "pkg/unrelated.go",
			Language:     "Go",
			Extension:    ".go",
			Size:         60,
			Status:       models.FileStatusIndexed,
			UpdatedAt:    time.Now(),
		},
	}
	_ = store.SaveManifestItems(context.Background(), "repo-conceptual", manifestItems)

	symbols := []*models.Symbol{
		{
			ID:            "sym-guard",
			RepositoryID:  "repo-conceptual",
			FileID:        "file-guard",
			RelativePath:  "pkg/guard.go",
			Name:          "VerifySecurityBoundary",
			QualifiedName: "guard.VerifySecurityBoundary",
			Kind:          models.SymbolKindFunction,
			Location:      models.Location{StartLine: 12, EndLine: 20},
			UpdatedAt:     time.Now(),
		},
	}
	_ = store.SaveSymbols(context.Background(), "repo-conceptual", symbols)

	retriever := retrieval.NewLexicalRetriever(store, nil)
	scope, _ := models.NewRepositoryScope("repo-conceptual")

	// Conceptual query asking about path traversal protection
	// Query terms "path", "traversal", "security", "boundary" do NOT appear in file path "pkg/guard.go"
	query := "how does path traversal security boundary validation work"
	items, err := retriever.Retrieve(context.Background(), scope, query, retrieval.IntentExplanation, 10)
	if err != nil {
		t.Fatalf("unexpected retrieval error: %v", err)
	}

	if len(items) == 0 {
		t.Fatalf("expected at least 1 item for conceptual query 'path traversal security boundary validation', got 0")
	}

	// 1. Verify top item is guard.go source code snippet
	top := items[0]
	if top.RelativePath != "pkg/guard.go" {
		t.Errorf("expected top result to be 'pkg/guard.go', got %s", top.RelativePath)
	}
	if top.RepositoryID != "repo-conceptual" {
		t.Errorf("expected RepositoryID 'repo-conceptual', got %s", top.RepositoryID)
	}
	if !strings.Contains(top.Content, "path traversal detected") && !strings.Contains(top.Content, "VerifySecurityBoundary") {
		t.Errorf("expected snippet content to contain implementation code, got:\n%s", top.Content)
	}
	if top.Location.StartLine < 1 || top.Location.EndLine < top.Location.StartLine {
		t.Errorf("invalid snippet location range: %d to %d", top.Location.StartLine, top.Location.EndLine)
	}

	// 2. Verify irrelevant file (pkg/unrelated.go) is NOT promoted to top results
	for _, item := range items {
		if item.RelativePath == "pkg/unrelated.go" {
			t.Errorf("unrelated file 'pkg/unrelated.go' should not be retrieved for path traversal query")
		}
	}

	// 3. Verify Sufficiency Evaluator accepts the evidence package
	evaluator := retrieval.NewDefaultSufficiencyEvaluator()
	sufficiency, err := evaluator.Evaluate(context.Background(), scope, query, retrieval.IntentExplanation, items)
	if err != nil {
		t.Fatalf("sufficiency evaluation error: %v", err)
	}
	if sufficiency.Status == models.SufficiencyInsufficient {
		t.Errorf("expected sufficient/partial evidence status for retrieved code body, got Insufficient (%v)", sufficiency.Reasons)
	}
}

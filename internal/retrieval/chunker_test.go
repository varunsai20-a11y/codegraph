package retrieval_test

import (
	"os"
	"path/filepath"
	"testing"

	"codegraph/internal/models"
	"codegraph/internal/retrieval"
)

func TestASTSymbolChunker_ExactRangeAndNoNeighborLeakage(t *testing.T) {
	tempDir := t.TempDir()
	sourceFile := filepath.Join(tempDir, "auth.go")

	codeContent := `package auth

// Helper function before target
func NeighborBefore() {
	println("before")
}

// Login performs authentication
func Login() error {
	return nil
}

// NeighborAfter function after target
func NeighborAfter() {
	println("after")
}
`
	if err := os.WriteFile(sourceFile, []byte(codeContent), 0644); err != nil {
		t.Fatalf("failed to write test source file: %v", err)
	}

	scope, _ := models.NewRepositoryScope("repo-1")
	symLogin := &models.Symbol{
		ID:            "sym-login",
		RepositoryID:  "repo-1",
		FileID:        "file-auth",
		RelativePath:  "auth.go",
		Name:          "Login",
		QualifiedName: "auth.Login",
		Kind:          models.SymbolKindFunction,
		Location:      models.Location{StartLine: 9, EndLine: 11}, // Exact line range of func Login()
	}

	chunker := retrieval.NewASTSymbolChunker()
	item, err := chunker.CreateSymbolEvidence(scope, symLogin, tempDir, 80.0)
	if err != nil {
		t.Fatalf("unexpected error creating symbol evidence: %v", err)
	}

	expectedSnippet := "func Login() error {\n\treturn nil\n}"
	if item.Content != expectedSnippet {
		t.Errorf("expected exact code snippet:\n%q\ngot:\n%q", expectedSnippet, item.Content)
	}

	// Verify preceding docstring is captured in metadata
	expectedDoc := "// Login performs authentication"
	if item.Metadata["documentation"] != expectedDoc {
		t.Errorf("expected documentation metadata %q, got %q", expectedDoc, item.Metadata["documentation"])
	}

	// Verify NO neighboring code leaked into content
	if item.Content == "" || item.Content != expectedSnippet {
		t.Errorf("code snippet leaked neighboring functions or didn't match exact range")
	}
}

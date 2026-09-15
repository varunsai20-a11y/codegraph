package analysis

import (
	"context"
	"testing"

	"codegraph/internal/models"
)

type DummyAnalyzer struct{}

func (d *DummyAnalyzer) Language() string { return "Dummy" }
func (d *DummyAnalyzer) Analyze(ctx context.Context, repoID, fileID, relPath, content string) (*models.FileAnalysis, error) {
	return &models.FileAnalysis{
		FileID:       fileID,
		RelativePath: relPath,
		Language:     "Dummy",
	}, nil
}

func TestAnalyzerRegistry(t *testing.T) {
	reg := NewAnalyzerRegistry()
	dummy := &DummyAnalyzer{}
	reg.Register(dummy)

	got, found := reg.Get("Dummy")
	if !found {
		t.Fatalf("expected Dummy analyzer to be registered")
	}
	if got.Language() != "Dummy" {
		t.Errorf("expected language Dummy, got %s", got.Language())
	}

	_, found = reg.Get("NonExistent")
	if found {
		t.Errorf("expected NonExistent analyzer to not be found")
	}
}

func TestFormatIDs(t *testing.T) {
	symID := FormatSymbolID("repo1", "src/main.go", "main", 10)
	if symID != "sym:repo1:src/main.go:main:10" {
		t.Errorf("unexpected symbol ID: %s", symID)
	}

	relID := FormatRelationshipID("repo1", "sym1", "sym2", models.RelTypeCalls, 42)
	if relID != "rel:repo1:sym1:sym2:CALLS:42" {
		t.Errorf("unexpected relationship ID: %s", relID)
	}
}

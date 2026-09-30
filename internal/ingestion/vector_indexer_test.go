package ingestion

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codegraph/internal/models"
	"codegraph/internal/vector"
)

func TestVectorIndexer_DeepSourceChunkingAndDuplicateSafety(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "vec_indexer_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create sample source files
	file1Path := filepath.Join(tmpDir, "agents.py")
	file1Content := `
from crewai import Agent

research_agent = Agent(
    role="Senior AI Researcher",
    goal="Gather raw web research",
    backstory="You are an expert researcher.",
    verbose=True
)
`
	if err := os.WriteFile(file1Path, []byte(file1Content), 0644); err != nil {
		t.Fatalf("failed to write file1: %v", err)
	}

	file2Path := filepath.Join(tmpDir, "tasks.py")
	file2Content := `
from crewai import Task

research_task = Task(
    description="Perform deep research on topic",
    expected_output="Comprehensive notes",
    agent=research_agent
)
`
	if err := os.WriteFile(file2Path, []byte(file2Content), 0644); err != nil {
		t.Fatalf("failed to write file2: %v", err)
	}

	vStore, err := vector.NewSQLiteSemanticStorePath(":memory:")
	if err != nil {
		t.Fatalf("failed to init vstore: %v", err)
	}
	defer vStore.Close()

	provider := vector.NewMockEmbeddingProvider()
	indexer := NewVectorIndexer(vStore, provider)

	scope := models.RepositoryScope{RepositoryID: "test-repo-1"}
	manifestItems := []*models.FileManifestItem{
		{RelativePath: "agents.py", SHA256: "hash1"},
		{RelativePath: "tasks.py", SHA256: "hash2"},
	}
	symbols := []*models.Symbol{
		{
			ID:            "sym-agent-1",
			RepositoryID:  "test-repo-1",
			FileID:        "agents.py",
			Name:          "research_agent",
			QualifiedName: "agents.research_agent",
			Kind:          models.SymbolKindVariable,
			RelativePath:  "agents.py",
			Location:      models.Location{StartLine: 4, EndLine: 4},
		},
	}

	ctx := context.Background()

	// 1. Initial Indexing
	err = indexer.IndexRepositoryVectors(ctx, scope, tmpDir, manifestItems, symbols)
	if err != nil {
		t.Fatalf("IndexRepositoryVectors failed: %v", err)
	}

	// Search results
	results, err := vStore.Search(ctx, scope, make([]float32, provider.Dimension()), provider.ModelName(), provider.Version(), 10, -1.0)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Requirement A: Multiple source files produce multiple vector chunks
	if len(results) < 2 {
		t.Errorf("expected at least 2 vector chunks, got %d", len(results))
	}

	// Requirement B & D: Function/variable bodies present in stored content, correct metadata
	foundAgentBody := false
	foundTaskFile := false

	for _, res := range results {
		rec := res.Record
		if rec.RepositoryID != "test-repo-1" {
			t.Errorf("expected RepositoryID test-repo-1, got %s", rec.RepositoryID)
		}
		if rec.RelativePath == "agents.py" && rec.SymbolID == "sym-agent-1" {
			foundAgentBody = true
			if rec.Location.StartLine != 4 {
				t.Errorf("expected StartLine 4, got %d", rec.Location.StartLine)
			}
			if rec.Content == "" {
				t.Error("expected non-empty content body for symbol chunk")
			}
		}
		if rec.RelativePath == "tasks.py" {
			foundTaskFile = true
		}
	}

	if !foundAgentBody {
		t.Error("symbol chunk for research_agent body was not found")
	}
	if !foundTaskFile {
		t.Error("source window chunk for tasks.py was not found")
	}

	// Requirement E: Re-indexing does not accumulate duplicate vectors
	err = indexer.IndexRepositoryVectors(ctx, scope, tmpDir, manifestItems, symbols)
	if err != nil {
		t.Fatalf("Re-indexing failed: %v", err)
	}

	resultsAfterReindex, err := vStore.Search(ctx, scope, make([]float32, provider.Dimension()), provider.ModelName(), provider.Version(), 100, -1.0)
	if err != nil {
		t.Fatalf("Search after re-index failed: %v", err)
	}

	if len(resultsAfterReindex) != len(results) {
		t.Errorf("expected re-indexed vector count %d, got %d (duplicate accumulation detected)", len(results), len(resultsAfterReindex))
	}

	// Requirement F: Repository Isolation
	isoScope := models.RepositoryScope{RepositoryID: "other-repo"}
	isoResults, err := vStore.Search(ctx, isoScope, make([]float32, provider.Dimension()), provider.ModelName(), provider.Version(), 10, -1.0)
	if err != nil {
		t.Fatalf("Isolation search failed: %v", err)
	}
	if len(isoResults) != 0 {
		t.Errorf("expected 0 results for other-repo, got %d", len(isoResults))
	}
}

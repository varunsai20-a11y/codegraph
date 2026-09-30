package guide

import (
	"context"
	"testing"

	"codegraph/internal/graph"
	"codegraph/internal/models"
	"codegraph/internal/storage"
)

type mockStorage struct {
	storage.Storage
}

func (m *mockStorage) GetManifestForRepository(ctx context.Context, repoID string) ([]*models.FileManifestItem, error) {
	return []*models.FileManifestItem{
		{RelativePath: "run.py", Language: "PYTHON"},
		{RelativePath: "crew.py", Language: "PYTHON"},
		{RelativePath: "tasks.py", Language: "PYTHON"},
		{RelativePath: "agents.py", Language: "PYTHON"},
		{RelativePath: "tools.py", Language: "PYTHON"},
	}, nil
}
func (m *mockStorage) GetSymbolsForRepository(ctx context.Context, repoID string) ([]*models.Symbol, error) {
	return []*models.Symbol{
		{ID: "sym-main", Name: "main", RelativePath: "run.py", Kind: models.SymbolKindFunction},
		{ID: "sym-crew", Name: "research_crew", RelativePath: "crew.py", Kind: models.SymbolKindFunction},
		{ID: "sym-task", Name: "research_task", RelativePath: "tasks.py", Kind: models.SymbolKindFunction},
		{ID: "sym-agent", Name: "research_agent", RelativePath: "agents.py", Kind: models.SymbolKindFunction},
		{ID: "sym-tool", Name: "search_tool", RelativePath: "tools.py", Kind: models.SymbolKindFunction},
	}, nil
}
func (m *mockStorage) GetRelationshipsForRepository(ctx context.Context, repoID string) ([]*models.Relationship, error) {
	return []*models.Relationship{
		{SourceID: "sym-main", TargetID: "sym-crew", Type: models.RelTypeCalls},
		{SourceID: "sym-crew", TargetID: "sym-task", Type: models.RelTypeCalls},
		{SourceID: "sym-task", TargetID: "sym-agent", Type: models.RelTypeCalls},
		{SourceID: "sym-agent", TargetID: "sym-tool", Type: models.RelTypeCalls},
	}, nil
}
func (m *mockStorage) GetGraphForRepository(ctx context.Context, repoID string) ([]*models.Node, []*models.Edge, error) {
	nodes := []*models.Node{
		{ID: "sym-main", Label: "main", Kind: models.NodeKindSymbol, RelativePath: "run.py"},
		{ID: "sym-crew", Label: "research_crew", Kind: models.NodeKindSymbol, RelativePath: "crew.py"},
		{ID: "sym-task", Label: "research_task", Kind: models.NodeKindSymbol, RelativePath: "tasks.py"},
		{ID: "sym-agent", Label: "research_agent", Kind: models.NodeKindSymbol, RelativePath: "agents.py"},
		{ID: "sym-tool", Label: "search_tool", Kind: models.NodeKindSymbol, RelativePath: "tools.py"},
	}
	edges := []*models.Edge{
		{SourceID: "sym-main", TargetID: "sym-crew", Kind: models.EdgeKindCalls},
		{SourceID: "sym-crew", TargetID: "sym-task", Kind: models.EdgeKindCalls},
		{SourceID: "sym-task", TargetID: "sym-agent", Kind: models.EdgeKindCalls},
		{SourceID: "sym-agent", TargetID: "sym-tool", Kind: models.EdgeKindCalls},
	}
	return nodes, edges, nil
}

func TestGuideOrchestrator_ProgressiveReverseEngineering(t *testing.T) {
	store := &mockStorage{}
	engine := graph.NewEngine("repo-test")
	nodes, edges, _ := store.GetGraphForRepository(context.Background(), "repo-test")
	engine.LoadGraph(nodes, edges)

	summaryGen := NewDefaultArchitectureSummaryGenerator(store, engine)
	orch := NewDefaultGuideOrchestrator(store, summaryGen, nil, nil)

	scope, err := models.NewRepositoryScope("repo-test")
	if err != nil {
		t.Fatalf("unexpected error creating scope: %v", err)
	}

	req, err := models.NewInvestigationRequest(scope, "START")
	if err != nil {
		t.Fatalf("unexpected error creating request: %v", err)
	}

	inv, err := orch.Guide(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error running guide: %v", err)
	}

	if len(inv.Steps) != 5 {
		t.Fatalf("expected 5 progressive steps, got %d", len(inv.Steps))
	}

	// Verify Step 1: System Purpose & Overview
	if inv.Steps[0].Type != models.StepOverview {
		t.Errorf("step 1 type mismatch: %v", inv.Steps[0].Type)
	}

	// Verify Step 2: Major Modules & Responsibilities
	if inv.Steps[1].Type != models.StepModuleStructure {
		t.Errorf("step 2 type mismatch: %v", inv.Steps[1].Type)
	}

	// Verify Step 3: Key Execution Components & Symbol Ranking
	if inv.Steps[2].Type != models.StepImportantSymbols {
		t.Errorf("step 3 type mismatch: %v", inv.Steps[2].Type)
	}

	// Verify Step 4: Execution Narrative & Static Call Flow
	if inv.Steps[3].Type != models.StepStaticFlow {
		t.Errorf("step 4 type mismatch: %v", inv.Steps[3].Type)
	}
	if inv.Steps[3].FlowResult == nil {
		t.Errorf("expected static flow result attached to step 4")
	}

	// Verify Step 5: End-to-End Architectural Synthesis
	if inv.Steps[4].Type != models.StepGroundedExplain {
		t.Errorf("step 5 type mismatch: %v", inv.Steps[4].Type)
	}
}

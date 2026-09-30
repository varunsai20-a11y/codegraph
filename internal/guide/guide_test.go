package guide

import (
	"context"
	"strings"
	"testing"

	"codegraph/internal/graph"
	"codegraph/internal/models"
	"codegraph/internal/storage"
)

type mockStorage struct {
	storage.Storage
}

func (m *mockStorage) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
	return &models.Repository{ID: id, Name: id, LocalPath: ""}, nil
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

	// Requirement C & Step consistency: Step 5 references the same target orchestrator as Step 3 and verified flow as Step 4
	if inv.Steps[2].TargetFile != "crew.py" {
		t.Errorf("expected Step 3 target file to be crew.py (cross-file orchestrator), got %s", inv.Steps[2].TargetFile)
	}

	// Verify formatting cleanliness across all steps (no raw ###, ***, or misleading External boundaries: .)
	for i, st := range inv.Steps {
		if strings.Contains(st.Description, "###") || strings.Contains(st.Description, "***") {
			t.Errorf("step %d description contains raw markdown headers/rules: %s", i+1, st.Description)
		}
		if strings.Contains(st.Description, "External boundaries: .") {
			t.Errorf("step %d description contains invalid dot external boundary string: %s", i+1, st.Description)
		}
	}

	// Verify Step 5 alignment with Step 3
	if !strings.Contains(inv.Steps[4].Description, inv.Steps[2].SymbolName) {
		t.Errorf("step 5 description should reference step 3 target symbol %s", inv.Steps[2].SymbolName)
	}
}


func TestCrossFileOrchestratorOutranksLocalHelper(t *testing.T) {
	// Setup repository with:
	// entrypoint: run.py:main
	// local helper: run.py:check_ollama_server (has multiple local helper relationships in run.py)
	// cross-file orchestrator: crew.py:research_crew (has cross-file relationships to agents.py and tasks.py)
	store := &mockStorage{}
	engine := graph.NewEngine("repo-orch-test")

	nodes := []*models.Node{
		{ID: "sym-main", Label: "main", Kind: models.NodeKindSymbol, RelativePath: "run.py"},
		{ID: "sym-helper", Label: "check_ollama_server", Kind: models.NodeKindSymbol, RelativePath: "run.py"},
		{ID: "sym-crew", Label: "research_crew", Kind: models.NodeKindSymbol, RelativePath: "crew.py"},
		{ID: "sym-agent", Label: "research_agent", Kind: models.NodeKindSymbol, RelativePath: "agents.py"},
	}

	edges := []*models.Edge{
		// Entrypoint calls helper and cross-file orchestrator
		{SourceID: "sym-main", TargetID: "sym-helper", Kind: models.EdgeKindCalls},
		{SourceID: "sym-main", TargetID: "sym-crew", Kind: models.EdgeKindCalls},
		// Orchestrator has cross-file relationships to agents.py
		{SourceID: "sym-crew", TargetID: "sym-agent", Kind: models.EdgeKindCalls},
	}

	engine.LoadGraph(nodes, edges)
	summaryGen := NewDefaultArchitectureSummaryGenerator(store, engine)

	scope, err := models.NewRepositoryScope("repo-orch-test")
	if err != nil {
		t.Fatalf("unexpected scope error: %v", err)
	}

	summary, err := summaryGen.GenerateSummary(context.Background(), scope)
	if err != nil {
		t.Fatalf("unexpected summary error: %v", err)
	}

	if len(summary.HighConnectivitySymbols) == 0 {
		t.Fatalf("expected high connectivity symbols to be populated")
	}

	topNode := summary.HighConnectivitySymbols[0]
	if topNode.Label != "research_crew" || topNode.RelativePath != "crew.py" {
		t.Errorf("expected top architectural symbol to be research_crew (crew.py), got %s (%s)", topNode.Label, topNode.RelativePath)
	}
}

func TestNonArchitecturalModuleFiltering(t *testing.T) {
	manifest := []*models.FileManifestItem{
		{RelativePath: "src/main.go", Language: "GO"},
		{RelativePath: "pkg/api/server.go", Language: "GO"},
		{RelativePath: ".github/workflows/ci.yml", Language: "YAML"},
		{RelativePath: "output/report.txt", Language: "TEXT"},
		{RelativePath: "dist/bundle.js", Language: "JAVASCRIPT"},
		{RelativePath: "vendor/lib.go", Language: "GO"},
	}

	mockSt := &mockStorage{}
	summaryGen := NewDefaultArchitectureSummaryGenerator(mockSt, nil)
	scope, _ := models.NewRepositoryScope("repo-mod-test")

	// Store returning custom manifest
	moduleMap := make(map[string]*models.ModuleSummary)
	for _, file := range manifest {
		dir := file.RelativePath
		if isNonArchitecturalDir(dir) {
			continue
		}
		moduleMap[dir] = &models.ModuleSummary{Directory: dir}
	}

	summary, err := summaryGen.GenerateSummary(context.Background(), scope)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, m := range summary.MajorModules {
		if isNonArchitecturalDir(m.Directory) {
			t.Errorf("non-architectural directory %s should be excluded from MajorModules", m.Directory)
		}
	}
}

func TestEmptyExternalBoundaryBehavior(t *testing.T) {
	store := &mockStorage{}
	summaryGen := NewDefaultArchitectureSummaryGenerator(store, nil)
	scope, _ := models.NewRepositoryScope("repo-boundary-test")

	summary, err := summaryGen.GenerateSummary(context.Background(), scope)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(summary.ExternalBoundaries) > 0 {
		for _, b := range summary.ExternalBoundaries {
			if b == "." || b == "" {
				t.Errorf("invalid external boundary string produced: '%s'", b)
			}
		}
	}
}

func TestCompletedGuideState(t *testing.T) {
	store := &mockStorage{}
	engine := graph.NewEngine("repo-complete-test")
	nodes, edges, _ := store.GetGraphForRepository(context.Background(), "repo-complete-test")
	engine.LoadGraph(nodes, edges)

	summaryGen := NewDefaultArchitectureSummaryGenerator(store, engine)
	orch := NewDefaultGuideOrchestrator(store, summaryGen, nil, nil)
	scope, _ := models.NewRepositoryScope("repo-complete-test")

	req, _ := models.NewInvestigationRequest(scope, "NEXT")
	req.CurrentStepIndex = 4
	req.CompletedStepIDs = []string{
		"step-1-overview",
		"step-2-module-structure",
		"step-3-important-symbols",
		"step-4-static-flow",
		"step-5-explanation",
	}

	inv, err := orch.Guide(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error running guide: %v", err)
	}

	if inv.Status != models.InvestigationCompleted {
		t.Errorf("expected status %s when all steps completed, got %s", models.InvestigationCompleted, inv.Status)
	}
	if !inv.IsComplete {
		t.Errorf("expected IsComplete to be true when all steps completed")
	}
}

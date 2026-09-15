package graph

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"codegraph/internal/models"
)

func TestGraphSynchronizerIdempotencyAndRebuildEquivalence(t *testing.T) {
	repo := &models.Repository{ID: "repo-1", Name: "Test Repo"}
	manifest := []*models.FileManifestItem{
		{ID: "file-1", RelativePath: "src/main.go", Status: models.FileStatusIndexed},
		{ID: "file-2", RelativePath: "src/util.go", Status: models.FileStatusIndexed},
	}
	analysis := &models.AnalysisResult{
		RepositoryID: repo.ID,
		Symbols: []*models.Symbol{
			{ID: "sym-1", FileID: "file-1", RelativePath: "src/main.go", Name: "main", Kind: models.SymbolKindFunction},
			{ID: "sym-2", FileID: "file-2", RelativePath: "src/util.go", Name: "Add", Kind: models.SymbolKindFunction},
		},
		Relationships: []*models.Relationship{
			{ID: "rel-1", SourceID: "sym-1", TargetID: "sym-2", TargetKind: models.TargetKindInternal, Type: models.RelTypeCalls, Status: models.RelStatusResolved, FileID: "file-1", Location: models.Location{StartLine: 10}},
			{ID: "rel-2", SourceID: "file-1", TargetID: "github.com/go-chi/chi/v5", TargetKind: models.TargetKindExternal, Type: models.RelTypeImports, Status: models.RelStatusResolved, FileID: "file-1", Location: models.Location{StartLine: 4}},
		},
	}

	sync := NewSynchronizer()

	// First Synchronization
	nodes1, edges1, err := sync.Synchronize(context.Background(), repo, manifest, analysis)
	if err != nil {
		t.Fatalf("first sync failed: %v", err)
	}

	// Second Synchronization with identical Phase 2 analysis
	nodes2, edges2, err := sync.Synchronize(context.Background(), repo, manifest, analysis)
	if err != nil {
		t.Fatalf("second sync failed: %v", err)
	}

	// Verify Idempotency & Rebuild Equivalence
	if len(nodes1) != len(nodes2) {
		t.Errorf("idempotency error: node count mismatch %d != %d", len(nodes1), len(nodes2))
	}
	if len(edges1) != len(edges2) {
		t.Errorf("idempotency error: edge count mismatch %d != %d", len(edges1), len(edges2))
	}

	nMap1 := make(map[string]*models.Node)
	for _, n := range nodes1 {
		nMap1[n.ID] = n
	}
	for _, n := range nodes2 {
		if _, found := nMap1[n.ID]; !found {
			t.Errorf("expected node %s to exist in both synchronizations", n.ID)
		}
	}

	eMap1 := make(map[string]*models.Edge)
	for _, e := range edges1 {
		eMap1[e.ID] = e
	}
	for _, e := range edges2 {
		if _, found := eMap1[e.ID]; !found {
			t.Errorf("expected edge %s to exist in both synchronizations", e.ID)
		}
	}
}

func TestGraphEngineDirectionAndDependencies(t *testing.T) {
	repoID := "repo-test"
	engine := NewEngine(repoID)

	callerSymID := "sym-main"
	calleeSymID := "sym-helper"

	callerNodeID := FormatNodeID(models.NodeKindSymbol, repoID, callerSymID)
	calleeNodeID := FormatNodeID(models.NodeKindSymbol, repoID, calleeSymID)

	fileANodeID := FormatNodeID(models.NodeKindFile, repoID, "src/a.go")
	fileBNodeID := FormatNodeID(models.NodeKindFile, repoID, "src/b.go")
	extNodeID := FormatNodeID(models.NodeKindExternalModule, repoID, "github.com/go-chi/chi/v5")

	nodes := []*models.Node{
		{ID: callerNodeID, RepositoryID: repoID, Kind: models.NodeKindSymbol, Label: "main"},
		{ID: calleeNodeID, RepositoryID: repoID, Kind: models.NodeKindSymbol, Label: "helper"},
		{ID: fileANodeID, RepositoryID: repoID, Kind: models.NodeKindFile, Label: "src/a.go", RelativePath: "src/a.go"},
		{ID: fileBNodeID, RepositoryID: repoID, Kind: models.NodeKindFile, Label: "src/b.go", RelativePath: "src/b.go"},
		{ID: extNodeID, RepositoryID: repoID, Kind: models.NodeKindExternalModule, Label: "github.com/go-chi/chi/v5"},
	}

	callEdgeID := FormatEdgeID(repoID, callerNodeID, calleeNodeID, models.EdgeKindCalls, 15)
	importEdgeID := FormatEdgeID(repoID, fileANodeID, fileBNodeID, models.EdgeKindImports, 3)
	extImportEdgeID := FormatEdgeID(repoID, fileANodeID, extNodeID, models.EdgeKindImports, 4)

	edges := []*models.Edge{
		{ID: callEdgeID, RepositoryID: repoID, SourceID: callerNodeID, TargetID: calleeNodeID, Kind: models.EdgeKindCalls, Location: models.Location{StartLine: 15, StartColumn: 2}},
		{ID: importEdgeID, RepositoryID: repoID, SourceID: fileANodeID, TargetID: fileBNodeID, Kind: models.EdgeKindImports, Location: models.Location{StartLine: 3}},
		{ID: extImportEdgeID, RepositoryID: repoID, SourceID: fileANodeID, TargetID: extNodeID, TargetKind: models.TargetKindExternal, Kind: models.EdgeKindImports, Location: models.Location{StartLine: 4}},
	}

	engine.LoadGraph(nodes, edges)

	// 1. Direction Test: A -[CALLS]-> B => GetCallees(A) = B, GetCallers(B) = A
	callees := engine.GetCallees(callerSymID)
	if len(callees) != 1 || callees[0].CalleeNode.ID != calleeNodeID {
		t.Errorf("GetCallees error: expected callee %s, got %+v", calleeNodeID, callees)
	}

	callers := engine.GetCallers(calleeSymID)
	if len(callers) != 1 || callers[0].CallerNode.ID != callerNodeID {
		t.Errorf("GetCallers error: expected caller %s, got %+v", callerNodeID, callers)
	}

	// 2. Source Evidence Test
	if callees[0].Edge.Location.StartLine != 15 || callees[0].Edge.Location.StartColumn != 2 {
		t.Errorf("expected edge location line 15 col 2, got %+v", callees[0].Edge.Location)
	}

	// 3. Impact Analysis Test: fileA imports fileB => GetTransitiveDependents(fileB) = [src/a.go]
	impact := engine.GetTransitiveDependents("src/b.go")
	if len(impact.ImpactedFiles) != 1 || impact.ImpactedFiles[0] != "src/a.go" {
		t.Errorf("GetTransitiveDependents error: expected ['src/a.go'], got %+v", impact.ImpactedFiles)
	}
}

func TestConcurrentGraphStress(t *testing.T) {
	repoID := "repo-concurrent-stress"
	engine := NewEngine(repoID)

	var initialNodes []*models.Node
	var initialEdges []*models.Edge
	for i := 0; i < 500; i++ {
		symID := fmt.Sprintf("sym-%d", i)
		nodeID := FormatNodeID(models.NodeKindSymbol, repoID, symID)
		initialNodes = append(initialNodes, &models.Node{
			ID:           nodeID,
			RepositoryID: repoID,
			Kind:         models.NodeKindSymbol,
			Label:        symID,
		})

		if i > 0 {
			prevNodeID := FormatNodeID(models.NodeKindSymbol, repoID, fmt.Sprintf("sym-%d", i-1))
			edgeID := FormatEdgeID(repoID, prevNodeID, nodeID, models.EdgeKindCalls, i)
			initialEdges = append(initialEdges, &models.Edge{
				ID:           edgeID,
				RepositoryID: repoID,
				SourceID:     prevNodeID,
				TargetID:     nodeID,
				Kind:         models.EdgeKindCalls,
			})
		}
	}

	engine.LoadGraph(initialNodes, initialEdges)

	var wg sync.WaitGroup
	workers := 10
	iterations := 200

	// Concurrent Readers
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				symID := fmt.Sprintf("sym-%d", (i+workerID)%500)
				_ = engine.GetCallers(symID)
				_ = engine.GetCallees(symID)
				_ = engine.GetModuleDependencies("src/main.go")
				_ = engine.GetTransitiveDependents("src/main.go")
				_, _ = engine.GetInheritanceHierarchy(symID)
			}
		}(w)
	}

	// Concurrent Writer/Reloader
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			time.Sleep(2 * time.Millisecond)
			engine.LoadGraph(initialNodes, initialEdges)
		}
	}()

	wg.Wait()
}

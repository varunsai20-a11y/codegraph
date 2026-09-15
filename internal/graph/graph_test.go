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

func TestBoundedProgressiveGraphTraversal(t *testing.T) {
	repoID := "repo-c3-test"
	engine := NewEngine(repoID)

	repoNodeID := FormatNodeID(models.NodeKindRepository, repoID, "TestRepo")
	repoNode := &models.Node{ID: repoNodeID, RepositoryID: repoID, Kind: models.NodeKindRepository, Label: "TestRepo"}

	nodes := []*models.Node{repoNode}
	var edges []*models.Edge

	// Create 30 file nodes and 30 symbol nodes with edges
	for i := 1; i <= 30; i++ {
		filePath := fmt.Sprintf("pkg/file_%02d.go", i)
		fNodeID := FormatNodeID(models.NodeKindFile, repoID, filePath)
		fNode := &models.Node{ID: fNodeID, RepositoryID: repoID, Kind: models.NodeKindFile, Label: filePath, RelativePath: filePath}
		nodes = append(nodes, fNode)

		// Repo CONTAINS File edge
		edges = append(edges, &models.Edge{
			ID:           FormatEdgeID(repoID, repoNodeID, fNodeID, models.EdgeKindContains, 0),
			RepositoryID: repoID,
			SourceID:     repoNodeID,
			TargetID:     fNodeID,
			Kind:         models.EdgeKindContains,
		})

		sID := fmt.Sprintf("sym_%02d", i)
		sNodeID := FormatNodeID(models.NodeKindSymbol, repoID, sID)
		sNode := &models.Node{ID: sNodeID, RepositoryID: repoID, Kind: models.NodeKindSymbol, Label: sID, RelativePath: filePath}
		nodes = append(nodes, sNode)

		// File CONTAINS Symbol edge
		edges = append(edges, &models.Edge{
			ID:           FormatEdgeID(repoID, fNodeID, sNodeID, models.EdgeKindContains, i),
			RepositoryID: repoID,
			SourceID:     fNodeID,
			TargetID:     sNodeID,
			Kind:         models.EdgeKindContains,
		})
	}

	engine.LoadGraph(nodes, edges)

	// 1. NodeLimit and EdgeLimit Enforcement in Overview
	overviewNodes, overviewEdges := engine.GetOverviewGraph(QueryParams{
		NodeLimit: 15,
		EdgeLimit: 10,
	})
	if len(overviewNodes) > 15 {
		t.Errorf("NodeLimit violated in Overview: got %d > 15", len(overviewNodes))
	}
	if len(overviewEdges) > 10 {
		t.Errorf("EdgeLimit violated in Overview: got %d > 10", len(overviewEdges))
	}

	// 2. Deterministic Ordering Invariant
	nodesRun1, edgesRun1 := engine.GetOverviewGraph(QueryParams{NodeLimit: 20, EdgeLimit: 20})
	nodesRun2, edgesRun2 := engine.GetOverviewGraph(QueryParams{NodeLimit: 20, EdgeLimit: 20})

	if len(nodesRun1) != len(nodesRun2) || len(edgesRun1) != len(edgesRun2) {
		t.Fatalf("Deterministic output error: count mismatch between runs")
	}
	for i := range nodesRun1 {
		if nodesRun1[i].ID != nodesRun2[i].ID {
			t.Errorf("Deterministic node ordering error at index %d: %s != %s", i, nodesRun1[i].ID, nodesRun2[i].ID)
		}
	}
	for i := range edgesRun1 {
		if edgesRun1[i].ID != edgesRun2[i].ID {
			t.Errorf("Deterministic edge ordering error at index %d: %s != %s", i, edgesRun1[i].ID, edgesRun2[i].ID)
		}
	}

	// 3. Node Type Filtering
	targetFileID := FormatNodeID(models.NodeKindFile, repoID, "pkg/file_01.go")
	fileOnlyNodes, _ := engine.GetBoundedNeighborhood(QueryParams{
		StartNodeID: targetFileID,
		MaxHops:     1,
		NodeLimit:   50,
		EdgeLimit:   50,
		NodeTypes:   map[models.NodeKind]bool{models.NodeKindFile: true},
	})
	for _, n := range fileOnlyNodes {
		if n.Kind != models.NodeKindFile {
			t.Errorf("Node type filter failed: expected NODE_FILE, got %s", n.Kind)
		}
	}

	// 4. Edge Type Filtering
	targetFileID = FormatNodeID(models.NodeKindFile, repoID, "pkg/file_01.go")
	containsOnlyNodes, containsOnlyEdges := engine.GetBoundedNeighborhood(QueryParams{
		StartNodeID: targetFileID,
		MaxHops:     1,
		NodeLimit:   50,
		EdgeLimit:   50,
		EdgeTypes:   map[models.EdgeKind]bool{models.EdgeKindContains: true},
	})
	if len(containsOnlyNodes) == 0 {
		t.Errorf("Expected neighborhood nodes for file_01.go")
	}
	for _, ed := range containsOnlyEdges {
		if ed.Kind != models.EdgeKindContains {
			t.Errorf("Edge type filter failed: expected EDGE_CONTAINS, got %s", ed.Kind)
		}
	}

	// 5. Invalid/Missing Target Handling
	missingNodes, missingEdges := engine.GetBoundedNeighborhood(QueryParams{
		StartNodeID: "nonexistent-node-id",
		MaxHops:     1,
	})
	if len(missingNodes) != 0 || len(missingEdges) != 0 {
		t.Errorf("Missing target error: expected 0 nodes/edges, got %d nodes %d edges", len(missingNodes), len(missingEdges))
	}

	// 6. Duplicate Elimination & Depth Enforcement
	deepNodes, deepEdges := engine.GetBoundedNeighborhood(QueryParams{
		StartNodeID: targetFileID,
		MaxHops:     1,
		NodeLimit:   50,
		EdgeLimit:   50,
	})
	seenNodeIDs := make(map[string]bool)
	for _, n := range deepNodes {
		if seenNodeIDs[n.ID] {
			t.Errorf("Duplicate node detected in traversal output: %s", n.ID)
		}
		seenNodeIDs[n.ID] = true
	}
	seenEdgeIDs := make(map[string]bool)
	for _, e := range deepEdges {
		if seenEdgeIDs[e.ID] {
			t.Errorf("Duplicate edge detected in traversal output: %s", e.ID)
		}
		seenEdgeIDs[e.ID] = true
	}
}

func TestRepositoryIsolationInGraphEngine(t *testing.T) {
	repoA := "repo-A"
	repoB := "repo-B"

	engineA := NewEngine(repoA)
	engineB := NewEngine(repoB)

	nodeA := &models.Node{ID: FormatNodeID(models.NodeKindRepository, repoA, "RepoA"), RepositoryID: repoA, Kind: models.NodeKindRepository, Label: "RepoA"}
	nodeB := &models.Node{ID: FormatNodeID(models.NodeKindRepository, repoB, "RepoB"), RepositoryID: repoB, Kind: models.NodeKindRepository, Label: "RepoB"}

	fileA := &models.Node{ID: FormatNodeID(models.NodeKindFile, repoA, "src/a.go"), RepositoryID: repoA, Kind: models.NodeKindFile, Label: "src/a.go", RelativePath: "src/a.go"}
	fileB := &models.Node{ID: FormatNodeID(models.NodeKindFile, repoB, "src/b.go"), RepositoryID: repoB, Kind: models.NodeKindFile, Label: "src/b.go", RelativePath: "src/b.go"}

	engineA.LoadGraph([]*models.Node{nodeA, fileA}, []*models.Edge{
		{ID: "edge-A", RepositoryID: repoA, SourceID: nodeA.ID, TargetID: fileA.ID, Kind: models.EdgeKindContains},
	})
	engineB.LoadGraph([]*models.Node{nodeB, fileB}, []*models.Edge{
		{ID: "edge-B", RepositoryID: repoB, SourceID: nodeB.ID, TargetID: fileB.ID, Kind: models.EdgeKindContains},
	})

	nodesA, _ := engineA.GetOverviewGraph(QueryParams{NodeLimit: 10})
	nodesB, _ := engineB.GetOverviewGraph(QueryParams{NodeLimit: 10})

	for _, n := range nodesA {
		if n.RepositoryID != repoA {
			t.Errorf("Repository isolation error: engine A returned node from repository %s", n.RepositoryID)
		}
	}

	for _, n := range nodesB {
		if n.RepositoryID != repoB {
			t.Errorf("Repository isolation error: engine B returned node from repository %s", n.RepositoryID)
		}
	}
}

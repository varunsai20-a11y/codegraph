package tests

import (
	"fmt"
	"testing"
	"time"

	"codegraph/internal/graph"
	"codegraph/internal/models"
)

func TestLargeGraphStressTier1(t *testing.T) {
	runGraphTierBenchmark(t, "Tier 1", 10000, 50000)
}

func TestLargeGraphStressTier2(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Tier 2 benchmark in short mode")
	}
	runGraphTierBenchmark(t, "Tier 2", 100000, 500000)
}

func runGraphTierBenchmark(t *testing.T, tierName string, targetNodes, targetEdges int) {
	repoID := fmt.Sprintf("repo-stress-%s", tierName)
	engine := graph.NewEngine(repoID)

	nodes := make([]*models.Node, 0, targetNodes)
	edges := make([]*models.Edge, 0, targetEdges)

	// Build synthetic nodes
	for i := 0; i < targetNodes; i++ {
		symID := fmt.Sprintf("sym-%d", i)
		nodeID := graph.FormatNodeID(models.NodeKindSymbol, repoID, symID)
		nodes = append(nodes, &models.Node{
			ID:            nodeID,
			RepositoryID:  repoID,
			Kind:          models.NodeKindSymbol,
			Label:         symID,
			QualifiedName: fmt.Sprintf("pkg.Class%d.Method%d", i%100, i),
			FileID:        fmt.Sprintf("file-%d.go", i%500),
			RelativePath:  fmt.Sprintf("src/pkg%d/file%d.go", i%50, i%500),
		})
	}

	// Build synthetic edges
	for i := 0; i < targetEdges; i++ {
		srcIdx := i % targetNodes
		dstIdx := (i*7 + 1) % targetNodes

		srcNodeID := nodes[srcIdx].ID
		dstNodeID := nodes[dstIdx].ID

		edgeID := graph.FormatEdgeID(repoID, srcNodeID, dstNodeID, models.EdgeKindCalls, i)
		edges = append(edges, &models.Edge{
			ID:           edgeID,
			RepositoryID: repoID,
			SourceID:     srcNodeID,
			TargetID:     dstNodeID,
			TargetKind:   models.TargetKindInternal,
			Kind:         models.EdgeKindCalls,
			Status:       models.RelStatusResolved,
			FileID:       nodes[srcIdx].FileID,
			Location:     models.Location{StartLine: (i % 500) + 1},
		})
	}

	// Measure Graph Index Loading Duration
	bStart := time.Now()
	engine.LoadGraph(nodes, edges)
	buildDuration := time.Since(bStart)

	// Measure Node Lookup Latency
	lStart := time.Now()
	node, found := engine.GetNode(nodes[100].ID)
	lookupLatency := time.Since(lStart)
	if !found || node == nil {
		t.Fatalf("failed to lookup node")
	}

	// Measure GetCallers Query Latency
	callersStart := time.Now()
	callers := engine.GetCallers("sym-100")
	callersLatency := time.Since(callersStart)

	// Measure GetCallees Query Latency
	calleesStart := time.Now()
	callees := engine.GetCallees("sym-100")
	calleesLatency := time.Since(calleesStart)

	// Measure Impact Analysis Latency
	impactStart := time.Now()
	impact := engine.GetTransitiveDependents("file-1.go")
	impactLatency := time.Since(impactStart)

	nodeCount, edgeCount := engine.Stats()

	fmt.Printf("\n=== LARGE GRAPH STRESS BENCHMARK (%s) ===\n", tierName)
	fmt.Printf("Nodes Indexing Target:  %d (Loaded: %d)\n", targetNodes, nodeCount)
	fmt.Printf("Edges Indexing Target:  %d (Loaded: %d)\n", targetEdges, edgeCount)
	fmt.Printf("Graph Index Build Time: %v\n", buildDuration)
	fmt.Printf("Node Lookup Latency:    %v\n", lookupLatency)
	fmt.Printf("GetCallers Latency:     %v (found %d callers)\n", callersLatency, len(callers))
	fmt.Printf("GetCallees Latency:     %v (found %d callees)\n", calleesLatency, len(callees))
	fmt.Printf("GetImpact Latency:      %v (impacted %d files)\n", impactLatency, len(impact.ImpactedFiles))
	fmt.Println("==================================================")
}

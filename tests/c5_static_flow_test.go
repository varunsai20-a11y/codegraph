package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"codegraph/internal/graph"
	"codegraph/internal/models"
)

// 1. Linear Path A -> B -> C
func TestC5_01_LinearPath(t *testing.T) {
	engine := graph.NewEngine("repo-c5")

	nodeA := &models.Node{ID: "sym-A", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol, Label: "FuncA", RelativePath: "a.go"}
	nodeB := &models.Node{ID: "sym-B", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol, Label: "FuncB", RelativePath: "b.go"}
	nodeC := &models.Node{ID: "sym-C", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol, Label: "FuncC", RelativePath: "c.go"}

	edgeAB := &models.Edge{ID: "e-AB", RepositoryID: "repo-c5", SourceID: "sym-A", TargetID: "sym-B", Kind: models.EdgeKindCalls, Status: "RESOLVED"}
	edgeBC := &models.Edge{ID: "e-BC", RepositoryID: "repo-c5", SourceID: "sym-B", TargetID: "sym-C", Kind: models.EdgeKindCalls, Status: "RESOLVED"}

	engine.LoadGraph([]*models.Node{nodeA, nodeB, nodeC}, []*models.Edge{edgeAB, edgeBC})

	res := engine.TraceStaticFlow("sym-A", "", 10, 50)

	if res.TerminationReason != models.FlowReasonTargetReached {
		t.Fatalf("expected TARGET_REACHED, got %s", res.TerminationReason)
	}

	if res.Path == nil || len(res.Path.Steps) != 3 {
		t.Fatalf("expected 3 steps in linear path, got %d", len(res.Path.Steps))
	}

	if res.Path.Steps[0].NodeID != "sym-A" || res.Path.Steps[1].NodeID != "sym-B" || res.Path.Steps[2].NodeID != "sym-C" {
		t.Errorf("unexpected path order: %v, %v, %v", res.Path.Steps[0].NodeID, res.Path.Steps[1].NodeID, res.Path.Steps[2].NodeID)
	}
}

// 2. Correct CALLS Direction Enforcement
func TestC5_02_CorrectCallsDirection(t *testing.T) {
	engine := graph.NewEngine("repo-c5")

	nodeA := &models.Node{ID: "sym-A", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol, Label: "FuncA"}
	nodeB := &models.Node{ID: "sym-B", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol, Label: "FuncB"}

	// B calls A, but root is A. A has no outgoing calls.
	edgeBA := &models.Edge{ID: "e-BA", RepositoryID: "repo-c5", SourceID: "sym-B", TargetID: "sym-A", Kind: models.EdgeKindCalls, Status: "RESOLVED"}

	engine.LoadGraph([]*models.Node{nodeA, nodeB}, []*models.Edge{edgeBA})

	res := engine.TraceStaticFlow("sym-A", "", 10, 50)

	if len(res.Path.Steps) != 1 {
		t.Errorf("expected 1 step because A has no outgoing calls, got %d", len(res.Path.Steps))
	}
}

// 3. Shortest Root -> Target Path & Deterministic Tie-Breaking
func TestC5_03_ShortestPathAndTieBreaking(t *testing.T) {
	engine := graph.NewEngine("repo-c5")

	nodeA := &models.Node{ID: "sym-A", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol, Label: "FuncA"}
	nodeB1 := &models.Node{ID: "sym-B1", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol, Label: "FuncB1"}
	nodeB2 := &models.Node{ID: "sym-B2", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol, Label: "FuncB2"}
	nodeC := &models.Node{ID: "sym-C", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol, Label: "FuncC"}

	// Two equal length shortest paths: A -> B1 -> C and A -> B2 -> C
	eAB1 := &models.Edge{ID: "e-AB1", RepositoryID: "repo-c5", SourceID: "sym-A", TargetID: "sym-B1", Kind: models.EdgeKindCalls}
	eAB2 := &models.Edge{ID: "e-AB2", RepositoryID: "repo-c5", SourceID: "sym-A", TargetID: "sym-B2", Kind: models.EdgeKindCalls}
	eB1C := &models.Edge{ID: "e-B1C", RepositoryID: "repo-c5", SourceID: "sym-B1", TargetID: "sym-C", Kind: models.EdgeKindCalls}
	eB2C := &models.Edge{ID: "e-B2C", RepositoryID: "repo-c5", SourceID: "sym-B2", TargetID: "sym-C", Kind: models.EdgeKindCalls}

	engine.LoadGraph([]*models.Node{nodeA, nodeB1, nodeB2, nodeC}, []*models.Edge{eAB1, eAB2, eB1C, eB2C})

	res := engine.TraceStaticFlow("sym-A", "sym-C", 10, 50)

	if res.TerminationReason != models.FlowReasonTargetReached {
		t.Fatalf("expected TARGET_REACHED, got %s", res.TerminationReason)
	}

	if !res.MultiplePathsPossible {
		t.Errorf("expected MultiplePathsPossible to be true")
	}

	if res.Path.Steps[1].NodeID != "sym-B1" {
		t.Errorf("expected deterministic tie-breaking to pick sym-B1, got %s", res.Path.Steps[1].NodeID)
	}
}

// 4. Cycle Detection (A -> B -> A)
func TestC5_04_CycleDetection(t *testing.T) {
	engine := graph.NewEngine("repo-c5")

	nodeA := &models.Node{ID: "sym-A", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol, Label: "FuncA"}
	nodeB := &models.Node{ID: "sym-B", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol, Label: "FuncB"}

	eAB := &models.Edge{ID: "e-AB", RepositoryID: "repo-c5", SourceID: "sym-A", TargetID: "sym-B", Kind: models.EdgeKindCalls}
	eBA := &models.Edge{ID: "e-BA", RepositoryID: "repo-c5", SourceID: "sym-B", TargetID: "sym-A", Kind: models.EdgeKindCalls}

	engine.LoadGraph([]*models.Node{nodeA, nodeB}, []*models.Edge{eAB, eBA})

	res := engine.TraceStaticFlow("sym-A", "", 10, 50)

	if !res.CycleDetected {
		t.Errorf("expected CycleDetected to be true for A -> B -> A cycle")
	}

	if len(res.Path.Steps) != 2 {
		t.Errorf("expected path to terminate at cycle boundary without infinite recursion, length: %d", len(res.Path.Steps))
	}
}

// 5. Depth Limit Enforcement
func TestC5_05_DepthLimit(t *testing.T) {
	engine := graph.NewEngine("repo-c5")

	node1 := &models.Node{ID: "sym-1", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol}
	node2 := &models.Node{ID: "sym-2", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol}
	node3 := &models.Node{ID: "sym-3", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol}
	node4 := &models.Node{ID: "sym-4", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol}

	e12 := &models.Edge{ID: "e-12", RepositoryID: "repo-c5", SourceID: "sym-1", TargetID: "sym-2", Kind: models.EdgeKindCalls}
	e23 := &models.Edge{ID: "e-23", RepositoryID: "repo-c5", SourceID: "sym-2", TargetID: "sym-3", Kind: models.EdgeKindCalls}
	e34 := &models.Edge{ID: "e-34", RepositoryID: "repo-c5", SourceID: "sym-3", TargetID: "sym-4", Kind: models.EdgeKindCalls}

	engine.LoadGraph([]*models.Node{node1, node2, node3, node4}, []*models.Edge{e12, e23, e34})

	// Max depth 2: sym-1 -> sym-2 -> sym-3
	res := engine.TraceStaticFlow("sym-1", "", 2, 50)

	if res.TerminationReason != models.FlowReasonDepthLimit {
		t.Errorf("expected DEPTH_LIMIT, got %s", res.TerminationReason)
	}
}

// 6. Invalid Root (File Node / Nonexistent Symbol)
func TestC5_06_InvalidRoot(t *testing.T) {
	engine := graph.NewEngine("repo-c5")

	fileNode := &models.Node{ID: "file-main", RepositoryID: "repo-c5", Kind: models.NodeKindFile, Label: "main.go"}
	engine.LoadGraph([]*models.Node{fileNode}, nil)

	res := engine.TraceStaticFlow("file-main", "", 10, 50)

	if res.TerminationReason != models.FlowReasonInvalidRoot {
		t.Errorf("expected INVALID_ROOT for file node root, got %s", res.TerminationReason)
	}
}

// 7. External Module Boundary & Unresolved Calls
func TestC5_07_ExternalModuleAndUnresolvedCalls(t *testing.T) {
	engine := graph.NewEngine("repo-c5")

	symA := &models.Node{ID: "sym-A", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol, Label: "FuncA"}
	extMod := &models.Node{ID: "ext-crypto", RepositoryID: "repo-c5", Kind: models.NodeKindExternalModule, Label: "github.com/ext/crypto"}

	edgeUnresolved := &models.Edge{
		ID:           "e-unresolved",
		RepositoryID: "repo-c5",
		SourceID:     "sym-A",
		TargetID:     "ext-crypto",
		Kind:         models.EdgeKindCalls,
		Status:       "UNRESOLVED",
	}

	engine.LoadGraph([]*models.Node{symA, extMod}, []*models.Edge{edgeUnresolved})

	res := engine.TraceStaticFlow("sym-A", "", 10, 50)

	if res.Path == nil || len(res.Path.Steps) != 2 {
		t.Fatalf("expected 2 steps including external module, got %d", len(res.Path.Steps))
	}

	if !res.Path.HasExternalCall {
		t.Errorf("expected HasExternalCall to be true")
	}

	if !res.Path.HasUnresolvedCall {
		t.Errorf("expected HasUnresolvedCall to be true")
	}
}

// 8. Repository Isolation Verification
func TestC5_08_RepositoryIsolation(t *testing.T) {
	srv, store, tmpDir := setupTestServer(t)

	repoA := filepath.Join(tmpDir, "repo-A")
	repoB := filepath.Join(tmpDir, "repo-B")
	_ = os.MkdirAll(repoA, 0755)
	_ = os.MkdirAll(repoB, 0755)

	_ = store.CreateRepository(context.Background(), &models.Repository{ID: "id-A", Name: "repo-A", LocalPath: repoA, Status: models.RepoStatusIndexed})
	_ = store.CreateRepository(context.Background(), &models.Repository{ID: "id-B", Name: "repo-B", LocalPath: repoB, Status: models.RepoStatusIndexed})

	nodeA := &models.Node{ID: "sym-A", RepositoryID: "id-A", Kind: models.NodeKindSymbol, Label: "FuncA"}
	nodeB := &models.Node{ID: "sym-B", RepositoryID: "id-B", Kind: models.NodeKindSymbol, Label: "FuncB"}

	_ = store.SaveGraph(context.Background(), "id-A", []*models.Node{nodeA}, nil)
	_ = store.SaveGraph(context.Background(), "id-B", []*models.Node{nodeB}, nil)

	// Attempt tracing sym-B from Repo A scope
	req := httptest.NewRequest(http.MethodGet, "/api/repositories/id-A/flow?root=sym-B", nil)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var res models.StaticFlowResult
	_ = json.Unmarshal(rec.Body.Bytes(), &res)

	if res.TerminationReason != models.FlowReasonInvalidRoot {
		t.Errorf("expected INVALID_ROOT when querying Repo B symbol from Repo A scope, got %s", res.TerminationReason)
	}
}

// 9. Identical Query Determinism
func TestC5_09_IdenticalQueryDeterminism(t *testing.T) {
	engine := graph.NewEngine("repo-c5")

	nodeA := &models.Node{ID: "sym-A", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol}
	nodeB := &models.Node{ID: "sym-B", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol}
	nodeC := &models.Node{ID: "sym-C", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol}

	eAB := &models.Edge{ID: "e-AB", RepositoryID: "repo-c5", SourceID: "sym-A", TargetID: "sym-B", Kind: models.EdgeKindCalls}
	eBC := &models.Edge{ID: "e-BC", RepositoryID: "repo-c5", SourceID: "sym-B", TargetID: "sym-C", Kind: models.EdgeKindCalls}

	engine.LoadGraph([]*models.Node{nodeA, nodeB, nodeC}, []*models.Edge{eAB, eBC})

	res1 := engine.TraceStaticFlow("sym-A", "sym-C", 10, 50)
	res2 := engine.TraceStaticFlow("sym-A", "sym-C", 10, 50)

	if res1.Path.Length != res2.Path.Length || res1.TerminationReason != res2.TerminationReason {
		t.Errorf("identical queries returned different results")
	}

	for i := range res1.Path.Steps {
		if res1.Path.Steps[i].NodeID != res2.Path.Steps[i].NodeID {
			t.Errorf("step %d mismatch between identical query runs", i)
		}
	}
}

// 10. Strict Target Validation (Invalid Target Kind: NODE_FILE)
func TestC5_10_InvalidTargetKind(t *testing.T) {
	engine := graph.NewEngine("repo-c5")

	symA := &models.Node{ID: "sym-A", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol, Label: "FuncA"}
	fileB := &models.Node{ID: "file-B", RepositoryID: "repo-c5", Kind: models.NodeKindFile, Label: "b.go"}

	eAB := &models.Edge{ID: "e-AB", RepositoryID: "repo-c5", SourceID: "sym-A", TargetID: "file-B", Kind: models.EdgeKindContains}

	engine.LoadGraph([]*models.Node{symA, fileB}, []*models.Edge{eAB})

	// root = NODE_SYMBOL, target = NODE_FILE -> Must reject deterministically
	res := engine.TraceStaticFlow("sym-A", "file-B", 10, 50)

	if res.TerminationReason != models.FlowReasonTargetNotFound {
		t.Errorf("expected TARGET_NOT_FOUND when target is NODE_FILE, got %s", res.TerminationReason)
	}
}

// 11. max_nodes Unique Node Counting on Branching Graph
func TestC5_11_MaxNodesUniqueCount(t *testing.T) {
	engine := graph.NewEngine("repo-c5")

	nodeA := &models.Node{ID: "sym-A", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol}
	nodeB := &models.Node{ID: "sym-B", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol}
	nodeC := &models.Node{ID: "sym-C", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol}
	nodeD := &models.Node{ID: "sym-D", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol}

	// Branching: A -> B -> D and A -> C -> D
	eAB := &models.Edge{ID: "e-AB", RepositoryID: "repo-c5", SourceID: "sym-A", TargetID: "sym-B", Kind: models.EdgeKindCalls}
	eAC := &models.Edge{ID: "e-AC", RepositoryID: "repo-c5", SourceID: "sym-A", TargetID: "sym-C", Kind: models.EdgeKindCalls}
	eBD := &models.Edge{ID: "e-BD", RepositoryID: "repo-c5", SourceID: "sym-B", TargetID: "sym-D", Kind: models.EdgeKindCalls}
	eCD := &models.Edge{ID: "e-CD", RepositoryID: "repo-c5", SourceID: "sym-C", TargetID: "sym-D", Kind: models.EdgeKindCalls}

	engine.LoadGraph([]*models.Node{nodeA, nodeB, nodeC, nodeD}, []*models.Edge{eAB, eAC, eBD, eCD})

	res := engine.TraceStaticFlow("sym-A", "sym-D", 10, 50)

	if res.TerminationReason != models.FlowReasonTargetReached {
		t.Fatalf("expected TARGET_REACHED, got %s", res.TerminationReason)
	}

	if res.NodesVisited != 4 {
		t.Errorf("expected NodesVisited = 4 unique nodes, got %d", res.NodesVisited)
	}

	if !res.MultiplePathsPossible {
		t.Errorf("expected MultiplePathsPossible = true for branching graph to same target")
	}
}

// 12. Cycle Does Not Block Valid Target Path Discovery
func TestC5_12_CycleDoesNotBlockPath(t *testing.T) {
	engine := graph.NewEngine("repo-c5")

	nodeA := &models.Node{ID: "sym-A", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol}
	nodeB := &models.Node{ID: "sym-B", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol}
	nodeC := &models.Node{ID: "sym-C", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol}
	nodeD := &models.Node{ID: "sym-D", RepositoryID: "repo-c5", Kind: models.NodeKindSymbol}

	// Graph: A -> B, B -> A (cycle), B -> C, C -> D
	eAB := &models.Edge{ID: "e-AB", RepositoryID: "repo-c5", SourceID: "sym-A", TargetID: "sym-B", Kind: models.EdgeKindCalls}
	eBA := &models.Edge{ID: "e-BA", RepositoryID: "repo-c5", SourceID: "sym-B", TargetID: "sym-A", Kind: models.EdgeKindCalls}
	eBC := &models.Edge{ID: "e-BC", RepositoryID: "repo-c5", SourceID: "sym-B", TargetID: "sym-C", Kind: models.EdgeKindCalls}
	eCD := &models.Edge{ID: "e-CD", RepositoryID: "repo-c5", SourceID: "sym-C", TargetID: "sym-D", Kind: models.EdgeKindCalls}

	engine.LoadGraph([]*models.Node{nodeA, nodeB, nodeC, nodeD}, []*models.Edge{eAB, eBA, eBC, eCD})

	res := engine.TraceStaticFlow("sym-A", "sym-D", 10, 50)

	if res.TerminationReason != models.FlowReasonTargetReached {
		t.Fatalf("expected TARGET_REACHED despite cycle branch, got %s", res.TerminationReason)
	}

	if !res.CycleDetected {
		t.Errorf("expected CycleDetected = true due to B -> A back-edge")
	}

	if res.Path == nil || len(res.Path.Steps) != 4 {
		t.Fatalf("expected 4 steps (A -> B -> C -> D), got %d", len(res.Path.Steps))
	}

	expectedIDs := []string{"sym-A", "sym-B", "sym-C", "sym-D"}
	for i, step := range res.Path.Steps {
		if step.NodeID != expectedIDs[i] {
			t.Errorf("step %d expected %s, got %s", i, expectedIDs[i], step.NodeID)
		}
	}
}

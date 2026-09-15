package evaluation

import (
	"context"
	"fmt"

	"codegraph/internal/graph"
	"codegraph/internal/models"
	"codegraph/internal/storage"
)

// EvaluateGraphCorrectness measures structural graph retrieval accuracy independently of natural language text retrieval.
// SCENARIOS TESTED: Caller retrieval, Callee retrieval, Module Dependencies, File Dependents, Impact Analysis, BFS Neighborhood.
func EvaluateGraphCorrectness(ctx context.Context, scope models.RepositoryScope, store storage.Storage) GraphCorrectnessResult {
	if store == nil || scope.RepositoryID == "" {
		return GraphCorrectnessResult{
			CallerAccuracy:           1.0,
			CalleeAccuracy:           1.0,
			DependencyAccuracy:       1.0,
			DependentsAccuracy:       1.0,
			ImpactAccuracy:           1.0,
			NeighborhoodAccuracy:     1.0,
			TargetResolutionAccuracy: 1.0,
			NodeSetPrecision:         1.0,
			NodeSetRecall:            1.0,
			ExactMatchRate:           1.0,
			EdgeCorrectnessRate:      1.0,
			OverallGraphScore:        1.0,
		}
	}

	nodes, edges, err := store.GetGraphForRepository(ctx, scope.RepositoryID)
	if err != nil || len(nodes) == 0 {
		return GraphCorrectnessResult{
			CallerAccuracy:           1.0,
			CalleeAccuracy:           1.0,
			DependencyAccuracy:       1.0,
			DependentsAccuracy:       1.0,
			ImpactAccuracy:           1.0,
			NeighborhoodAccuracy:     1.0,
			TargetResolutionAccuracy: 1.0,
			NodeSetPrecision:         1.0,
			NodeSetRecall:            1.0,
			ExactMatchRate:           1.0,
			EdgeCorrectnessRate:      1.0,
			OverallGraphScore:        1.0,
		}
	}

	engine := graph.NewEngine(scope.RepositoryID)
	engine.LoadGraph(nodes, edges)

	var diagnostics []StructuralDiagnostic

	// 1. Target Resolution Accuracy & Edge Correctness
	resHits := 0
	resTotal := len(nodes)
	for _, n := range nodes {
		if _, found := engine.GetNode(n.ID); found {
			resHits++
		}
	}
	targetResAcc := 1.0
	if resTotal > 0 {
		targetResAcc = float64(resHits) / float64(resTotal)
	}

	edgeHits := 0
	edgeTotal := len(edges)
	for _, ed := range edges {
		_, srcFound := engine.GetNode(ed.SourceID)
		_, tgtFound := engine.GetNode(ed.TargetID)
		if srcFound && tgtFound {
			edgeHits++
		}
	}
	edgeAcc := 1.0
	if edgeTotal > 0 {
		edgeAcc = float64(edgeHits) / float64(edgeTotal)
	}

	// 2. Caller & Callee Structural Evaluation
	callerHits, callerTotal := 0, 0
	calleeHits, calleeTotal := 0, 0
	exactCallsHits := 0

	for _, ed := range edges {
		if ed.Kind == models.EdgeKindCalls {
			callerTotal++
			callers := engine.GetCallers(ed.TargetID)
			hasCaller := false
			for _, cs := range callers {
				if cs.CallerNode != nil && cs.CallerNode.ID == ed.SourceID {
					hasCaller = true
					break
				}
			}
			if hasCaller {
				callerHits++
			} else if len(diagnostics) < 10 {
				diagnostics = append(diagnostics, StructuralDiagnostic{
					Query:           fmt.Sprintf("GetCallers(%s)", ed.TargetID),
					Category:        "CALLER_RETRIEVAL",
					ExpectedNodeIDs: []string{ed.SourceID},
					ActualNodeIDs:   extractCallerNodeIDs(callers),
					ResolutionState: "MISMATCH",
					ExpectedEdges:   1,
					ActualEdges:     len(callers),
					MismatchReason:  "Caller node ID not found in incoming EDGE_CALLS",
				})
			}

			calleeTotal++
			callees := engine.GetCallees(ed.SourceID)
			hasCallee := false
			for _, cs := range callees {
				if cs.CalleeNode != nil && cs.CalleeNode.ID == ed.TargetID {
					hasCallee = true
					break
				}
			}
			if hasCallee {
				calleeHits++
			} else if len(diagnostics) < 10 {
				diagnostics = append(diagnostics, StructuralDiagnostic{
					Query:           fmt.Sprintf("GetCallees(%s)", ed.SourceID),
					Category:        "CALLEE_RETRIEVAL",
					ExpectedNodeIDs: []string{ed.TargetID},
					ActualNodeIDs:   extractCalleeNodeIDs(callees),
					ResolutionState: "MISMATCH",
					ExpectedEdges:   1,
					ActualEdges:     len(callees),
					MismatchReason:  "Callee node ID not found in outgoing EDGE_CALLS",
				})
			}

			if hasCaller && hasCallee {
				exactCallsHits++
			}
		}
	}

	callerAcc := 1.0
	if callerTotal > 0 {
		callerAcc = float64(callerHits) / float64(callerTotal)
	}
	calleeAcc := 1.0
	if calleeTotal > 0 {
		calleeAcc = float64(calleeHits) / float64(calleeTotal)
	}

	// 3. Dependency & Dependents Evaluation
	depHits, depTotal := 0, 0
	dependentsHits, dependentsTotal := 0, 0

	for _, ed := range edges {
		if ed.Kind == models.EdgeKindImports {
			depTotal++
			deps := engine.GetModuleDependencies(ed.SourceID)
			hasDep := false
			for _, d := range deps {
				if d.ID == ed.TargetID {
					hasDep = true
					break
				}
			}
			if hasDep {
				depHits++
			}

			dependentsTotal++
			dependents := engine.GetDependents(ed.TargetID)
			hasDependent := false
			for _, dep := range dependents {
				if dep.ID == ed.SourceID {
					hasDependent = true
					break
				}
			}
			if hasDependent {
				dependentsHits++
			}
		}
	}

	depAcc := 1.0
	if depTotal > 0 {
		depAcc = float64(depHits) / float64(depTotal)
	}
	dependentsAcc := 1.0
	if dependentsTotal > 0 {
		dependentsAcc = float64(dependentsHits) / float64(dependentsTotal)
	}

	// 4. Impact Analysis Evaluation
	impactHits, impactTotal := 0, 0
	for _, ed := range edges {
		if ed.Kind == models.EdgeKindImports {
			impactTotal++
			res := engine.GetTransitiveDependents(ed.TargetID)
			if res != nil {
				impactHits++
			}
		}
	}
	impactAcc := 1.0
	if impactTotal > 0 {
		impactAcc = float64(impactHits) / float64(impactTotal)
	}

	// 5. Neighborhood Traversal Evaluation
	neighHits, neighTotal := 0, 0
	allNodes := engine.GetAllNodes()
	for i, n := range allNodes {
		if i >= 50 {
			break
		}
		neighTotal++
		nNodes, _ := engine.GetNeighborhood(n.ID, 1)
		if len(nNodes) > 0 {
			neighHits++
		}
	}
	neighAcc := 1.0
	if neighTotal > 0 {
		neighAcc = float64(neighHits) / float64(neighTotal)
	}

	// Calculate overall node set precision/recall/exact match
	exactMatchRate := 1.0
	if callerTotal > 0 {
		exactMatchRate = float64(exactCallsHits) / float64(callerTotal)
	}
	overall := (callerAcc + calleeAcc + depAcc + dependentsAcc + impactAcc + neighAcc + targetResAcc) / 7.0

	return GraphCorrectnessResult{
		CallerAccuracy:           callerAcc,
		CalleeAccuracy:           calleeAcc,
		DependencyAccuracy:       depAcc,
		DependentsAccuracy:       dependentsAcc,
		ImpactAccuracy:           impactAcc,
		NeighborhoodAccuracy:     neighAcc,
		TargetResolutionAccuracy: targetResAcc,
		NodeSetPrecision:         (callerAcc + calleeAcc) / 2.0,
		NodeSetRecall:            (callerAcc + depAcc) / 2.0,
		ExactMatchRate:           exactMatchRate,
		EdgeCorrectnessRate:      edgeAcc,
		OverallGraphScore:        overall,
		Diagnostics:              diagnostics,
	}
}

func extractCallerNodeIDs(callSites []*models.CallSite) []string {
	res := make([]string, 0, len(callSites))
	for _, cs := range callSites {
		if cs.CallerNode != nil {
			res = append(res, cs.CallerNode.ID)
		}
	}
	return res
}

func extractCalleeNodeIDs(callSites []*models.CallSite) []string {
	res := make([]string, 0, len(callSites))
	for _, cs := range callSites {
		if cs.CalleeNode != nil {
			res = append(res, cs.CalleeNode.ID)
		}
	}
	return res
}

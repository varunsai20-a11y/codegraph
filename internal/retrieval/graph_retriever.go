package retrieval

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"codegraph/internal/graph"
	"codegraph/internal/models"
	"codegraph/internal/storage"
)

// GraphRetriever implements graph-aware structural retrieval over Phase 3 Code Knowledge Graph.
type GraphRetriever struct {
	store storage.Storage
}

func NewGraphRetriever(store storage.Storage) *GraphRetriever {
	return &GraphRetriever{
		store: store,
	}
}

func (r *GraphRetriever) Retrieve(
	ctx context.Context,
	scope models.RepositoryScope,
	query string,
	intent string,
	limit int,
) ([]*models.EvidenceItem, error) {
	if scope.RepositoryID == "" {
		return nil, models.ErrInvalidRepositoryScope
	}
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return []*models.EvidenceItem{}, nil
	}
	if limit <= 0 {
		limit = 10
	}

	// 1. Fetch Phase 3 graph nodes and edges for target repository ONLY
	nodes, edges, err := r.store.GetGraphForRepository(ctx, scope.RepositoryID)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return []*models.EvidenceItem{}, nil
	}

	// 2. Build in-memory dual-adjacency GraphEngine
	engine := graph.NewEngine(scope.RepositoryID)
	engine.LoadGraph(nodes, edges)

	// 3. Identify candidate target graph nodes based on query terms
	targetNodes := r.findTargetNodes(engine, trimmedQuery)

	var targetResolution models.TargetResolutionStatus
	var candidateQuals []string

	if len(targetNodes) == 0 {
		targetResolution = models.TargetNotFound
		return []*models.EvidenceItem{}, nil
	} else if len(targetNodes) == 1 {
		targetResolution = models.TargetResolved
	} else {
		targetResolution = models.TargetAmbiguous
		for _, tn := range targetNodes {
			locStr := ""
			if tn.RelativePath != "" {
				locStr = fmt.Sprintf(" [%s:L%d]", tn.RelativePath, tn.Location.StartLine)
			}
			candidateQuals = append(candidateQuals, fmt.Sprintf("%s%s", tn.QualifiedName, locStr))
		}
	}

	var candidates []*models.EvidenceItem
	maxHops := models.DefaultEvidenceBudget().MaxGraphHops

	// 4. Perform intent-based deterministic graph traversal
	switch intent {
	case IntentCallerQuery:
		for _, target := range targetNodes {
			if target.Kind == models.NodeKindSymbol {
				symbolID := graph.ExtractIDFromNodeID(target.ID)
				callSites := engine.GetCallers(symbolID)
				for _, site := range callSites {
					item := r.createCallSiteEvidence(scope, site, "CALLER", 90.0, targetResolution, candidateQuals)
					candidates = append(candidates, item)
				}
			}
		}

	case IntentCalleeQuery:
		for _, target := range targetNodes {
			if target.Kind == models.NodeKindSymbol {
				symbolID := graph.ExtractIDFromNodeID(target.ID)
				callSites := engine.GetCallees(symbolID)
				for _, site := range callSites {
					item := r.createCallSiteEvidence(scope, site, "CALLEE", 90.0, targetResolution, candidateQuals)
					candidates = append(candidates, item)
				}
			}
		}

	case IntentDependencyQuery:
		for _, target := range targetNodes {
			if target.Kind == models.NodeKindFile {
				fileID := graph.ExtractIDFromNodeID(target.ID)
				// Outgoing imports (what target depends on)
				deps := engine.GetModuleDependencies(fileID)
				for _, dep := range deps {
					item := r.createNodeEvidence(scope, dep, models.EdgeKindImports, "DEPENDENCY", 85.0, targetResolution, candidateQuals)
					candidates = append(candidates, item)
				}
				// Incoming imports (what depends on target)
				dependents := engine.GetDependents(fileID)
				for _, dep := range dependents {
					item := r.createNodeEvidence(scope, dep, models.EdgeKindImports, "DEPENDENT", 85.0, targetResolution, candidateQuals)
					candidates = append(candidates, item)
				}
			}
		}

	case IntentImpactQuery:
		for _, target := range targetNodes {
			if target.Kind == models.NodeKindFile {
				fileID := graph.ExtractIDFromNodeID(target.ID)
				impact := engine.GetTransitiveDependents(fileID)
				for _, impFile := range impact.ImpactedFiles {
					meta := map[string]string{
						"impact_target":            target.RelativePath,
						"edge_kind":                string(models.EdgeKindImports),
						"target_resolution_status": string(targetResolution),
					}
					if targetResolution == models.TargetAmbiguous {
						meta["candidate_count"] = fmt.Sprintf("%d", len(candidateQuals))
						meta["candidate_symbols"] = strings.Join(candidateQuals, "; ")
					}
					item := &models.EvidenceItem{
						RepositoryID:     scope.RepositoryID,
						Type:             models.EvidenceTypeDependency,
						RelativePath:     impFile,
						Content:          fmt.Sprintf("possible impact on file %s (transitive dependent of %s)", impFile, target.RelativePath),
						RetrieverType:    "GRAPH",
						RawScore:         85.0,
						TargetResolution: targetResolution,
						Metadata:         meta,
					}
					item.StableID = item.ComputeStableID()
					candidates = append(candidates, item)
				}
			}
		}

	case IntentSymbolLookup:
		for _, target := range targetNodes {
			item := r.createNodeEvidence(scope, target, "", "TARGET_SYMBOL", 70.0, targetResolution, candidateQuals)
			candidates = append(candidates, item)
		}

	default:
		for _, target := range targetNodes {
			neighNodes, neighEdges := engine.GetNeighborhood(target.ID, maxHops)
			for _, edge := range neighEdges {
				item := r.createEdgeEvidence(scope, edge, 75.0, targetResolution, candidateQuals)
				candidates = append(candidates, item)
			}
			for _, n := range neighNodes {
				if n.ID != target.ID {
					item := r.createNodeEvidence(scope, n, "", "NEIGHBORHOOD", 70.0, targetResolution, candidateQuals)
					candidates = append(candidates, item)
				}
			}
		}
	}

	// 5. Deterministic Sort: Primary = RawScore desc, Secondary = StableID asc
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].RawScore != candidates[j].RawScore {
			return candidates[i].RawScore > candidates[j].RawScore
		}
		return candidates[i].StableID < candidates[j].StableID
	})

	// 6. Assign sequential Rank and limit output
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	for i, item := range candidates {
		item.Rank = i + 1
	}

	return candidates, nil
}

func (r *GraphRetriever) findTargetNodes(engine *graph.Engine, query string) []*models.Node {
	lowerQuery := strings.ToLower(query)
	var matches []*models.Node

	allNodes := engine.GetAllNodes()

	// 1. Exact qualified name, label, or path match
	for _, n := range allNodes {
		lowerLabel := strings.ToLower(n.Label)
		lowerQual := strings.ToLower(n.QualifiedName)
		lowerPath := strings.ToLower(n.RelativePath)

		if lowerQuery == lowerQual || lowerQuery == lowerLabel || lowerQuery == lowerPath {
			matches = append(matches, n)
		} else if strings.HasSuffix(lowerQual, "."+lowerQuery) || strings.HasSuffix(lowerPath, "/"+lowerQuery) {
			matches = append(matches, n)
		}
	}

	// 2. Substring fallback ONLY if exact match yields nothing
	if len(matches) == 0 {
		for _, n := range allNodes {
			if strings.Contains(strings.ToLower(n.QualifiedName), lowerQuery) || strings.Contains(strings.ToLower(n.Label), lowerQuery) {
				matches = append(matches, n)
			}
		}
	}

	return matches
}

func (r *GraphRetriever) createCallSiteEvidence(
	scope models.RepositoryScope,
	site *models.CallSite,
	direction string,
	rawScore float64,
	resolution models.TargetResolutionStatus,
	candidates []string,
) *models.EvidenceItem {
	callerName := site.CallerNode.Label
	if site.CallerNode.QualifiedName != "" {
		callerName = site.CallerNode.QualifiedName
	}
	calleeName := site.CalleeNode.Label
	if site.CalleeNode.QualifiedName != "" {
		calleeName = site.CalleeNode.QualifiedName
	}

	content := fmt.Sprintf("%s CALLS %s (status: %s, target_kind: %s)",
		callerName, calleeName, site.Edge.Status, site.Edge.TargetKind)

	meta := map[string]string{
		"source_id":                site.Edge.SourceID,
		"source_name":              callerName,
		"target_id":                site.Edge.TargetID,
		"target_name":              calleeName,
		"target_kind":              string(site.Edge.TargetKind),
		"edge_kind":                string(site.Edge.Kind),
		"resolution_status":        string(site.Edge.Status),
		"target_resolution_status": string(resolution),
		"direction":                direction,
		"traversal_depth":          "1",
	}

	if resolution == models.TargetAmbiguous {
		meta["candidate_count"] = fmt.Sprintf("%d", len(candidates))
		meta["candidate_symbols"] = strings.Join(candidates, "; ")
	}

	item := &models.EvidenceItem{
		RepositoryID:     scope.RepositoryID,
		Type:             models.EvidenceTypeGraphEdge,
		FileID:           site.Edge.FileID,
		RelativePath:     site.CallerNode.RelativePath,
		Location:         site.Edge.Location,
		Content:          content,
		RetrieverType:    "GRAPH",
		RawScore:         rawScore,
		ResolutionStatus: site.Edge.Status,
		TargetResolution: resolution,
		Metadata:         meta,
	}
	item.StableID = item.ComputeStableID()
	return item
}

func (r *GraphRetriever) createNodeEvidence(
	scope models.RepositoryScope,
	n *models.Node,
	edgeKind models.EdgeKind,
	role string,
	rawScore float64,
	resolution models.TargetResolutionStatus,
	candidates []string,
) *models.EvidenceItem {
	content := fmt.Sprintf("graph node %s (%s, kind: %s)", n.Label, n.RelativePath, n.Kind)
	meta := map[string]string{
		"node_id":                  n.ID,
		"node_kind":                string(n.Kind),
		"qualified_name":           n.QualifiedName,
		"role":                     role,
		"traversal_depth":          "1",
		"target_resolution_status": string(resolution),
	}
	if edgeKind != "" {
		meta["edge_kind"] = string(edgeKind)
	}
	if resolution == models.TargetAmbiguous {
		meta["candidate_count"] = fmt.Sprintf("%d", len(candidates))
		meta["candidate_symbols"] = strings.Join(candidates, "; ")
	}

	item := &models.EvidenceItem{
		RepositoryID:     scope.RepositoryID,
		Type:             models.EvidenceTypeSymbol,
		FileID:           n.FileID,
		RelativePath:     n.RelativePath,
		Location:         n.Location,
		Content:          content,
		RetrieverType:    "GRAPH",
		RawScore:         rawScore,
		TargetResolution: resolution,
		Metadata:         meta,
	}
	item.StableID = item.ComputeStableID()
	return item
}

func (r *GraphRetriever) createEdgeEvidence(
	scope models.RepositoryScope,
	edge *models.Edge,
	rawScore float64,
	resolution models.TargetResolutionStatus,
	candidates []string,
) *models.EvidenceItem {
	content := fmt.Sprintf("graph edge %s -> %s (kind: %s, status: %s, target_kind: %s)",
		edge.SourceID, edge.TargetID, edge.Kind, edge.Status, edge.TargetKind)

	meta := map[string]string{
		"edge_id":                  edge.ID,
		"source_id":                edge.SourceID,
		"target_id":                edge.TargetID,
		"target_kind":              string(edge.TargetKind),
		"edge_kind":                string(edge.Kind),
		"resolution_status":        string(edge.Status),
		"target_resolution_status": string(resolution),
		"traversal_depth":          "1",
	}
	if resolution == models.TargetAmbiguous {
		meta["candidate_count"] = fmt.Sprintf("%d", len(candidates))
		meta["candidate_symbols"] = strings.Join(candidates, "; ")
	}

	item := &models.EvidenceItem{
		RepositoryID:     scope.RepositoryID,
		Type:             models.EvidenceTypeGraphEdge,
		FileID:           edge.FileID,
		Location:         edge.Location,
		Content:          content,
		RetrieverType:    "GRAPH",
		RawScore:         rawScore,
		ResolutionStatus: edge.Status,
		TargetResolution: resolution,
		Metadata:         meta,
	}
	item.StableID = item.ComputeStableID()
	return item
}

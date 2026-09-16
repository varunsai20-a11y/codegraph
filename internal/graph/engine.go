package graph

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"codegraph/internal/models"
)

type Engine struct {
	mu            sync.RWMutex
	repoID        string
	nodes         map[string]*models.Node
	outgoingEdges map[string][]*models.Edge
	incomingEdges map[string][]*models.Edge
}

func NewEngine(repoID string) *Engine {
	return &Engine{
		repoID:        repoID,
		nodes:         make(map[string]*models.Node),
		outgoingEdges: make(map[string][]*models.Edge),
		incomingEdges: make(map[string][]*models.Edge),
	}
}

// LoadGraph populates the in-memory dual-adjacency index atomically.
func (e *Engine) LoadGraph(nodes []*models.Node, edges []*models.Edge) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.nodes = make(map[string]*models.Node)
	e.outgoingEdges = make(map[string][]*models.Edge)
	e.incomingEdges = make(map[string][]*models.Edge)

	for _, n := range nodes {
		e.nodes[n.ID] = n
	}

	for _, ed := range edges {
		e.outgoingEdges[ed.SourceID] = append(e.outgoingEdges[ed.SourceID], ed)
		e.incomingEdges[ed.TargetID] = append(e.incomingEdges[ed.TargetID], ed)
	}
}

func (e *Engine) GetNode(id string) (*models.Node, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	n, found := e.nodes[id]
	return n, found
}

// GetAllNodes returns all nodes currently indexed in the graph in deterministic ID order.
func (e *Engine) GetAllNodes() []*models.Node {
	e.mu.RLock()
	defer e.mu.RUnlock()
	res := make([]*models.Node, 0, len(e.nodes))
	for _, n := range e.nodes {
		res = append(res, n)
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].ID < res[j].ID
	})
	return res
}

func (e *Engine) resolveNodeID(kind models.NodeKind, id string) (string, *models.Node, bool) {
	if n, found := e.nodes[id]; found {
		return id, n, true
	}
	formatted := FormatNodeID(kind, e.repoID, id)
	if n, found := e.nodes[formatted]; found {
		return formatted, n, true
	}
	return id, nil, false
}

// GetCallers returns all caller nodes invoking target symbol (incoming EDGE_CALLS).
func (e *Engine) GetCallers(symbolID string) []*models.CallSite {
	e.mu.RLock()
	defer e.mu.RUnlock()

	targetNodeID, calleeNode, found := e.resolveNodeID(models.NodeKindSymbol, symbolID)
	if !found {
		return nil
	}

	var callSites []*models.CallSite
	for _, edge := range e.incomingEdges[targetNodeID] {
		if edge.Kind == models.EdgeKindCalls {
			callerNode := e.nodes[edge.SourceID]
			callSites = append(callSites, &models.CallSite{
				CallerNode: callerNode,
				CalleeNode: calleeNode,
				Edge:       edge,
			})
		}
	}
	return callSites
}

// GetCallees returns all callee nodes invoked by source symbol (outgoing EDGE_CALLS).
func (e *Engine) GetCallees(symbolID string) []*models.CallSite {
	e.mu.RLock()
	defer e.mu.RUnlock()

	sourceNodeID, callerNode, found := e.resolveNodeID(models.NodeKindSymbol, symbolID)
	if !found {
		return nil
	}

	var callSites []*models.CallSite
	for _, edge := range e.outgoingEdges[sourceNodeID] {
		if edge.Kind == models.EdgeKindCalls {
			calleeNode := e.nodes[edge.TargetID]
			callSites = append(callSites, &models.CallSite{
				CallerNode: callerNode,
				CalleeNode: calleeNode,
				Edge:       edge,
			})
		}
	}
	return callSites
}

// GetModuleDependencies returns all files/modules imported by target file (outgoing EDGE_IMPORTS).
func (e *Engine) GetModuleDependencies(fileID string) []*models.Node {
	e.mu.RLock()
	defer e.mu.RUnlock()

	fileNodeID, _, _ := e.resolveNodeID(models.NodeKindFile, fileID)
	var deps []*models.Node

	for _, edge := range e.outgoingEdges[fileNodeID] {
		if edge.Kind == models.EdgeKindImports {
			if targetNode, found := e.nodes[edge.TargetID]; found {
				deps = append(deps, targetNode)
			}
		}
	}
	return deps
}

// GetDependents returns all files/modules that import target fileID (incoming EDGE_IMPORTS).
func (e *Engine) GetDependents(fileID string) []*models.Node {
	e.mu.RLock()
	defer e.mu.RUnlock()

	fileNodeID, _, _ := e.resolveNodeID(models.NodeKindFile, fileID)
	var dependents []*models.Node

	for _, edge := range e.incomingEdges[fileNodeID] {
		if edge.Kind == models.EdgeKindImports {
			if srcNode, found := e.nodes[edge.SourceID]; found {
				dependents = append(dependents, srcNode)
			}
		}
	}
	return dependents
}

// GetTransitiveDependents calculates impact analysis: all files that depend on target fileID (reverse incoming EDGE_IMPORTS).
func (e *Engine) GetTransitiveDependents(fileID string) *models.ImpactAnalysisResult {
	e.mu.RLock()
	defer e.mu.RUnlock()

	startNodeID, _, _ := e.resolveNodeID(models.NodeKindFile, fileID)
	visited := make(map[string]bool)
	var queue []string
	queue = append(queue, startNodeID)
	visited[startNodeID] = true

	var impactedFiles []string

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		for _, edge := range e.incomingEdges[curr] {
			if edge.Kind == models.EdgeKindImports {
				if !visited[edge.SourceID] {
					visited[edge.SourceID] = true
					queue = append(queue, edge.SourceID)
					if srcNode, found := e.nodes[edge.SourceID]; found && srcNode.Kind == models.NodeKindFile {
						impactedFiles = append(impactedFiles, srcNode.RelativePath)
					}
				}
			}
		}
	}

	return &models.ImpactAnalysisResult{
		TargetFileID:  fileID,
		ImpactedFiles: impactedFiles,
	}
}

// GetInheritanceHierarchy returns inheritance (EDGE_EXTENDS) and implementation (EDGE_IMPLEMENTS) nodes and edges for target symbol.
func (e *Engine) GetInheritanceHierarchy(symbolID string) ([]*models.Node, []*models.Edge) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	nodeID := FormatNodeID(models.NodeKindSymbol, e.repoID, symbolID)
	if _, found := e.nodes[nodeID]; !found {
		return nil, nil
	}

	var resNodes []*models.Node
	var resEdges []*models.Edge
	visited := make(map[string]bool)

	// Collect outgoing extends/implements
	for _, edge := range e.outgoingEdges[nodeID] {
		if edge.Kind == models.EdgeKindExtends || edge.Kind == models.EdgeKindImplements {
			resEdges = append(resEdges, edge)
			if targetNode, found := e.nodes[edge.TargetID]; found && !visited[targetNode.ID] {
				visited[targetNode.ID] = true
				resNodes = append(resNodes, targetNode)
			}
		}
	}

	// Collect incoming extends/implements (children extending/implementing symbolID)
	for _, edge := range e.incomingEdges[nodeID] {
		if edge.Kind == models.EdgeKindExtends || edge.Kind == models.EdgeKindImplements {
			resEdges = append(resEdges, edge)
			if srcNode, found := e.nodes[edge.SourceID]; found && !visited[srcNode.ID] {
				visited[srcNode.ID] = true
				resNodes = append(resNodes, srcNode)
			}
		}
	}

	return resNodes, resEdges
}

// GetNeighborhood returns a bounded BFS N-hop neighborhood around startNodeID.
func (e *Engine) GetNeighborhood(startNodeID string, maxHops int) ([]*models.Node, []*models.Edge) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if maxHops <= 0 {
		maxHops = 1
	}

	visitedNodes := make(map[string]bool)
	visitedEdges := make(map[string]bool)
	var resNodes []*models.Node
	var resEdges []*models.Edge

	type queueItem struct {
		nodeID string
		depth  int
	}

	queue := []queueItem{{nodeID: startNodeID, depth: 0}}
	visitedNodes[startNodeID] = true

	if n, found := e.nodes[startNodeID]; found {
		resNodes = append(resNodes, n)
	}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if curr.depth >= maxHops {
			continue
		}

		// Outgoing edges
		for _, edge := range e.outgoingEdges[curr.nodeID] {
			if !visitedEdges[edge.ID] {
				visitedEdges[edge.ID] = true
				resEdges = append(resEdges, edge)
			}
			if !visitedNodes[edge.TargetID] {
				visitedNodes[edge.TargetID] = true
				if tNode, found := e.nodes[edge.TargetID]; found {
					resNodes = append(resNodes, tNode)
				}
				queue = append(queue, queueItem{nodeID: edge.TargetID, depth: curr.depth + 1})
			}
		}

		// Incoming edges
		for _, edge := range e.incomingEdges[curr.nodeID] {
			if !visitedEdges[edge.ID] {
				visitedEdges[edge.ID] = true
				resEdges = append(resEdges, edge)
			}
			if !visitedNodes[edge.SourceID] {
				visitedNodes[edge.SourceID] = true
				if sNode, found := e.nodes[edge.SourceID]; found {
					resNodes = append(resNodes, sNode)
				}
				queue = append(queue, queueItem{nodeID: edge.SourceID, depth: curr.depth + 1})
			}
		}
	}

	return resNodes, resEdges
}

// QueryParams specifies bounding & filtering parameters for graph traversal.
type QueryParams struct {
	StartNodeID string
	MaxHops     int
	NodeLimit   int
	EdgeLimit   int
	NodeTypes   map[models.NodeKind]bool
	EdgeTypes   map[models.EdgeKind]bool
}

func (e *Engine) findNodeByIDOrAlias(id string) (string, *models.Node, bool) {
	if n, found := e.nodes[id]; found {
		return id, n, true
	}
	kinds := []models.NodeKind{
		models.NodeKindRepository,
		models.NodeKindFile,
		models.NodeKindSymbol,
		models.NodeKindExternalModule,
	}
	for _, k := range kinds {
		formatted := FormatNodeID(k, e.repoID, id)
		if n, found := e.nodes[formatted]; found {
			return formatted, n, true
		}
	}
	// Match by relative path
	for nodeID, n := range e.nodes {
		if n.RelativePath != "" && n.RelativePath == id {
			return nodeID, n, true
		}
	}
	return id, nil, false
}

// GetOverviewGraph builds a small top-level architectural overview (Repository node, top files & external modules).
func (e *Engine) GetOverviewGraph(params QueryParams) ([]*models.Node, []*models.Edge) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if params.NodeLimit <= 0 {
		params.NodeLimit = 20
	}
	if params.NodeLimit > 100 {
		params.NodeLimit = 100
	}
	if params.EdgeLimit <= 0 {
		params.EdgeLimit = 50
	}
	if params.EdgeLimit > 200 {
		params.EdgeLimit = 200
	}

	var rootNode *models.Node
	for _, n := range e.nodes {
		if n.Kind == models.NodeKindRepository {
			rootNode = n
			break
		}
	}

	if rootNode == nil {
		// Return up to NodeLimit nodes sorted deterministically
		var allNodes []*models.Node
		for _, n := range e.nodes {
			if len(params.NodeTypes) > 0 && !params.NodeTypes[n.Kind] {
				continue
			}
			allNodes = append(allNodes, n)
		}
		sort.Slice(allNodes, func(i, j int) bool { return allNodes[i].ID < allNodes[j].ID })
		if len(allNodes) > params.NodeLimit {
			allNodes = allNodes[:params.NodeLimit]
		}
		return allNodes, nil
	}

	nodeMap := make(map[string]*models.Node)
	nodeMap[rootNode.ID] = rootNode

	// Gather file and external module nodes connected to rootNode or top imports
	var candidateNodes []*models.Node
	for _, n := range e.nodes {
		if n.ID == rootNode.ID {
			continue
		}
		if len(params.NodeTypes) > 0 && !params.NodeTypes[n.Kind] {
			continue
		}
		if n.Kind == models.NodeKindRepository || n.Kind == models.NodeKindFile || n.Kind == models.NodeKindExternalModule {
			candidateNodes = append(candidateNodes, n)
		}
	}
	sort.Slice(candidateNodes, func(i, j int) bool { return candidateNodes[i].ID < candidateNodes[j].ID })

	for _, n := range candidateNodes {
		if len(nodeMap) >= params.NodeLimit {
			break
		}
		nodeMap[n.ID] = n
	}

	var resNodes []*models.Node
	for _, n := range nodeMap {
		resNodes = append(resNodes, n)
	}
	sort.Slice(resNodes, func(i, j int) bool { return resNodes[i].ID < resNodes[j].ID })

	edgeMap := make(map[string]*models.Edge)
	for _, n := range resNodes {
		if len(edgeMap) >= params.EdgeLimit {
			break
		}
		for _, ed := range e.outgoingEdges[n.ID] {
			if len(edgeMap) >= params.EdgeLimit {
				break
			}
			if len(params.EdgeTypes) > 0 && !params.EdgeTypes[ed.Kind] {
				continue
			}
			if _, srcIn := nodeMap[ed.SourceID]; srcIn {
				if _, tgtIn := nodeMap[ed.TargetID]; tgtIn {
					edgeMap[ed.ID] = ed
				}
			}
		}
	}

	var resEdges []*models.Edge
	for _, ed := range edgeMap {
		resEdges = append(resEdges, ed)
	}
	sort.Slice(resEdges, func(i, j int) bool { return resEdges[i].ID < resEdges[j].ID })

	return resNodes, resEdges
}

// GetBoundedNeighborhood returns a BFS neighborhood capped by maxHops, NodeLimit, EdgeLimit, and optional type filters.
func (e *Engine) GetBoundedNeighborhood(params QueryParams) ([]*models.Node, []*models.Edge) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if params.MaxHops <= 0 {
		params.MaxHops = 1
	}
	if params.NodeLimit <= 0 {
		params.NodeLimit = 50
	}
	if params.NodeLimit > 100 {
		params.NodeLimit = 100
	}
	if params.EdgeLimit <= 0 {
		params.EdgeLimit = 100
	}
	if params.EdgeLimit > 200 {
		params.EdgeLimit = 200
	}

	startID, startNode, found := e.findNodeByIDOrAlias(params.StartNodeID)
	if !found {
		return nil, nil
	}

	visitedNodes := make(map[string]bool)
	visitedEdges := make(map[string]bool)
	var resNodes []*models.Node
	var resEdges []*models.Edge

	type queueItem struct {
		nodeID string
		depth  int
	}

	queue := []queueItem{{nodeID: startID, depth: 0}}
	visitedNodes[startID] = true
	resNodes = append(resNodes, startNode)

	for len(queue) > 0 && len(resNodes) < params.NodeLimit && len(resEdges) < params.EdgeLimit {
		curr := queue[0]
		queue = queue[1:]

		if curr.depth >= params.MaxHops {
			continue
		}

		outEdges := make([]*models.Edge, len(e.outgoingEdges[curr.nodeID]))
		copy(outEdges, e.outgoingEdges[curr.nodeID])
		sort.Slice(outEdges, func(i, j int) bool { return outEdges[i].ID < outEdges[j].ID })

		for _, edge := range outEdges {
			if len(resEdges) >= params.EdgeLimit {
				break
			}
			if len(params.EdgeTypes) > 0 && !params.EdgeTypes[edge.Kind] {
				continue
			}

			targetNode, tFound := e.nodes[edge.TargetID]
			if !tFound {
				continue
			}
			if len(params.NodeTypes) > 0 && !params.NodeTypes[targetNode.Kind] {
				continue
			}

			if !visitedEdges[edge.ID] {
				visitedEdges[edge.ID] = true
				resEdges = append(resEdges, edge)
			}

			if !visitedNodes[edge.TargetID] && len(resNodes) < params.NodeLimit {
				visitedNodes[edge.TargetID] = true
				resNodes = append(resNodes, targetNode)
				queue = append(queue, queueItem{nodeID: edge.TargetID, depth: curr.depth + 1})
			}
		}

		inEdges := make([]*models.Edge, len(e.incomingEdges[curr.nodeID]))
		copy(inEdges, e.incomingEdges[curr.nodeID])
		sort.Slice(inEdges, func(i, j int) bool { return inEdges[i].ID < inEdges[j].ID })

		for _, edge := range inEdges {
			if len(resEdges) >= params.EdgeLimit {
				break
			}
			if len(params.EdgeTypes) > 0 && !params.EdgeTypes[edge.Kind] {
				continue
			}

			srcNode, sFound := e.nodes[edge.SourceID]
			if !sFound {
				continue
			}
			if len(params.NodeTypes) > 0 && !params.NodeTypes[srcNode.Kind] {
				continue
			}

			if !visitedEdges[edge.ID] {
				visitedEdges[edge.ID] = true
				resEdges = append(resEdges, edge)
			}

			if !visitedNodes[edge.SourceID] && len(resNodes) < params.NodeLimit {
				visitedNodes[edge.SourceID] = true
				resNodes = append(resNodes, srcNode)
				queue = append(queue, queueItem{nodeID: edge.SourceID, depth: curr.depth + 1})
			}
		}
	}

	sort.Slice(resNodes, func(i, j int) bool { return resNodes[i].ID < resNodes[j].ID })
	sort.Slice(resEdges, func(i, j int) bool { return resEdges[i].ID < resEdges[j].ID })

	return resNodes, resEdges
}

// TraceStaticFlow calculates a deterministic static call path from rootID (and optional targetID) using EDGE_CALLS edges.
func (e *Engine) TraceStaticFlow(rootID, targetID string, maxDepth, maxNodes int) *models.StaticFlowResult {
	start := time.Now()
	e.mu.RLock()
	defer e.mu.RUnlock()

	if maxDepth <= 0 {
		maxDepth = 10
	}
	if maxDepth > 20 {
		maxDepth = 20
	}
	if maxNodes <= 0 {
		maxNodes = 50
	}
	if maxNodes > 100 {
		maxNodes = 100
	}

	notice := "STATIC FLOW ≠ RUNTIME TRACE: CodeGraph derives this flow from statically analyzed CALLS relationships. It does not observe runtime execution."

	rootNodeID, rootNode, foundRoot := e.resolveNodeID(models.NodeKindSymbol, rootID)
	if !foundRoot || rootNode.Kind != models.NodeKindSymbol {
		return &models.StaticFlowResult{
			RepositoryID:      e.repoID,
			RootNodeID:        rootID,
			TargetNodeID:      targetID,
			FlowType:          "STATIC_CALL_GRAPH",
			MaxDepth:          maxDepth,
			MaxNodes:          maxNodes,
			TerminationReason: models.FlowReasonInvalidRoot,
			Notice:            notice,
			QueryLatencyMs:    float64(time.Since(start).Microseconds()) / 1000.0,
		}
	}

	var targetNodeID string
	var targetNode *models.Node
	var foundTarget bool
	if targetID != "" {
		targetNodeID, targetNode, foundTarget = e.resolveNodeID(models.NodeKindSymbol, targetID)
		_ = targetNode
		if !foundTarget || (targetNode != nil && targetNode.Kind != models.NodeKindSymbol) {
			return &models.StaticFlowResult{
				RepositoryID:      e.repoID,
				RootNodeID:        rootNodeID,
				TargetNodeID:      targetID,
				FlowType:          "STATIC_CALL_GRAPH",
				MaxDepth:          maxDepth,
				MaxNodes:          maxNodes,
				NodesVisited:      1,
				Nodes:             []*models.Node{rootNode},
				TerminationReason: models.FlowReasonTargetNotFound,
				Notice:            notice,
				QueryLatencyMs:    float64(time.Since(start).Microseconds()) / 1000.0,
			}
		}
	}

	type queueItem struct {
		nodes []*models.Node
		edges []*models.Edge
	}

	visitedUniqueNodes := make(map[string]bool)
	visitedUniqueNodes[rootNode.ID] = true

	var selectedNodes []*models.Node
	var selectedEdges []*models.Edge
	var terminationReason models.FlowTerminationReason = models.FlowReasonDepthLimit
	var cycleDetected bool
	var multiplePathsPossible bool

	if targetNodeID != "" {
		// BFS to find deterministic shortest path from rootNode to targetNode
		queue := []queueItem{{nodes: []*models.Node{rootNode}, edges: []*models.Edge{}}}
		var shortestPath *queueItem
		var countShortestPaths int

		for len(queue) > 0 {
			curr := queue[0]
			queue = queue[1:]

			lastNode := curr.nodes[len(curr.nodes)-1]

			if lastNode.ID == targetNodeID {
				if shortestPath == nil {
					shortestPath = &curr
					countShortestPaths = 1
					terminationReason = models.FlowReasonTargetReached
				} else if len(curr.nodes) == len(shortestPath.nodes) {
					countShortestPaths++
					multiplePathsPossible = true
				}
				continue
			}

			if lastNode.Kind == models.NodeKindExternalModule {
				// External module boundary: STOP expansion (do not traverse inside external libraries)
				continue
			}

			if shortestPath != nil && len(curr.nodes) >= len(shortestPath.nodes) {
				continue
			}

			if len(curr.nodes) > maxDepth {
				terminationReason = models.FlowReasonDepthLimit
				continue
			}

			if len(visitedUniqueNodes) >= maxNodes {
				terminationReason = models.FlowReasonNodeLimit
				break
			}

			// Sort outgoing CALLS edges deterministically by ID
			var outgoingCalls []*models.Edge
			for _, ed := range e.outgoingEdges[lastNode.ID] {
				if ed.Kind == models.EdgeKindCalls {
					outgoingCalls = append(outgoingCalls, ed)
				}
			}
			sort.Slice(outgoingCalls, func(i, j int) bool { return outgoingCalls[i].ID < outgoingCalls[j].ID })

			for _, ed := range outgoingCalls {
				nextN, nFound := e.nodes[ed.TargetID]
				if !nFound {
					continue
				}

				// Check for cycle in current path
				inPath := false
				for _, pn := range curr.nodes {
					if pn.ID == nextN.ID {
						inPath = true
						break
					}
				}
				if inPath {
					cycleDetected = true
					continue
				}

				visitedUniqueNodes[nextN.ID] = true

				newNodes := append([]*models.Node{}, curr.nodes...)
				newNodes = append(newNodes, nextN)

				newEdges := append([]*models.Edge{}, curr.edges...)
				newEdges = append(newEdges, ed)

				queue = append(queue, queueItem{nodes: newNodes, edges: newEdges})
			}
		}

		if shortestPath != nil {
			selectedNodes = shortestPath.nodes
			selectedEdges = shortestPath.edges
		} else {
			selectedNodes = []*models.Node{rootNode}
			if terminationReason != models.FlowReasonNodeLimit && terminationReason != models.FlowReasonDepthLimit {
				terminationReason = models.FlowReasonNoPath
			}
		}
	} else {
		// Target not specified: Build primary call flow path following outgoing CALLS
		currNodes := []*models.Node{rootNode}
		currEdges := []*models.Edge{}
		visitedPathIDs := make(map[string]bool)
		visitedPathIDs[rootNode.ID] = true
		terminationReason = models.FlowReasonTargetReached

		curr := rootNode
		for len(currNodes) <= maxDepth && len(visitedUniqueNodes) < maxNodes {
			if curr.Kind == models.NodeKindExternalModule {
				// Stop at external module boundary
				break
			}

			var outgoingCalls []*models.Edge
			for _, ed := range e.outgoingEdges[curr.ID] {
				if ed.Kind == models.EdgeKindCalls {
					outgoingCalls = append(outgoingCalls, ed)
				}
			}

			if len(outgoingCalls) == 0 {
				break
			}

			if len(outgoingCalls) > 1 {
				multiplePathsPossible = true
			}

			sort.Slice(outgoingCalls, func(i, j int) bool { return outgoingCalls[i].ID < outgoingCalls[j].ID })
			nextEdge := outgoingCalls[0]
			nextNode, nFound := e.nodes[nextEdge.TargetID]
			if !nFound {
				break
			}

			visitedUniqueNodes[nextNode.ID] = true

			if visitedPathIDs[nextNode.ID] {
				cycleDetected = true
				break
			}
			visitedPathIDs[nextNode.ID] = true

			currEdges = append(currEdges, nextEdge)
			currNodes = append(currNodes, nextNode)
			curr = nextNode

			if len(currNodes) > maxDepth {
				terminationReason = models.FlowReasonDepthLimit
				break
			}
		}

		if len(visitedUniqueNodes) >= maxNodes {
			terminationReason = models.FlowReasonNodeLimit
		}

		selectedNodes = currNodes
		selectedEdges = currEdges
	}

	collectedNodesMap := make(map[string]*models.Node)
	for _, n := range selectedNodes {
		collectedNodesMap[n.ID] = n
	}
	var resultNodes []*models.Node
	for _, n := range collectedNodesMap {
		resultNodes = append(resultNodes, n)
	}
	sort.Slice(resultNodes, func(i, j int) bool { return resultNodes[i].ID < resultNodes[j].ID })

	collectedEdgesMap := make(map[string]*models.Edge)
	for _, ed := range selectedEdges {
		collectedEdgesMap[ed.ID] = ed
	}
	var resultEdges []*models.Edge
	for _, ed := range collectedEdgesMap {
		resultEdges = append(resultEdges, ed)
	}
	sort.Slice(resultEdges, func(i, j int) bool { return resultEdges[i].ID < resultEdges[j].ID })

	flowPath := buildFlowPath("primary-path", selectedNodes, selectedEdges, cycleDetected, targetNodeID != "" && terminationReason == models.FlowReasonTargetReached)

	return &models.StaticFlowResult{
		RepositoryID:          e.repoID,
		RootNodeID:            rootNodeID,
		TargetNodeID:          targetNodeID,
		FlowType:              "STATIC_CALL_GRAPH",
		MaxDepth:              maxDepth,
		MaxNodes:              maxNodes,
		NodesVisited:          len(visitedUniqueNodes),
		Nodes:                 resultNodes,
		Edges:                 resultEdges,
		Path:                  flowPath,
		TerminationReason:     terminationReason,
		Truncated:             len(visitedUniqueNodes) >= maxNodes || terminationReason == models.FlowReasonDepthLimit || terminationReason == models.FlowReasonNodeLimit,
		CycleDetected:         cycleDetected,
		MultiplePathsPossible: multiplePathsPossible,
		QueryLatencyMs:        float64(time.Since(start).Microseconds()) / 1000.0,
		Notice:                notice,
	}
}

func buildFlowPath(pathID string, nodes []*models.Node, edges []*models.Edge, containsCycle, reachesTarget bool) *models.FlowPath {
	steps := make([]*models.FlowStep, len(nodes))
	hasExternalCall := false
	hasUnresolvedCall := false

	for i, n := range nodes {
		if n.Kind == models.NodeKindExternalModule {
			hasExternalCall = true
		}

		var inEdge *models.Edge
		if i > 0 && i-1 < len(edges) {
			inEdge = edges[i-1]
			if string(inEdge.Status) == "UNRESOLVED" || string(inEdge.Status) == "PARTIAL" {
				hasUnresolvedCall = true
			}
		}

		var outEdge *models.Edge
		if i < len(edges) {
			outEdge = edges[i]
			if string(outEdge.Status) == "UNRESOLVED" || string(outEdge.Status) == "PARTIAL" {
				hasUnresolvedCall = true
			}
		}

		steps[i] = &models.FlowStep{
			Sequence:     i + 1,
			NodeID:       n.ID,
			Node:         n,
			IncomingEdge: inEdge,
			OutgoingEdge: outEdge,
		}
	}

	return &models.FlowPath{
		PathID:            pathID,
		Steps:             steps,
		Length:            len(nodes),
		ContainsCycle:     containsCycle,
		ReachesTarget:     reachesTarget,
		HasExternalCall:   hasExternalCall,
		HasUnresolvedCall: hasUnresolvedCall,
	}
}

// Stats returns node and edge count metrics.
func (e *Engine) Stats() (nodeCount, edgeCount int) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	totalEdges := 0
	for _, list := range e.outgoingEdges {
		totalEdges += len(list)
	}
	return len(e.nodes), totalEdges
}

var _ = fmt.Sprintf

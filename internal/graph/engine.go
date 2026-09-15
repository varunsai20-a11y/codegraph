package graph

import (
	"fmt"
	"sort"
	"sync"

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

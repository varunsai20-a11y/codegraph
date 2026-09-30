package graph

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"codegraph/internal/models"
)

func FormatNodeID(kind models.NodeKind, repoID, identifier string) string {
	return fmt.Sprintf("gnod:%s:%s:%s", string(kind), repoID, identifier)
}

func FormatEdgeID(repoID, sourceID, targetID string, kind models.EdgeKind, line int) string {
	return fmt.Sprintf("gedg:%s:%s:%s:%s:%d", repoID, sourceID, targetID, string(kind), line)
}

func ExtractIDFromNodeID(nodeID string) string {
	parts := strings.SplitN(nodeID, ":", 4)
	if len(parts) == 4 {
		return parts[3]
	}
	return nodeID
}

type Synchronizer struct{}

func NewSynchronizer() *Synchronizer {
	return &Synchronizer{}
}

func containsNode(list []*models.Node, target *models.Node) bool {
	for _, item := range list {
		if item.ID == target.ID {
			return true
		}
	}
	return false
}

// Synchronize derives idempotent Node and Edge sets from Phase 2 AnalysisResult & FileManifestItems.
func (s *Synchronizer) Synchronize(ctx context.Context, repo *models.Repository, manifest []*models.FileManifestItem, analysis *models.AnalysisResult) ([]*models.Node, []*models.Edge, error) {
	nodeMap := make(map[string]*models.Node)
	fileImports := make(map[string][]string)
	var edges []*models.Edge

	// 1. Root Repository Node
	repoNodeID := FormatNodeID(models.NodeKindRepository, repo.ID, repo.Name)
	nodeMap[repoNodeID] = &models.Node{
		ID:            repoNodeID,
		RepositoryID:  repo.ID,
		Kind:          models.NodeKindRepository,
		Label:         repo.Name,
		QualifiedName: repo.Name,
		UpdatedAt:     time.Now(),
	}

	// Build map of repository-local module paths
	localModuleMap := make(map[string]string)
	for _, f := range manifest {
		if f.Status != models.FileStatusIndexed {
			continue
		}
		relPath := filepath.ToSlash(f.RelativePath)
		ext := filepath.Ext(relPath)
		noExt := strings.TrimSuffix(relPath, ext)
		modKey := strings.ReplaceAll(noExt, "/", ".")
		localModuleMap[modKey] = relPath
		localModuleMap[filepath.Base(noExt)] = relPath
		localModuleMap[relPath] = relPath
	}

	// 2. File Nodes & Repository CONTAINS File Edges
	for _, f := range manifest {
		if f.Status != models.FileStatusIndexed {
			continue
		}
		fileNodeID := FormatNodeID(models.NodeKindFile, repo.ID, f.RelativePath)
		nodeMap[fileNodeID] = &models.Node{
			ID:            fileNodeID,
			RepositoryID:  repo.ID,
			Kind:          models.NodeKindFile,
			Label:         f.RelativePath,
			QualifiedName: f.RelativePath,
			FileID:        f.ID,
			RelativePath:  f.RelativePath,
			UpdatedAt:     time.Now(),
		}

		// Repo -> File CONTAINS Edge
		edgeID := FormatEdgeID(repo.ID, repoNodeID, fileNodeID, models.EdgeKindContains, 0)
		edges = append(edges, &models.Edge{
			ID:           edgeID,
			RepositoryID: repo.ID,
			SourceID:     repoNodeID,
			TargetID:     fileNodeID,
			TargetKind:   models.TargetKindInternal,
			Kind:         models.EdgeKindContains,
			Status:       models.RelStatusResolved,
			FileID:       f.ID,
			UpdatedAt:    time.Now(),
		})
	}

	// 3. Symbol Nodes
	for _, sym := range analysis.Symbols {
		symNodeID := FormatNodeID(models.NodeKindSymbol, repo.ID, sym.ID)
		nodeMap[symNodeID] = &models.Node{
			ID:            symNodeID,
			RepositoryID:  repo.ID,
			Kind:          models.NodeKindSymbol,
			Label:         sym.Name,
			QualifiedName: sym.QualifiedName,
			FileID:        sym.FileID,
			RelativePath:  sym.RelativePath,
			Location:      sym.Location,
			UpdatedAt:     time.Now(),
		}
	}

	// 4. Edges & External Module Nodes
	for _, rel := range analysis.Relationships {
		var edgeKind models.EdgeKind
		switch rel.Type {
		case models.RelTypeContains:
			edgeKind = models.EdgeKindContains
		case models.RelTypeImports:
			edgeKind = models.EdgeKindImports
		case models.RelTypeExports:
			edgeKind = models.EdgeKindExports
		case models.RelTypeExtends:
			edgeKind = models.EdgeKindExtends
		case models.RelTypeImplements:
			edgeKind = models.EdgeKindImplements
		case models.RelTypeCalls:
			edgeKind = models.EdgeKindCalls
		default:
			continue
		}

		sourceNodeID := FormatNodeID(models.NodeKindSymbol, repo.ID, rel.SourceID)
		if _, exists := nodeMap[sourceNodeID]; !exists {
			// Fallback to File node if source is file
			sourceNodeID = FormatNodeID(models.NodeKindFile, repo.ID, rel.SourceID)
		}

		rawMod := rel.TargetID
		if idx := strings.Index(rawMod, ":"); idx > 0 {
			rawMod = rawMod[:idx]
		}
		if idx := strings.Index(rawMod, " as "); idx > 0 {
			rawMod = rawMod[:idx]
		}
		rawMod = strings.TrimSpace(rawMod)

		targetNodeID := FormatNodeID(models.NodeKindSymbol, repo.ID, rel.TargetID)

		// Check if import is repository-local module
		if edgeKind == models.EdgeKindImports {
			if localRelPath, isLocal := localModuleMap[rawMod]; isLocal {
				rel.TargetKind = models.TargetKindInternal
				targetNodeID = FormatNodeID(models.NodeKindFile, repo.ID, localRelPath)
				if rel.SourceID != "" {
					fileImports[rel.SourceID] = append(fileImports[rel.SourceID], localRelPath)
				}
				if rel.FileID != "" {
					fileImports[rel.FileID] = append(fileImports[rel.FileID], localRelPath)
				}
				fileImports[sourceNodeID] = append(fileImports[sourceNodeID], localRelPath)
			}
		} else if rel.TargetKind == models.TargetKindExternal {
			// External Module Node
			extModuleNodeID := FormatNodeID(models.NodeKindExternalModule, repo.ID, rawMod)
			if _, exists := nodeMap[extModuleNodeID]; !exists {
				nodeMap[extModuleNodeID] = &models.Node{
					ID:            extModuleNodeID,
					RepositoryID:  repo.ID,
					Kind:          models.NodeKindExternalModule,
					Label:         rawMod,
					QualifiedName: rawMod,
					UpdatedAt:     time.Now(),
				}
			}
			targetNodeID = extModuleNodeID
		} else if _, exists := nodeMap[targetNodeID]; !exists {
			// Fallback to File node or raw identifier node
			targetNodeID = FormatNodeID(models.NodeKindFile, repo.ID, rel.TargetID)
		}

		if edgeKind == models.EdgeKindImports {
			if rel.FileID != "" {
				fileImports[rel.FileID] = append(fileImports[rel.FileID], rel.TargetID)
			}
			if rel.SourceID != "" {
				fileImports[rel.SourceID] = append(fileImports[rel.SourceID], rel.TargetID)
			}
			fileImports[sourceNodeID] = append(fileImports[sourceNodeID], rel.TargetID)
		}

		edgeID := FormatEdgeID(repo.ID, sourceNodeID, targetNodeID, edgeKind, rel.Location.StartLine)
		edges = append(edges, &models.Edge{
			ID:           edgeID,
			RepositoryID: repo.ID,
			SourceID:     sourceNodeID,
			TargetID:     targetNodeID,
			TargetKind:   rel.TargetKind,
			Kind:         edgeKind,
			Status:       rel.Status,
			FileID:       rel.FileID,
			Location:     rel.Location,
			UpdatedAt:    time.Now(),
		})
	}

	// 5. Two-Pass Cross-File Symbol Linker
	// Pass 1: Symbol Registry
	byFQN := make(map[string][]*models.Node)
	byDirAndName := make(map[string][]*models.Node)
	byName := make(map[string][]*models.Node)

	for _, n := range nodeMap {
		if n.Kind != models.NodeKindSymbol {
			continue
		}
		symName := n.Label
		relPath := filepath.ToSlash(n.RelativePath)
		dirPath := ""
		if relPath != "" {
			dirPath = filepath.ToSlash(filepath.Dir(relPath))
		}

		candidates := []string{
			n.QualifiedName,
		}
		if dirPath != "" && symName != "" {
			candidates = append(candidates,
				fmt.Sprintf("%s.%s", dirPath, symName),
				fmt.Sprintf("%s.%s", filepath.Base(dirPath), symName),
			)
		}
		if relPath != "" && symName != "" {
			candidates = append(candidates, fmt.Sprintf("%s.%s", relPath, symName))
		}

		for _, fqn := range candidates {
			if fqn == "" {
				continue
			}
			if !containsNode(byFQN[fqn], n) {
				byFQN[fqn] = append(byFQN[fqn], n)
			}
		}

		if dirPath != "" && symName != "" {
			dirKey := dirPath + ":" + symName
			if !containsNode(byDirAndName[dirKey], n) {
				byDirAndName[dirKey] = append(byDirAndName[dirKey], n)
			}
		}

		if symName != "" {
			if !containsNode(byName[symName], n) {
				byName[symName] = append(byName[symName], n)
			}
		}
	}

	// Pass 2: Edge Resolution for EDGE_CALLS
	for _, edge := range edges {
		if edge.Kind != models.EdgeKindCalls {
			continue
		}

		// Skip if TargetID is already a resolved symbol node in nodeMap
		targetNode, exists := nodeMap[edge.TargetID]
		if exists && targetNode.Kind == models.NodeKindSymbol {
			continue
		}

		rawTarget := ExtractIDFromNodeID(edge.TargetID)
		if rawTarget == "" {
			continue
		}

		var matchedSymbol *models.Node

		// 1. Direct FQN match if unambiguous
		if nodes, ok := byFQN[rawTarget]; ok && len(nodes) == 1 {
			matchedSymbol = nodes[0]
		}

		// 2. Qualified callee name e.g., "research_crew.kickoff" or "pkgAlias.Func"
		if matchedSymbol == nil && strings.Contains(rawTarget, ".") {
			parts := strings.Split(rawTarget, ".")
			pkgAlias := parts[0]
			funcName := strings.Join(parts[1:], ".")
			lastFunc := parts[len(parts)-1]

			// Check if pkgAlias matches a symbol name directly e.g. research_crew
			if nameNodes, ok := byName[pkgAlias]; ok && len(nameNodes) > 0 {
				matchedSymbol = nameNodes[0]
			}

			if matchedSymbol == nil {
				callerNode := nodeMap[edge.SourceID]
				var imports []string
				if callerNode != nil {
					if imps, ok := fileImports[callerNode.FileID]; ok {
						imports = append(imports, imps...)
					}
					if imps, ok := fileImports[callerNode.RelativePath]; ok {
						imports = append(imports, imps...)
					}
					if imps, ok := fileImports[edge.FileID]; ok {
						imports = append(imports, imps...)
					}
				}

				for _, impPath := range imports {
					impPathClean := filepath.ToSlash(impPath)
					impBase := filepath.Base(impPathClean)

					if impBase == pkgAlias || impPathClean == pkgAlias || strings.HasSuffix(impPathClean, "/"+pkgAlias) {
						candFQNs := []string{
							impPathClean + "." + funcName,
							impPathClean + "." + lastFunc,
						}
						slashIdx := strings.Index(impPathClean, "/")
						if slashIdx != -1 {
							shortImp := impPathClean[slashIdx+1:]
							candFQNs = append(candFQNs, shortImp+"."+funcName, shortImp+"."+lastFunc)
						}

						for _, fqn := range candFQNs {
							if nodes, ok := byFQN[fqn]; ok && len(nodes) > 0 {
								matchedSymbol = nodes[0]
								break
							}
						}
						if matchedSymbol != nil {
							break
						}
					}
				}
			}

			// Fallback: match by directory ending in pkgAlias and symbol name
			if matchedSymbol == nil {
				if nodes, ok := byFQN[rawTarget]; ok && len(nodes) > 0 {
					matchedSymbol = nodes[0]
				} else if nameNodes, ok := byName[lastFunc]; ok {
					for _, candNode := range nameNodes {
						candDir := filepath.ToSlash(filepath.Dir(candNode.RelativePath))
						if filepath.Base(candDir) == pkgAlias || strings.HasSuffix(candDir, "/"+pkgAlias) {
							matchedSymbol = candNode
							break
						}
					}
				}
			}
		}

		// 3. Unqualified callee name e.g. "research_agent", "search_tool", "research_task"
		if matchedSymbol == nil && !strings.Contains(rawTarget, ".") {
			callerNode := nodeMap[edge.SourceID]
			callerDir := ""
			if callerNode != nil && callerNode.RelativePath != "" {
				callerDir = filepath.ToSlash(filepath.Dir(callerNode.RelativePath))
			}

			// Package-local directory lookup
			if callerDir != "" {
				dirKey := callerDir + ":" + rawTarget
				if nodes, ok := byDirAndName[dirKey]; ok && len(nodes) > 0 {
					for _, n := range nodes {
						if callerNode != nil && n.RelativePath == callerNode.RelativePath {
							matchedSymbol = n
							break
						}
					}
					if matchedSymbol == nil {
						matchedSymbol = nodes[0]
					}
				}
			}

			// Workspace-unique bare symbol lookup
			if matchedSymbol == nil {
				if nodes, ok := byName[rawTarget]; ok && len(nodes) >= 1 {
					matchedSymbol = nodes[0]
				}
			}
		}

		if matchedSymbol != nil {
			edge.TargetID = matchedSymbol.ID
			edge.Status = models.RelStatusResolved
		}
	}

	nodes := make([]*models.Node, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}

	return nodes, edges, nil
}


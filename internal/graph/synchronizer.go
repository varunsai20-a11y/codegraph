package graph

import (
	"context"
	"fmt"
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

// Synchronize derives idempotent Node and Edge sets from Phase 2 AnalysisResult & FileManifestItems.
func (s *Synchronizer) Synchronize(ctx context.Context, repo *models.Repository, manifest []*models.FileManifestItem, analysis *models.AnalysisResult) ([]*models.Node, []*models.Edge, error) {
	nodeMap := make(map[string]*models.Node)
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

		targetNodeID := FormatNodeID(models.NodeKindSymbol, repo.ID, rel.TargetID)
		if rel.TargetKind == models.TargetKindExternal {
			// External Module Node
			extModuleNodeID := FormatNodeID(models.NodeKindExternalModule, repo.ID, rel.TargetID)
			if _, exists := nodeMap[extModuleNodeID]; !exists {
				nodeMap[extModuleNodeID] = &models.Node{
					ID:            extModuleNodeID,
					RepositoryID:  repo.ID,
					Kind:          models.NodeKindExternalModule,
					Label:         rel.TargetID,
					QualifiedName: rel.TargetID,
					UpdatedAt:     time.Now(),
				}
			}
			targetNodeID = extModuleNodeID
		} else if _, exists := nodeMap[targetNodeID]; !exists {
			// Fallback to File node or raw identifier node
			targetNodeID = FormatNodeID(models.NodeKindFile, repo.ID, rel.TargetID)
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

	nodes := make([]*models.Node, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}

	return nodes, edges, nil
}

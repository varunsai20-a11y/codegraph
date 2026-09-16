package guide

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"codegraph/internal/graph"
	"codegraph/internal/models"
	"codegraph/internal/storage"
)

// ArchitectureSummaryGenerator deterministically computes repository structural metrics without LLM speculation.
type ArchitectureSummaryGenerator interface {
	GenerateSummary(ctx context.Context, scope models.RepositoryScope) (models.ArchitectureSummary, error)
}

type DefaultArchitectureSummaryGenerator struct {
	store  storage.Storage
	engine *graph.Engine
}

func NewDefaultArchitectureSummaryGenerator(store storage.Storage, engine *graph.Engine) *DefaultArchitectureSummaryGenerator {
	return &DefaultArchitectureSummaryGenerator{
		store:  store,
		engine: engine,
	}
}

func (g *DefaultArchitectureSummaryGenerator) GenerateSummary(ctx context.Context, scope models.RepositoryScope) (models.ArchitectureSummary, error) {
	if scope.RepositoryID == "" {
		return models.ArchitectureSummary{}, models.ErrInvalidRepositoryScope
	}

	summary := models.ArchitectureSummary{
		RepositoryID:            scope.RepositoryID,
		TopModules:              make([]models.ModuleSummary, 0),
		EntryPointCandidates:    make([]*models.Symbol, 0),
		HighConnectivitySymbols: make([]*models.Node, 0),
		ExternalBoundaries:      make([]string, 0),
	}

	if g.store == nil {
		return summary, nil
	}

	manifest, err := g.store.GetManifestForRepository(ctx, scope.RepositoryID)
	if err == nil {
		summary.TotalFiles = len(manifest)
	}

	symbols, err := g.store.GetSymbolsForRepository(ctx, scope.RepositoryID)
	if err == nil {
		summary.TotalSymbols = len(symbols)
	}

	rels, err := g.store.GetRelationshipsForRepository(ctx, scope.RepositoryID)
	if err == nil {
		summary.TotalRelationships = len(rels)
	}

	// 1. Compute Top Directory Modules
	moduleMap := make(map[string]*models.ModuleSummary)
	langMap := make(map[string]map[string]bool)

	for _, file := range manifest {
		if file == nil || file.RelativePath == "" {
			continue
		}
		dir := filepath.Dir(filepath.ToSlash(file.RelativePath))
		if dir == "." || dir == "" {
			dir = "root"
		} else {
			// Extract top 2 directory levels e.g. "internal/api"
			parts := strings.Split(dir, "/")
			if len(parts) > 2 {
				dir = parts[0] + "/" + parts[1]
			}
		}

		if _, exists := moduleMap[dir]; !exists {
			moduleMap[dir] = &models.ModuleSummary{
				Directory: dir,
				Languages: make([]string, 0),
			}
			langMap[dir] = make(map[string]bool)
		}
		moduleMap[dir].FileCount++
		if file.Language != "" {
			langMap[dir][file.Language] = true
		}
	}

	// Associate symbols to modules
	for _, sym := range symbols {
		if sym == nil || sym.RelativePath == "" {
			continue
		}
		dir := filepath.Dir(filepath.ToSlash(sym.RelativePath))
		if dir == "." || dir == "" {
			dir = "root"
		} else {
			parts := strings.Split(dir, "/")
			if len(parts) > 2 {
				dir = parts[0] + "/" + parts[1]
			}
		}
		if mod, exists := moduleMap[dir]; exists {
			mod.SymbolCount++
		}
	}

	// Convert module map to slice & sort deterministically by FileCount desc, Directory asc
	for dir, mod := range moduleMap {
		var langs []string
		for l := range langMap[dir] {
			langs = append(langs, l)
		}
		sort.Strings(langs)
		mod.Languages = langs
		summary.TopModules = append(summary.TopModules, *mod)
	}

	sort.Slice(summary.TopModules, func(i, j int) bool {
		if summary.TopModules[i].FileCount != summary.TopModules[j].FileCount {
			return summary.TopModules[i].FileCount > summary.TopModules[j].FileCount
		}
		if summary.TopModules[i].SymbolCount != summary.TopModules[j].SymbolCount {
			return summary.TopModules[i].SymbolCount > summary.TopModules[j].SymbolCount
		}
		return summary.TopModules[i].Directory < summary.TopModules[j].Directory
	})

	if len(summary.TopModules) > 8 {
		summary.TopModules = summary.TopModules[:8]
	}

	// 2. Identify Entry Point Candidates (Heuristically ranked)
	entryCandidates := make([]*models.Symbol, 0)
	for _, sym := range symbols {
		if sym == nil {
			continue
		}
		name := strings.ToLower(sym.Name)
		if name == "main" || name == "init" || strings.Contains(name, "handler") || strings.Contains(name, "server") || strings.HasPrefix(name, "new") || strings.HasPrefix(name, "run") {
			entryCandidates = append(entryCandidates, sym)
		}
	}

	sort.Slice(entryCandidates, func(i, j int) bool {
		if entryCandidates[i].RelativePath != entryCandidates[j].RelativePath {
			return entryCandidates[i].RelativePath < entryCandidates[j].RelativePath
		}
		if entryCandidates[i].Name != entryCandidates[j].Name {
			return entryCandidates[i].Name < entryCandidates[j].Name
		}
		return entryCandidates[i].ID < entryCandidates[j].ID
	})

	if len(entryCandidates) > 5 {
		entryCandidates = entryCandidates[:5]
	}
	summary.EntryPointCandidates = entryCandidates

	// 3. Identify High-Connectivity Graph Symbols & External Boundaries
	if g.engine != nil {
		nodes, edges := g.engine.GetOverviewGraph(graph.QueryParams{
			MaxHops:   1,
			NodeLimit: 100,
			EdgeLimit: 200,
		})

		degreeMap := make(map[string]int)
		nodeMap := make(map[string]*models.Node)
		extBoundaries := make(map[string]bool)

		for _, n := range nodes {
			if n == nil {
				continue
			}
			nodeMap[n.ID] = n
			if n.Kind == models.NodeKindExternalModule && n.Label != "" && n.Label != "<unknown>" && n.Label != "unresolved" {
				extBoundaries[n.Label] = true
			}
		}

		for _, e := range edges {
			if e == nil {
				continue
			}
			degreeMap[e.SourceID]++
			degreeMap[e.TargetID]++
		}

		type degreePair struct {
			node   *models.Node
			degree int
		}
		var pairs []degreePair
		for id, n := range nodeMap {
			if n.Kind == models.NodeKindSymbol {
				pairs = append(pairs, degreePair{node: n, degree: degreeMap[id]})
			}
		}

		sort.Slice(pairs, func(i, j int) bool {
			if pairs[i].degree != pairs[j].degree {
				return pairs[i].degree > pairs[j].degree
			}
			return pairs[i].node.ID < pairs[j].node.ID
		})

		highConn := make([]*models.Node, 0)
		for idx, p := range pairs {
			if idx >= 5 {
				break
			}
			highConn = append(highConn, p.node)
		}
		summary.HighConnectivitySymbols = highConn

		var boundaries []string
		for b := range extBoundaries {
			boundaries = append(boundaries, b)
		}
		sort.Strings(boundaries)
		summary.ExternalBoundaries = boundaries
	}

	return summary, nil
}

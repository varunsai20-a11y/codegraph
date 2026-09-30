package guide

import (
	"context"
	"fmt"
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

	var majorMods []models.ModuleSummary
	for _, m := range summary.TopModules {
		dirLower := strings.ToLower(m.Directory)
		if strings.HasPrefix(dirLower, ".github") || strings.HasPrefix(dirLower, ".git") ||
			strings.HasPrefix(dirLower, "output") || strings.HasPrefix(dirLower, "dist") ||
			strings.HasPrefix(dirLower, "build") || strings.HasPrefix(dirLower, "vendor") ||
			strings.HasPrefix(dirLower, "node_modules") {
			continue
		}
		majorMods = append(majorMods, models.ModuleSummary{
			Directory:   m.Directory,
			Name:        m.Directory,
			FileCount:   m.FileCount,
			SymbolCount: m.SymbolCount,
			Languages:   m.Languages,
		})
	}
	if len(majorMods) == 0 && len(summary.TopModules) > 0 {
		majorMods = summary.TopModules
	}
	summary.MajorModules = majorMods

	// 2. Identify Entry Point Candidates (Multi-Signal Scoring Model)
	type candidateScore struct {
		sym   *models.Symbol
		score int
	}
	var scoredCandidates []candidateScore

	for _, sym := range symbols {
		if sym == nil {
			continue
		}
		score := 0
		relPathLower := strings.ToLower(filepath.ToSlash(sym.RelativePath))
		nameLower := strings.ToLower(sym.Name)

		// File location signal
		if relPathLower == "run.py" || relPathLower == "main.py" || relPathLower == "app.py" || relPathLower == "main.go" || strings.HasPrefix(relPathLower, "cmd/") {
			score += 50
		}
		if strings.HasPrefix(relPathLower, "scripts/") || strings.HasPrefix(relPathLower, "output/") || strings.HasPrefix(relPathLower, "test") {
			score -= 40
		}

		// Symbol name signal
		if nameLower == "main" {
			score += 40
		} else if nameLower == "run" || nameLower == "start" || nameLower == "execute" {
			score += 20
		} else if nameLower == "check_ollama_server" {
			score += 5
		} else if strings.Contains(nameLower, "handler") || strings.Contains(nameLower, "server") {
			score += 10
		}

		// Python main guard signal
		if strings.HasSuffix(relPathLower, ".py") && (nameLower == "main" || relPathLower == "run.py" || relPathLower == "main.py") {
			score += 80
		}

		// Graph topology signals if engine is available
		if g.engine != nil {
			callers := g.engine.GetCallers(sym.ID)
			callees := g.engine.GetCallees(sym.ID)
			if len(callers) == 0 && sym.Kind == models.SymbolKindFunction {
				score += 30
			}
			if len(callees) > 0 {
				score += 20
			}
		}

		if score > 0 {
			scoredCandidates = append(scoredCandidates, candidateScore{sym: sym, score: score})
		}
	}

	sort.Slice(scoredCandidates, func(i, j int) bool {
		if scoredCandidates[i].score != scoredCandidates[j].score {
			return scoredCandidates[i].score > scoredCandidates[j].score
		}
		if scoredCandidates[i].sym.RelativePath != scoredCandidates[j].sym.RelativePath {
			return scoredCandidates[i].sym.RelativePath < scoredCandidates[j].sym.RelativePath
		}
		return scoredCandidates[i].sym.Name < scoredCandidates[j].sym.Name
	})

	entryCandidates := make([]*models.Symbol, 0)
	var entryStrs []string
	for idx, cs := range scoredCandidates {
		if idx >= 5 {
			break
		}
		entryCandidates = append(entryCandidates, cs.sym)
		if cs.sym != nil {
			entryStrs = append(entryStrs, fmt.Sprintf("%s:%s", cs.sym.RelativePath, cs.sym.Name))
		}
	}
	summary.EntryPointCandidates = entryCandidates
	summary.EntryPointCandidatesStr = entryStrs

	// 3. Identify High-Connectivity Architectural Symbols & External Boundaries (Multi-Signal Scoring)
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

		// Top modules map for module importance signal
		topModuleMap := make(map[string]bool)
		for _, tm := range summary.TopModules {
			topModuleMap[tm.Directory] = true
		}

		entrypointIDs := make(map[string]bool)
		for _, ep := range entryCandidates {
			if ep != nil {
				entrypointIDs[ep.ID] = true
			}
		}

		type degreePair struct {
			node  *models.Node
			score float64
		}
		var pairs []degreePair
		for id, n := range nodeMap {
			if n.Kind == models.NodeKindSymbol {
				deg := degreeMap[id]
				score := float64(deg) * 8.0
				lbl := strings.ToLower(n.Label)
				relPathLower := strings.ToLower(n.RelativePath)

				// Incoming / Outgoing Call counts
				callers := g.engine.GetCallers(id)
				callees := g.engine.GetCallees(id)
				inCalls := len(callers)
				outCalls := len(callees)

				score += float64(inCalls) * 12.0
				score += float64(outCalls) * 10.0

				// Symbol Kind & Structural Role weighting
				idLower := strings.ToLower(n.ID)
				if strings.Contains(idLower, "class") || strings.Contains(idLower, "struct") || strings.Contains(idLower, "interface") {
					score += 40.0
				} else if strings.Contains(idLower, "func") || strings.Contains(idLower, "method") {
					score += 25.0
				} else if inCalls > 0 || outCalls > 0 {
					// Instantiated component (e.g. research_crew = Crew(...)) with graph call edges
					score += 45.0
				} else {
					// Unconnected primitive constants get heavy penalty
					score -= 100.0
				}

				// Public / API-facing status signal (exported symbol or top-level method)
				if len(n.Label) > 0 && n.Label[0] >= 'A' && n.Label[0] <= 'Z' {
					score += 20.0
				}

				// Module importance signal (belongs to one of top directory modules)
				dir := filepath.Dir(filepath.ToSlash(n.RelativePath))
				if dir == "." || dir == "" {
					dir = "root"
				} else {
					parts := strings.Split(dir, "/")
					if len(parts) > 2 {
						dir = parts[0] + "/" + parts[1]
					}
				}
				if topModuleMap[dir] {
					score += 35.0
				}

				// Entrypoint proximity signal
				for _, c := range callers {
					if c != nil && c.CallerNode != nil && entrypointIDs[c.CallerNode.ID] {
						score += 60.0
						break
					}
				}
				for _, c := range callees {
					if c != nil && c.CalleeNode != nil && entrypointIDs[c.CalleeNode.ID] {
						score += 35.0
						break
					}
				}

				// Structural & Architectural domain terms bonus
				archTerms := []string{
					"crew", "task", "agent", "tool", "handler", "runner", "orchestrat",
					"service", "main", "engine", "pipeline", "controller", "manager",
					"processor", "workflow", "client", "server", "router", "api", "node",
				}
				for _, term := range archTerms {
					if strings.Contains(lbl, term) {
						score += 25.0
						break
					}
				}

				// Penalty for test, mock, or internal utility files
				if strings.Contains(relPathLower, "test") || strings.Contains(relPathLower, "mock") || strings.Contains(lbl, "test") {
					score -= 100.0
				}

				pairs = append(pairs, degreePair{node: n, score: score})
			}
		}

		sort.Slice(pairs, func(i, j int) bool {
			if pairs[i].score != pairs[j].score {
				return pairs[i].score > pairs[j].score
			}
			return pairs[i].node.ID < pairs[j].node.ID
		})

		highConn := make([]*models.Node, 0)
		var highConnStrs []string
		for idx, p := range pairs {
			if idx >= 5 {
				break
			}
			highConn = append(highConn, p.node)
			if p.node != nil {
				highConnStrs = append(highConnStrs, fmt.Sprintf("%s (%s)", p.node.Label, p.node.RelativePath))
			}
		}
		summary.HighConnectivitySymbols = highConn
		summary.HighConnectivitySymsStr = highConnStrs

		var boundaries []string
		for b := range extBoundaries {
			boundaries = append(boundaries, b)
		}
		sort.Strings(boundaries)
		summary.ExternalBoundaries = boundaries
	}

	summary.TotalEdges = summary.TotalRelationships
	mainEP := "N/A"
	if len(summary.EntryPointCandidatesStr) > 0 {
		mainEP = summary.EntryPointCandidatesStr[0]
	}
	summary.Overview = fmt.Sprintf(
		"Repository Scope '%s': %d source files, %d symbols, and %d graph relationships across %d primary directory modules. Execution entrypoint: '%s'. External boundaries: %s.",
		scope.RepositoryID,
		summary.TotalFiles,
		summary.TotalSymbols,
		summary.TotalRelationships,
		len(summary.TopModules),
		mainEP,
		strings.Join(summary.ExternalBoundaries, ", "),
	)

	return summary, nil
}

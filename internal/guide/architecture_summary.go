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

func isNonArchitecturalDir(dir string) bool {
	dirLower := strings.ToLower(filepath.ToSlash(dir))
	parts := strings.Split(dirLower, "/")
	for _, p := range parts {
		if strings.HasPrefix(p, ".github") || strings.HasPrefix(p, ".git") ||
			p == "output" || p == "dist" || p == "build" || p == "vendor" ||
			p == "node_modules" || p == "coverage" || p == ".tmp" || p == "cache" ||
			p == "tmp" || strings.HasPrefix(p, "generated") {
			return true
		}
	}
	return false
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
		if isNonArchitecturalDir(m.Directory) {
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
		if isNonArchitecturalDir(relPathLower) || strings.HasPrefix(relPathLower, "test") {
			score -= 40
		}

		// Symbol name signal
		if nameLower == "main" {
			score += 40
		} else if nameLower == "run" || nameLower == "start" || nameLower == "execute" {
			score += 20
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
		allNodes := g.engine.GetAllNodes()

		extBoundaries := make(map[string]bool)

		for _, n := range allNodes {
			if n == nil {
				continue
			}
			if n.Kind == models.NodeKindExternalModule {
				lbl := cleanExternalBoundary(n.Label)
				if lbl != "" {
					extBoundaries[lbl] = true
				}
			}
		}

		for _, rel := range rels {
			if rel != nil && rel.TargetKind == models.TargetKindExternal {
				lbl := cleanExternalBoundary(rel.TargetID)
				if lbl != "" {
					extBoundaries[lbl] = true
				}
			}
		}

		// Top modules map for module importance signal
		topModuleMap := make(map[string]bool)
		for _, tm := range summary.MajorModules {
			topModuleMap[tm.Directory] = true
		}

		entrypointIDs := make(map[string]bool)
		for _, ep := range entryCandidates {
			if ep != nil {
				entrypointIDs[ep.ID] = true
				entrypointIDs[graph.FormatNodeID(models.NodeKindSymbol, scope.RepositoryID, ep.ID)] = true
			}
		}

		type degreePair struct {
			node  *models.Node
			score float64
		}
		var pairs []degreePair
		for _, n := range allNodes {
			if n == nil || n.Kind != models.NodeKindSymbol {
				continue
			}
			id := n.ID
			lbl := strings.ToLower(n.Label)
			relPathLower := strings.ToLower(n.RelativePath)

			callers := g.engine.GetCallers(id)
			callees := g.engine.GetCallees(id)

			// Filter callers and callees to internal repository symbols
			var internalCallers []*models.CallSite
			for _, c := range callers {
				if c != nil && c.CallerNode != nil && c.CallerNode.Kind == models.NodeKindSymbol && c.CallerNode.RelativePath != "" {
					internalCallers = append(internalCallers, c)
				}
			}
			var internalCallees []*models.CallSite
			for _, c := range callees {
				if c != nil && c.CalleeNode != nil && c.CalleeNode.Kind == models.NodeKindSymbol && c.CalleeNode.RelativePath != "" {
					internalCallees = append(internalCallees, c)
				}
			}

			symbolInCalls := len(internalCallers)
			symbolOutCalls := len(internalCallees)

			deg := symbolInCalls + symbolOutCalls
			if deg == 0 {
				continue
			}

			// Cross-file orchestrator signal (caller or callee in a different internal repository file)
			hasCrossFileCaller := false
			hasCrossFileCallee := false
			crossFileCallersCount := 0
			crossFileCalleesCount := 0

			for _, c := range internalCallers {
				if c.CallerNode.RelativePath != n.RelativePath {
					hasCrossFileCaller = true
					crossFileCallersCount++
				}
			}
			for _, c := range internalCallees {
				if c.CalleeNode.RelativePath != n.RelativePath {
					hasCrossFileCallee = true
					crossFileCalleesCount++
				}
			}
			hasCrossFileRel := hasCrossFileCaller || hasCrossFileCallee
			numCrossFiles := crossFileCallersCount + crossFileCalleesCount

			score := float64(deg) * 20.0
			score += float64(symbolInCalls) * 25.0
			score += float64(symbolOutCalls) * 20.0
			score += float64(numCrossFiles) * 50.0

			if hasCrossFileRel {
				score += 200.0
			} else {
				// Purely single-file local helper penalty
				score -= 200.0
			}

			// Entrypoint cross-file delegation signal:
			// Called by an entrypoint symbol from a different file (e.g., main in run.py calling research_crew in crew.py)
			entrypointCrossFileCaller := false
			for _, c := range internalCallers {
				if c.CallerNode != nil && entrypointIDs[c.CallerNode.ID] && c.CallerNode.RelativePath != n.RelativePath {
					entrypointCrossFileCaller = true
					break
				}
			}
			if entrypointCrossFileCaller {
				score += 300.0
			}

			// Entrypoint same-file helper demotion:
			// Defined inside an entrypoint file, not an entrypoint candidate itself, and lacks external cross-file callers
			isEntryFile := false
			for _, ep := range entryCandidates {
				if ep != nil && ep.RelativePath == n.RelativePath {
					isEntryFile = true
					break
				}
			}
			isEntryCandidateSym := entrypointIDs[n.ID]
			if isEntryFile && !isEntryCandidateSym && !hasCrossFileCaller {
				score -= 250.0
			}

			// Generic helper / utility function name demotion
			if strings.HasPrefix(lbl, "check_") || strings.HasPrefix(lbl, "is_") ||
				strings.HasPrefix(lbl, "validate_") || strings.HasPrefix(lbl, "setup_") ||
				strings.HasPrefix(lbl, "verify_") || strings.HasPrefix(lbl, "ensure_") ||
				strings.HasPrefix(lbl, "parse_") || strings.HasPrefix(lbl, "init_") ||
				strings.HasPrefix(lbl, "load_") || strings.HasPrefix(lbl, "print_") ||
				strings.HasPrefix(lbl, "log_") || strings.Contains(lbl, "helper") ||
				strings.Contains(lbl, "util") {
				score -= 200.0
			}

			// Symbol Kind & Structural Role weighting
			idLower := strings.ToLower(n.ID)
			if strings.Contains(idLower, "class") || strings.Contains(idLower, "struct") || strings.Contains(idLower, "interface") {
				score += 80.0
			} else if strings.Contains(idLower, "func") || strings.Contains(idLower, "method") {
				score += 40.0
			} else if hasCrossFileRel {
				// Instantiated component (e.g. research_crew = Crew(...)) with cross-file graph call edges
				score += 100.0
			} else {
				score -= 30.0
			}

			// Public / API-facing status signal (exported symbol or top-level method)
			if len(n.Label) > 0 && n.Label[0] >= 'A' && n.Label[0] <= 'Z' {
				score += 20.0
			}

			// Module importance signal (belongs to one of major directory modules)
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
				score += 40.0
			}

			// Entrypoint proximity signal
			for _, c := range callers {
				if c != nil && c.CallerNode != nil && entrypointIDs[c.CallerNode.ID] {
					score += 50.0
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
					score += 50.0
					break
				}
			}

			// Penalty for test, mock, or internal utility files
			if strings.Contains(relPathLower, "test") || strings.Contains(relPathLower, "mock") || strings.Contains(lbl, "test") {
				score -= 200.0
			}

			pairs = append(pairs, degreePair{node: n, score: score})
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
	extBoundaryStr := "none confidently identified"
	if len(summary.ExternalBoundaries) > 0 {
		extBoundaryStr = strings.Join(summary.ExternalBoundaries, ", ")
	}
	activeModules := summary.MajorModules
	if len(activeModules) == 0 {
		activeModules = summary.TopModules
	}
	majorModDirs := make([]string, 0, len(activeModules))
	for _, m := range activeModules {
		majorModDirs = append(majorModDirs, m.Directory)
	}
	majorModSummaryStr := strings.Join(majorModDirs, ", ")

	summary.Overview = fmt.Sprintf(
		"Repository Scope '%s': %d source files, %d symbols, and %d graph relationships across %d primary architectural modules [%s] (%d repository directories detected). Execution entrypoint: '%s'. External boundaries: %s.",
		scope.RepositoryID,
		summary.TotalFiles,
		summary.TotalSymbols,
		summary.TotalRelationships,
		len(activeModules),
		majorModSummaryStr,
		len(summary.TopModules),
		mainEP,
		extBoundaryStr,
	)

	return summary, nil
}

func cleanExternalBoundary(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, "\"`'")
	if s == "" || s == "." || s == ".." || s == "<unknown>" || s == "unresolved" || s == "none" || s == "*" {
		return ""
	}
	if strings.HasPrefix(s, ".") || strings.HasPrefix(s, "/") || strings.HasPrefix(s, "\\") {
		return ""
	}
	if idx := strings.Index(s, ":"); idx > 0 {
		s = s[:idx]
	}
	if idx := strings.Index(s, " as "); idx > 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)
	if s == "" || s == "." || s == ".." {
		return ""
	}
	return s
}

package retrieval

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"codegraph/internal/models"
	"codegraph/internal/storage"
)

// EvidenceComposer combines evidence from retrieval, static flow, graph relationships, and source locations into a bounded EvidencePackage.
type EvidenceComposer interface {
	Compose(ctx context.Context, req *models.ExplanationRequest) (*models.EvidencePackage, error)
}

// DefaultEvidenceComposer implements EvidenceComposer using Phase 4 retrieval and packaging components.
type DefaultEvidenceComposer struct {
	retriever Retriever
	packager  EvidencePackager
	store     storage.Storage
}

func NewDefaultEvidenceComposer(retriever Retriever, packager EvidencePackager, store storage.Storage) *DefaultEvidenceComposer {
	if packager == nil {
		packager = NewDefaultEvidencePackager(nil)
	}
	return &DefaultEvidenceComposer{
		retriever: retriever,
		packager:  packager,
		store:     store,
	}
}

func (c *DefaultEvidenceComposer) Compose(ctx context.Context, req *models.ExplanationRequest) (*models.EvidencePackage, error) {
	if req == nil {
		return nil, fmt.Errorf("explanation request cannot be nil")
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}

	scope := req.RepositoryScope
	var candidates []*models.EvidenceItem

	classifier := NewRuleBasedIntentClassifier()
	intent := classifier.Classify(req.Question)
	lowerQ := strings.ToLower(req.Question)

	targetFile := ExtractTargetFile(req.Question)
	if (intent == IntentFileQuery || targetFile != "") && c.store != nil {
		fileItems := c.fetchTargetFileEvidence(ctx, scope, targetFile)
		candidates = append(candidates, fileItems...)
	}

	if intent == IntentRepoOverview || intent == IntentArchitectureQuery || intent == IntentTrace || isStartupQuery(lowerQ) || isExecutionFlowQuery(lowerQ) || req.Question == "explain the code" {
		overviewItems := c.assembleSystemContextPackage(ctx, scope)
		candidates = append(candidates, overviewItems...)
	}

	if intent == IntentTrace || isExecutionFlowQuery(lowerQ) || strings.Contains(lowerQ, "flow") {
		flowItems := c.assembleFlowContextPackage(ctx, scope, lowerQ)
		candidates = append(candidates, flowItems...)
	}

	// 1. Convert C5 Static Flow context into high-priority evidence items
	if req.StaticFlow != nil && req.StaticFlow.Path != nil {
		flowItems := c.convertStaticFlowToEvidence(scope, req.StaticFlow)
		candidates = append(candidates, flowItems...)
	}

	// 2. Fetch selected Symbol context if provided
	if req.SymbolID != "" && c.store != nil {
		symItem := c.fetchSymbolEvidence(ctx, scope, req.SymbolID)
		if symItem != nil {
			candidates = append(candidates, symItem)
		}
	}

	// 3. Fetch Phase 4 retrieval evidence if retriever is available
	if c.retriever != nil {
		retrievedItems, err := c.retriever.Retrieve(ctx, scope, req.Question, "EXPLANATION", 20)
		if err == nil && len(retrievedItems) > 0 {
			candidates = append(candidates, retrievedItems...)
		}
	}

	// 4. Enforce strict repository scope on all collected evidence candidates
	validCandidates := make([]*models.EvidenceItem, 0, len(candidates))
	for _, item := range candidates {
		if item == nil {
			continue
		}
		if err := scope.ValidateItem(item); err == nil {
			validCandidates = append(validCandidates, item)
		}
	}

	// 5. Package evidence using EvidencePackager (budget enforcement, ranking, sufficiency evaluation)
	budget := req.Budget
	if budget.MaxTokens <= 0 {
		budget = models.DefaultEvidenceBudget()
	}

	pkg, err := c.packager.BuildPackage(ctx, scope, req.Question, "EXPLANATION", validCandidates, budget)
	if err != nil {
		return nil, fmt.Errorf("failed to compose evidence package: %w", err)
	}

	return pkg, nil
}

func (c *DefaultEvidenceComposer) fetchTargetFileEvidence(ctx context.Context, scope models.RepositoryScope, targetFile string) []*models.EvidenceItem {
	if c.store == nil || targetFile == "" {
		return nil
	}
	manifest, err := c.store.GetManifestForRepository(ctx, scope.RepositoryID)
	if err != nil || len(manifest) == 0 {
		return nil
	}

	repoLocalPath := ""
	func() {
		defer func() { _ = recover() }()
		if repo, err := c.store.GetRepository(ctx, scope.RepositoryID); err == nil && repo != nil {
			repoLocalPath = repo.LocalPath
		}
	}()

	targetLower := strings.ToLower(targetFile)
	targetBase := filepath.Base(targetLower)

	var items []*models.EvidenceItem
	var matchedFile *models.FileManifestItem

	for _, item := range manifest {
		relLower := strings.ToLower(item.RelativePath)
		baseLower := filepath.Base(relLower)
		if relLower == targetLower || baseLower == targetBase || strings.HasSuffix(relLower, "/"+targetLower) {
			matchedFile = item
			break
		}
	}

	if matchedFile != nil {
		content := readEvidenceFileSnippet(repoLocalPath, matchedFile.RelativePath, 4096)
		if content == "" {
			content = fmt.Sprintf("Source file: %s (%s)", matchedFile.RelativePath, matchedFile.Language)
		}
		evidenceType := models.EvidenceTypeCodeSnippet
		ext := strings.ToLower(filepath.Ext(matchedFile.RelativePath))
		if ext == ".json" || ext == ".yml" || ext == ".yaml" || ext == ".toml" || ext == ".js" && strings.Contains(matchedFile.RelativePath, "config") {
			evidenceType = models.EvidenceTypeDependency
		}
		evItem := &models.EvidenceItem{
			RepositoryID:  scope.RepositoryID,
			Type:          evidenceType,
			FileID:        matchedFile.ID,
			RelativePath:  matchedFile.RelativePath,
			Content:       content,
			RetrieverType: "TARGET_FILE",
			RRFScore:      150.0,
			Rank:          1,
		}
		evItem.StableID = evItem.ComputeStableID()
		items = append(items, evItem)

		// Also fetch symbols defined in the target file
		symbols, symErr := c.store.GetSymbolsForRepository(ctx, scope.RepositoryID)
		if symErr == nil {
			for _, sym := range symbols {
				if strings.EqualFold(sym.RelativePath, matchedFile.RelativePath) || strings.EqualFold(filepath.Base(sym.RelativePath), targetBase) {
					symItem := &models.EvidenceItem{
						RepositoryID:  scope.RepositoryID,
						Type:          models.EvidenceTypeSymbol,
						FileID:        sym.FileID,
						RelativePath:  sym.RelativePath,
						Location:      sym.Location,
						Content:       fmt.Sprintf("Symbol %s (%s) defined in %s", sym.Name, sym.Kind, sym.RelativePath),
						RetrieverType: "TARGET_FILE_SYMBOL",
						RRFScore:      140.0,
						Rank:          2,
						Metadata: map[string]string{
							"symbol_id":   sym.ID,
							"symbol_name": sym.Name,
						},
					}
					symItem.StableID = symItem.ComputeStableID()
					items = append(items, symItem)
				}
			}
		}
	}

	return items
}

func (c *DefaultEvidenceComposer) assembleSystemContextPackage(ctx context.Context, scope models.RepositoryScope) []*models.EvidenceItem {
	if c.store == nil {
		return nil
	}

	manifest, err := c.store.GetManifestForRepository(ctx, scope.RepositoryID)
	if err != nil || len(manifest) == 0 {
		return nil
	}

	repoLocalPath := ""
	func() {
		defer func() { _ = recover() }()
		if repo, err := c.store.GetRepository(ctx, scope.RepositoryID); err == nil && repo != nil {
			repoLocalPath = repo.LocalPath
		}
	}()

	var items []*models.EvidenceItem
	seenDirs := make(map[string]int)
	var dirList []string

	for _, item := range manifest {
		relPath := item.RelativePath
		lowerPath := strings.ToLower(relPath)
		baseName := strings.ToLower(filepath.Base(relPath))

		// Build 2-level directory tree counts
		parts := strings.Split(relPath, "/")
		if len(parts) > 1 {
			topDir := parts[0] + "/"
			seenDirs[topDir]++
			if len(parts) > 2 && (parts[0] == "src" || parts[0] == "pkg" || parts[0] == "internal" || parts[0] == "api" || parts[0] == "cmd" || parts[0] == "apps" || parts[0] == "packages") {
				subDir := parts[0] + "/" + parts[1] + "/"
				seenDirs[subDir]++
			}
		}

		// 1. README & Architecture Documentation
		isArchivedDoc := strings.HasPrefix(lowerPath, "docs/archive/") || strings.HasPrefix(lowerPath, "docs/todos/") ||
			strings.HasPrefix(lowerPath, "docs/plans/") || strings.HasPrefix(lowerPath, "docs/solutions/") ||
			strings.HasPrefix(lowerPath, ".agent/") || strings.HasPrefix(lowerPath, ".agents/") || strings.HasPrefix(lowerPath, ".gsd/")

		if (baseName == "readme.md" || baseName == "architecture.md" || baseName == "contributing.md" || (strings.HasPrefix(lowerPath, "docs/") && strings.HasSuffix(lowerPath, ".md"))) && !isArchivedDoc {
			content := readEvidenceFileSnippet(repoLocalPath, item.RelativePath, 4096)
			if content == "" {
				content = fmt.Sprintf("Documentation file: %s", item.RelativePath)
			}
			score := 100.0
			if baseName == "readme.md" || baseName == "architecture.md" {
				score = 150.0
			}
			evItem := &models.EvidenceItem{
				RepositoryID:  scope.RepositoryID,
				Type:          models.EvidenceTypeDocumentation,
				FileID:        item.ID,
				RelativePath:  item.RelativePath,
				Content:       content,
				RetrieverType: "SYSTEM_CONTEXT_README",
				RRFScore:      score,
				Rank:          1,
			}
			evItem.StableID = evItem.ComputeStableID()
			items = append(items, evItem)
		}

		// 2. Setup / Manifests / Configuration files (Language Agnostic)
		if baseName == "package.json" || baseName == "go.mod" || baseName == "cargo.toml" || baseName == "pyproject.toml" ||
			baseName == "requirements.txt" || baseName == "setup.py" || baseName == "setup.cfg" || baseName == "pipfile" ||
			baseName == "pom.xml" || baseName == "build.gradle" || baseName == "composer.json" || baseName == "gemfile" ||
			baseName == "docker-compose.yml" || baseName == "dockerfile" || baseName == "vite.config.ts" || baseName == "next.config.js" ||
			baseName == "tsconfig.json" || baseName == "wrangler.json" || baseName == "makefile" || baseName == "cmakelists.txt" {
			content := readEvidenceFileSnippet(repoLocalPath, item.RelativePath, 2048)
			if content == "" {
				content = fmt.Sprintf("Dependency / Configuration setup file: %s", item.RelativePath)
			}
			evItem := &models.EvidenceItem{
				RepositoryID:  scope.RepositoryID,
				Type:          models.EvidenceTypeCodeSnippet,
				FileID:        item.ID,
				RelativePath:  item.RelativePath,
				Content:       content,
				RetrieverType: "SYSTEM_CONTEXT_SETUP",
				RRFScore:      135.0,
				Rank:          2,
			}
			evItem.StableID = evItem.ComputeStableID()
			items = append(items, evItem)
		}

		// 3. Primary Entrypoints & Core App/Library files (Language Agnostic: Go, Python, Rust, TS/JS, Java, C/C++)
		if baseName == "main.go" || baseName == "gin.go" || baseName == "main.py" || baseName == "app.py" || baseName == "run.py" ||
			baseName == "__main__.py" || baseName == "asgi.py" || baseName == "wsgi.py" || baseName == "main.rs" || baseName == "lib.rs" ||
			baseName == "index.ts" || baseName == "main.ts" || baseName == "app.tsx" || baseName == "main.tsx" ||
			baseName == "index.tsx" || baseName == "index.js" || baseName == "server.js" || baseName == "app.js" ||
			baseName == "server.ts" || baseName == "page.tsx" || baseName == "main.c" || baseName == "main.cpp" ||
			strings.HasPrefix(lowerPath, "cmd/") || strings.HasPrefix(lowerPath, "api/") || strings.HasPrefix(lowerPath, "src/main") || strings.HasPrefix(lowerPath, "src/lib") {
			content := readEvidenceFileSnippet(repoLocalPath, item.RelativePath, 2048)
			if content == "" {
				content = fmt.Sprintf("Primary application entrypoint component: %s", item.RelativePath)
			}
			evItem := &models.EvidenceItem{
				RepositoryID:  scope.RepositoryID,
				Type:          models.EvidenceTypeCodeSnippet,
				FileID:        item.ID,
				RelativePath:  item.RelativePath,
				Content:       content,
				RetrieverType: "SYSTEM_CONTEXT_ENTRYPOINT",
				RRFScore:      130.0,
				Rank:          3,
			}
			evItem.StableID = evItem.ComputeStableID()
			items = append(items, evItem)
		}
	}

	// Format directory tree breakdown
	for dirPath, count := range seenDirs {
		dirList = append(dirList, fmt.Sprintf("%s (%d files)", dirPath, count))
	}
	if len(dirList) > 0 {
		treeContent := fmt.Sprintf("Repository Directory Structure (Total %d discovered files):\n%s", len(manifest), strings.Join(dirList, "\n"))
		treeItem := &models.EvidenceItem{
			RepositoryID:  scope.RepositoryID,
			Type:          models.EvidenceTypeDocumentation,
			RelativePath:  "workspace_tree",
			Content:       treeContent,
			RetrieverType: "SYSTEM_CONTEXT_TREE",
			RRFScore:      125.0,
			Rank:          4,
		}
		treeItem.StableID = treeItem.ComputeStableID()
		items = append(items, treeItem)
	}

	// Fetch top representative symbols (up to 15 major functions/classes/structs)
	symbols, symErr := c.store.GetSymbolsForRepository(ctx, scope.RepositoryID)
	if symErr == nil && len(symbols) > 0 {
		added := 0
		for _, sym := range symbols {
			if sym.Kind == models.SymbolKindFunction || sym.Kind == models.SymbolKindClass || sym.Kind == models.SymbolKindStruct || sym.Kind == models.SymbolKindInterface {
				symItem := &models.EvidenceItem{
					RepositoryID:  scope.RepositoryID,
					Type:          models.EvidenceTypeSymbol,
					FileID:        sym.FileID,
					RelativePath:  sym.RelativePath,
					Location:      sym.Location,
					Content:       fmt.Sprintf("Representative symbol: %s %s in %s", sym.Kind, sym.Name, sym.RelativePath),
					RetrieverType: "SYSTEM_CONTEXT_SYMBOL",
					RRFScore:      120.0,
					Rank:          5,
					Metadata: map[string]string{
						"symbol_id":   sym.ID,
						"symbol_name": sym.Name,
					},
				}
				symItem.StableID = symItem.ComputeStableID()
				items = append(items, symItem)
				added++
				if added >= 15 {
					break
				}
			}
		}
	}

	return items
}

func readEvidenceFileSnippet(repoLocalPath, relPath string, maxBytes int) string {
	if repoLocalPath == "" || relPath == "" {
		return ""
	}
	fullPath := filepath.Join(repoLocalPath, filepath.FromSlash(relPath))
	data, err := os.ReadFile(fullPath)
	if err != nil || len(data) == 0 {
		return ""
	}
	if len(data) > maxBytes {
		data = data[:maxBytes]
	}
	return strings.TrimSpace(string(data))
}

func (c *DefaultEvidenceComposer) convertStaticFlowToEvidence(scope models.RepositoryScope, flow *models.StaticFlowResult) []*models.EvidenceItem {
	if flow == nil || flow.Path == nil {
		return nil
	}

	var items []*models.EvidenceItem
	pathID := flow.Path.PathID
	if pathID == "" {
		pathID = fmt.Sprintf("flow-%s-%s", flow.RootNodeID, flow.TargetNodeID)
	}

	for idx, step := range flow.Path.Steps {
		if step == nil || step.Node == nil {
			continue
		}
		node := step.Node
		seq := step.Sequence
		if seq <= 0 {
			seq = idx + 1
		}

		contentBuilder := fmt.Sprintf("Flow Step %d: Symbol '%s' (%s)", seq, node.Label, node.QualifiedName)
		if step.OutgoingEdge != nil {
			contentBuilder += fmt.Sprintf(" calls target node ID '%s'", step.OutgoingEdge.TargetID)
		}

		item := &models.EvidenceItem{
			RepositoryID:  scope.RepositoryID,
			Type:          models.EvidenceTypeStaticFlow,
			FileID:        node.FileID,
			RelativePath:  node.RelativePath,
			Location:      node.Location,
			Content:       contentBuilder,
			RetrieverType: "STATIC_FLOW",
			RRFScore:      100.0 - float64(seq)*0.1, // Priority ordering for flow sequence
			Rank:          seq,
			Metadata: map[string]string{
				"symbol_id":          node.ID,
				"symbol_name":        node.Label,
				"flow_step_sequence": strconv.Itoa(seq),
				"flow_path_id":       pathID,
			},
		}

		if step.OutgoingEdge != nil {
			item.Metadata["relationship_id"] = step.OutgoingEdge.ID
			item.Metadata["edge_kind"] = string(step.OutgoingEdge.Kind)
		}

		item.StableID = item.ComputeStableID()
		items = append(items, item)
	}

	return items
}

func (c *DefaultEvidenceComposer) fetchSymbolEvidence(ctx context.Context, scope models.RepositoryScope, symbolID string) *models.EvidenceItem {
	syms, err := c.store.GetSymbolsForRepository(ctx, scope.RepositoryID)
	if err != nil || len(syms) == 0 {
		return nil
	}

	var targetSym *models.Symbol
	for _, s := range syms {
		if s.ID == symbolID {
			targetSym = s
			break
		}
	}
	if targetSym == nil {
		return nil
	}

	item := &models.EvidenceItem{
		RepositoryID:  scope.RepositoryID,
		Type:          models.EvidenceTypeSymbol,
		FileID:        targetSym.FileID,
		RelativePath:  targetSym.RelativePath,
		Location:      targetSym.Location,
		Content:       fmt.Sprintf("Symbol %s (%s) defined in %s", targetSym.Name, targetSym.Kind, targetSym.RelativePath),
		RetrieverType: "SELECTED_SYMBOL",
		RRFScore:      105.0, // Top priority for explicitly selected target symbol
		Rank:          1,
		Metadata: map[string]string{
			"symbol_id":   targetSym.ID,
			"symbol_name": targetSym.Name,
		},
	}
	item.StableID = item.ComputeStableID()
	return item
}

func (c *DefaultEvidenceComposer) assembleFlowContextPackage(ctx context.Context, scope models.RepositoryScope, lowerQ string) []*models.EvidenceItem {
	if c.store == nil {
		return nil
	}

	manifest, err := c.store.GetManifestForRepository(ctx, scope.RepositoryID)
	if err != nil || len(manifest) == 0 {
		return nil
	}

	repoLocalPath := ""
	func() {
		defer func() { _ = recover() }()
		if repo, err := c.store.GetRepository(ctx, scope.RepositoryID); err == nil && repo != nil {
			repoLocalPath = repo.LocalPath
		}
	}()

	var items []*models.EvidenceItem

	// 1. Identify Flow/Router/Handler/Controller/API Files in Manifest
	var flowFiles []*models.FileManifestItem
	for _, item := range manifest {
		relLower := strings.ToLower(item.RelativePath)
		baseLower := filepath.Base(relLower)

		if baseLower == "gin.go" || baseLower == "applications.py" || baseLower == "routing.py" ||
			baseLower == "route.ts" || baseLower == "route.js" || baseLower == "routergroup.go" ||
			baseLower == "context.go" || baseLower == "server.ts" || baseLower == "server.js" ||
			baseLower == "app.py" || baseLower == "main.py" || baseLower == "main.go" || baseLower == "main.rs" ||
			baseLower == "asgi.py" || baseLower == "wsgi.py" || baseLower == "response_writer.go" ||
			strings.Contains(relLower, "/api/") || strings.Contains(relLower, "/routes/") ||
			strings.Contains(relLower, "/routing/") || strings.Contains(relLower, "/controllers/") ||
			strings.Contains(relLower, "/handlers/") || strings.Contains(relLower, "/endpoints/") ||
			strings.Contains(relLower, "/services/") || strings.Contains(relLower, "/workers/") {
			flowFiles = append(flowFiles, item)
		}
	}

	// 2. Extract Graph Nodes & Edges to build multi-symbol flow trace
	nodes, edges, graphErr := c.store.GetGraphForRepository(ctx, scope.RepositoryID)
	if graphErr == nil && len(nodes) > 0 {
		nodeMap := make(map[string]*models.Node)
		for _, n := range nodes {
			nodeMap[n.ID] = n
		}

		for _, edge := range edges {
			srcNode := nodeMap[edge.SourceID]
			tgtNode := nodeMap[edge.TargetID]

			if srcNode != nil && tgtNode != nil {
				srcPath := strings.ToLower(srcNode.RelativePath)
				tgtPath := strings.ToLower(tgtNode.RelativePath)

				isSrcFlow := isFlowFileOrSymbol(srcPath, srcNode.Label, srcNode.Kind)
				isTgtFlow := isFlowFileOrSymbol(tgtPath, tgtNode.Label, tgtNode.Kind)

				if isSrcFlow || isTgtFlow {
					content := fmt.Sprintf("Request Flow Step: %s [%s] %s -> %s [%s]",
						srcNode.Label, srcNode.RelativePath, edge.Kind, tgtNode.Label, tgtNode.RelativePath)

					evItem := &models.EvidenceItem{
						RepositoryID:  scope.RepositoryID,
						Type:          models.EvidenceTypeGraphEdge,
						FileID:        edge.FileID,
						RelativePath:  srcNode.RelativePath,
						Location:      edge.Location,
						Content:       content,
						RetrieverType: "FLOW_CONTEXT",
						RRFScore:      145.0,
						Rank:          1,
						Metadata: map[string]string{
							"source_name": srcNode.Label,
							"target_name": tgtNode.Label,
							"edge_kind":   string(edge.Kind),
						},
					}
					evItem.StableID = evItem.ComputeStableID()
					items = append(items, evItem)
				}
			}
		}
	}

	// 3. For top flow files (up to 5), include concise code snippets
	addedSnippets := 0
	for _, f := range flowFiles {
		content := readEvidenceFileSnippet(repoLocalPath, f.RelativePath, 2048)
		if content != "" {
			evItem := &models.EvidenceItem{
				RepositoryID:  scope.RepositoryID,
				Type:          models.EvidenceTypeCodeSnippet,
				FileID:        f.ID,
				RelativePath:  f.RelativePath,
				Content:       content,
				RetrieverType: "FLOW_CONTEXT",
				RRFScore:      140.0,
				Rank:          2,
			}
			evItem.StableID = evItem.ComputeStableID()
			items = append(items, evItem)
			addedSnippets++
			if addedSnippets >= 5 {
				break
			}
		}
	}

	return items
}

func isFlowFileOrSymbol(relPath string, label string, kind models.NodeKind) bool {
	lowerPath := strings.ToLower(relPath)
	lowerLabel := strings.ToLower(label)

	if strings.Contains(lowerPath, "route") || strings.Contains(lowerPath, "router") ||
		strings.Contains(lowerPath, "handler") || strings.Contains(lowerPath, "controller") ||
		strings.Contains(lowerPath, "api") || strings.Contains(lowerPath, "server") ||
		strings.Contains(lowerPath, "worker") || strings.Contains(lowerPath, "endpoint") ||
		strings.Contains(lowerPath, "service") || strings.Contains(lowerPath, "app.") ||
		strings.Contains(lowerPath, "main.") || strings.Contains(lowerPath, "gin.") {
		return true
	}

	if strings.Contains(lowerLabel, "serve") || strings.Contains(lowerLabel, "handle") ||
		strings.Contains(lowerLabel, "route") || strings.Contains(lowerLabel, "request") ||
		strings.Contains(lowerLabel, "get") || strings.Contains(lowerLabel, "post") ||
		strings.Contains(lowerLabel, "call") || strings.Contains(lowerLabel, "process") {
		return true
	}

	return false
}

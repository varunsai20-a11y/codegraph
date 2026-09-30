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

	if intent == IntentRepoOverview || intent == IntentArchitectureQuery || intent == IntentTrace || isStartupQuery(lowerQ) || isExecutionFlowQuery(lowerQ) || req.Question == "explain the code" {
		overviewItems := c.assembleSystemContextPackage(ctx, scope)
		candidates = append(candidates, overviewItems...)
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
	var dirList []string
	seenDirs := make(map[string]bool)

	for _, item := range manifest {
		relPath := item.RelativePath
		lowerPath := strings.ToLower(relPath)

		parts := strings.Split(relPath, "/")
		if len(parts) > 1 {
			topDir := parts[0]
			if !seenDirs[topDir] {
				seenDirs[topDir] = true
				dirList = append(dirList, topDir+"/")
			}
		}

		// 1. README.md
		if lowerPath == "readme.md" || strings.HasSuffix(lowerPath, "/readme.md") {
			content := readEvidenceFileSnippet(repoLocalPath, item.RelativePath, 4096)
			if content == "" {
				content = fmt.Sprintf("README Documentation in %s", item.RelativePath)
			}
			evItem := &models.EvidenceItem{
				RepositoryID:  scope.RepositoryID,
				Type:          models.EvidenceTypeDocumentation,
				FileID:        item.ID,
				RelativePath:  item.RelativePath,
				Content:       content,
				RetrieverType: "SYSTEM_CONTEXT_README",
				RRFScore:      120.0,
				Rank:          1,
			}
			evItem.StableID = evItem.ComputeStableID()
			items = append(items, evItem)
		}

		// 2. Setup/Dependency files
		if lowerPath == "requirements.txt" || lowerPath == "package.json" || lowerPath == "go.mod" || lowerPath == "pom.xml" || lowerPath == "cargo.toml" || lowerPath == "pyproject.toml" {
			content := readEvidenceFileSnippet(repoLocalPath, item.RelativePath, 2048)
			if content == "" {
				content = fmt.Sprintf("Dependency / Environment setup file: %s", item.RelativePath)
			}
			evItem := &models.EvidenceItem{
				RepositoryID:  scope.RepositoryID,
				Type:          models.EvidenceTypeCodeSnippet,
				FileID:        item.ID,
				RelativePath:  item.RelativePath,
				Content:       content,
				RetrieverType: "SYSTEM_CONTEXT_SETUP",
				RRFScore:      115.0,
				Rank:          2,
			}
			evItem.StableID = evItem.ComputeStableID()
			items = append(items, evItem)
		}

		// 3. Entrypoint files
		if lowerPath == "run.py" || lowerPath == "main.py" || lowerPath == "app.py" || lowerPath == "main.go" || lowerPath == "index.ts" || lowerPath == "server.js" || lowerPath == "index.js" || strings.HasPrefix(lowerPath, "cmd/") {
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
				RRFScore:      110.0,
				Rank:          3,
			}
			evItem.StableID = evItem.ComputeStableID()
			items = append(items, evItem)
		}
	}

	// 4. Top-level Directory Tree summary item
	if len(dirList) > 0 {
		treeContent := fmt.Sprintf("Top-level Repository Directory Tree: %s", strings.Join(dirList, ", "))
		treeItem := &models.EvidenceItem{
			RepositoryID:  scope.RepositoryID,
			Type:          models.EvidenceTypeDocumentation,
			RelativePath:  "workspace_tree",
			Content:       treeContent,
			RetrieverType: "SYSTEM_CONTEXT_TREE",
			RRFScore:      105.0,
			Rank:          4,
		}
		treeItem.StableID = treeItem.ComputeStableID()
		items = append(items, treeItem)
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

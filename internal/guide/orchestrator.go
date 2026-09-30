package guide

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codegraph/internal/graph"
	"codegraph/internal/llm"
	"codegraph/internal/models"
	"codegraph/internal/retrieval"
	"codegraph/internal/storage"
)

// GuideOrchestrator leads users through a bounded, deterministic reverse-engineering investigation tour.
//
// TRUST MODEL & PROGRESSION CONTRACT (Option A - Stateless Client UI State):
//  1. The guide endpoint operates statelessly.
//  2. Client-submitted `CompletedStepIDs` and `CurrentStepIndex` are treated as client-provided UI navigation state,
//     NOT as authoritative or cryptographic server-enforced proof of completed user work.
//  3. The server safely normalizes client-provided UI state against the authoritative, deterministically generated
//     step sequence for the repository:
//     - Unknown, duplicate, or out-of-order step IDs are safely filtered out/normalized.
//     - Cross-repository state is strictly rejected via RepositoryScope validation.
//     - Maximum investigation steps are strictly bounded to DefaultMaxInvestigationSteps.
//  4. All underlying evidence, architecture facts, and static flow paths originate deterministically from AST graph storage.
type GuideOrchestrator interface {
	Guide(ctx context.Context, req *models.InvestigationRequest) (*models.Investigation, error)
}

type DefaultGuideOrchestrator struct {
	store          storage.Storage
	summaryGen     ArchitectureSummaryGenerator
	composer       retrieval.EvidenceComposer
	explanationSvc llm.ExplanationService
}

func NewDefaultGuideOrchestrator(
	store storage.Storage,
	summaryGen ArchitectureSummaryGenerator,
	composer retrieval.EvidenceComposer,
	explanationSvc llm.ExplanationService,
) *DefaultGuideOrchestrator {
	if summaryGen == nil {
		summaryGen = NewDefaultArchitectureSummaryGenerator(store, nil)
	}
	if composer == nil {
		composer = retrieval.NewDefaultEvidenceComposer(nil, nil, store)
	}
	if explanationSvc == nil {
		explanationSvc = llm.NewGroundedExplanationService(nil, nil, composer, nil)
	}
	return &DefaultGuideOrchestrator{
		store:          store,
		summaryGen:     summaryGen,
		composer:       composer,
		explanationSvc: explanationSvc,
	}
}

func (o *DefaultGuideOrchestrator) Guide(ctx context.Context, req *models.InvestigationRequest) (*models.Investigation, error) {
	if req == nil {
		return nil, fmt.Errorf("investigation request cannot be nil")
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}

	scope := req.RepositoryScope

	// 1. Generate deterministic repository architecture summary
	summary, err := o.summaryGen.GenerateSummary(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to generate architecture summary: %w", err)
	}

	// 2. Initialize or restore Investigation state
	inv, err := models.NewInvestigation(scope, summary)
	if err != nil {
		return nil, err
	}

	// 3. Construct deterministic step sequence
	steps := o.buildDeterministicSteps(ctx, scope, summary, req)
	inv.Steps = steps

	// 4. Handle Progression based on req.Action
	maxSteps := req.MaxSteps
	if maxSteps <= 0 || maxSteps > models.DefaultMaxInvestigationSteps {
		maxSteps = models.DefaultMaxInvestigationSteps
	}
	if len(inv.Steps) > maxSteps {
		inv.Steps = inv.Steps[:maxSteps]
	}

	action := strings.ToUpper(strings.TrimSpace(req.Action))
	switch action {
	case "START", "RESET":
		inv.CurrentStepIndex = 0
		inv.CompletedStepIDs = make([]string, 0)
		for idx, st := range inv.Steps {
			if idx == 0 {
				st.Status = models.StepActive
			} else {
				st.Status = models.StepPending
			}
		}

	case "NEXT":
		// Validate client-submitted completed IDs against authoritative valid step sequence.
		// Strict Rule: A step i (i > 0) can only be marked completed if preceding step i-1 is also completed.
		// Forged or out-of-order IDs are ignored.
		validCompleted := make(map[string]bool)
		for i, st := range inv.Steps {
			submitted := false
			for _, cid := range req.CompletedStepIDs {
				if cid == st.ID {
					submitted = true
					break
				}
			}
			if i == 0 {
				if submitted {
					validCompleted[st.ID] = true
				}
			} else {
				if submitted && validCompleted[inv.Steps[i-1].ID] {
					validCompleted[st.ID] = true
				}
			}
		}

		// Always mark the current step completed when transitioning forward
		if req.CurrentStepIndex >= 0 && req.CurrentStepIndex < len(inv.Steps) {
			currID := inv.Steps[req.CurrentStepIndex].ID
			if req.CurrentStepIndex == 0 || validCompleted[inv.Steps[req.CurrentStepIndex-1].ID] {
				validCompleted[currID] = true
			}
		} else if len(inv.Steps) > 0 {
			validCompleted[inv.Steps[0].ID] = true
		}

		inv.CompletedStepIDs = make([]string, 0, len(validCompleted))
		nextIdx := 0
		for idx, st := range inv.Steps {
			if validCompleted[st.ID] {
				st.Status = models.StepCompleted
				inv.CompletedStepIDs = append(inv.CompletedStepIDs, st.ID)
				nextIdx = idx + 1
			} else {
				st.Status = models.StepPending
			}
		}

		if nextIdx < len(inv.Steps) {
			inv.CurrentStepIndex = nextIdx
			inv.Steps[nextIdx].Status = models.StepActive
		} else {
			inv.CurrentStepIndex = len(inv.Steps) - 1
			inv.Status = models.InvestigationCompleted
			inv.IsComplete = true
		}

	default:
		inv.CurrentStepIndex = 0
		if len(inv.Steps) > 0 {
			inv.Steps[0].Status = models.StepActive
		}
	}

	if inv.CurrentStepIndex >= len(inv.Steps)-1 && len(inv.Steps) > 0 {
		if inv.Steps[len(inv.Steps)-1].Status == models.StepCompleted {
			inv.Status = models.InvestigationCompleted
			inv.IsComplete = true
		} else {
			inv.Status = models.InvestigationInProgress
		}
	} else {
		inv.Status = models.InvestigationInProgress
	}

	inv.UpdatedAt = time.Now()
	return inv, nil
}

func (o *DefaultGuideOrchestrator) buildDeterministicSteps(
	ctx context.Context,
	scope models.RepositoryScope,
	summary models.ArchitectureSummary,
	req *models.InvestigationRequest,
) []*models.InvestigationStep {
	var steps []*models.InvestigationStep

	entrySymName := "N/A"
	var entrySym *models.Symbol
	if len(summary.EntryPointCandidates) > 0 && summary.EntryPointCandidates[0] != nil {
		entrySym = summary.EntryPointCandidates[0]
		entrySymName = fmt.Sprintf("%s:%s", entrySym.RelativePath, entrySym.Name)
	}

	highConnName := "N/A"
	var highConnNode *models.Node
	if len(summary.HighConnectivitySymbols) > 0 && summary.HighConnectivitySymbols[0] != nil {
		highConnNode = summary.HighConnectivitySymbols[0]
		highConnName = fmt.Sprintf("%s (%s)", highConnNode.Label, highConnNode.RelativePath)
	}

	topModDirs := make([]string, 0)
	for _, m := range summary.TopModules {
		topModDirs = append(topModDirs, m.Directory)
	}
	topModSummaryStr := strings.Join(topModDirs, ", ")

	// Step 1: System Purpose & Scope
	step1 := &models.InvestigationStep{
		ID:          "step-1-overview",
		Sequence:    1,
		Type:        models.StepOverview,
		StepType:    models.StepOverview,
		Title:       "1. System Purpose & Architecture Scope",
		Description: fmt.Sprintf("Repository '%s' comprises %d source files, %d symbols, and %d AST relationships across %d primary modules (%s). Execution is driven by primary entrypoint '%s'. External boundaries & services include: %s.", scope.RepositoryID, summary.TotalFiles, summary.TotalSymbols, summary.TotalRelationships, len(summary.TopModules), topModSummaryStr, entrySymName, strings.Join(summary.ExternalBoundaries, ", ")),
		SuggestedQuestions: []string{
			"What is the overall architectural purpose of this repository?",
			"What external services or frameworks does this system depend on?",
			"Which entrypoint files drive execution?",
		},
		Status: models.StepPending,
	}
	if entrySym != nil {
		step1.FileID = entrySym.FileID
		step1.RelativePath = entrySym.RelativePath
		step1.TargetFile = entrySym.RelativePath
		step1.SymbolID = entrySym.ID
		step1.TargetSymbol = entrySym.ID
		step1.SymbolName = entrySym.Name
		step1.TargetNode = entrySym.Name
	}
	steps = append(steps, step1)

	// Step 2: Major Modules & Responsibilities
	targetDir := "root"
	if len(summary.TopModules) > 0 {
		targetDir = summary.TopModules[0].Directory
	}

	step2 := &models.InvestigationStep{
		ID:          "step-2-module-structure",
		Sequence:    2,
		Type:        models.StepModuleStructure,
		StepType:    models.StepModuleStructure,
		Title:       fmt.Sprintf("2. Major Modules & Responsibilities (%s)", targetDir),
		Description: fmt.Sprintf("Building on the system overview, execution flow partitions responsibilities across primary modules [%s]. Control flow enters at '%s' (symbol '%s') and delegates domain logic to core module '%s'.", topModSummaryStr, entrySymName, getSymbolName(entrySym), targetDir),
		SuggestedQuestions: []string{
			fmt.Sprintf("What are the primary responsibilities of module '%s'?", targetDir),
			"How does control move from the entrypoint to internal modules?",
			"What dependencies exist between major modules?",
		},
		Status: models.StepPending,
	}
	if entrySym != nil {
		step2.FileID = entrySym.FileID
		step2.RelativePath = entrySym.RelativePath
		step2.TargetFile = entrySym.RelativePath
		step2.SymbolID = entrySym.ID
		step2.TargetSymbol = entrySym.ID
		step2.SymbolName = entrySym.Name
		step2.TargetNode = entrySym.Name
	}
	steps = append(steps, step2)

	// Step 3: Key Execution Components & Important Symbols
	step3 := &models.InvestigationStep{
		ID:          "step-3-important-symbols",
		Sequence:    3,
		Type:        models.StepImportantSymbols,
		StepType:    models.StepImportantSymbols,
		Title:       "3. Important Execution Components",
		Description: fmt.Sprintf("Within key module '%s', multi-signal architectural scoring pinpoints primary driving component '%s'. Key orchestration symbols include: %s. These components manage task orchestration, domain execution, and tool delegation.", targetDir, highConnName, strings.Join(summary.HighConnectivitySymsStr, "; ")),
		SuggestedQuestions: []string{
			fmt.Sprintf("What is the role of '%s' in system execution?", getNodeName(highConnNode)),
			"Which functions or methods call this component?",
			"What child symbols or tools does this component depend on?",
		},
		Status: models.StepPending,
	}
	if highConnNode != nil {
		step3.FileID = highConnNode.FileID
		step3.RelativePath = highConnNode.RelativePath
		step3.TargetFile = highConnNode.RelativePath
		step3.SymbolID = highConnNode.ID
		step3.TargetSymbol = highConnNode.ID
		step3.SymbolName = highConnNode.Label
		step3.TargetNode = highConnNode.Label
	}
	steps = append(steps, step3)

	// Step 4: Static Execution Narrative & Call Flow (With Data Flow)
	step4 := &models.InvestigationStep{
		ID:       "step-4-static-flow",
		Sequence: 4,
		Type:     models.StepStaticFlow,
		StepType: models.StepStaticFlow,
		Title:    "4. Execution Narrative & Static Call Flow",
		SuggestedQuestions: []string{
			"Trace the complete static call path from entrypoint to tool execution.",
			"What data items move through this execution path?",
			"Where do terminal side effects or external service calls occur?",
		},
		Status: models.StepPending,
	}

	var flowSummaryStr string
	if entrySym != nil {
		step4.FileID = entrySym.FileID
		step4.RelativePath = entrySym.RelativePath
		step4.TargetFile = entrySym.RelativePath
		step4.SymbolID = entrySym.ID
		step4.TargetSymbol = entrySym.ID
		step4.SymbolName = entrySym.Name
		step4.TargetNode = entrySym.Name

		if o.store != nil {
			nodes, edges, err := o.store.GetGraphForRepository(ctx, scope.RepositoryID)
			if err == nil && len(nodes) > 0 {
				flowEngine := graph.NewEngine(scope.RepositoryID)
				flowEngine.LoadGraph(nodes, edges)

				targetID := ""
				if highConnNode != nil && highConnNode.RelativePath != entrySym.RelativePath {
					targetID = highConnNode.ID
				} else {
					for _, hcn := range summary.HighConnectivitySymbols {
						if hcn != nil && hcn.RelativePath != entrySym.RelativePath {
							targetID = hcn.ID
							break
						}
					}
				}
				step4.FlowResult = flowEngine.TraceStaticFlow(entrySym.ID, targetID, 10, 50)
			}
		}
	}

	callNarrative, dataFlowNarrative, pathSummary := generateDynamicFlowNarrative(step4.FlowResult, entrySym, highConnNode, scope.RepositoryID)
	step4.Description = fmt.Sprintf("%s %s [Statically Established: Call edges & module imports. Inferred: Parameter data handoff. Unavailable Runtime: Dynamic LLM outputs & un-analyzed runtime state].", callNarrative, dataFlowNarrative)
	flowSummaryStr = pathSummary
	steps = append(steps, step4)

	// Step 5: End-to-End Architectural Synthesis
	step5 := &models.InvestigationStep{
		ID:          "step-5-explanation",
		Sequence:    5,
		Type:        models.StepGroundedExplain,
		StepType:    models.StepGroundedExplain,
		Title:       "5. End-to-End Architectural Synthesis",
		Description: fmt.Sprintf("Architectural Synthesis: Repository '%s' is an AST-verified system structured into %d primary modules [%s]. System execution originates in entrypoint '%s', passes control through key component '%s', and follows verified call flow (%s). Data moves from entry parameters through module handoffs to external boundaries (%s).", scope.RepositoryID, len(summary.TopModules), topModSummaryStr, entrySymName, highConnName, flowSummaryStr, strings.Join(summary.ExternalBoundaries, ", ")),
		SuggestedQuestions: []string{
			"Summarize the end-to-end architectural design of this codebase.",
			"How do entrypoints, modules, and key symbols interact across file boundaries?",
			"What are the main security, API, or external integration boundaries?",
		},
		Status: models.StepPending,
	}
	if entrySym != nil {
		step5.FileID = entrySym.FileID
		step5.RelativePath = entrySym.RelativePath
		step5.TargetFile = entrySym.RelativePath
		step5.SymbolID = entrySym.ID
		step5.TargetSymbol = entrySym.ID
		step5.SymbolName = entrySym.Name
		step5.TargetNode = entrySym.Name
	}
	steps = append(steps, step5)

	// Attach evidence items to steps via EvidenceComposer
	for _, st := range steps {
		expReq, _ := models.NewExplanationRequest(scope, st.Title)
		expReq.SymbolID = st.SymbolID
		expReq.StaticFlow = st.FlowResult
		pkg, err := o.composer.Compose(ctx, expReq)
		if err == nil && pkg != nil {
			evList := make([]*models.ExplanationEvidence, 0, len(pkg.Items))
			for _, item := range pkg.Items {
				ev, convErr := models.NewExplanationEvidenceFromItem(scope, item)
				if convErr == nil {
					evList = append(evList, ev)
				}
			}
			st.Evidence = evList
		}
	}

	return steps
}

func getSymbolName(s *models.Symbol) string {
	if s == nil {
		return "N/A"
	}
	return s.Name
}

func getNodeName(n *models.Node) string {
	if n == nil {
		return "N/A"
	}
	return n.Label
}

func generateDynamicFlowNarrative(
	flow *models.StaticFlowResult,
	entrySym *models.Symbol,
	highConn *models.Node,
	repoID string,
) (callNarrative string, dataFlowNarrative string, pathSummary string) {
	entryStr := "entrypoint"
	if entrySym != nil {
		entryStr = fmt.Sprintf("%s:%s", entrySym.RelativePath, entrySym.Name)
	}

	highConnStr := "core component"
	if highConn != nil {
		highConnStr = fmt.Sprintf("%s (%s)", highConn.Label, highConn.RelativePath)
	}

	if flow != nil && flow.Path != nil && len(flow.Path.Steps) > 0 {
		var labels []string
		var dataLabels []string
		for _, step := range flow.Path.Steps {
			if step.Node != nil {
				lbl := step.Node.Label
				if step.Node.RelativePath != "" {
					lbl = fmt.Sprintf("%s:%s", step.Node.RelativePath, step.Node.Label)
				}
				labels = append(labels, fmt.Sprintf("`%s`", lbl))
				dataLabels = append(dataLabels, fmt.Sprintf("%s Handoff", step.Node.Label))
			}
		}

		pathSummary = strings.Join(labels, " → ")
		callNarrative = fmt.Sprintf("Execution Narrative: System execution originates in '%s', validating environment/parameters, then delegating control along static call path: %s.", entryStr, pathSummary)
		dataFlowNarrative = fmt.Sprintf("Static Data Flow: User Input / Config → %s → System Output / External Service.", strings.Join(dataLabels, " → "))
		return callNarrative, dataFlowNarrative, pathSummary
	}

	if flow != nil && len(flow.Nodes) > 0 {
		var labels []string
		var dataLabels []string
		count := 0
		for _, node := range flow.Nodes {
			if node != nil && node.Kind == models.NodeKindSymbol {
				lbl := node.Label
				if node.RelativePath != "" {
					lbl = fmt.Sprintf("%s:%s", node.RelativePath, node.Label)
				}
				labels = append(labels, fmt.Sprintf("`%s`", lbl))
				dataLabels = append(dataLabels, fmt.Sprintf("%s Data", node.Label))
				count++
				if count >= 4 {
					break
				}
			}
		}

		if len(labels) > 0 {
			pathSummary = strings.Join(labels, " → ")
			callNarrative = fmt.Sprintf("Execution Narrative: System execution starts in '%s' and delegates control to key symbols: %s.", entryStr, pathSummary)
			dataFlowNarrative = fmt.Sprintf("Static Data Flow: CLI / Request Input → %s → Processed Result.", strings.Join(dataLabels, " → "))
			return callNarrative, dataFlowNarrative, pathSummary
		}
	}

	pathSummary = fmt.Sprintf("`%s` → `%s`", entryStr, highConnStr)
	callNarrative = fmt.Sprintf("Execution Narrative: System execution initiates in entrypoint '%s' and directs control to primary architectural component '%s'.", entryStr, highConnStr)
	dataFlowNarrative = fmt.Sprintf("Static Data Flow: User Input / CLI → [%s] Parameters → [%s] Processing → Result Output.", entryStr, highConnStr)
	return callNarrative, dataFlowNarrative, pathSummary
}


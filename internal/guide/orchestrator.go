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

	// Step 1: Overview
	step1 := &models.InvestigationStep{
		ID:          "step-1-overview",
		Sequence:    1,
		Type:        models.StepOverview,
		Title:       "Repository Architectural Overview",
		Description: fmt.Sprintf("Repository '%s' contains %d files, %d symbols, and %d top directory modules.", scope.RepositoryID, summary.TotalFiles, summary.TotalSymbols, len(summary.TopModules)),
		SuggestedQuestions: []string{
			"How is this repository structured?",
			"What are the main modules in this project?",
			"Where are the likely entry points?",
		},
		Status: models.StepPending,
	}
	steps = append(steps, step1)

	// Step 2: Module Structure & Entry Points
	targetDir := "root"
	if len(summary.TopModules) > 0 {
		targetDir = summary.TopModules[0].Directory
	}
	var entrySym *models.Symbol
	if len(summary.EntryPointCandidates) > 0 {
		entrySym = summary.EntryPointCandidates[0]
	}

	step2 := &models.InvestigationStep{
		ID:          "step-2-module-structure",
		Sequence:    2,
		Type:        models.StepModuleStructure,
		Title:       fmt.Sprintf("Module Structure: %s", targetDir),
		Description: fmt.Sprintf("Primary concentration found in '%s'. Entry point candidate: '%s'.", targetDir, getSymbolName(entrySym)),
		SuggestedQuestions: []string{
			fmt.Sprintf("What does module '%s' do?", targetDir),
			"Which entry points initiate execution?",
			"What are the core dependencies of this module?",
		},
		Status: models.StepPending,
	}
	if entrySym != nil {
		step2.SymbolID = entrySym.ID
		step2.SymbolName = entrySym.Name
		step2.FileID = entrySym.FileID
		step2.RelativePath = entrySym.RelativePath
	}
	steps = append(steps, step2)

	// Step 3: Important Symbols & High Connectivity
	var highConnNode *models.Node
	if len(summary.HighConnectivitySymbols) > 0 {
		highConnNode = summary.HighConnectivitySymbols[0]
	}

	step3 := &models.InvestigationStep{
		ID:          "step-3-important-symbols",
		Sequence:    3,
		Type:        models.StepImportantSymbols,
		Title:       "High-Connectivity Symbol Analysis",
		Description: fmt.Sprintf("Highest degree centrality symbol: '%s'.", getNodeName(highConnNode)),
		SuggestedQuestions: []string{
			"What does this function do?",
			"Who calls this function?",
			"What does this function call?",
		},
		Status: models.StepPending,
	}
	if highConnNode != nil {
		step3.SymbolID = highConnNode.ID
		step3.SymbolName = highConnNode.Label
		step3.FileID = highConnNode.FileID
		step3.RelativePath = highConnNode.RelativePath
	}
	steps = append(steps, step3)

	// Step 4: Static Call Flow
	step4 := &models.InvestigationStep{
		ID:          "step-4-static-flow",
		Sequence:    4,
		Type:        models.StepStaticFlow,
		Title:       "Static Call Flow Traversal",
		Description: "Statically inferred call graph relationship derived from CodeGraph static analysis.",
		SuggestedQuestions: []string{
			"Explain this call path.",
			"Where does static execution flow lead?",
			"Where does this flow terminate?",
		},
		Status: models.StepPending,
	}
	if entrySym != nil && highConnNode != nil {
		step4.SymbolID = entrySym.ID
		step4.SymbolName = entrySym.Name
		step4.RelativePath = entrySym.RelativePath
		// Attach static flow trace if engine is available
		flowEngine := graph.NewEngine(scope.RepositoryID)
		flowRes := flowEngine.TraceStaticFlow(entrySym.ID, highConnNode.ID, 10, 50)
		step4.FlowResult = flowRes
	}
	steps = append(steps, step4)

	// Step 5: Grounded AI Explanation
	step5 := &models.InvestigationStep{
		ID:          "step-5-explanation",
		Sequence:    5,
		Type:        models.StepGroundedExplain,
		Title:       "Grounded Architectural Synthesis",
		Description: "Evidence-backed AI summary interpreting repository architecture and flow facts.",
		SuggestedQuestions: []string{
			"Explain the purpose of this codebase.",
			"Summarize key architectural dependencies.",
			"What are potential security or API boundaries?",
		},
		Status: models.StepPending,
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

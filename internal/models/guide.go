package models

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidStepSequence = errors.New("invalid investigation step sequence")
	ErrNilStep             = errors.New("investigation step cannot be nil")
)

type InvestigationStepType string

const (
	StepOverview         InvestigationStepType = "STEP_OVERVIEW"
	StepModuleStructure  InvestigationStepType = "STEP_MODULE_STRUCTURE"
	StepImportantSymbols InvestigationStepType = "STEP_IMPORTANT_SYMBOLS"
	StepDependencyGraph  InvestigationStepType = "STEP_DEPENDENCY_GRAPH"
	StepStaticFlow       InvestigationStepType = "STEP_STATIC_FLOW"
	StepGroundedExplain  InvestigationStepType = "STEP_EXPLANATION"
)

type InvestigationStepStatus string

const (
	StepPending   InvestigationStepStatus = "STEP_PENDING"
	StepActive    InvestigationStepStatus = "STEP_ACTIVE"
	StepCompleted InvestigationStepStatus = "STEP_COMPLETED"
)

type ModuleSummary struct {
	Directory   string   `json:"directory"`
	Name        string   `json:"name,omitempty"`
	FileCount   int      `json:"file_count"`
	SymbolCount int      `json:"symbol_count"`
	Languages   []string `json:"languages,omitempty"`
}

type ArchitectureSummary struct {
	RepositoryID            string          `json:"repository_id"`
	Overview                string          `json:"overview,omitempty"`
	TotalFiles              int             `json:"total_files"`
	TotalSymbols            int             `json:"total_symbols"`
	TotalRelationships      int             `json:"total_relationships"`
	TotalEdges              int             `json:"total_edges,omitempty"`
	TopModules              []ModuleSummary `json:"top_modules"`
	MajorModules            []ModuleSummary `json:"major_modules,omitempty"`
	EntryPointCandidates    []*Symbol       `json:"entry_point_candidates_syms,omitempty"`
	EntryPointCandidatesStr []string        `json:"entry_point_candidates,omitempty"`
	HighConnectivitySymbols []*Node         `json:"high_connectivity_symbols_nodes,omitempty"`
	HighConnectivitySymsStr []string        `json:"high_connectivity_symbols,omitempty"`
	ExternalBoundaries      []string        `json:"external_boundaries"`
}

type InvestigationStep struct {
	ID                 string                  `json:"id"`
	Sequence           int                     `json:"sequence"`
	Type               InvestigationStepType   `json:"type"`
	StepType           InvestigationStepType   `json:"step_type,omitempty"`
	Title              string                  `json:"title"`
	Description        string                  `json:"description"`
	FileID             string                  `json:"file_id,omitempty"`
	RelativePath       string                  `json:"relative_path,omitempty"`
	TargetFile         string                  `json:"target_file,omitempty"`
	SymbolID           string                  `json:"symbol_id,omitempty"`
	TargetSymbol       string                  `json:"target_symbol,omitempty"`
	SymbolName         string                  `json:"symbol_name,omitempty"`
	TargetNode         string                  `json:"target_node,omitempty"`
	FlowResult         *StaticFlowResult       `json:"flow_result,omitempty"`
	Evidence           []*ExplanationEvidence  `json:"evidence,omitempty"`
	SuggestedQuestions []string                `json:"suggested_questions,omitempty"`
	Status             InvestigationStepStatus `json:"status"`
}

type InvestigationStatus string

const (
	InvestigationInitialized InvestigationStatus = "INVESTIGATION_INITIALIZED"
	InvestigationInProgress  InvestigationStatus = "INVESTIGATION_IN_PROGRESS"
	InvestigationCompleted   InvestigationStatus = "INVESTIGATION_COMPLETED"
	InvestigationFailed      InvestigationStatus = "INVESTIGATION_FAILED"
)

const DefaultMaxInvestigationSteps = 10

// InvestigationRequest defines input criteria for the guided architectural reverse-engineering tour.
// Note: CompletedStepIDs and CurrentStepIndex represent client-provided UI tracking state used for session
// navigation. They are validated for repository scope, known step IDs, uniqueness, and bounds, but are not
// treated as server-enforced cryptographic proof of completed user work.
type InvestigationRequest struct {
	RepositoryScope  RepositoryScope       `json:"repository_scope"`
	StepType         InvestigationStepType `json:"step_type,omitempty"`
	TargetFileID     string                `json:"target_file_id,omitempty"`
	TargetSymbolID   string                `json:"target_symbol_id,omitempty"`
	RootSymbolID     string                `json:"root_symbol_id,omitempty"`
	TargetNodeID     string                `json:"target_node_id,omitempty"`
	MaxSteps         int                   `json:"max_steps,omitempty"`
	CompletedStepIDs []string              `json:"completed_step_ids,omitempty"`
	CurrentStepIndex int                   `json:"current_step_index,omitempty"`
	Action           string                `json:"action,omitempty"` // "START", "NEXT", "RESET"
}

func NewInvestigationRequest(scope RepositoryScope, action string) (*InvestigationRequest, error) {
	if scope.RepositoryID == "" {
		return nil, ErrInvalidRepositoryScope
	}
	act := strings.ToUpper(strings.TrimSpace(action))
	if act == "" {
		act = "START"
	}
	return &InvestigationRequest{
		RepositoryScope:  scope,
		Action:           act,
		MaxSteps:         DefaultMaxInvestigationSteps,
		CompletedStepIDs: make([]string, 0),
	}, nil
}

func (r *InvestigationRequest) Validate() error {
	if r.RepositoryScope.RepositoryID == "" {
		return ErrInvalidRepositoryScope
	}
	if r.MaxSteps <= 0 || r.MaxSteps > DefaultMaxInvestigationSteps {
		r.MaxSteps = DefaultMaxInvestigationSteps
	}
	return nil
}

type Investigation struct {
	ID                  string               `json:"id"`
	RepositoryID        string               `json:"repository_id"`
	Status              InvestigationStatus  `json:"status"`
	Summary             ArchitectureSummary  `json:"summary"`
	ArchitectureSummary ArchitectureSummary  `json:"architecture_summary"`
	Steps               []*InvestigationStep `json:"steps"`
	CurrentStepIndex    int                  `json:"current_step_index"`
	CurrentStep         *InvestigationStep   `json:"current_step,omitempty"`
	SuggestedQuestions  []string             `json:"suggested_questions,omitempty"`
	CompletedStepIDs    []string             `json:"completed_step_ids"`
	IsComplete          bool                 `json:"is_complete"`
	UpdatedAt           time.Time            `json:"updated_at"`
}

func NewInvestigation(scope RepositoryScope, summary ArchitectureSummary) (*Investigation, error) {
	if scope.RepositoryID == "" {
		return nil, ErrInvalidRepositoryScope
	}
	return &Investigation{
		ID:                  fmt.Sprintf("inv-%s", scope.RepositoryID),
		RepositoryID:        scope.RepositoryID,
		Status:              InvestigationInitialized,
		Summary:             summary,
		ArchitectureSummary: summary,
		Steps:               make([]*InvestigationStep, 0),
		CurrentStepIndex:    0,
		CompletedStepIDs:    make([]string, 0),
		IsComplete:          false,
		UpdatedAt:           time.Now(),
	}, nil
}

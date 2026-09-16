package models

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrEmptyQuestion      = errors.New("explanation question cannot be empty")
	ErrNilEvidence        = errors.New("explanation evidence cannot be nil")
	ErrContradictoryState = errors.New("contradictory explanation response state")
)

// ExplanationRequest specifies the input criteria for generating a grounded AI code explanation.
type ExplanationRequest struct {
	RepositoryScope RepositoryScope   `json:"repository_scope"`
	Question        string            `json:"question"`
	SymbolID        string            `json:"symbol_id,omitempty"`
	StaticFlow      *StaticFlowResult `json:"static_flow,omitempty"`
	NodeIDs         []string          `json:"node_ids,omitempty"`
	EdgeIDs         []string          `json:"edge_ids,omitempty"`
	Budget          EvidenceBudget    `json:"budget"`
	Provider        string            `json:"provider,omitempty"`
	Model           string            `json:"model,omitempty"`
}

// NewExplanationRequest creates a validated ExplanationRequest scoped to a single repository.
func NewExplanationRequest(scope RepositoryScope, question string) (*ExplanationRequest, error) {
	if scope.RepositoryID == "" {
		return nil, ErrInvalidRepositoryScope
	}
	trimmed := strings.TrimSpace(question)
	if trimmed == "" {
		return nil, ErrEmptyQuestion
	}
	return &ExplanationRequest{
		RepositoryScope: scope,
		Question:        trimmed,
		NodeIDs:         make([]string, 0),
		EdgeIDs:         make([]string, 0),
		Budget:          DefaultEvidenceBudget(),
	}, nil
}

// Validate verifies request constraints and repository isolation.
func (r *ExplanationRequest) Validate() error {
	if r.RepositoryScope.RepositoryID == "" {
		return ErrInvalidRepositoryScope
	}
	if strings.TrimSpace(r.Question) == "" {
		return ErrEmptyQuestion
	}
	if r.StaticFlow != nil && r.StaticFlow.RepositoryID != "" && r.StaticFlow.RepositoryID != r.RepositoryScope.RepositoryID {
		return fmt.Errorf("%w: static flow repository ID '%s' != scope repository ID '%s'", ErrRepositoryMismatch, r.StaticFlow.RepositoryID, r.RepositoryScope.RepositoryID)
	}
	return nil
}

// ExplanationEvidence represents an authoritative, non-synthesized evidence item with full provenance.
type ExplanationEvidence struct {
	ID               string            `json:"id"`
	StableID         string            `json:"stable_id"`
	Label            string            `json:"label"`
	RepositoryID     string            `json:"repository_id"`
	Type             EvidenceType      `json:"type"`
	FileID           string            `json:"file_id,omitempty"`
	RelativePath     string            `json:"relative_path"`
	Location         Location          `json:"location"`
	SymbolID         string            `json:"symbol_id,omitempty"`
	SymbolName       string            `json:"symbol_name,omitempty"`
	RelationshipID   string            `json:"relationship_id,omitempty"`
	EdgeKind         EdgeKind          `json:"edge_kind,omitempty"`
	FlowStepSequence int               `json:"flow_step_sequence,omitempty"`
	FlowPathID       string            `json:"flow_path_id,omitempty"`
	Content          string            `json:"content"`
	RetrieverType    string            `json:"retriever_type"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

// NewExplanationEvidenceFromItem converts a Phase 4 EvidenceItem into an ExplanationEvidence while validating repository scope.
func NewExplanationEvidenceFromItem(scope RepositoryScope, item *EvidenceItem) (*ExplanationEvidence, error) {
	if err := scope.ValidateItem(item); err != nil {
		return nil, err
	}
	if item.StableID == "" {
		item.StableID = item.ComputeStableID()
	}
	idVal := item.Label
	if idVal == "" {
		idVal = item.StableID
	}
	ev := &ExplanationEvidence{
		ID:            idVal,
		StableID:      item.StableID,
		Label:         item.Label,
		RepositoryID:  item.RepositoryID,
		Type:          item.Type,
		FileID:        item.FileID,
		RelativePath:  item.RelativePath,
		Location:      item.Location,
		Content:       item.Content,
		RetrieverType: item.RetrieverType,
	}
	if item.Metadata != nil {
		ev.Metadata = make(map[string]string, len(item.Metadata))
		for k, v := range item.Metadata {
			ev.Metadata[k] = v
		}
		ev.SymbolID = item.Metadata["symbol_id"]
		ev.SymbolName = item.Metadata["symbol_name"]
		ev.RelationshipID = item.Metadata["relationship_id"]
		ev.EdgeKind = EdgeKind(item.Metadata["edge_kind"])
		if seqStr, ok := item.Metadata["flow_step_sequence"]; ok {
			fmt.Sscanf(seqStr, "%d", &ev.FlowStepSequence)
		}
		ev.FlowPathID = item.Metadata["flow_path_id"]
	}
	return ev, nil
}

// ValidateScope checks that the evidence belongs exclusively to the target repository scope.
func (e *ExplanationEvidence) ValidateScope(scope RepositoryScope) error {
	if e == nil {
		return ErrNilEvidence
	}
	if e.RepositoryID != scope.RepositoryID {
		return fmt.Errorf("%w: evidence repository '%s' != scope repository '%s'", ErrRepositoryMismatch, e.RepositoryID, scope.RepositoryID)
	}
	return nil
}

// EvidenceReference explicitly links a claim to evidence using display label (Label) and machine identity (StableID).
type EvidenceReference struct {
	Label    string `json:"label"`     // Display citation reference (e.g. "E1")
	StableID string `json:"stable_id"` // Cryptographic/machine identity hash
}

// ClaimGroundingStatus defines the status of an LLM claim relative to provided evidence.
type ClaimGroundingStatus string

const (
	ClaimGrounded     ClaimGroundingStatus = "CLAIM_GROUNDED"
	ClaimUnsupported  ClaimGroundingStatus = "CLAIM_UNSUPPORTED"
	ClaimContradicted ClaimGroundingStatus = "CLAIM_CONTRADICTED"
	ClaimUnchecked    ClaimGroundingStatus = "CLAIM_UNCHECKED"
)

// ExplanationClaim represents an individual assertion produced by an LLM, linked to cited evidence references.
type ExplanationClaim struct {
	ID                 string               `json:"id"`
	Text               string               `json:"text"`
	EvidenceReferences []EvidenceReference  `json:"evidence_references"` // Explicit evidence references
	GroundingStatus    ClaimGroundingStatus `json:"grounding_status"`
	IsValid            bool                 `json:"is_valid"`
	Reason             string               `json:"reason,omitempty"`
}

// GroundingStatus describes overall response grounding quality.
type GroundingStatus string

const (
	GroundingPassed               GroundingStatus = "GROUNDING_PASSED"
	GroundingPartial              GroundingStatus = "GROUNDING_PARTIAL"
	GroundingFailed               GroundingStatus = "GROUNDING_FAILED"
	GroundingInsufficientEvidence GroundingStatus = "GROUNDING_INSUFFICIENT_EVIDENCE"
)

// GroundingMetadata encapsulates deterministic metrics evaluating LLM response grounding.
type GroundingMetadata struct {
	EvidenceCount            int               `json:"evidence_count"`
	CitedEvidenceCount       int               `json:"cited_evidence_count"`
	CitationValidationStatus string            `json:"citation_validation_status"`
	TotalClaims              int               `json:"total_claims"`
	GroundedClaims           int               `json:"grounded_claims"`
	UnsupportedClaimCount    int               `json:"unsupported_claim_count"`
	Status                   GroundingStatus   `json:"status"`
	Sufficiency              SufficiencyStatus `json:"sufficiency"`
}

// ExplanationStatus defines the top-level outcome of an explanation generation.
type ExplanationStatus string

const (
	ExplanationStatusSuccess              ExplanationStatus = "EXPLANATION_SUCCESS"
	ExplanationStatusInsufficientEvidence ExplanationStatus = "INSUFFICIENT_EVIDENCE"
	ExplanationStatusFailed               ExplanationStatus = "EXPLANATION_FAILED"
)

// ExplanationResponse represents the complete grounded explanation output.
type ExplanationResponse struct {
	RepositoryID             string                    `json:"repository_id"`
	Question                 string                    `json:"question"`
	Status                   ExplanationStatus         `json:"status"`
	Answer                   string                    `json:"answer"`
	Claims                   []ExplanationClaim        `json:"claims,omitempty"`
	Evidence                 []*ExplanationEvidence    `json:"evidence,omitempty"`
	Citations                []Citation                `json:"citations,omitempty"`
	Grounding                GroundingMetadata         `json:"grounding"`
	Provider                 string                    `json:"provider,omitempty"`
	Model                    string                    `json:"model,omitempty"`
	PromptTokens             int                       `json:"prompt_tokens,omitempty"`
	CompletionTokens         int                       `json:"completion_tokens,omitempty"`
	TotalTokens              int                       `json:"total_tokens,omitempty"`
	LatencyMs                float64                   `json:"latency_ms,omitempty"`
	CitationValidationStatus string                    `json:"citation_validation_status"`
	IsInsufficientEvidence   bool                      `json:"is_insufficient_evidence"`
	Sufficiency              EvidenceSufficiencyResult `json:"sufficiency"`
	ErrorMessage             string                    `json:"error_message,omitempty"`
}

// NewInsufficientEvidenceResponse constructs a response representing explicit insufficient evidence while preserving any available evidence.
func NewInsufficientEvidenceResponse(
	scope RepositoryScope,
	question string,
	sufficiency EvidenceSufficiencyResult,
	availableEvidence []*ExplanationEvidence,
	citations []Citation,
) (*ExplanationResponse, error) {
	if scope.RepositoryID == "" {
		return nil, ErrInvalidRepositoryScope
	}
	for _, ev := range availableEvidence {
		if err := ev.ValidateScope(scope); err != nil {
			return nil, err
		}
	}

	evList := availableEvidence
	if evList == nil {
		evList = make([]*ExplanationEvidence, 0)
	}
	citList := citations
	if citList == nil {
		citList = make([]Citation, 0)
	}

	return &ExplanationResponse{
		RepositoryID:           scope.RepositoryID,
		Question:               question,
		Status:                 ExplanationStatusInsufficientEvidence,
		Answer:                 "Insufficient evidence available in repository to generate a fully grounded answer.",
		Claims:                 make([]ExplanationClaim, 0),
		Evidence:               evList,
		Citations:              citList,
		IsInsufficientEvidence: true,
		Sufficiency:            sufficiency,
		Grounding: GroundingMetadata{
			EvidenceCount:            len(evList),
			CitedEvidenceCount:       len(citList),
			CitationValidationStatus: "NO_CITATIONS_PRESENT",
			TotalClaims:              0,
			GroundedClaims:           0,
			UnsupportedClaimCount:    0,
			Status:                   GroundingInsufficientEvidence,
			Sufficiency:              sufficiency.Status,
		},
	}, nil
}

// Validate checks internal consistency and prevents contradictory response states.
func (resp *ExplanationResponse) Validate() error {
	if resp.RepositoryID == "" {
		return ErrInvalidRepositoryScope
	}
	if resp.Status == ExplanationStatusSuccess && resp.IsInsufficientEvidence {
		return fmt.Errorf("%w: status SUCCESS cannot have IsInsufficientEvidence = true", ErrContradictoryState)
	}
	if resp.Status == ExplanationStatusInsufficientEvidence && !resp.IsInsufficientEvidence {
		return fmt.Errorf("%w: status INSUFFICIENT_EVIDENCE must have IsInsufficientEvidence = true", ErrContradictoryState)
	}
	if resp.Status == ExplanationStatusInsufficientEvidence && resp.Grounding.Status == GroundingPassed {
		return fmt.Errorf("%w: status INSUFFICIENT_EVIDENCE cannot have GroundingStatus PASSED", ErrContradictoryState)
	}
	if resp.Status == ExplanationStatusInsufficientEvidence && resp.Sufficiency.Status == SufficiencySufficient {
		return fmt.Errorf("%w: status INSUFFICIENT_EVIDENCE cannot have SufficiencyStatus SUFFICIENT", ErrContradictoryState)
	}
	if resp.Status == ExplanationStatusInsufficientEvidence && len(resp.Claims) > 0 {
		return fmt.Errorf("%w: status INSUFFICIENT_EVIDENCE cannot contain generated claims", ErrContradictoryState)
	}
	if resp.Status == ExplanationStatusSuccess && resp.Grounding.Status == GroundingInsufficientEvidence {
		return fmt.Errorf("%w: status SUCCESS cannot have GroundingStatus INSUFFICIENT_EVIDENCE", ErrContradictoryState)
	}
	return nil
}

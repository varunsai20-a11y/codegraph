package models

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidRepositoryScope = errors.New("invalid repository scope: repository ID cannot be empty")
	ErrRepositoryMismatch     = errors.New("repository mismatch: item does not belong to the target repository scope")
	ErrBudgetExceeded         = errors.New("evidence budget exceeded")
)

// RepositoryScope enforces repository isolation at the domain boundary.
type RepositoryScope struct {
	RepositoryID string `json:"repository_id"`
}

func NewRepositoryScope(repoID string) (RepositoryScope, error) {
	trimmed := strings.TrimSpace(repoID)
	if trimmed == "" {
		return RepositoryScope{}, ErrInvalidRepositoryScope
	}
	return RepositoryScope{RepositoryID: trimmed}, nil
}

func (s RepositoryScope) ValidateItem(item *EvidenceItem) error {
	if item == nil {
		return errors.New("evidence item cannot be nil")
	}
	if item.RepositoryID != s.RepositoryID {
		return fmt.Errorf("%w: item repo '%s' != scope repo '%s'", ErrRepositoryMismatch, item.RepositoryID, s.RepositoryID)
	}
	return nil
}

type EvidenceType string

const (
	EvidenceTypeSymbol      EvidenceType = "EVIDENCE_SYMBOL"
	EvidenceTypeGraphEdge   EvidenceType = "EVIDENCE_GRAPH_EDGE"
	EvidenceTypeCodeSnippet EvidenceType = "EVIDENCE_CODE_SNIPPET"
	EvidenceTypeDependency  EvidenceType = "EVIDENCE_DEPENDENCY"
	EvidenceTypeStaticFlow  EvidenceType = "EVIDENCE_STATIC_FLOW"
)

type TargetResolutionStatus string

const (
	TargetResolved  TargetResolutionStatus = "TARGET_RESOLVED"
	TargetAmbiguous TargetResolutionStatus = "TARGET_AMBIGUOUS"
	TargetNotFound  TargetResolutionStatus = "TARGET_NOT_FOUND"
)

// EvidenceItem represents a single piece of evidence retrieved from CodeGraph.
// Retrieval scores are explicitly separated: RawScore (retriever raw), Rank (retriever rank), RRFScore (fused rank score).
type EvidenceItem struct {
	StableID         string                 `json:"stable_id"` // Cryptographically/deterministically derived hash
	Label            string                 `json:"label"`     // Human-readable citation label e.g. "E1", "E2"
	RepositoryID     string                 `json:"repository_id"`
	Type             EvidenceType           `json:"type"`
	FileID           string                 `json:"file_id"`
	RelativePath     string                 `json:"relative_path"`
	Location         Location               `json:"location"`
	Content          string                 `json:"content"`
	RetrieverType    string                 `json:"retriever_type"`              // GRAPH, LEXICAL, VECTOR, HYBRID
	RawScore         float64                `json:"raw_score"`                   // Score from raw retriever
	Rank             int                    `json:"rank"`                        // Rank position from raw retriever
	RRFScore         float64                `json:"rrf_score"`                   // Fused Reciprocal Rank Fusion score
	ResolutionStatus RelationStatus         `json:"resolution_status,omitempty"` // RESOLVED, PARTIAL, UNRESOLVED
	TargetResolution TargetResolutionStatus `json:"target_resolution,omitempty"` // TARGET_RESOLVED, TARGET_AMBIGUOUS, TARGET_NOT_FOUND
	Metadata         map[string]string      `json:"metadata,omitempty"`
}

// ComputeStableID produces a deterministic hash from immutable evidence attributes.
func (item *EvidenceItem) ComputeStableID() string {
	raw := fmt.Sprintf("%s|%s|%s|%d:%d-%d:%d|%s|%s",
		item.RepositoryID,
		item.RelativePath,
		item.Type,
		item.Location.StartLine, item.Location.StartColumn,
		item.Location.EndLine, item.Location.EndColumn,
		item.Content,
		item.RetrieverType,
	)
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:16])
}

// Clone returns a deep copy of EvidenceItem to guarantee input immutability.
func (item *EvidenceItem) Clone() *EvidenceItem {
	if item == nil {
		return nil
	}
	cp := *item
	if item.Metadata != nil {
		cp.Metadata = make(map[string]string, len(item.Metadata))
		for k, v := range item.Metadata {
			cp.Metadata[k] = v
		}
	}
	return &cp
}

// EstimateTokens returns a deterministic character-based heuristic token estimation (approx 4 chars per token).
func EstimateTokens(content string) int {
	if len(content) == 0 {
		return 0
	}
	return (len(content) + 3) / 4
}

// EvidenceBudget configures limits for evidence package construction.
type EvidenceBudget struct {
	MaxTokens       int `json:"max_tokens"`
	MaxFiles        int `json:"max_files"`
	MaxSymbols      int `json:"max_symbols"`
	MaxGraphHops    int `json:"max_graph_hops"`
	MaxSnippetLines int `json:"max_snippet_lines"`
}

func DefaultEvidenceBudget() EvidenceBudget {
	return EvidenceBudget{
		MaxTokens:       4000,
		MaxFiles:        20,
		MaxSymbols:      50,
		MaxGraphHops:    2,
		MaxSnippetLines: 100,
	}
}

type SufficiencyStatus string

const (
	SufficiencySufficient   SufficiencyStatus = "SUFFICIENCY_SUFFICIENT"
	SufficiencyInsufficient SufficiencyStatus = "SUFFICIENCY_INSUFFICIENT"
	SufficiencyPartial      SufficiencyStatus = "SUFFICIENCY_PARTIAL"
	SufficiencyConflicting  SufficiencyStatus = "SUFFICIENCY_CONFLICTING"
)

// EvidenceSufficiencyResult represents an independent assessment of evidence quality/completeness.
// Note: HeuristicScore is an internal scoring metric, NOT a calibrated probability or factual confidence.
type EvidenceSufficiencyResult struct {
	Status             SufficiencyStatus      `json:"status"`
	TargetFound        bool                   `json:"target_found"`
	SourceAvailable    bool                   `json:"source_available"`
	TargetResolution   TargetResolutionStatus `json:"target_resolution"`
	ResolutionCoverage float64                `json:"resolution_coverage"` // 0.0 to 1.0 ratio of resolved relations
	EvidenceCoverage   float64                `json:"evidence_coverage"`   // 0.0 to 1.0 type variety coverage
	HeuristicScore     float64                `json:"heuristic_score"`     // Non-probabilistic heuristic assessment
	Conflicts          []string               `json:"conflicts,omitempty"`
	Reasons            []string               `json:"reasons,omitempty"`
	MissingInfo        []string               `json:"missing_info,omitempty"`
}

// EvidencePackage represents a structured package ready for LLM consumption.
type EvidencePackage struct {
	RepositoryID string                    `json:"repository_id"`
	Question     string                    `json:"question"`
	Intent       string                    `json:"intent"`
	Items        []*EvidenceItem           `json:"items"`
	Citations    []Citation                `json:"citations,omitempty"`
	Budget       EvidenceBudget            `json:"budget"`
	Sufficiency  EvidenceSufficiencyResult `json:"sufficiency"`
	TotalTokens  int                       `json:"total_tokens"`
}

func NewEvidencePackage(scope RepositoryScope, question string, intent string, budget EvidenceBudget) (*EvidencePackage, error) {
	if scope.RepositoryID == "" {
		return nil, ErrInvalidRepositoryScope
	}
	return &EvidencePackage{
		RepositoryID: scope.RepositoryID,
		Question:     question,
		Intent:       intent,
		Items:        make([]*EvidenceItem, 0),
		Citations:    make([]Citation, 0),
		Budget:       budget,
		TotalTokens:  0,
	}, nil
}

func (p *EvidencePackage) AddItem(scope RepositoryScope, item *EvidenceItem) error {
	if err := scope.ValidateItem(item); err != nil {
		return err
	}
	if item.StableID == "" {
		item.StableID = item.ComputeStableID()
	}
	p.Items = append(p.Items, item)
	return nil
}

// FinalizePackage assigns stable display labels (E1, E2, ...) in the selected C5 relevance ranking order,
// constructs deterministic Citations from structured item provenance, and calculates TotalTokens.
func (p *EvidencePackage) FinalizePackage() {
	p.Citations = make([]Citation, 0, len(p.Items))
	totalToks := 0
	for i, item := range p.Items {
		item.Label = fmt.Sprintf("E%d", i+1)
		p.Citations = append(p.Citations, Citation{
			EvidenceID:   item.Label,
			StableID:     item.StableID,
			RelativePath: item.RelativePath,
			Location:     item.Location,
			IsValid:      true,
		})
		totalToks += EstimateTokens(item.Content)
	}
	p.TotalTokens = totalToks
}

type Citation struct {
	EvidenceID   string   `json:"evidence_id"` // Display Label (e.g. E1) or StableID
	StableID     string   `json:"stable_id"`
	RelativePath string   `json:"relative_path"`
	Location     Location `json:"location"`
	IsValid      bool     `json:"is_valid"`
	Reason       string   `json:"reason,omitempty"`
}

type ExplanationResult struct {
	RepositoryID             string                    `json:"repository_id"`
	Question                 string                    `json:"question"`
	Answer                   string                    `json:"answer"`
	RawCitations             []string                  `json:"raw_citations,omitempty"`
	ValidatedCitations       []Citation                `json:"validated_citations,omitempty"`
	InvalidCitations         []string                  `json:"invalid_citations,omitempty"`
	MalformedCitations       []string                  `json:"malformed_citations,omitempty"`
	CitationValidationStatus string                    `json:"citation_validation_status"` // VALIDATION_PASSED, INVALID_CITATIONS_FOUND, NO_CITATIONS_PRESENT
	TotalCitations           int                       `json:"total_citations"`
	ValidCount               int                       `json:"valid_count"`
	InvalidCount             int                       `json:"invalid_count"`
	CitationCoverage         float64                   `json:"citation_coverage"` // 0.0 to 1.0 ratio of valid citations to total raw citations
	Provider                 string                    `json:"provider,omitempty"`
	Model                    string                    `json:"model,omitempty"`
	Sufficiency              EvidenceSufficiencyResult `json:"sufficiency"`
}

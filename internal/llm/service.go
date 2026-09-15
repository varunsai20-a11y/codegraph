package llm

import (
	"context"
	"errors"
	"fmt"

	"codegraph/internal/models"
	"codegraph/internal/retrieval"
)

// ExplanationService defines the contract for generating grounded code explanations from an EvidencePackage.
type ExplanationService interface {
	Explain(ctx context.Context, scope models.RepositoryScope, pkg *models.EvidencePackage) (*models.ExplanationResult, error)
}

// GroundedExplanationService orchestrates prompt building, LLM provider invocation, and deterministic citation validation.
type GroundedExplanationService struct {
	provider  LLMProvider
	validator *CitationValidator
}

func NewGroundedExplanationService(provider LLMProvider, validator *CitationValidator) *GroundedExplanationService {
	if provider == nil {
		provider = NewMockLLMProvider("Evidence insufficient to answer query.", nil)
	}
	if validator == nil {
		validator = NewCitationValidator()
	}
	return &GroundedExplanationService{
		provider:  provider,
		validator: validator,
	}
}

func (s *GroundedExplanationService) Explain(
	ctx context.Context,
	scope models.RepositoryScope,
	pkg *models.EvidencePackage,
) (*models.ExplanationResult, error) {
	if scope.RepositoryID == "" {
		return nil, models.ErrInvalidRepositoryScope
	}
	if pkg == nil {
		return nil, errors.New("evidence package cannot be nil")
	}
	if pkg.RepositoryID != scope.RepositoryID {
		return nil, fmt.Errorf("%w: package repository '%s' != scope repository '%s'", models.ErrRepositoryMismatch, pkg.RepositoryID, scope.RepositoryID)
	}

	// Handle insufficient / empty evidence package explicitly without fabricating claims
	if pkg.Sufficiency.Status == models.SufficiencyInsufficient || len(pkg.Items) == 0 {
		return &models.ExplanationResult{
			RepositoryID:             scope.RepositoryID,
			Question:                 pkg.Question,
			Answer:                   "The retrieved evidence is insufficient to answer the query for this repository codebase.",
			CitationValidationStatus: "NO_CITATIONS_PRESENT",
			TotalCitations:           0,
			ValidCount:               0,
			InvalidCount:             0,
			CitationCoverage:         1.0,
			Provider:                 s.provider.Name(),
			Model:                    s.provider.Model(),
			Sufficiency:              pkg.Sufficiency,
		}, nil
	}

	// Build prompt-injection-resistant grounded context
	groundedCtx := retrieval.BuildGroundedContext(pkg)

	req := LLMRequest{
		SystemInstruction: BuildSystemInstruction(),
		UserQuery:         pkg.Question,
		GroundedContext:   groundedCtx,
	}

	// Invoke LLM Provider with context cancellation/deadline support
	resp, err := s.provider.Generate(ctx, req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("%w: %v", ErrProviderTimeout, err)
		}
		return nil, fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}

	// Deterministically validate all citations against evidence package provenance
	report := s.validator.Validate(resp.Content, pkg)

	return &models.ExplanationResult{
		RepositoryID:             scope.RepositoryID,
		Question:                 pkg.Question,
		Answer:                   resp.Content,
		RawCitations:             report.RawCitations,
		ValidatedCitations:       report.ValidatedCitations,
		InvalidCitations:         report.InvalidCitations,
		MalformedCitations:       report.MalformedCitations,
		CitationValidationStatus: report.CitationValidationStatus,
		TotalCitations:           report.TotalCitations,
		ValidCount:               report.ValidCount,
		InvalidCount:             report.InvalidCount,
		CitationCoverage:         report.CitationCoverage,
		Provider:                 resp.Provider,
		Model:                    resp.Model,
		Sufficiency:              pkg.Sufficiency,
	}, nil
}

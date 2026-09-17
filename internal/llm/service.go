package llm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"codegraph/internal/models"
	"codegraph/internal/retrieval"
)

// ExplanationService defines the contract for generating grounded code explanations from an EvidencePackage or ExplanationRequest.
type ExplanationService interface {
	Explain(ctx context.Context, scope models.RepositoryScope, pkg *models.EvidencePackage) (*models.ExplanationResult, error)
	ExplainRequest(ctx context.Context, req *models.ExplanationRequest) (*models.ExplanationResponse, error)
}

// GroundedExplanationService orchestrates evidence composition, prompt building, LLM provider invocation, and deterministic citation validation.
type GroundedExplanationService struct {
	provider      LLMProvider
	validator     *CitationValidator
	composer      retrieval.EvidenceComposer
	promptBuilder *GroundedPromptBuilder
}

func NewGroundedExplanationService(
	provider LLMProvider,
	validator *CitationValidator,
	composer retrieval.EvidenceComposer,
	promptBuilder *GroundedPromptBuilder,
) *GroundedExplanationService {
	if provider == nil {
		provider = NewMockLLMProvider("Evidence insufficient to answer query.", nil)
	}
	if validator == nil {
		validator = NewCitationValidator()
	}
	if promptBuilder == nil {
		promptBuilder = NewGroundedPromptBuilder()
	}
	return &GroundedExplanationService{
		provider:      provider,
		validator:     validator,
		composer:      composer,
		promptBuilder: promptBuilder,
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

	groundedCtx := retrieval.BuildGroundedContext(pkg)

	req := LLMRequest{
		SystemInstruction: BuildSystemInstruction(),
		UserQuery:         pkg.Question,
		GroundedContext:   groundedCtx,
	}

	resp, err := s.provider.Generate(ctx, req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || ctx.Err() != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("%w: %w", ErrProviderTimeout, err)
		}
		return nil, fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}

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

// ExplainRequest handles an end-to-end C6.1 ExplanationRequest producing a validated ExplanationResponse.
func (s *GroundedExplanationService) ExplainRequest(
	ctx context.Context,
	req *models.ExplanationRequest,
) (*models.ExplanationResponse, error) {
	startTime := time.Now()
	if req == nil {
		return nil, errors.New("explanation request cannot be nil")
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}

	scope := req.RepositoryScope

	// 1. Compose evidence package using EvidenceComposer if available
	var pkg *models.EvidencePackage
	var err error

	if s.composer != nil {
		pkg, err = s.composer.Compose(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("evidence composition failed: %w", err)
		}
	} else {
		// Fallback empty evidence package
		pkg, err = models.NewEvidencePackage(scope, req.Question, "EXPLANATION", req.Budget)
		if err != nil {
			return nil, err
		}
		pkg.Sufficiency = models.EvidenceSufficiencyResult{
			Status:           models.SufficiencyInsufficient,
			TargetResolution: models.TargetNotFound,
		}
	}

	// Convert package items to ExplanationEvidence
	evList := make([]*models.ExplanationEvidence, 0, len(pkg.Items))
	for _, item := range pkg.Items {
		ev, convErr := models.NewExplanationEvidenceFromItem(scope, item)
		if convErr == nil {
			evList = append(evList, ev)
		}
	}

	// 2. Explicit INSUFFICIENT_EVIDENCE handling
	if pkg.Sufficiency.Status == models.SufficiencyInsufficient || len(pkg.Items) == 0 {
		insuffResp, insuffErr := models.NewInsufficientEvidenceResponse(
			scope,
			req.Question,
			pkg.Sufficiency,
			evList,
			pkg.Citations,
		)
		if insuffErr != nil {
			return nil, insuffErr
		}
		insuffResp.LatencyMs = float64(time.Since(startTime).Milliseconds())
		if err := insuffResp.Validate(); err != nil {
			return nil, err
		}
		return insuffResp, nil
	}

	// 3. Build prompt
	llmReq, err := s.promptBuilder.BuildPrompt(req, pkg)
	if err != nil {
		return nil, fmt.Errorf("prompt building failed: %w", err)
	}

	// 4. Generate LLM completion
	llmResp, err := s.provider.Generate(ctx, llmReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || ctx.Err() != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("%w: %w", ErrProviderTimeout, err)
		}
		return nil, fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}

	// 5. Validate citations deterministically
	report := s.validator.Validate(llmResp.Content, pkg)

	// 6. Build claim-evidence references
	claims := make([]models.ExplanationClaim, 0)
	groundedCount := 0
	unsupportedCount := 0

	for i, cit := range report.ValidatedCitations {
		claims = append(claims, models.ExplanationClaim{
			ID:   fmt.Sprintf("claim-valid-%d", i+1),
			Text: fmt.Sprintf("Assertion cited by %s (%s)", cit.EvidenceID, cit.RelativePath),
			EvidenceReferences: []models.EvidenceReference{
				{
					Label:    cit.EvidenceID,
					StableID: cit.StableID,
				},
			},
			GroundingStatus: models.ClaimGrounded,
			IsValid:         true,
		})
		groundedCount++
	}

	for i, invCit := range report.InvalidCitations {
		claims = append(claims, models.ExplanationClaim{
			ID:                 fmt.Sprintf("claim-invalid-%d", i+1),
			Text:               fmt.Sprintf("Assertion citing invalid token %s", invCit),
			EvidenceReferences: make([]models.EvidenceReference, 0),
			GroundingStatus:    models.ClaimUnsupported,
			IsValid:            false,
			Reason:             fmt.Sprintf("Citation %s does not exist in evidence package provenance", invCit),
		})
		unsupportedCount++
	}

	// 7. Grounding status calculation
	gStatus := models.GroundingPassed
	if report.CitationValidationStatus == "INVALID_CITATIONS_FOUND" {
		gStatus = models.GroundingPartial
	} else if report.CitationValidationStatus == "NO_CITATIONS_PRESENT" {
		gStatus = models.GroundingFailed
	}

	groundingMeta := models.GroundingMetadata{
		EvidenceCount:            len(evList),
		CitedEvidenceCount:       report.ValidCount,
		CitationValidationStatus: report.CitationValidationStatus,
		TotalClaims:              len(claims),
		GroundedClaims:           groundedCount,
		UnsupportedClaimCount:    unsupportedCount,
		Status:                   gStatus,
		Sufficiency:              pkg.Sufficiency.Status,
	}

	resultResp := &models.ExplanationResponse{
		RepositoryID:             scope.RepositoryID,
		Question:                 req.Question,
		Status:                   models.ExplanationStatusSuccess,
		Answer:                   llmResp.Content,
		Claims:                   claims,
		Evidence:                 evList,
		Citations:                report.ValidatedCitations,
		Grounding:                groundingMeta,
		Provider:                 llmResp.Provider,
		Model:                    llmResp.Model,
		PromptTokens:             llmResp.Usage.PromptTokens,
		CompletionTokens:         llmResp.Usage.CompletionTokens,
		TotalTokens:              llmResp.Usage.TotalTokens,
		LatencyMs:                float64(time.Since(startTime).Milliseconds()),
		CitationValidationStatus: report.CitationValidationStatus,
		IsInsufficientEvidence:   false,
		Sufficiency:              pkg.Sufficiency,
	}

	if err := resultResp.Validate(); err != nil {
		return nil, fmt.Errorf("response validation failed: %w", err)
	}

	return resultResp, nil
}

package llm

import (
	"fmt"

	"codegraph/internal/models"
	"codegraph/internal/retrieval"
)

// GroundedPromptBuilder constructs prompt-injection-resistant LLM requests using system instructions and formatted evidence packages.
type GroundedPromptBuilder struct{}

func NewGroundedPromptBuilder() *GroundedPromptBuilder {
	return &GroundedPromptBuilder{}
}

// BuildPrompt constructs an LLMRequest from an ExplanationRequest and EvidencePackage.
func (b *GroundedPromptBuilder) BuildPrompt(req *models.ExplanationRequest, pkg *models.EvidencePackage) (LLMRequest, error) {
	if req == nil {
		return LLMRequest{}, fmt.Errorf("explanation request cannot be nil")
	}
	if pkg == nil {
		return LLMRequest{}, fmt.Errorf("evidence package cannot be nil")
	}
	if req.RepositoryScope.RepositoryID != pkg.RepositoryID {
		return LLMRequest{}, fmt.Errorf("%w: request repo '%s' != package repo '%s'", models.ErrRepositoryMismatch, req.RepositoryScope.RepositoryID, pkg.RepositoryID)
	}

	groundedCtx := retrieval.BuildGroundedContext(pkg)

	return LLMRequest{
		SystemInstruction: BuildSystemInstruction(),
		UserQuery:         req.Question,
		GroundedContext:   groundedCtx,
	}, nil
}

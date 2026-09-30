package llm

import (
	"fmt"
	"strings"

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

	if len(req.History) > 0 {
		historyWindow := req.History
		if len(historyWindow) > 6 {
			historyWindow = historyWindow[len(historyWindow)-6:]
		}
		var hLines []string
		hLines = append(hLines, "--- PRIOR CONVERSATION HISTORY ---")
		for _, msg := range historyWindow {
			roleLabel := "User"
			if strings.EqualFold(msg.Role, "assistant") || strings.EqualFold(msg.Role, "system") {
				roleLabel = "Assistant"
			}
			hLines = append(hLines, fmt.Sprintf("%s: %s", roleLabel, msg.Content))
		}
		hLines = append(hLines, "--- END PRIOR CONVERSATION HISTORY ---")
		groundedCtx = groundedCtx + "\n\n" + strings.Join(hLines, "\n")
	}

	return LLMRequest{
		SystemInstruction: BuildSystemInstruction(),
		UserQuery:         req.Question,
		GroundedContext:   groundedCtx,
	}, nil
}

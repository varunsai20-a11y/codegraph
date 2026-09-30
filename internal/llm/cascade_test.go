package llm_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"codegraph/internal/llm"
	"codegraph/internal/models"
)

type mockProvider struct {
	name       string
	model      string
	generateFn func(ctx context.Context, req llm.LLMRequest) (*llm.LLMResponse, error)
	called     bool
}

func (m *mockProvider) Name() string  { return m.name }
func (m *mockProvider) Model() string { return m.model }
func (m *mockProvider) Generate(ctx context.Context, req llm.LLMRequest) (*llm.LLMResponse, error) {
	m.called = true
	if m.generateFn != nil {
		return m.generateFn(ctx, req)
	}
	return &llm.LLMResponse{
		Content:  "Mock response with citation [E1].",
		Provider: m.name,
		Model:    m.model,
	}, nil
}

func TestCascadeLLMProvider_Test1_GeminiSucceeds(t *testing.T) {
	gemini := &mockProvider{name: "gemini", model: "gemini-3.8-flash"}
	groq := &mockProvider{name: "groq", model: "openai/gpt-oss-120b"}

	cascade := llm.NewCascadeLLMProvider(gemini, groq)
	resp, err := cascade.Generate(context.Background(), llm.LLMRequest{UserQuery: "test"})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if resp.Provider != "gemini" {
		t.Errorf("expected provider gemini, got %s", resp.Provider)
	}
	if !gemini.called {
		t.Errorf("expected Gemini to be called")
	}
	if groq.called {
		t.Errorf("expected Groq NOT to be called when Gemini succeeds")
	}
}

func TestCascadeLLMProvider_Test2_Gemini429_GroqSucceeds(t *testing.T) {
	gemini := &mockProvider{
		name:  "gemini",
		model: "gemini-3.8-flash",
		generateFn: func(ctx context.Context, req llm.LLMRequest) (*llm.LLMResponse, error) {
			return nil, fmtError("Gemini rate limit exceeded (HTTP 429)")
		},
	}
	groq := &mockProvider{
		name:  "groq",
		model: "openai/gpt-oss-120b",
		generateFn: func(ctx context.Context, req llm.LLMRequest) (*llm.LLMResponse, error) {
			return &llm.LLMResponse{
				Content:  "Groq grounded answer citing [E1].",
				Provider: "groq",
				Model:    "openai/gpt-oss-120b",
			}, nil
		},
	}

	cascade := llm.NewCascadeLLMProvider(gemini, groq)
	resp, err := cascade.Generate(context.Background(), llm.LLMRequest{UserQuery: "test"})
	if err != nil {
		t.Fatalf("expected Groq failover success, got error: %v", err)
	}
	if resp.Provider != "groq" {
		t.Errorf("expected provider groq, got %s", resp.Provider)
	}
	if !gemini.called || !groq.called {
		t.Errorf("expected both Gemini and Groq to be called in failover sequence")
	}
}

func TestCascadeLLMProvider_Test3_Gemini503_GroqSucceeds(t *testing.T) {
	gemini := &mockProvider{
		name:  "gemini",
		model: "gemini-3.8-flash",
		generateFn: func(ctx context.Context, req llm.LLMRequest) (*llm.LLMResponse, error) {
			return nil, fmtError("Gemini API returned HTTP 503")
		},
	}
	groq := &mockProvider{
		name:  "groq",
		model: "openai/gpt-oss-120b",
	}

	cascade := llm.NewCascadeLLMProvider(gemini, groq)
	resp, err := cascade.Generate(context.Background(), llm.LLMRequest{UserQuery: "test"})
	if err != nil {
		t.Fatalf("expected Groq failover success, got error: %v", err)
	}
	if resp.Provider != "groq" {
		t.Errorf("expected provider groq, got %s", resp.Provider)
	}
}

func TestCascadeLLMProvider_Test4_Gemini429_Groq429_ReturnsProviderError(t *testing.T) {
	gemini := &mockProvider{
		name:  "gemini",
		model: "gemini-3.8-flash",
		generateFn: func(ctx context.Context, req llm.LLMRequest) (*llm.LLMResponse, error) {
			return nil, fmtError("Gemini rate limit exceeded (HTTP 429)")
		},
	}
	groq := &mockProvider{
		name:  "groq",
		model: "openai/gpt-oss-120b",
		generateFn: func(ctx context.Context, req llm.LLMRequest) (*llm.LLMResponse, error) {
			return nil, fmtError("Groq rate limit exceeded (HTTP 429)")
		},
	}

	cascade := llm.NewCascadeLLMProvider(gemini, groq)
	_, err := cascade.Generate(context.Background(), llm.LLMRequest{UserQuery: "test"})
	if err == nil {
		t.Fatalf("expected error when both providers fail")
	}
	if !errors.Is(err, llm.ErrProviderUnavailable) {
		t.Errorf("expected ErrProviderUnavailable, got %v", err)
	}
	if !strings.Contains(err.Error(), "all LLM providers failed") {
		t.Errorf("expected provider error message, got %s", err.Error())
	}
}

func TestCascadeLLMProvider_Test5_Gemini500_Groq500_ReturnsProviderError(t *testing.T) {
	gemini := &mockProvider{
		name:  "gemini",
		model: "gemini-3.8-flash",
		generateFn: func(ctx context.Context, req llm.LLMRequest) (*llm.LLMResponse, error) {
			return nil, fmtError("HTTP 500 internal server error")
		},
	}
	groq := &mockProvider{
		name:  "groq",
		model: "openai/gpt-oss-120b",
		generateFn: func(ctx context.Context, req llm.LLMRequest) (*llm.LLMResponse, error) {
			return nil, fmtError("HTTP 500 internal server error")
		},
	}

	cascade := llm.NewCascadeLLMProvider(gemini, groq)
	_, err := cascade.Generate(context.Background(), llm.LLMRequest{UserQuery: "test"})
	if err == nil {
		t.Fatalf("expected error when both providers fail")
	}
	if !errors.Is(err, llm.ErrProviderUnavailable) {
		t.Errorf("expected ErrProviderUnavailable, got %v", err)
	}
}

func TestCascadeLLMProvider_Test6_GeminiCitationValidationFailed(t *testing.T) {
	gemini := &mockProvider{
		name:  "gemini",
		model: "gemini-3.8-flash",
		generateFn: func(ctx context.Context, req llm.LLMRequest) (*llm.LLMResponse, error) {
			return &llm.LLMResponse{
				Content:  "Answer with bogus citation [E999].",
				Provider: "gemini",
				Model:    "gemini-3.8-flash",
			}, nil
		},
	}

	svc := llm.NewGroundedExplanationService(gemini, nil, nil, nil)
	pkg := &models.EvidencePackage{
		RepositoryID: "repo1",
		Question:     "q",
		Sufficiency:  models.EvidenceSufficiencyResult{Status: models.SufficiencySufficient},
		Items:        []*models.EvidenceItem{{Label: "E1", StableID: "E1"}},
	}
	res, err := svc.Explain(context.Background(), models.RepositoryScope{RepositoryID: "repo1"}, pkg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.CitationValidationStatus == "GROUNDED" {
		t.Errorf("expected citation validation to NOT report GROUNDED for invalid citation E999")
	}
}

func TestCascadeLLMProvider_Test7_GroqFailoverCitationValidationFailed(t *testing.T) {
	gemini := &mockProvider{
		name:  "gemini",
		model: "gemini-3.8-flash",
		generateFn: func(ctx context.Context, req llm.LLMRequest) (*llm.LLMResponse, error) {
			return nil, fmtError("Gemini 429")
		},
	}
	groq := &mockProvider{
		name:  "groq",
		model: "openai/gpt-oss-120b",
		generateFn: func(ctx context.Context, req llm.LLMRequest) (*llm.LLMResponse, error) {
			return &llm.LLMResponse{
				Content:  "Groq answer with no citations at all.",
				Provider: "groq",
				Model:    "openai/gpt-oss-120b",
			}, nil
		},
	}

	cascade := llm.NewCascadeLLMProvider(gemini, groq)
	svc := llm.NewGroundedExplanationService(cascade, nil, nil, nil)
	pkg := &models.EvidencePackage{
		RepositoryID: "repo1",
		Question:     "q",
		Sufficiency:  models.EvidenceSufficiencyResult{Status: models.SufficiencySufficient},
		Items:        []*models.EvidenceItem{{Label: "E1", StableID: "E1"}},
	}
	res, err := svc.Explain(context.Background(), models.RepositoryScope{RepositoryID: "repo1"}, pkg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Provider != "groq" {
		t.Errorf("expected provider groq, got %s", res.Provider)
	}
	if res.CitationValidationStatus == "ALL_CITATIONS_VALID" {
		t.Errorf("expected NO_CITATIONS_PRESENT, got %s", res.CitationValidationStatus)
	}
}

func TestCascadeLLMProvider_Test8_OnlyGroqConfigured(t *testing.T) {
	groq := &mockProvider{name: "groq", model: "openai/gpt-oss-120b"}
	cascade := llm.NewCascadeLLMProvider(groq)

	resp, err := cascade.Generate(context.Background(), llm.LLMRequest{UserQuery: "test"})
	if err != nil {
		t.Fatalf("expected success with Groq only, got error: %v", err)
	}
	if resp.Provider != "groq" {
		t.Errorf("expected provider groq, got %s", resp.Provider)
	}
}

func TestCascadeLLMProvider_Test9_OnlyGeminiConfigured(t *testing.T) {
	gemini := &mockProvider{name: "gemini", model: "gemini-3.8-flash"}
	cascade := llm.NewCascadeLLMProvider(gemini)

	resp, err := cascade.Generate(context.Background(), llm.LLMRequest{UserQuery: "test"})
	if err != nil {
		t.Fatalf("expected success with Gemini only, got error: %v", err)
	}
	if resp.Provider != "gemini" {
		t.Errorf("expected provider gemini, got %s", resp.Provider)
	}
}

func TestCascadeLLMProvider_Test10_NoProvidersConfigured(t *testing.T) {
	cascade := llm.NewCascadeLLMProvider()
	_, err := cascade.Generate(context.Background(), llm.LLMRequest{UserQuery: "test"})
	if err == nil {
		t.Fatalf("expected error when no providers configured")
	}
	if !errors.Is(err, llm.ErrInvalidConfiguration) {
		t.Errorf("expected ErrInvalidConfiguration, got %v", err)
	}
}

func fmtError(msg string) error {
	return errors.New(msg)
}

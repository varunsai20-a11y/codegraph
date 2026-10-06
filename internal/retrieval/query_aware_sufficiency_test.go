package retrieval_test

import (
	"context"
	"errors"
	"testing"

	"codegraph/internal/llm"
	"codegraph/internal/models"
	"codegraph/internal/retrieval"
)

type testDummyRetriever struct {
	items []*models.EvidenceItem
}

func (d *testDummyRetriever) Retrieve(ctx context.Context, scope models.RepositoryScope, query string, intent string, limit int) ([]*models.EvidenceItem, error) {
	var res []*models.EvidenceItem
	for _, item := range d.items {
		if item.RepositoryID == scope.RepositoryID {
			res = append(res, item)
		}
	}
	return res, nil
}

type testMockLLMProvider struct {
	name  string
	model string
	fn    func(ctx context.Context, req llm.LLMRequest) (*llm.LLMResponse, error)
}

func (m *testMockLLMProvider) Name() string  { return m.name }
func (m *testMockLLMProvider) Model() string { return m.model }
func (m *testMockLLMProvider) Generate(ctx context.Context, req llm.LLMRequest) (*llm.LLMResponse, error) {
	if m.fn != nil {
		return m.fn(ctx, req)
	}
	return &llm.LLMResponse{
		Content:  "Grounded explanation response citing [E1].",
		Provider: m.name,
		Model:    m.model,
	}, nil
}

// 1. Small file + file-specific query -> sufficient evidence
func TestQueryAwareSufficiency_SmallFileFileSpecificQuery(t *testing.T) {
	evaluator := retrieval.NewDefaultSufficiencyEvaluator()
	scope, _ := models.NewRepositoryScope("repo-small")

	// postcss.config.js - small file snippet, 0 symbols, 0 graph edges
	items := []*models.EvidenceItem{
		{
			RepositoryID:  "repo-small",
			Type:          models.EvidenceTypeCodeSnippet,
			RelativePath:  "postcss.config.js",
			Content:       "module.exports = { plugins: { tailwindcss: {}, autoprefixer: {} } }",
			RetrieverType: "TARGET_FILE",
		},
	}

	queries := []string{
		"Explain postcss.config.js",
		"Explain the architectural responsibilities and key symbols of file postcss.config.js.",
		"What does postcss.config.js do?",
	}

	for _, q := range queries {
		res, err := evaluator.Evaluate(context.Background(), scope, q, "EXPLANATION", items)
		if err != nil {
			t.Fatalf("unexpected error for query '%s': %v", q, err)
		}
		if res.Status != models.SufficiencySufficient {
			t.Errorf("query '%s': expected SUFFICIENCY_SUFFICIENT, got %s (reasons: %v)", q, res.Status, res.Reasons)
		}
		if !res.TargetFound {
			t.Errorf("query '%s': expected TargetFound = true", q)
		}
	}
}

// 2. Small file + architecture-wide query -> insufficient unless broader repository evidence is retrieved
func TestQueryAwareSufficiency_SmallFileArchitectureQuery(t *testing.T) {
	evaluator := retrieval.NewDefaultSufficiencyEvaluator()
	scope, _ := models.NewRepositoryScope("repo-small")

	// Small file ONLY
	smallFileOnly := []*models.EvidenceItem{
		{
			RepositoryID:  "repo-small",
			Type:          models.EvidenceTypeCodeSnippet,
			RelativePath:  "postcss.config.js",
			Content:       "module.exports = { plugins: {} }",
			RetrieverType: "LEXICAL",
		},
	}

	archQuery := "Explain the complete architecture of this repository"

	// Case A: Small file ONLY -> Insufficient
	resA, err := evaluator.Evaluate(context.Background(), scope, archQuery, "ARCHITECTURE_QUERY", smallFileOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resA.Status != models.SufficiencyInsufficient {
		t.Errorf("expected SUFFICIENCY_INSUFFICIENT when architecture query only has small config file, got %s", resA.Status)
	}

	// Case B: Broader repository evidence added (README + entry point) -> Sufficient
	broaderEvidence := append(smallFileOnly,
		&models.EvidenceItem{
			RepositoryID:  "repo-small",
			Type:          models.EvidenceTypeDocumentation,
			RelativePath:  "README.md",
			Content:       "# Project Overview\nThis is a Next.js web application.",
			RetrieverType: "SYSTEM_CONTEXT_README",
		},
		&models.EvidenceItem{
			RepositoryID:  "repo-small",
			Type:          models.EvidenceTypeCodeSnippet,
			RelativePath:  "main.go",
			Content:       "package main\nfunc main() { startServer() }",
			RetrieverType: "SYSTEM_CONTEXT_ENTRYPOINT",
		},
	)

	resB, err := evaluator.Evaluate(context.Background(), scope, archQuery, "ARCHITECTURE_QUERY", broaderEvidence)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resB.Status != models.SufficiencySufficient {
		t.Errorf("expected SUFFICIENCY_SUFFICIENT when architecture query has README & entrypoint, got %s", resB.Status)
	}
}

// 3. Function query -> symbol and call relationships prioritized
func TestQueryAwareSufficiency_FunctionQuery(t *testing.T) {
	evaluator := retrieval.NewDefaultSufficiencyEvaluator()
	scope, _ := models.NewRepositoryScope("repo-fn")

	items := []*models.EvidenceItem{
		{
			RepositoryID:  "repo-fn",
			Type:          models.EvidenceTypeSymbol,
			RelativePath:  "auth/service.go",
			Content:       "Symbol AuthenticateUser (Function) defined in auth/service.go",
			RetrieverType: "TARGET_SYMBOL",
		},
		{
			RepositoryID:  "repo-fn",
			Type:          models.EvidenceTypeCodeSnippet,
			RelativePath:  "auth/service.go",
			Content:       "func AuthenticateUser(ctx context.Context, token string) (*User, error)",
			RetrieverType: "LEXICAL",
		},
	}

	q := "Where is function AuthenticateUser defined?"
	res, err := evaluator.Evaluate(context.Background(), scope, q, "SYMBOL_LOOKUP", items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != models.SufficiencySufficient {
		t.Errorf("expected SUFFICIENCY_SUFFICIENT for symbol lookup, got %s", res.Status)
	}
	if !res.TargetFound {
		t.Errorf("expected TargetFound = true")
	}
}

// 4. Project-purpose query -> README/overview + architecture evidence prioritized
func TestQueryAwareSufficiency_ProjectPurposeQuery(t *testing.T) {
	evaluator := retrieval.NewDefaultSufficiencyEvaluator()
	scope, _ := models.NewRepositoryScope("repo-proj")

	items := []*models.EvidenceItem{
		{
			RepositoryID:  "repo-proj",
			Type:          models.EvidenceTypeDocumentation,
			RelativePath:  "README.md",
			Content:       "CodeGraph: Code Intelligence and Graph Analytics Engine",
			RetrieverType: "SYSTEM_CONTEXT_README",
		},
		{
			RepositoryID:  "repo-proj",
			Type:          models.EvidenceTypeCodeSnippet,
			RelativePath:  "package.json",
			Content:       `{ "name": "codegraph-app", "version": "1.0.0" }`,
			RetrieverType: "SYSTEM_CONTEXT_SETUP",
		},
	}

	q := "What does this project do?"
	res, err := evaluator.Evaluate(context.Background(), scope, q, "ARCHITECTURE_QUERY", items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != models.SufficiencySufficient {
		t.Errorf("expected SUFFICIENCY_SUFFICIENT for project purpose query with README/manifest, got %s", res.Status)
	}
}

// 5. Flow query -> connected path evidence prioritized
func TestQueryAwareSufficiency_FlowQuery(t *testing.T) {
	evaluator := retrieval.NewDefaultSufficiencyEvaluator()
	scope, _ := models.NewRepositoryScope("repo-flow")

	items := []*models.EvidenceItem{
		{
			RepositoryID:  "repo-flow",
			Type:          models.EvidenceTypeStaticFlow,
			RelativePath:  "handler.go",
			Content:       "Flow Step 1: Handler calls AuthenticateUser",
			RetrieverType: "STATIC_FLOW",
		},
	}

	q := "Trace the request flow for authentication"
	res, err := evaluator.Evaluate(context.Background(), scope, q, "TRACE", items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != models.SufficiencySufficient {
		t.Errorf("expected SUFFICIENCY_SUFFICIENT for flow query, got %s", res.Status)
	}
}

// 6. Truly insufficient repository evidence -> still returns RETRIEVAL_INSUFFICIENT
func TestQueryAwareSufficiency_TrulyInsufficientEvidence(t *testing.T) {
	evaluator := retrieval.NewDefaultSufficiencyEvaluator()
	scope, _ := models.NewRepositoryScope("repo-none")

	// Irrelevant items that don't match quantum encryption or any requested target
	items := []*models.EvidenceItem{
		{
			RepositoryID:  "repo-none",
			Type:          models.EvidenceTypeCodeSnippet,
			RelativePath:  "utils/math.go",
			Content:       "func Add(a, b int) int { return a + b }",
			RetrieverType: "LEXICAL",
		},
	}

	q := "Where is quantum encryption implemented?"
	res, err := evaluator.Evaluate(context.Background(), scope, q, "FEATURE_SEARCH", items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != models.SufficiencyInsufficient {
		t.Errorf("expected SUFFICIENCY_INSUFFICIENT for unknown feature query, got %s", res.Status)
	}
}

// 7. Sufficient evidence -> real LLM is invoked
func TestQueryAwareSufficiency_RealLLMInvoked(t *testing.T) {
	scope, _ := models.NewRepositoryScope("repo-llm")
	item := &models.EvidenceItem{
		StableID:     "s1",
		Label:        "E1",
		RepositoryID: "repo-llm",
		Type:         models.EvidenceTypeCodeSnippet,
		RelativePath: "postcss.config.js",
		Content:      "module.exports = { plugins: {} }",
	}

	llmCalled := false
	prov := &testMockLLMProvider{
		name:  "gemini",
		model: "gemini-3.8-flash",
		fn: func(ctx context.Context, req llm.LLMRequest) (*llm.LLMResponse, error) {
			llmCalled = true
			return &llm.LLMResponse{
				Content:  "PostCSS config exports Tailwind and Autoprefixer plugins [E1].",
				Provider: "gemini",
				Model:    "gemini-3.8-flash",
			}, nil
		},
	}

	composer := retrieval.NewDefaultEvidenceComposer(&testDummyRetriever{items: []*models.EvidenceItem{item}}, nil, nil)
	svc := llm.NewGroundedExplanationService(prov, nil, composer, nil)

	req, _ := models.NewExplanationRequest(scope, "Explain postcss.config.js")
	resp, err := svc.ExplainRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !llmCalled {
		t.Errorf("expected LLM provider Generate to be invoked when evidence is sufficient")
	}
	if resp.Status != models.ExplanationStatusSuccess {
		t.Errorf("expected EXPLANATION_SUCCESS, got %s", resp.Status)
	}
}

// 8. Gemini 429 -> Groq receives the exact same grounded evidence package
func TestQueryAwareSufficiency_Gemini429_GroqFailoverSamePackage(t *testing.T) {
	var geminiPayload, groqPayload string

	gemini := &testMockLLMProvider{
		name:  "gemini",
		model: "gemini-3.8-flash",
		fn: func(ctx context.Context, req llm.LLMRequest) (*llm.LLMResponse, error) {
			geminiPayload = req.GroundedContext
			return nil, errors.New("Gemini HTTP 429 rate limit exceeded")
		},
	}
	groq := &testMockLLMProvider{
		name:  "groq",
		model: "openai/gpt-oss-120b",
		fn: func(ctx context.Context, req llm.LLMRequest) (*llm.LLMResponse, error) {
			groqPayload = req.GroundedContext
			return &llm.LLMResponse{
				Content:  "Groq failover response citing [E1].",
				Provider: "groq",
				Model:    "openai/gpt-oss-120b",
			}, nil
		},
	}

	cascade := llm.NewCascadeLLMProvider(gemini, groq)
	scope, _ := models.NewRepositoryScope("repo-cascade")
	item := &models.EvidenceItem{
		StableID:     "s1",
		Label:        "E1",
		RepositoryID: "repo-cascade",
		Type:         models.EvidenceTypeCodeSnippet,
		RelativePath: "config.go",
		Content:      "type Config struct{}",
	}

	composer := retrieval.NewDefaultEvidenceComposer(&testDummyRetriever{items: []*models.EvidenceItem{item}}, nil, nil)
	svc := llm.NewGroundedExplanationService(cascade, nil, composer, nil)

	req, _ := models.NewExplanationRequest(scope, "Explain config.go")
	resp, err := svc.ExplainRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error on Groq failover: %v", err)
	}

	if resp.Provider != "groq" {
		t.Errorf("expected provider groq, got %s", resp.Provider)
	}
	if geminiPayload == "" || groqPayload == "" || geminiPayload != groqPayload {
		t.Errorf("expected Groq to receive exact same grounded context payload as Gemini")
	}
}

// 9. Both providers fail -> LLM_PROVIDER_ERROR, never deterministic fallback
func TestQueryAwareSufficiency_BothProvidersFail_NoDeterministicFallback(t *testing.T) {
	gemini := &testMockLLMProvider{
		name: "gemini",
		fn: func(ctx context.Context, req llm.LLMRequest) (*llm.LLMResponse, error) {
			return nil, errors.New("Gemini 500 internal server error")
		},
	}
	groq := &testMockLLMProvider{
		name: "groq",
		fn: func(ctx context.Context, req llm.LLMRequest) (*llm.LLMResponse, error) {
			return nil, errors.New("Groq 500 internal server error")
		},
	}

	cascade := llm.NewCascadeLLMProvider(gemini, groq)
	scope, _ := models.NewRepositoryScope("repo-fail")
	item := &models.EvidenceItem{
		StableID:     "s1",
		Label:        "E1",
		RepositoryID: "repo-fail",
		Type:         models.EvidenceTypeCodeSnippet,
		RelativePath: "config.go",
		Content:      "type Config struct{}",
	}

	composer := retrieval.NewDefaultEvidenceComposer(&testDummyRetriever{items: []*models.EvidenceItem{item}}, nil, nil)
	svc := llm.NewGroundedExplanationService(cascade, nil, composer, nil)

	req, _ := models.NewExplanationRequest(scope, "Explain config.go")
	resp, err := svc.ExplainRequest(context.Background(), req)

	// Must fail with provider error, NEVER produce deterministic explanation
	if err == nil {
		t.Fatalf("expected error when both providers fail, got nil response: status=%s, answer=%s", resp.Status, resp.Answer)
	}
	if !errors.Is(err, llm.ErrProviderUnavailable) {
		t.Errorf("expected ErrProviderUnavailable, got %v", err)
	}
}

// 10. Citation validation remains enforced
func TestQueryAwareSufficiency_CitationValidationEnforced(t *testing.T) {
	scope, _ := models.NewRepositoryScope("repo-cite")
	item := &models.EvidenceItem{
		StableID:     "s1",
		Label:        "E1",
		RepositoryID: "repo-cite",
		Type:         models.EvidenceTypeCodeSnippet,
		RelativePath: "service.go",
		Content:      "func Process()",
	}

	// LLM returns invalid citation E99
	prov := &testMockLLMProvider{
		name: "gemini",
		fn: func(ctx context.Context, req llm.LLMRequest) (*llm.LLMResponse, error) {
			return &llm.LLMResponse{
				Content:  "Process executes tasks citing invalid [E99].",
				Provider: "gemini",
			}, nil
		},
	}

	composer := retrieval.NewDefaultEvidenceComposer(&testDummyRetriever{items: []*models.EvidenceItem{item}}, nil, nil)
	svc := llm.NewGroundedExplanationService(prov, nil, composer, nil)

	req, _ := models.NewExplanationRequest(scope, "Explain service.go")
	resp, err := svc.ExplainRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Grounding.CitationValidationStatus == "VALIDATION_PASSED" {
		t.Errorf("citation validation must enforce invalid citation detection and NOT return VALIDATION_PASSED for E99")
	}
	if resp.Grounding.UnsupportedClaimCount < 1 && resp.CitationValidationStatus == "VALIDATION_PASSED" {
		t.Errorf("expected invalid citation or unsupported claim count for E99")
	}
}

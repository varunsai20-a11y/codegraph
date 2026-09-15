package llm

import (
	"context"
	"sync"
)

// MockLLMProvider is a deterministic mock provider for testing LLM integration.
type MockLLMProvider struct {
	mu             sync.Mutex
	name           string
	model          string
	cannedResponse string
	err            error
	requests       []LLMRequest
}

func NewMockLLMProvider(cannedResponse string, err error) *MockLLMProvider {
	return &MockLLMProvider{
		name:           "mock",
		model:          "mock-model-v1",
		cannedResponse: cannedResponse,
		err:            err,
		requests:       make([]LLMRequest, 0),
	}
}

func (m *MockLLMProvider) Name() string {
	return m.name
}

func (m *MockLLMProvider) Model() string {
	return m.model
}

func (m *MockLLMProvider) SetCannedResponse(response string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cannedResponse = response
}

func (m *MockLLMProvider) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

func (m *MockLLMProvider) Generate(ctx context.Context, req LLMRequest) (*LLMResponse, error) {
	m.mu.Lock()
	m.requests = append(m.requests, req)
	canned := m.cannedResponse
	err := m.err
	provider := m.name
	model := m.model
	m.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if err != nil {
		return nil, err
	}

	promptToks := (len(req.SystemInstruction) + len(req.UserQuery) + len(req.GroundedContext) + 3) / 4
	compToks := (len(canned) + 3) / 4

	return &LLMResponse{
		Content:  canned,
		Provider: provider,
		Model:    model,
		Usage: TokenUsage{
			PromptTokens:     promptToks,
			CompletionTokens: compToks,
			TotalTokens:      promptToks + compToks,
		},
	}, nil
}

func (m *MockLLMProvider) Requests() []LLMRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]LLMRequest, len(m.requests))
	copy(cp, m.requests)
	return cp
}

func (m *MockLLMProvider) LastRequest() (LLMRequest, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.requests) == 0 {
		return LLMRequest{}, false
	}
	return m.requests[len(m.requests)-1], true
}

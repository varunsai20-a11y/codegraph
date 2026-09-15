package llm

import (
	"context"
	"errors"
	"time"
)

var (
	ErrProviderUnavailable  = errors.New("llm provider unavailable")
	ErrProviderTimeout      = errors.New("llm provider request timed out")
	ErrInvalidConfiguration = errors.New("invalid llm provider configuration")
)

// TokenUsage holds prompt, completion, and total token usage metrics.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// LLMRequest represents a request sent to an LLMProvider.
type LLMRequest struct {
	SystemInstruction string `json:"system_instruction"`
	UserQuery         string `json:"user_query"`
	GroundedContext   string `json:"grounded_context"`
}

// LLMResponse represents the output received from an LLMProvider.
type LLMResponse struct {
	Content  string     `json:"content"`
	Provider string     `json:"provider"`
	Model    string     `json:"model"`
	Usage    TokenUsage `json:"usage"`
}

// LLMConfig specifies configuration settings for an LLMProvider.
type LLMConfig struct {
	Provider string        `json:"provider"`
	Model    string        `json:"model"`
	Endpoint string        `json:"endpoint,omitempty"`
	APIKey   string        `json:"-"` // Never serialized
	Timeout  time.Duration `json:"timeout"`
}

func DefaultLLMConfig() LLMConfig {
	return LLMConfig{
		Provider: "mock",
		Model:    "mock-v1",
		Timeout:  30 * time.Second,
	}
}

func (c LLMConfig) Validate() error {
	if c.Provider == "" {
		return ErrInvalidConfiguration
	}
	if c.Timeout <= 0 {
		return ErrInvalidConfiguration
	}
	return nil
}

// LLMProvider defines the contract for all LLM providers (Mock, HTTP/API, Gemini, OpenAI, Anthropic, Ollama).
type LLMProvider interface {
	Name() string
	Model() string
	Generate(ctx context.Context, req LLMRequest) (*LLMResponse, error)
}

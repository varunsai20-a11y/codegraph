package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ProviderProtocol defines the contract for wire-protocol request formatting and response parsing.
type ProviderProtocol interface {
	FormatRequest(ctx context.Context, config LLMConfig, req LLMRequest) (*http.Request, error)
	ParseResponse(config LLMConfig, resp *http.Response) (*LLMResponse, error)
}

// HTTPLLMProvider is a configuration-driven HTTP REST provider boundary supporting multiple LLM wire protocols.
type HTTPLLMProvider struct {
	config   LLMConfig
	client   *http.Client
	protocol ProviderProtocol
}

func NewHTTPLLMProvider(config LLMConfig, client *http.Client) (*HTTPLLMProvider, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if client == nil {
		client = &http.Client{
			Timeout: config.Timeout,
		}
	}

	var protocol ProviderProtocol
	switch strings.ToLower(config.Provider) {
	case "gemini":
		protocol = &GeminiAdapter{}
	case "anthropic":
		protocol = &AnthropicAdapter{}
	case "openai", "ollama":
		protocol = &OpenAIAdapter{}
	default:
		protocol = &OpenAIAdapter{}
	}

	return &HTTPLLMProvider{
		config:   config,
		client:   client,
		protocol: protocol,
	}, nil
}

func (p *HTTPLLMProvider) Name() string {
	return p.config.Provider
}

func (p *HTTPLLMProvider) Model() string {
	return p.config.Model
}

func (p *HTTPLLMProvider) Generate(ctx context.Context, req LLMRequest) (*LLMResponse, error) {
	if p.config.Endpoint == "" {
		return nil, fmt.Errorf("%w: endpoint URL missing for HTTP provider %s", ErrInvalidConfiguration, p.config.Provider)
	}

	httpReq, err := p.protocol.FormatRequest(ctx, p.config, req)
	if err != nil {
		return nil, fmt.Errorf("failed to format request for %s: %w", p.config.Provider, err)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w: %v", ErrProviderTimeout, ctx.Err())
		}
		// Return safe sanitized error without exposing raw request URL query tokens or secrets
		return nil, fmt.Errorf("%w: provider %s HTTP request failed", ErrProviderUnavailable, p.config.Provider)
	}
	defer resp.Body.Close()

	return p.protocol.ParseResponse(p.config, resp)
}

// OpenAIAdapter handles OpenAI & Ollama REST chat/completions wire protocol.
type OpenAIAdapter struct{}

func (a *OpenAIAdapter) FormatRequest(ctx context.Context, config LLMConfig, req LLMRequest) (*http.Request, error) {
	payload := map[string]interface{}{
		"model": config.Model,
		"messages": []map[string]string{
			{"role": "system", "content": req.SystemInstruction},
			{"role": "user", "content": fmt.Sprintf("Query: %s\n\n%s", req.UserQuery, req.GroundedContext)},
		},
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, config.Endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if config.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+config.APIKey)
	}
	return httpReq, nil
}

func (a *OpenAIAdapter) ParseResponse(config LLMConfig, resp *http.Response) (*LLMResponse, error) {
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: provider %s returned status %d", ErrProviderUnavailable, config.Provider, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("%w: invalid JSON response format from provider %s", ErrProviderUnavailable, config.Provider)
	}
	return &LLMResponse{
		Content:  parsed.Choices[0].Message.Content,
		Provider: config.Provider,
		Model:    config.Model,
		Usage: TokenUsage{
			PromptTokens:     parsed.Usage.PromptTokens,
			CompletionTokens: parsed.Usage.CompletionTokens,
			TotalTokens:      parsed.Usage.TotalTokens,
		},
	}, nil
}

// GeminiAdapter handles Google Gemini AI Studio REST generateContent wire protocol.
type GeminiAdapter struct{}

func (a *GeminiAdapter) FormatRequest(ctx context.Context, config LLMConfig, req LLMRequest) (*http.Request, error) {
	payload := map[string]interface{}{
		"systemInstruction": map[string]interface{}{
			"parts": []map[string]string{{"text": req.SystemInstruction}},
		},
		"contents": []map[string]interface{}{
			{
				"role":  "user",
				"parts": []map[string]string{{"text": fmt.Sprintf("Query: %s\n\n%s", req.UserQuery, req.GroundedContext)}},
			},
		},
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, config.Endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if config.APIKey != "" {
		httpReq.Header.Set("x-goog-api-key", config.APIKey)
	}
	return httpReq, nil
}

func (a *GeminiAdapter) ParseResponse(config LLMConfig, resp *http.Response) (*LLMResponse, error) {
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: provider %s returned status %d", ErrProviderUnavailable, config.Provider, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("%w: invalid Gemini JSON response format", ErrProviderUnavailable)
	}
	return &LLMResponse{
		Content:  parsed.Candidates[0].Content.Parts[0].Text,
		Provider: config.Provider,
		Model:    config.Model,
		Usage: TokenUsage{
			PromptTokens:     parsed.UsageMetadata.PromptTokenCount,
			CompletionTokens: parsed.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      parsed.UsageMetadata.TotalTokenCount,
		},
	}, nil
}

// AnthropicAdapter handles Anthropic Messages API wire protocol.
type AnthropicAdapter struct{}

func (a *AnthropicAdapter) FormatRequest(ctx context.Context, config LLMConfig, req LLMRequest) (*http.Request, error) {
	payload := map[string]interface{}{
		"model":      config.Model,
		"system":     req.SystemInstruction,
		"max_tokens": 4096,
		"messages": []map[string]string{
			{"role": "user", "content": fmt.Sprintf("Query: %s\n\n%s", req.UserQuery, req.GroundedContext)},
		},
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, config.Endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	if config.APIKey != "" {
		httpReq.Header.Set("x-api-key", config.APIKey)
	}
	return httpReq, nil
}

func (a *AnthropicAdapter) ParseResponse(config LLMConfig, resp *http.Response) (*LLMResponse, error) {
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: provider %s returned status %d", ErrProviderUnavailable, config.Provider, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Content) == 0 {
		return nil, fmt.Errorf("%w: invalid Anthropic JSON response format", ErrProviderUnavailable)
	}
	return &LLMResponse{
		Content:  parsed.Content[0].Text,
		Provider: config.Provider,
		Model:    config.Model,
		Usage: TokenUsage{
			PromptTokens:     parsed.Usage.InputTokens,
			CompletionTokens: parsed.Usage.OutputTokens,
			TotalTokens:      parsed.Usage.InputTokens + parsed.Usage.OutputTokens,
		},
	}, nil
}

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
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
	case "ollama":
		protocol = &OllamaAdapter{}
	case "openai":
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

	log.Printf("[LLM] Starting request to provider=%s model=%s endpoint=%s (timeout=%v)", p.config.Provider, p.config.Model, p.config.Endpoint, p.config.Timeout)

	var lastErr error
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("%w: %v", ErrProviderTimeout, ctx.Err())
			case <-time.After(time.Duration(attempt) * 1500 * time.Millisecond):
			}
		}

		httpReq, err := p.protocol.FormatRequest(ctx, p.config, req)
		if err != nil {
			return nil, fmt.Errorf("failed to format request for %s: %w", p.config.Provider, err)
		}

		genStart := time.Now()
		resp, err := p.client.Do(httpReq)
		genDuration := time.Since(genStart)

		if err != nil {
			log.Printf("[LLM] Provider %s HTTP request failed after %v: %v", p.config.Provider, genDuration, err)
			if ctx.Err() != nil {
				return nil, fmt.Errorf("%w: %v", ErrProviderTimeout, ctx.Err())
			}
			errStr := err.Error()
			if errors.Is(err, context.DeadlineExceeded) || strings.Contains(errStr, "Client.Timeout") || strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded") {
				return nil, fmt.Errorf("%w: %v", ErrProviderTimeout, err)
			}
			lastErr = fmt.Errorf("%w: provider %s HTTP request failed: %v", ErrProviderUnavailable, p.config.Provider, err)
			continue
		}

		llmResp, err := p.protocol.ParseResponse(p.config, resp)
		resp.Body.Close()
		if err != nil {
			if strings.Contains(err.Error(), "503") || strings.Contains(err.Error(), "429") {
				lastErr = fmt.Errorf("%w: %w", ErrProviderUnavailable, err)
				continue
			}
			return nil, err
		}

		log.Printf("[LLM] Provider %s completed in %v (prompt_tokens=%d completion_tokens=%d total_tokens=%d)",
			p.config.Provider, genDuration, llmResp.Usage.PromptTokens, llmResp.Usage.CompletionTokens, llmResp.Usage.TotalTokens)

		return llmResp, nil
	}

	return nil, lastErr
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
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)

		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, fmt.Errorf("%w (HTTP %d)", ErrAuthFailed, resp.StatusCode)
		case http.StatusNotFound:
			return nil, fmt.Errorf("%w: model '%s' unavailable (HTTP 404)", ErrModelUnavailable, config.Model)
		case http.StatusTooManyRequests:
			return nil, fmt.Errorf("%w (HTTP 429)", ErrRateLimited)
		default:
			if resp.StatusCode >= 500 {
				return nil, fmt.Errorf("%w: Gemini API returned HTTP %d", ErrProviderUnavailable, resp.StatusCode)
			}
			if strings.Contains(bodyStr, "not found") || strings.Contains(bodyStr, "no longer available") {
				return nil, fmt.Errorf("%w: model '%s' unavailable (HTTP %d)", ErrModelUnavailable, config.Model, resp.StatusCode)
			}
			return nil, fmt.Errorf("%w: Gemini API returned HTTP %d", ErrProviderUnavailable, resp.StatusCode)
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read response body: %v", ErrMalformedResponse, err)
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
		return nil, fmt.Errorf("%w: invalid or empty response candidates from Gemini API", ErrMalformedResponse)
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

// OllamaAdapter handles local Ollama REST chat API (/api/chat & /v1/chat/completions) wire protocol.
type OllamaAdapter struct{}

func (a *OllamaAdapter) FormatRequest(ctx context.Context, config LLMConfig, req LLMRequest) (*http.Request, error) {
	endpoint := config.Endpoint
	if endpoint == "" {
		endpoint = "http://localhost:11434/api/chat"
	} else if strings.HasSuffix(endpoint, "/") {
		endpoint = endpoint + "api/chat"
	} else if !strings.Contains(endpoint, "/api/") && !strings.Contains(endpoint, "/v1/") {
		endpoint = endpoint + "/api/chat"
	}

	model := config.Model
	if model == "" {
		model = "qwen2.5:3b"
	}

	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": req.SystemInstruction},
			{"role": "user", "content": fmt.Sprintf("Query: %s\n\n%s", req.UserQuery, req.GroundedContext)},
		},
		"stream": false,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	return httpReq, nil
}

func (a *OllamaAdapter) ParseResponse(config LLMConfig, resp *http.Response) (*LLMResponse, error) {
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: Ollama provider returned HTTP status %d: %s", ErrProviderUnavailable, resp.StatusCode, string(body))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read response from Ollama: %v", ErrMalformedResponse, err)
	}

	// 1. Try parsing Ollama native /api/chat format
	var nativeParsed struct {
		Model   string `json:"model"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		PromptEvalCount int    `json:"prompt_eval_count"`
		EvalCount       int    `json:"eval_count"`
		Done            bool   `json:"done"`
		Error           string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &nativeParsed); err == nil && nativeParsed.Message.Content != "" {
		if nativeParsed.Error != "" {
			return nil, fmt.Errorf("%w: Ollama error: %s", ErrProviderUnavailable, nativeParsed.Error)
		}
		return &LLMResponse{
			Content:  nativeParsed.Message.Content,
			Provider: "ollama",
			Model:    config.Model,
			Usage: TokenUsage{
				PromptTokens:     nativeParsed.PromptEvalCount,
				CompletionTokens: nativeParsed.EvalCount,
				TotalTokens:      nativeParsed.PromptEvalCount + nativeParsed.EvalCount,
			},
		}, nil
	}

	// 2. Try parsing OpenAI format (/v1/chat/completions)
	var openAIParsed struct {
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
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &openAIParsed); err == nil && len(openAIParsed.Choices) > 0 && openAIParsed.Choices[0].Message.Content != "" {
		return &LLMResponse{
			Content:  openAIParsed.Choices[0].Message.Content,
			Provider: "ollama",
			Model:    config.Model,
			Usage: TokenUsage{
				PromptTokens:     openAIParsed.Usage.PromptTokens,
				CompletionTokens: openAIParsed.Usage.CompletionTokens,
				TotalTokens:      openAIParsed.Usage.TotalTokens,
			},
		}, nil
	}

	return nil, fmt.Errorf("%w: invalid or empty response format from Ollama endpoint", ErrMalformedResponse)
}

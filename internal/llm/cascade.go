package llm

import (
	"context"
	"fmt"
	"log"
	"strings"
)

// CascadeLLMProvider implements a multi-provider LLM fallback cascade (e.g. Gemini -> Groq).
// It attempts generation using registered providers in sequential order.
// If a provider fails due to rate limits (429), capacity errors (503), timeouts, or server errors,
// it logs a safe diagnostic (without exposing secrets) and immediately fails over to the next provider
// using the exact same grounded LLMRequest payload (system instructions, user query, grounded context).
type CascadeLLMProvider struct {
	providers []LLMProvider
}

// NewCascadeLLMProvider constructs a new cascade provider from a list of LLMProviders.
func NewCascadeLLMProvider(providers ...LLMProvider) *CascadeLLMProvider {
	var active []LLMProvider
	for _, p := range providers {
		if p != nil {
			active = append(active, p)
		}
	}
	return &CascadeLLMProvider{providers: active}
}

func (c *CascadeLLMProvider) Name() string {
	if len(c.providers) > 0 {
		return c.providers[0].Name()
	}
	return "none"
}

func (c *CascadeLLMProvider) Model() string {
	if len(c.providers) > 0 {
		return c.providers[0].Model()
	}
	return "none"
}

func (c *CascadeLLMProvider) Providers() []LLMProvider {
	return c.providers
}

func (c *CascadeLLMProvider) Generate(ctx context.Context, req LLMRequest) (*LLMResponse, error) {
	if len(c.providers) == 0 {
		return nil, fmt.Errorf("%w: no LLM providers configured", ErrInvalidConfiguration)
	}

	var errorsList []string
	for idx, provider := range c.providers {
		resp, err := provider.Generate(ctx, req)
		if err == nil && resp != nil {
			if resp.Provider == "" {
				resp.Provider = provider.Name()
			}
			if resp.Model == "" {
				resp.Model = provider.Model()
			}
			if idx > 0 {
				log.Printf("[LLM Cascade] Primary provider failed. Successfully failover-generated response via provider: %s (model: %s)", resp.Provider, resp.Model)
			}
			return resp, nil
		}

		errStr := "unknown error"
		if err != nil {
			errStr = err.Error()
		}
		errorsList = append(errorsList, fmt.Sprintf("%s: %s", provider.Name(), errStr))
		log.Printf("[LLM Cascade] Provider %d (%s) failed: %v", idx+1, provider.Name(), errStr)
	}

	return nil, fmt.Errorf("%w: all LLM providers failed: %s", ErrProviderUnavailable, strings.Join(errorsList, "; "))
}

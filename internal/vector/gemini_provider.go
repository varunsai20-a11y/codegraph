package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GeminiEmbeddingProvider implements EmbeddingProvider for Google Gemini text-embedding-004 API.
type GeminiEmbeddingProvider struct {
	apiKey    string
	endpoint  string
	modelName string
	dimension int
	version   string
	client    *http.Client
}

func NewGeminiEmbeddingProvider(apiKey string, endpoint string) *GeminiEmbeddingProvider {
	if endpoint == "" {
		endpoint = "https://generativelanguage.googleapis.com/v1beta/models/gemini-embedding-001:embedContent"
	}
	return &GeminiEmbeddingProvider{
		apiKey:    apiKey,
		endpoint:  endpoint,
		modelName: "gemini-embedding-001",
		dimension: 3072,
		version:   "1.0.0",
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *GeminiEmbeddingProvider) SetHTTPClient(c *http.Client) {
	if c != nil {
		p.client = c
	}
}

func (p *GeminiEmbeddingProvider) ModelName() string { return p.modelName }
func (p *GeminiEmbeddingProvider) Dimension() int { return p.dimension }
func (p *GeminiEmbeddingProvider) Version() string   { return p.version }

func (p *GeminiEmbeddingProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		return make([]float32, p.dimension), nil
	}

	if p.apiKey == "" {
		return nil, errors.New("gemini api key is required for embedding generation")
	}

	payload := map[string]interface{}{
		"model": "models/" + p.modelName,
		"content": map[string]interface{}{
			"parts": []map[string]string{
				{"text": text},
			},
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embedding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create embedding request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("embedding provider timeout: %w", ctx.Err())
		}
		return nil, fmt.Errorf("gemini embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini embedding API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		Embedding struct {
			Values []float32 `json:"values"`
		} `json:"embedding"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to decode gemini embedding response: %w", err)
	}

	if len(parsed.Embedding.Values) == 0 {
		return nil, errors.New("empty embedding vector returned from gemini API")
	}

	return parsed.Embedding.Values, nil
}

func (p *GeminiEmbeddingProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, t := range texts {
		vec, err := p.Embed(ctx, t)
		if err != nil {
			return nil, fmt.Errorf("batch embedding failed at index %d: %w", i, err)
		}
		results[i] = vec
	}
	return results, nil
}

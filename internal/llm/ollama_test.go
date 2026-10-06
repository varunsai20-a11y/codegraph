package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"codegraph/internal/models"
)

func TestOllamaProvider_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/api/chat") {
			t.Errorf("expected /api/chat path, got %s", r.URL.Path)
		}

		resp := map[string]interface{}{
			"model": "qwen2.5:3b",
			"message": map[string]string{
				"role":    "assistant",
				"content": "### Explanation\nThis project provides static code intelligence [E1].\n\n### Evidence\n[E1] README.md (L1-L10) - Overview",
			},
			"prompt_eval_count": 50,
			"eval_count":        100,
			"done":              true,
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	cfg := LLMConfig{
		Provider: "ollama",
		Model:    "qwen2.5:3b",
		Endpoint: ts.URL + "/api/chat",
		Timeout:  5 * time.Second,
	}

	provider, err := NewHTTPLLMProvider(cfg, ts.Client())
	if err != nil {
		t.Fatalf("failed to create Ollama provider: %v", err)
	}

	if provider.Name() != "ollama" {
		t.Errorf("expected provider name 'ollama', got '%s'", provider.Name())
	}
	if provider.Model() != "qwen2.5:3b" {
		t.Errorf("expected model 'qwen2.5:3b', got '%s'", provider.Model())
	}

	req := LLMRequest{
		SystemInstruction: "System prompt",
		UserQuery:         "What does this project do?",
		GroundedContext:   "[E1] README.md (L1-L10)\nProject Overview",
	}

	resp, err := provider.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Ollama generation failed: %v", err)
	}

	if resp.Provider != "ollama" {
		t.Errorf("expected response provider 'ollama', got '%s'", resp.Provider)
	}
	if resp.Model != "qwen2.5:3b" {
		t.Errorf("expected response model 'qwen2.5:3b', got '%s'", resp.Model)
	}
	if !strings.Contains(resp.Content, "### Explanation") {
		t.Errorf("expected content to contain '### Explanation', got: %s", resp.Content)
	}
}

func TestOllamaProvider_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"model 'qwen2.5:3b' not found"}`, http.StatusNotFound)
	}))
	defer ts.Close()

	cfg := LLMConfig{
		Provider: "ollama",
		Model:    "qwen2.5:3b",
		Endpoint: ts.URL + "/api/chat",
		Timeout:  2 * time.Second,
	}

	provider, err := NewHTTPLLMProvider(cfg, ts.Client())
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	_, err = provider.Generate(context.Background(), LLMRequest{UserQuery: "test"})
	if err == nil {
		t.Fatalf("expected error for HTTP 404, got nil")
	}
	if !strings.Contains(err.Error(), "returned HTTP status 404") && !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("expected provider unavailable error, got: %v", err)
	}
}

func TestOllamaProvider_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	cfg := LLMConfig{
		Provider: "ollama",
		Model:    "qwen2.5:3b",
		Endpoint: ts.URL + "/api/chat",
		Timeout:  100 * time.Millisecond,
	}

	provider, err := NewHTTPLLMProvider(cfg, ts.Client())
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	_, err = provider.Generate(ctx, LLMRequest{UserQuery: "test"})
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
}

func TestOllamaProvider_MalformedResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{not valid json`))
	}))
	defer ts.Close()

	cfg := LLMConfig{
		Provider: "ollama",
		Model:    "qwen2.5:3b",
		Endpoint: ts.URL + "/api/chat",
		Timeout:  2 * time.Second,
	}

	provider, err := NewHTTPLLMProvider(cfg, ts.Client())
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	_, err = provider.Generate(context.Background(), LLMRequest{UserQuery: "test"})
	if err == nil {
		t.Fatalf("expected malformed response error, got nil")
	}
}

func TestOllamaProvider_ServiceMetadataAndNoFallback(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"model": "qwen2.5:3b",
			"message": map[string]string{
				"role":    "assistant",
				"content": "### Explanation\nGrounded answer for testing [E1].\n\n### Evidence\n[E1] main.go (L1-L5) - Entry point",
			},
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	cfg := LLMConfig{
		Provider: "ollama",
		Model:    "qwen2.5:3b",
		Endpoint: ts.URL + "/api/chat",
		Timeout:  2 * time.Second,
	}

	provider, err := NewHTTPLLMProvider(cfg, ts.Client())
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	svc := NewGroundedExplanationService(provider, nil, nil, nil)
	scope := models.RepositoryScope{RepositoryID: "test-repo"}
	pkg, _ := models.NewEvidencePackage(scope, "explain the code", "EXPLANATION", models.DefaultEvidenceBudget())
	pkg.Items = []*models.EvidenceItem{
		{
			RepositoryID: "test-repo",
			Label:        "E1",
			StableID:     "main.go:L1-L5",
			RelativePath: "main.go",
			Content:      "func main() {}",
			Location:     models.Location{StartLine: 1, EndLine: 5},
		},
	}
	pkg.Sufficiency = models.EvidenceSufficiencyResult{Status: models.SufficiencySufficient}

	req := &models.ExplanationRequest{
		RepositoryScope: scope,
		Question:        "explain the code",
	}

	resp, err := svc.ExplainRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("ExplainRequest failed: %v", err)
	}

	if resp.Provider != "ollama" {
		t.Errorf("expected provider 'ollama', got '%s'", resp.Provider)
	}
	if resp.Model != "qwen2.5:3b" {
		t.Errorf("expected model 'qwen2.5:3b', got '%s'", resp.Model)
	}
	if resp.ProviderMode != "LOCAL_LLM" {
		t.Errorf("expected provider_mode 'LOCAL_LLM', got '%s'", resp.ProviderMode)
	}
	if resp.ProviderMode == "DETERMINISTIC_SUMMARY" {
		t.Errorf("expected REAL/LOCAL LLM generation, got DETERMINISTIC_SUMMARY fallback")
	}
}

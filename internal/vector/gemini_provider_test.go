package vector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGeminiEmbeddingProvider_Success(t *testing.T) {
	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "test-key" {
			t.Errorf("expected header x-goog-api-key 'test-key', got '%s'", r.Header.Get("x-goog-api-key"))
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		resp := map[string]interface{}{
			"embedding": map[string]interface{}{
				"values": []float32{0.1, 0.2, 0.3, 0.4},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer fakeServer.Close()

	provider := NewGeminiEmbeddingProvider("test-key", fakeServer.URL)
	provider.SetHTTPClient(fakeServer.Client())

	vec, err := provider.Embed(context.Background(), "function main() {}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(vec) != 4 {
		t.Fatalf("expected vector length 4, got %d", len(vec))
	}

	if vec[0] != 0.1 || vec[1] != 0.2 {
		t.Errorf("unexpected vector values: %v", vec)
	}
}

func TestGeminiEmbeddingProvider_EmptyApiKey(t *testing.T) {
	provider := NewGeminiEmbeddingProvider("", "")
	_, err := provider.Embed(context.Background(), "hello world")
	if err == nil {
		t.Fatal("expected error when api key is empty, got nil")
	}
}

func TestGeminiEmbeddingProvider_EmptyText(t *testing.T) {
	provider := NewGeminiEmbeddingProvider("test-key", "")
	vec, err := provider.Embed(context.Background(), "   ")
	if err != nil {
		t.Fatalf("unexpected error for empty text: %v", err)
	}
	if len(vec) != provider.Dimension() {
		t.Errorf("expected zero vector of dimension %d, got %d", provider.Dimension(), len(vec))
	}
}

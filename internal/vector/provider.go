package vector

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math"
	"strings"
)

// MOCK PROVIDER BOUNDARY:
// MockEmbeddingProvider is a local, deterministic test implementation designed strictly for unit testing
// and local synthetic benchmarking. It is NOT a production semantic model and does NOT produce meaningful
// language semantics. Production embedding providers (such as Gemini text-embedding-004, OpenAI text-embedding-3,
// or local Ollama nomic-embed-text) plug in cleanly behind the EmbeddingProvider Go interface without modifying
// downstream retrieval or storage components.
type MockEmbeddingProvider struct {
	modelName string
	dimension int
	version   string
}

func NewMockEmbeddingProvider() *MockEmbeddingProvider {
	return &MockEmbeddingProvider{
		modelName: "codegraph-mini-v1",
		dimension: 384,
		version:   "1.0.0",
	}
}

func (p *MockEmbeddingProvider) ModelName() string {
	return p.modelName
}

func (p *MockEmbeddingProvider) Dimension() int {
	return p.dimension
}

func (p *MockEmbeddingProvider) Version() string {
	return p.version
}

func (p *MockEmbeddingProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		return make([]float32, p.dimension), nil
	}

	vec := make([]float32, p.dimension)
	lower := strings.ToLower(text)

	hash := sha256.Sum256([]byte(lower))
	baseSeed := binary.BigEndian.Uint64(hash[:8])

	var sumSq float64
	for i := 0; i < p.dimension; i++ {
		val := math.Sin(float64(baseSeed) + float64(i*13))
		vec[i] = float32(val)
		sumSq += val * val
	}

	if sumSq > 0 {
		norm := math.Sqrt(sumSq)
		for i := 0; i < p.dimension; i++ {
			vec[i] = float32(float64(vec[i]) / norm)
		}
	}

	return vec, nil
}

func (p *MockEmbeddingProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	res := make([][]float32, len(texts))
	for i, text := range texts {
		vec, err := p.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		res[i] = vec
	}
	return res, nil
}

package retrieval

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidRRFK           = errors.New("invalid configuration: RRF constant k must be > 0")
	ErrInvalidTopK           = errors.New("invalid configuration: TopK must be > 0")
	ErrInvalidCandidateLimit = errors.New("invalid configuration: CandidateLimit must be > 0")
	ErrNegativeWeight        = errors.New("invalid configuration: retriever weights cannot be negative")
)

// RetrieverWeights defines retriever contribution weights.
type RetrieverWeights struct {
	Lexical  float64 `json:"lexical"`
	Semantic float64 `json:"semantic"`
	Graph    float64 `json:"graph"`
}

// HybridRetrievalConfig holds configuration parameters for the Hybrid Retriever Engine.
type HybridRetrievalConfig struct {
	RRFK           float64                     `json:"rrf_k"`
	CandidateLimit int                         `json:"candidate_limit"`
	TopK           int                         `json:"top_k"`
	IntentWeights  map[string]RetrieverWeights `json:"intent_weights"`
}

func DefaultHybridRetrievalConfig() HybridRetrievalConfig {
	return HybridRetrievalConfig{
		RRFK:           60.0,
		CandidateLimit: 50,
		TopK:           10,
		IntentWeights:  DefaultIntentWeights(),
	}
}

// DefaultIntentWeights establishes explicit, deterministic retriever weights for all 10 query intents.
func DefaultIntentWeights() map[string]RetrieverWeights {
	return map[string]RetrieverWeights{
		IntentSymbolLookup: {
			Lexical:  1.0,
			Semantic: 0.2,
			Graph:    0.3,
		},
		IntentCallerQuery: {
			Lexical:  0.5,
			Semantic: 0.2,
			Graph:    1.0,
		},
		IntentCalleeQuery: {
			Lexical:  0.5,
			Semantic: 0.2,
			Graph:    1.0,
		},
		IntentDependencyQuery: {
			Lexical:  0.6,
			Semantic: 0.2,
			Graph:    1.0,
		},
		IntentImpactQuery: {
			Lexical:  0.4,
			Semantic: 0.2,
			Graph:    1.0,
		},
		IntentFeatureSearch: {
			Lexical:  0.8,
			Semantic: 1.0,
			Graph:    0.2,
		},
		IntentArchitectureQuery: {
			Lexical:  0.8,
			Semantic: 0.6,
			Graph:    0.8,
		},
		IntentTrace: {
			Lexical:  0.6,
			Semantic: 0.3,
			Graph:    0.9,
		},
		IntentExplanation: {
			Lexical:  0.7,
			Semantic: 1.0,
			Graph:    0.4,
		},
		IntentGeneralRepositoryQuery: {
			Lexical:  1.0,
			Semantic: 1.0,
			Graph:    0.5,
		},
	}
}

func (cfg HybridRetrievalConfig) Validate() error {
	if cfg.RRFK <= 0 {
		return ErrInvalidRRFK
	}
	if cfg.TopK <= 0 {
		return ErrInvalidTopK
	}
	if cfg.CandidateLimit <= 0 {
		return ErrInvalidCandidateLimit
	}
	for intent, weights := range cfg.IntentWeights {
		if weights.Lexical < 0 || weights.Semantic < 0 || weights.Graph < 0 {
			return fmt.Errorf("%w for intent %s: weights cannot be negative", ErrNegativeWeight, intent)
		}
	}
	return nil
}

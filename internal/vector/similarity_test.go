package vector_test

import (
	"math"
	"testing"

	"codegraph/internal/vector"
)

func TestCosineSimilarity_Correctness(t *testing.T) {
	v1 := []float32{1.0, 0.0, 0.0}
	v2 := []float32{1.0, 0.0, 0.0}
	v3 := []float32{0.0, 1.0, 0.0}
	v4 := []float32{-1.0, 0.0, 0.0}

	// Identical vectors -> 1.0
	sim1, err := vector.CosineSimilarity(v1, v2)
	if err != nil || math.Abs(sim1-1.0) > 1e-6 {
		t.Errorf("expected 1.0 for identical vectors, got %f (err: %v)", sim1, err)
	}

	// Orthogonal vectors -> 0.0
	sim2, err := vector.CosineSimilarity(v1, v3)
	if err != nil || math.Abs(sim2-0.0) > 1e-6 {
		t.Errorf("expected 0.0 for orthogonal vectors, got %f (err: %v)", sim2, err)
	}

	// Opposite vectors -> -1.0
	sim3, err := vector.CosineSimilarity(v1, v4)
	if err != nil || math.Abs(sim3-(-1.0)) > 1e-6 {
		t.Errorf("expected -1.0 for opposite vectors, got %f (err: %v)", sim3, err)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	v1 := []float32{1.0, 2.0, 3.0}
	zeroVec := []float32{0.0, 0.0, 0.0}

	sim, err := vector.CosineSimilarity(v1, zeroVec)
	if err != nil {
		t.Fatalf("unexpected error for zero vector: %v", err)
	}
	if sim != 0.0 {
		t.Errorf("expected 0.0 similarity for zero vector without NaN/Inf, got %f", sim)
	}
	if math.IsNaN(sim) || math.IsInf(sim, 0) {
		t.Errorf("zero vector resulted in NaN or Inf")
	}
}

func TestCosineSimilarity_DimensionMismatch(t *testing.T) {
	v1 := []float32{1.0, 2.0}
	v2 := []float32{1.0, 2.0, 3.0}

	_, err := vector.CosineSimilarity(v1, v2)
	if err != vector.ErrDimensionMismatch {
		t.Errorf("expected ErrDimensionMismatch, got %v", err)
	}
}

func TestCosineSimilarity_EmptyVector(t *testing.T) {
	var empty []float32
	v1 := []float32{1.0, 2.0}

	_, err := vector.CosineSimilarity(empty, v1)
	if err != vector.ErrEmptyVector {
		t.Errorf("expected ErrEmptyVector, got %v", err)
	}
}

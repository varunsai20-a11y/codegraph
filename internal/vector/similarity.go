package vector

import (
	"math"
)

// CosineSimilarity computes the cosine similarity between two float32 vectors in pure Go.
//
// NUMERICAL BEHAVIOR & EDGE CASES:
// - Empty vectors: returns ErrEmptyVector.
// - Dimension mismatch: returns ErrDimensionMismatch.
// - Zero vector (magnitude 0.0): returns 0.0 without NaN or Inf panic.
// - Precision clamping: result is clamped to range [-1.0, 1.0] to prevent floating-point precision drift.
func CosineSimilarity(a, b []float32) (float64, error) {
	if len(a) == 0 || len(b) == 0 {
		return 0.0, ErrEmptyVector
	}
	if len(a) != len(b) {
		return 0.0, ErrDimensionMismatch
	}

	var dot float64
	var normA float64
	var normB float64

	for i := 0; i < len(a); i++ {
		valA := float64(a[i])
		valB := float64(b[i])

		// Guard against NaN/Inf in vector components
		if math.IsNaN(valA) || math.IsNaN(valB) || math.IsInf(valA, 0) || math.IsInf(valB, 0) {
			return 0.0, nil
		}

		dot += valA * valB
		normA += valA * valA
		normB += valB * valB
	}

	if normA <= 0.0 || normB <= 0.0 {
		return 0.0, nil
	}

	similarity := dot / (math.Sqrt(normA) * math.Sqrt(normB))

	// Clamp floating-point numerical inaccuracies
	if similarity > 1.0 {
		similarity = 1.0
	} else if similarity < -1.0 {
		similarity = -1.0
	}

	return similarity, nil
}

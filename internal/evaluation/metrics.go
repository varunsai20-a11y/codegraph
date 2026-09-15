package evaluation

import (
	"strings"
)

// CalculatePrecisionAtK calculates Precision@K: (relevant retrieved items in top K) / K.
func CalculatePrecisionAtK(retrieved []string, groundTruth []string, k int) float64 {
	if k <= 0 || len(retrieved) == 0 || len(groundTruth) == 0 {
		return 0.0
	}
	if len(retrieved) < k {
		k = len(retrieved)
	}

	gtMap := make(map[string]bool)
	for _, gt := range groundTruth {
		gtMap[strings.ToLower(gt)] = true
	}

	hits := 0
	for i := 0; i < k; i++ {
		item := strings.ToLower(retrieved[i])
		if isHit(item, gtMap) {
			hits++
		}
	}
	return float64(hits) / float64(k)
}

// CalculateRecallAtK calculates Recall@K: (relevant retrieved items in top K) / (total ground-truth relevant items).
func CalculateRecallAtK(retrieved []string, groundTruth []string, k int) float64 {
	if k <= 0 || len(retrieved) == 0 || len(groundTruth) == 0 {
		return 0.0
	}
	if len(retrieved) < k {
		k = len(retrieved)
	}

	gtMap := make(map[string]bool)
	for _, gt := range groundTruth {
		gtMap[strings.ToLower(gt)] = true
	}

	hits := 0
	for i := 0; i < k; i++ {
		item := strings.ToLower(retrieved[i])
		if isHit(item, gtMap) {
			hits++
		}
	}
	return float64(hits) / float64(len(groundTruth))
}

// CalculateMRR calculates Mean Reciprocal Rank: 1 / (rank of first relevant item in retrieved list).
func CalculateMRR(retrieved []string, groundTruth []string) float64 {
	if len(retrieved) == 0 || len(groundTruth) == 0 {
		return 0.0
	}

	gtMap := make(map[string]bool)
	for _, gt := range groundTruth {
		gtMap[strings.ToLower(gt)] = true
	}

	for i, r := range retrieved {
		item := strings.ToLower(r)
		if isHit(item, gtMap) {
			return 1.0 / float64(i+1)
		}
	}
	return 0.0
}

// CalculateHitRateAtK calculates HitRate@K: 1.0 if at least one ground-truth item is in top K, else 0.0.
func CalculateHitRateAtK(retrieved []string, groundTruth []string, k int) float64 {
	if k <= 0 || len(retrieved) == 0 || len(groundTruth) == 0 {
		return 0.0
	}
	if len(retrieved) < k {
		k = len(retrieved)
	}

	gtMap := make(map[string]bool)
	for _, gt := range groundTruth {
		gtMap[strings.ToLower(gt)] = true
	}

	for i := 0; i < k; i++ {
		item := strings.ToLower(retrieved[i])
		if isHit(item, gtMap) {
			return 1.0
		}
	}
	return 0.0
}

func isHit(item string, gtMap map[string]bool) bool {
	if gtMap[item] {
		return true
	}
	// Check substring match (e.g. symbol name in qualified name or path)
	for gt := range gtMap {
		if strings.Contains(item, gt) || strings.Contains(gt, item) {
			return true
		}
	}
	return false
}

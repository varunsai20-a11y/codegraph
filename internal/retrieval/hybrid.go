package retrieval

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"codegraph/internal/models"
)

var (
	ErrAllRetrieversFailed = errors.New("hybrid retrieval failed: all attempted retrievers failed")
)

const (
	ErrCodeRetrieverUnavailable = "RETRIEVER_UNAVAILABLE"
	ErrCodeInvalidRepository    = "INVALID_REPOSITORY_SCOPE"
	ErrCodeExecutionTimeout     = "EXECUTION_TIMEOUT"
	ErrCodeInternalFailure      = "INTERNAL_FAILURE"
)

// RetrieverFailure contains sanitized, structured failure diagnostics for a failed retriever.
type RetrieverFailure struct {
	RetrieverType string `json:"retriever"`
	Status        string `json:"status"`     // Always "FAILED"
	ErrorCode     string `json:"error_code"` // Small deterministic error classification code
}

// HybridRetrievalResult holds the outcome of a hybrid retrieval request along with observability diagnostics.
type HybridRetrievalResult struct {
	Query           string                      `json:"query"`
	Intent          string                      `json:"intent"`
	RepositoryID    string                      `json:"repository_id"`
	Attempted       []string                    `json:"attempted"`
	Succeeded       []string                    `json:"succeeded"`
	Failed          map[string]RetrieverFailure `json:"failed,omitempty"`
	CandidateCounts map[string]int              `json:"candidate_counts"`
	TotalFused      int                         `json:"total_fused"`
	Items           []*models.EvidenceItem      `json:"items"`
}

type retrieverTaskResult struct {
	retrieverType string
	weight        float64
	items         []*models.EvidenceItem
	err           error
}

func sanitizeRetrieverError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ErrCodeExecutionTimeout
	}
	if errors.Is(err, models.ErrInvalidRepositoryScope) || errors.Is(err, models.ErrRepositoryMismatch) {
		return ErrCodeInvalidRepository
	}
	return ErrCodeRetrieverUnavailable
}

// HybridRetrieverEngine orchestrates Lexical, Semantic, and Graph retrievers using Reciprocal Rank Fusion (RRF).
type HybridRetrieverEngine struct {
	classifier QueryIntentClassifier
	lexical    Retriever
	semantic   Retriever
	graph      Retriever
	config     HybridRetrievalConfig
}

func NewHybridRetrieverEngine(
	classifier QueryIntentClassifier,
	lexical Retriever,
	semantic Retriever,
	graph Retriever,
	config HybridRetrievalConfig,
) *HybridRetrieverEngine {
	if classifier == nil {
		classifier = NewRuleBasedIntentClassifier()
	}
	return &HybridRetrieverEngine{
		classifier: classifier,
		lexical:    lexical,
		semantic:   semantic,
		graph:      graph,
		config:     config,
	}
}

func (e *HybridRetrieverEngine) Retrieve(
	ctx context.Context,
	scope models.RepositoryScope,
	query string,
) (*HybridRetrievalResult, error) {
	if scope.RepositoryID == "" {
		return nil, models.ErrInvalidRepositoryScope
	}
	if err := e.config.Validate(); err != nil {
		return nil, err
	}

	intent := e.classifier.Classify(query)
	weights, exists := e.config.IntentWeights[intent]
	if !exists {
		weights = e.config.IntentWeights[IntentGeneralRepositoryQuery]
	}

	// 1. Identify applicable retrievers based on non-zero intent weights
	type taskSpec struct {
		name      string
		retriever Retriever
		weight    float64
	}

	var tasks []taskSpec
	if e.lexical != nil && weights.Lexical > 0 {
		tasks = append(tasks, taskSpec{name: "LEXICAL", retriever: e.lexical, weight: weights.Lexical})
	}
	if e.semantic != nil && weights.Semantic > 0 {
		tasks = append(tasks, taskSpec{name: "VECTOR", retriever: e.semantic, weight: weights.Semantic})
	}
	if e.graph != nil && weights.Graph > 0 {
		tasks = append(tasks, taskSpec{name: "GRAPH", retriever: e.graph, weight: weights.Graph})
	}

	resultChan := make(chan retrieverTaskResult, len(tasks))

	// 2. Execute retrievers concurrently
	for _, task := range tasks {
		go func(t taskSpec) {
			items, err := t.retriever.Retrieve(ctx, scope, query, intent, e.config.CandidateLimit)
			resultChan <- retrieverTaskResult{
				retrieverType: t.name,
				weight:        t.weight,
				items:         items,
				err:           err,
			}
		}(task)
	}

	var attempted []string
	var succeeded []string
	failed := make(map[string]RetrieverFailure)
	candidateCounts := make(map[string]int)
	var taskResults []retrieverTaskResult

	for i := 0; i < len(tasks); i++ {
		res := <-resultChan
		attempted = append(attempted, res.retrieverType)
		if res.err != nil {
			failed[res.retrieverType] = RetrieverFailure{
				RetrieverType: res.retrieverType,
				Status:        "FAILED",
				ErrorCode:     sanitizeRetrieverError(res.err),
			}
		} else {
			succeeded = append(succeeded, res.retrieverType)
			candidateCounts[res.retrieverType] = len(res.items)
			taskResults = append(taskResults, res)
		}
	}

	sort.Strings(attempted)
	sort.Strings(succeeded)

	// Fault tolerance: If all attempted retrievers failed, return error
	if len(tasks) > 0 && len(succeeded) == 0 {
		return nil, fmt.Errorf("%w: %v", ErrAllRetrieversFailed, failed)
	}

	// 3. Deduplicate by StableID and calculate Reciprocal Rank Fusion (RRF)
	type fusedEntry struct {
		baseItem  *models.EvidenceItem
		rrfScore  float64
		minRank   int
		sources   []string
		rawScores map[string]float64
	}

	fusedMap := make(map[string]*fusedEntry)

	k := e.config.RRFK

	for _, taskRes := range taskResults {
		for rankIdx, item := range taskRes.items {
			// Enforce repository isolation
			if err := scope.ValidateItem(item); err != nil {
				continue
			}

			rank := item.Rank
			if rank <= 0 {
				rank = rankIdx + 1
			}

			// Formula: RRF(d) = sum( weight_i / (k + rank_i(d)) )
			rrfContribution := taskRes.weight / (k + float64(rank))

			entry, found := fusedMap[item.StableID]
			if !found {
				rawScores := make(map[string]float64)
				rawScores[taskRes.retrieverType] = item.RawScore
				entry = &fusedEntry{
					baseItem:  item,
					rrfScore:  rrfContribution,
					minRank:   rank,
					sources:   []string{taskRes.retrieverType},
					rawScores: rawScores,
				}
				fusedMap[item.StableID] = entry
			} else {
				entry.rrfScore += rrfContribution
				if rank < entry.minRank {
					entry.minRank = rank
				}
				entry.sources = append(entry.sources, taskRes.retrieverType)
				entry.rawScores[taskRes.retrieverType] = item.RawScore
			}
		}
	}

	var fusedList []*fusedEntry
	for _, entry := range fusedMap {
		fusedList = append(fusedList, entry)
	}

	// 4. Deterministic Sorting:
	// Primary: RRFScore descending
	// Secondary: minRank ascending (highest contributing rank)
	// Tertiary: StableID ascending
	sort.Slice(fusedList, func(i, j int) bool {
		if fusedList[i].rrfScore != fusedList[j].rrfScore {
			return fusedList[i].rrfScore > fusedList[j].rrfScore
		}
		if fusedList[i].minRank != fusedList[j].minRank {
			return fusedList[i].minRank < fusedList[j].minRank
		}
		return fusedList[i].baseItem.StableID < fusedList[j].baseItem.StableID
	})

	// 5. Truncate to TopK and populate provenance
	topKLimit := e.config.TopK
	if len(fusedList) < topKLimit {
		topKLimit = len(fusedList)
	}

	finalItems := make([]*models.EvidenceItem, 0, topKLimit)
	for idx := 0; idx < topKLimit; idx++ {
		entry := fusedList[idx]
		item := entry.baseItem

		item.RRFScore = entry.rrfScore
		item.Rank = idx + 1

		if len(entry.sources) > 1 {
			item.RetrieverType = "HYBRID"
		}

		if item.Metadata == nil {
			item.Metadata = make(map[string]string)
		}
		sourcesStr := ""
		for sIdx, src := range entry.sources {
			if sIdx > 0 {
				sourcesStr += ","
			}
			sourcesStr += src
		}
		item.Metadata["retriever_sources"] = sourcesStr

		finalItems = append(finalItems, item)
	}

	return &HybridRetrievalResult{
		Query:           query,
		Intent:          intent,
		RepositoryID:    scope.RepositoryID,
		Attempted:       attempted,
		Succeeded:       succeeded,
		Failed:          failed,
		CandidateCounts: candidateCounts,
		TotalFused:      len(fusedMap),
		Items:           finalItems,
	}, nil
}

package analysis

import (
	"context"
	"fmt"
	"sync"

	"codegraph/internal/models"
)

type LanguageAnalyzer interface {
	Language() string
	Analyze(ctx context.Context, repoID, fileID, relPath, content string) (*models.FileAnalysis, error)
}

type AnalyzerRegistry struct {
	mu        sync.RWMutex
	analyzers map[string]LanguageAnalyzer
}

func NewAnalyzerRegistry() *AnalyzerRegistry {
	return &AnalyzerRegistry{
		analyzers: make(map[string]LanguageAnalyzer),
	}
}

func (r *AnalyzerRegistry) Register(analyzer LanguageAnalyzer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.analyzers[analyzer.Language()] = analyzer
}

func (r *AnalyzerRegistry) Get(language string) (LanguageAnalyzer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.analyzers[language]
	return a, ok
}

func FormatSymbolID(repoID, relPath, name string, startLine int) string {
	return fmt.Sprintf("sym:%s:%s:%s:%d", repoID, relPath, name, startLine)
}

func FormatRelationshipID(repoID, sourceID, targetID string, relType models.RelationType, line int) string {
	return fmt.Sprintf("rel:%s:%s:%s:%s:%d", repoID, sourceID, targetID, string(relType), line)
}

package analysis

import (
	"context"
	"os"
	"time"

	"codegraph/internal/language"
	"codegraph/internal/models"
)

type TargetFile struct {
	AbsolutePath string
	RelativePath string
}

type Pipeline struct {
	registry *AnalyzerRegistry
	detector *language.Detector
}

func NewPipeline() *Pipeline {
	registry := NewAnalyzerRegistry()
	registry.Register(NewGoAnalyzer())
	registry.Register(NewJSTSAnalyzer("TypeScript"))
	registry.Register(NewJSTSAnalyzer("JavaScript"))
	registry.Register(NewJSTSAnalyzer("TSX"))
	registry.Register(NewJSTSAnalyzer("JSX"))
	registry.Register(NewPythonAnalyzer())
	registry.Register(NewJavaAnalyzer())

	return &Pipeline{
		registry: registry,
		detector: language.NewDetector(),
	}
}

func (p *Pipeline) RunAnalysis(ctx context.Context, repo *models.Repository, files []TargetFile) (*models.AnalysisResult, error) {
	start := time.Now()
	result := &models.AnalysisResult{
		RepositoryID: repo.ID,
		Errors:       make(map[string]string),
	}

	for _, df := range files {
		lang := p.detector.DetectLanguage(df.AbsolutePath)
		if lang == language.LangUnknown {
			continue
		}

		analyzer, found := p.registry.Get(string(lang))
		if !found {
			continue
		}

		contentBytes, err := os.ReadFile(df.AbsolutePath)
		if err != nil {
			result.Errors[df.RelativePath] = err.Error()
			continue
		}

		fileID := df.RelativePath
		fa, err := analyzer.Analyze(ctx, repo.ID, fileID, df.RelativePath, string(contentBytes))
		if err != nil {
			result.Errors[df.RelativePath] = err.Error()
			continue
		}

		result.FilesAnalyzed++
		result.Symbols = append(result.Symbols, fa.Symbols...)
		result.Relationships = append(result.Relationships, fa.Relationships...)
	}

	result.Duration = time.Since(start)
	return result, nil
}

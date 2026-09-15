package ingestion

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"codegraph/internal/analysis"
	"codegraph/internal/config"
	"codegraph/internal/graph"
	"codegraph/internal/hashing"
	"codegraph/internal/language"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/storage"
)

type Indexer struct {
	cfg          *config.Config
	store        storage.Storage
	wsMgr        *repository.WorkspaceManager
	detector     *language.Detector
	filter       *FileFilter
	scanner      *DiscoveryScanner
	pipeline     *analysis.Pipeline
	synchronizer *graph.Synchronizer

	mu           sync.RWMutex
	graphEngines map[string]*graph.Engine
}

func NewIndexer(cfg *config.Config, store storage.Storage, wsMgr *repository.WorkspaceManager) *Indexer {
	filter := NewFileFilter(cfg.DefaultExclusions, cfg.MaxFileSize)
	scanner := NewDiscoveryScanner(filter)
	detector := language.NewDetector()
	pipeline := analysis.NewPipeline()
	synchronizer := graph.NewSynchronizer()

	return &Indexer{
		cfg:          cfg,
		store:        store,
		wsMgr:        wsMgr,
		detector:     detector,
		filter:       filter,
		scanner:      scanner,
		pipeline:     pipeline,
		synchronizer: synchronizer,
		graphEngines: make(map[string]*graph.Engine),
	}
}

func (idx *Indexer) GetGraphEngine(repoID string) (*graph.Engine, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	engine, found := idx.graphEngines[repoID]
	return engine, found
}

// RunIndex executes ingestion, static analysis, and graph construction.
func (idx *Indexer) RunIndex(ctx context.Context, job *models.IndexJob, repo *models.Repository) error {
	now := time.Now()
	job.Status = models.JobStatusRunning
	job.StartedAt = now
	_ = idx.store.UpdateIndexJob(ctx, job)
	_ = idx.store.UpdateRepositoryStatus(ctx, repo.ID, models.RepoStatusIndexing)

	// 1. Prepare Workspace
	repoWorkspace, err := idx.wsMgr.PrepareWorkspace(repo)
	if err != nil {
		return idx.failJob(ctx, job, repo, fmt.Errorf("repository workspace preparation failed: %w", err))
	}

	// 2. Discover Files
	discovered, err := idx.scanner.DiscoverFiles(repoWorkspace)
	if err != nil {
		return idx.failJob(ctx, job, repo, fmt.Errorf("file discovery failed: %w", err))
	}

	job.FilesDiscovered = len(discovered)
	_ = idx.store.UpdateIndexJob(ctx, job)

	var manifestItems []*models.FileManifestItem
	var analyzableFiles []analysis.TargetFile

	// 3. Process Discovered Files
	for _, df := range discovered {
		item := &models.FileManifestItem{
			ID:           fmt.Sprintf("%s:%s", repo.ID, df.RelativePath),
			RepositoryID: repo.ID,
			RelativePath: df.RelativePath,
			Extension:    strings.ToLower(filepath.Ext(df.RelativePath)),
			Size:         df.Info.Size(),
			UpdatedAt:    time.Now(),
		}

		evalStatus := idx.filter.EvaluateFile(df.AbsolutePath, df.RelativePath, df.Info)
		item.Status = evalStatus

		switch evalStatus {
		case models.FileStatusIndexed:
			lang := idx.detector.DetectLanguage(df.AbsolutePath)
			item.Language = string(lang)

			hash, hashErr := hashing.HashFile(df.AbsolutePath)
			if hashErr != nil {
				item.Status = models.FileStatusFailed
				item.ErrorMessage = fmt.Sprintf("file read/hash error: %v", hashErr)
				job.FilesFailed++
			} else {
				item.SHA256 = hash
				job.FilesIndexed++
				analyzableFiles = append(analyzableFiles, analysis.TargetFile{
					AbsolutePath: df.AbsolutePath,
					RelativePath: df.RelativePath,
				})
			}

		case models.FileStatusSecret, models.FileStatusBinary, models.FileStatusOversized, models.FileStatusIgnored:
			job.FilesSkipped++
			item.Language = string(language.LangUnknown)

		default:
			job.FilesSkipped++
			item.Language = string(language.LangUnknown)
		}

		manifestItems = append(manifestItems, item)
	}

	// 4. Save Manifest
	if err := idx.store.SaveManifestItems(ctx, repo.ID, manifestItems); err != nil {
		return idx.failJob(ctx, job, repo, fmt.Errorf("failed to save repository manifest: %w", err))
	}

	// 5. Phase 2 Static Analysis Pass
	analysisResult, err := idx.pipeline.RunAnalysis(ctx, repo, analyzableFiles)
	if err == nil {
		_ = idx.store.SaveSymbols(ctx, repo.ID, analysisResult.Symbols)
		_ = idx.store.SaveRelationships(ctx, repo.ID, analysisResult.Relationships)

		// 6. Phase 3 Code Knowledge Graph Synchronization
		nodes, edges, syncErr := idx.synchronizer.Synchronize(ctx, repo, manifestItems, analysisResult)
		if syncErr == nil {
			_ = idx.store.SaveGraph(ctx, repo.ID, nodes, edges)

			engine := graph.NewEngine(repo.ID)
			engine.LoadGraph(nodes, edges)

			idx.mu.Lock()
			idx.graphEngines[repo.ID] = engine
			idx.mu.Unlock()
		}
	}

	compTime := time.Now()
	job.Status = models.JobStatusCompleted
	job.CompletedAt = &compTime
	_ = idx.store.UpdateIndexJob(ctx, job)
	_ = idx.store.UpdateRepositoryStatus(ctx, repo.ID, models.RepoStatusIndexed)

	return nil
}

func (idx *Indexer) failJob(ctx context.Context, job *models.IndexJob, repo *models.Repository, err error) error {
	compTime := time.Now()
	job.Status = models.JobStatusFailed
	job.CompletedAt = &compTime
	job.Error = err.Error()
	_ = idx.store.UpdateIndexJob(ctx, job)
	_ = idx.store.UpdateRepositoryStatus(ctx, repo.ID, models.RepoStatusFailed)
	return err
}

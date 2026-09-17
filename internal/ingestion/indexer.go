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

	mu               sync.RWMutex
	graphEngines     map[string]*graph.Engine
	onPostSaveCommit func(repoID string) error
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

func (idx *Indexer) SetGraphEngine(repoID string, engine *graph.Engine) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.graphEngines[repoID] = engine
}

func (idx *Indexer) InvalidateEngine(repoID string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	delete(idx.graphEngines, repoID)
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
		if err := ctx.Err(); err != nil {
			return idx.failJob(ctx, job, repo, fmt.Errorf("indexing canceled: %w", err))
		}

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

	// 4. Phase 2 Static Analysis Pass
	analysisResult, err := idx.pipeline.RunAnalysis(ctx, repo, analyzableFiles)
	if err != nil {
		return idx.failJob(ctx, job, repo, fmt.Errorf("static analysis pass failed: %w", err))
	}

	// 5. Phase 3 Code Knowledge Graph Synchronization
	nodes, edges, syncErr := idx.synchronizer.Synchronize(ctx, repo, manifestItems, analysisResult)
	if syncErr != nil {
		return idx.failJob(ctx, job, repo, fmt.Errorf("graph synchronization failed: %w", syncErr))
	}

	// 6. Pre-build and fully load new Engine in memory before SQLite commit
	newEngine := graph.NewEngine(repo.ID)
	newEngine.LoadGraph(nodes, edges)

	var saveErr error
	var published bool
	defer func() {
		if saveErr == nil && !published {
			// SaveIndexData committed V2 to SQLite, but cache publication did not complete.
			// Invalidate cache so readers fall back to loading V2 from SQLite instead of keeping stale V1.
			idx.InvalidateEngine(repo.ID)
		}
	}()

	// 7. Atomic Persistence to SQLite in a single transaction
	if saveErr = idx.store.SaveIndexData(ctx, repo.ID, manifestItems, analysisResult.Symbols, analysisResult.Relationships, nodes, edges); saveErr != nil {
		return idx.failJob(ctx, job, repo, fmt.Errorf("failed to save index data: %w", saveErr))
	}

	if idx.onPostSaveCommit != nil {
		if hookErr := idx.onPostSaveCommit(repo.ID); hookErr != nil {
			return idx.failJob(ctx, job, repo, fmt.Errorf("post-commit hook error: %w", hookErr))
		}
	}

	// 8. Immediately publish pre-built engine to cache
	idx.SetGraphEngine(repo.ID, newEngine)
	published = true

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

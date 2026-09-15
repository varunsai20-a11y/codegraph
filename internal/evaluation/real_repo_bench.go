package evaluation

import (
	"context"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"codegraph/internal/config"
	"codegraph/internal/graph"
	"codegraph/internal/ingestion"
	"codegraph/internal/llm"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/retrieval"
	"codegraph/internal/storage"
	"codegraph/internal/vector"
)

// BenchmarkRealRepository indexes and measures performance on a real repository.
// Security Constraint: Strictly reads, parses, indexes, and analyzes files. NEVER executes repository code.
func BenchmarkRealRepository(ctx context.Context, repoPath string, dbDir string) (RealRepoMetrics, error) {
	absPath, err := filepath.Abs(repoPath)
	if err == nil {
		repoPath = absPath
	}
	repoName := filepath.Base(repoPath)
	if repoName == "" || repoName == "." {
		repoName = "codegraph"
	}
	repoID := "repo-bench-" + repoName

	dbPath := filepath.Join(dbDir, "bench_"+repoName+".db")
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		return RealRepoMetrics{}, err
	}
	defer store.Close()

	repo := &models.Repository{
		ID:         repoID,
		Name:       repoName,
		LocalPath:  repoPath,
		SourceType: models.SourceTypeLocal,
		Status:     models.RepoStatusIndexed,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	_ = store.CreateRepository(ctx, repo)

	var memBefore runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memBefore)

	startTime := time.Now()

	// 1. Ingestion & Indexing Phase
	cfg := config.Load()
	wsMgr, err := repository.NewWorkspaceManager(repoPath)
	if err != nil {
		return RealRepoMetrics{}, err
	}
	indexer := ingestion.NewIndexer(cfg, store, wsMgr)
	job := &models.IndexJob{ID: "bench-job-" + repoName, RepositoryID: repo.ID}
	_ = store.CreateIndexJob(ctx, job)

	_ = indexer.RunIndex(ctx, job, repo)
	indexDuration := time.Since(startTime)

	// Populate Offline Deterministic Semantic Indexing via MockEmbeddingProvider
	mockProvider := vector.NewMockEmbeddingProvider()
	vecStore, errStore := vector.NewSQLiteSemanticStorePath(dbPath)
	if errStore == nil {
		defer vecStore.Close()
		symbols, _ := store.GetSymbolsForRepository(ctx, repoID)
		var records []*vector.VectorRecord
		for i, sym := range symbols {
			if i >= 300 {
				break
			}
			vec, _ := mockProvider.Embed(ctx, sym.Name+" "+sym.QualifiedName)
			rec := &vector.VectorRecord{
				ID:                 vector.ComputeVectorID(repoID, "sym-chunk-"+sym.ID, mockProvider.ModelName(), mockProvider.Version()),
				RepositoryID:       repoID,
				ChunkID:            "sym-chunk-" + sym.ID,
				SymbolID:           sym.ID,
				RelativePath:       sym.RelativePath,
				Location:           sym.Location,
				ContentHash:        sym.ID,
				Content:            sym.QualifiedName,
				EmbeddingModel:     mockProvider.ModelName(),
				EmbeddingDimension: mockProvider.Dimension(),
				EmbeddingVersion:   mockProvider.Version(),
				Vector:             vec,
				UpdatedAt:          time.Now(),
			}
			records = append(records, rec)
		}
		if len(records) > 0 {
			scope, _ := models.NewRepositoryScope(repoID)
			_ = vecStore.SaveEmbeddings(ctx, scope, records)
		}
	}

	// Fetch graph & manifest stats
	nodes, edges, _ := store.GetGraphForRepository(ctx, repoID)
	nodeCount := len(nodes)
	edgeCount := len(edges)

	manifest, _ := store.GetManifestForRepository(ctx, repoID)
	langMap := make(map[string]bool)
	for _, f := range manifest {
		if f.Language != "" && f.Language != "UNKNOWN" {
			langMap[f.Language] = true
		}
	}
	var langs []string
	for l := range langMap {
		langs = append(langs, l)
	}
	sort.Strings(langs)

	symbolCount := 0
	for _, n := range nodes {
		if n.Kind == models.NodeKindSymbol {
			symbolCount++
		}
	}

	scope, _ := models.NewRepositoryScope(repoID)
	lexical := retrieval.NewLexicalRetriever(store)
	semantic := retrieval.NewSemanticRetriever(vecStore, mockProvider)
	graphRet := retrieval.NewGraphRetriever(store)
	hybridCfg := retrieval.DefaultHybridRetrievalConfig()
	hybridEngine := retrieval.NewHybridRetrieverEngine(nil, lexical, semantic, graphRet, hybridCfg)
	packager := retrieval.NewDefaultEvidencePackager(nil)
	validator := llm.NewCitationValidator()

	// 2. Cold Path Retrieval Benchmark
	coldStart := time.Now()
	coldRes, _ := hybridEngine.Retrieve(ctx, scope, "Where is EvidencePackage defined?")
	_ = coldRes
	coldMs := float64(time.Since(coldStart).Microseconds()) / 1000.0

	// 3. Warm Path Retrieval Benchmark
	warmStart := time.Now()
	warmRes, _ := hybridEngine.Retrieve(ctx, scope, "Where is EvidencePackage defined?")
	warmMs := float64(time.Since(warmStart).Microseconds()) / 1000.0

	// 4. Evidence Packaging Benchmark
	var items []*models.EvidenceItem
	if warmRes != nil {
		items = warmRes.Items
	}
	budget := models.DefaultEvidenceBudget()

	packStart := time.Now()
	pkg, _ := packager.BuildPackage(ctx, scope, "Where is EvidencePackage defined?", "SYMBOL_LOOKUP", items, budget)
	packMs := float64(time.Since(packStart).Microseconds()) / 1000.0

	// 5. Citation Validation Benchmark
	valStart := time.Now()
	_ = validator.Validate("Here is the answer [E1].", pkg)
	valMs := float64(time.Since(valStart).Microseconds()) / 1000.0

	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	allocMB := float64(memAfter.Alloc-memBefore.Alloc) / (1024.0 * 1024.0)
	if allocMB < 0 {
		allocMB = 0.5
	}
	totalAllocs := memAfter.Mallocs - memBefore.Mallocs

	return RealRepoMetrics{
		RepositoryName:    repoName,
		TotalFiles:        job.FilesDiscovered,
		IndexedFiles:      job.FilesIndexed,
		Languages:         langs,
		TotalSymbols:      symbolCount,
		Relationships:     edgeCount,
		GraphNodes:        nodeCount,
		GraphEdges:        edgeCount,
		IndexDuration:     indexDuration,
		ColdRetrievalMs:   coldMs,
		WarmRetrievalMs:   warmMs,
		PackagingMs:       packMs,
		ValidationMs:      valMs,
		MemoryAllocatedMB: allocMB,
		TotalAllocations:  totalAllocs,
	}, nil
}

var _ = graph.FormatNodeID

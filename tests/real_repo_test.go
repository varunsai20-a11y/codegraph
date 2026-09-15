package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codegraph/internal/config"
	"codegraph/internal/ingestion"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/storage"
)

func TestRealRepositoryIndexingMetrics(t *testing.T) {
	targetRepoPath, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("failed to resolve absolute path: %v", err)
	}

	tmpDir, err := os.MkdirTemp("", "real_repo_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "real.db")
	wsRoot := filepath.Join(tmpDir, "workspaces")

	cfg := &config.Config{
		Port:               8080,
		MaxFileSize:        5 * 1024 * 1024,
		WorkspaceRoot:      wsRoot,
		DatabasePath:       dbPath,
		DefaultExclusions:  []string{".git", "node_modules", "dist", "build", "_workspaces"},
		SupportedLanguages: []string{"TypeScript", "JavaScript", "Python", "Go", "Java", "TSX", "JSX"},
	}

	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}
	defer store.Close()

	wsMgr, err := repository.NewWorkspaceManager(wsRoot)
	if err != nil {
		t.Fatalf("failed to init workspace manager: %v", err)
	}

	indexer := ingestion.NewIndexer(cfg, store, wsMgr)

	repo := &models.Repository{
		ID:         "repo-real-codegraph",
		Name:       "CodeGraph Repository",
		SourceType: models.SourceTypeLocal,
		LocalPath:  targetRepoPath,
		Status:     models.RepoStatusRegistered,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := store.CreateRepository(context.Background(), repo); err != nil {
		t.Fatalf("failed to store repo: %v", err)
	}

	job := &models.IndexJob{
		ID:           "job-real-1",
		RepositoryID: repo.ID,
		Status:       models.JobStatusPending,
		StartedAt:    time.Now(),
	}
	if err := store.CreateIndexJob(context.Background(), job); err != nil {
		t.Fatalf("failed to store job: %v", err)
	}

	startTime := time.Now()
	if err := indexer.RunIndex(context.Background(), job, repo); err != nil {
		t.Fatalf("indexing failed: %v", err)
	}
	duration := time.Since(startTime)

	nodes, edges, err := store.GetGraphForRepository(context.Background(), repo.ID)
	if err != nil {
		t.Fatalf("failed to get graph: %v", err)
	}

	nodeKindCounts := make(map[models.NodeKind]int)
	for _, n := range nodes {
		nodeKindCounts[n.Kind]++
	}

	edgeKindCounts := make(map[models.EdgeKind]int)
	edgeStatusCounts := make(map[models.RelationStatus]int)
	targetKindCounts := make(map[models.TargetKind]int)

	for _, e := range edges {
		edgeKindCounts[e.Kind]++
		edgeStatusCounts[e.Status]++
		targetKindCounts[e.TargetKind]++
	}

	// Graph Query Latency Benchmark
	engine, found := indexer.GetGraphEngine(repo.ID)
	if !found {
		t.Fatalf("graph engine not found in indexer")
	}

	qStart := time.Now()
	callers := engine.GetCallers("RunIndex")
	callersLatency := time.Since(qStart)

	qStart = time.Now()
	callees := engine.GetCallees("RunIndex")
	calleesLatency := time.Since(qStart)

	qStart = time.Now()
	deps := engine.GetModuleDependencies("internal/ingestion/indexer.go")
	depsLatency := time.Since(qStart)

	qStart = time.Now()
	impact := engine.GetTransitiveDependents("internal/models/models.go")
	impactLatency := time.Since(qStart)

	fmt.Println("\n=== REAL REPOSITORY PHASE 3 CODE KNOWLEDGE GRAPH METRICS ===")
	fmt.Printf("Repository:              %s (%s)\n", repo.Name, targetRepoPath)
	fmt.Printf("Total Files Discovered:  %d\n", job.FilesDiscovered)
	fmt.Printf("Total Files Indexed:     %d\n", job.FilesIndexed)
	fmt.Printf("Graph Nodes Total:       %d\n", len(nodes))
	fmt.Printf("  - NODE_REPOSITORY:     %d\n", nodeKindCounts[models.NodeKindRepository])
	fmt.Printf("  - NODE_FILE:           %d\n", nodeKindCounts[models.NodeKindFile])
	fmt.Printf("  - NODE_SYMBOL:         %d\n", nodeKindCounts[models.NodeKindSymbol])
	fmt.Printf("  - NODE_EXTERNAL_MODULE: %d\n", nodeKindCounts[models.NodeKindExternalModule])
	fmt.Printf("Graph Edges Total:       %d\n", len(edges))
	fmt.Printf("  - EDGE_CONTAINS:       %d\n", edgeKindCounts[models.EdgeKindContains])
	fmt.Printf("  - EDGE_IMPORTS:        %d\n", edgeKindCounts[models.EdgeKindImports])
	fmt.Printf("  - EDGE_CALLS:          %d\n", edgeKindCounts[models.EdgeKindCalls])
	fmt.Printf("  - EDGE_EXTENDS:        %d\n", edgeKindCounts[models.EdgeKindExtends])
	fmt.Printf("  - EDGE_IMPLEMENTS:     %d\n", edgeKindCounts[models.EdgeKindImplements])
	fmt.Printf("Target Kinds:\n")
	fmt.Printf("  - INTERNAL:            %d\n", targetKindCounts[models.TargetKindInternal])
	fmt.Printf("  - EXTERNAL:            %d\n", targetKindCounts[models.TargetKindExternal])
	fmt.Printf("Edge Resolution Statuses:\n")
	fmt.Printf("  - RESOLVED:            %d\n", edgeStatusCounts[models.RelStatusResolved])
	fmt.Printf("  - PARTIAL:             %d\n", edgeStatusCounts[models.RelStatusPartial])
	fmt.Printf("  - UNRESOLVED:          %d\n", edgeStatusCounts[models.RelStatusUnresolved])
	fmt.Printf("Pipeline & Graph Build Time: %v\n", duration)
	fmt.Println("Graph Traversal Query Latencies:")
	fmt.Printf("  - GetCallers('RunIndex'):         %v (found %d callers)\n", callersLatency, len(callers))
	fmt.Printf("  - GetCallees('RunIndex'):         %v (found %d callees)\n", calleesLatency, len(callees))
	fmt.Printf("  - GetDependencies('indexer.go'):   %v (found %d modules)\n", depsLatency, len(deps))
	fmt.Printf("  - GetImpact('models.go'):          %v (impacted %d files)\n", impactLatency, len(impact.ImpactedFiles))
	fmt.Println("============================================================")
}

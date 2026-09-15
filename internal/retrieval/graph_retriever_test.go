package retrieval_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"codegraph/internal/graph"
	"codegraph/internal/models"
	"codegraph/internal/retrieval"
	"codegraph/internal/storage"
)

func setupTestGraphStorage(t *testing.T) storage.Storage {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test_graph_retrieval.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create sqlite test storage: %v", err)
	}

	repoA := &models.Repository{
		ID:        "repo-A",
		Name:      "Repository A",
		LocalPath: t.TempDir(),
		Status:    models.RepoStatusIndexed,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	repoB := &models.Repository{
		ID:        "repo-B",
		Name:      "Repository B",
		LocalPath: t.TempDir(),
		Status:    models.RepoStatusIndexed,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_ = store.CreateRepository(context.Background(), repoA)
	_ = store.CreateRepository(context.Background(), repoB)

	// Nodes for Repo A
	fileNodeA1 := &models.Node{
		ID:           graph.FormatNodeID(models.NodeKindFile, "repo-A", "file-1"),
		RepositoryID: "repo-A",
		Kind:         models.NodeKindFile,
		Label:        "auth/login.go",
		RelativePath: "auth/login.go",
		FileID:       "file-1",
	}
	fileNodeA2 := &models.Node{
		ID:           graph.FormatNodeID(models.NodeKindFile, "repo-A", "file-2"),
		RepositoryID: "repo-A",
		Kind:         models.NodeKindFile,
		Label:        "service/auth.go",
		RelativePath: "service/auth.go",
		FileID:       "file-2",
	}
	symNodeCaller := &models.Node{
		ID:            graph.FormatNodeID(models.NodeKindSymbol, "repo-A", "sym-caller"),
		RepositoryID:  "repo-A",
		Kind:          models.NodeKindSymbol,
		Label:         "LoginHandler",
		QualifiedName: "auth.LoginHandler",
		FileID:        "file-1",
		RelativePath:  "auth/login.go",
		Location:      models.Location{StartLine: 10, EndLine: 20},
	}
	symNodeCallee := &models.Node{
		ID:            graph.FormatNodeID(models.NodeKindSymbol, "repo-A", "sym-callee"),
		RepositoryID:  "repo-A",
		Kind:          models.NodeKindSymbol,
		Label:         "Authenticate",
		QualifiedName: "service.Authenticate",
		FileID:        "file-2",
		RelativePath:  "service/auth.go",
		Location:      models.Location{StartLine: 5, EndLine: 15},
	}
	nodeExtModule := &models.Node{
		ID:            graph.FormatNodeID(models.NodeKindExternalModule, "repo-A", "github.com/ext/crypto"),
		RepositoryID:  "repo-A",
		Kind:          models.NodeKindExternalModule,
		Label:         "crypto",
		QualifiedName: "github.com/ext/crypto",
	}

	nodesA := []*models.Node{fileNodeA1, fileNodeA2, symNodeCaller, symNodeCallee, nodeExtModule}

	// Edges for Repo A:
	// 1. symNodeCaller (LoginHandler) -> CALLS -> symNodeCallee (Authenticate) [RESOLVED]
	// 2. fileNodeA1 (login.go) -> IMPORTS -> fileNodeA2 (auth.go) [RESOLVED]
	// 3. symNodeCallee (Authenticate) -> CALLS -> nodeExtModule (crypto.Hash) [UNRESOLVED, EXTERNAL]
	edgeCalls := &models.Edge{
		ID:           "edge-calls-1",
		RepositoryID: "repo-A",
		SourceID:     symNodeCaller.ID,
		TargetID:     symNodeCallee.ID,
		TargetKind:   models.TargetKindInternal,
		Kind:         models.EdgeKindCalls,
		Status:       models.RelStatusResolved,
		FileID:       "file-1",
		Location:     models.Location{StartLine: 15, EndLine: 15},
	}
	edgeImports := &models.Edge{
		ID:           "edge-imports-1",
		RepositoryID: "repo-A",
		SourceID:     fileNodeA1.ID,
		TargetID:     fileNodeA2.ID,
		TargetKind:   models.TargetKindInternal,
		Kind:         models.EdgeKindImports,
		Status:       models.RelStatusResolved,
		FileID:       "file-1",
	}
	edgeUnresolvedExt := &models.Edge{
		ID:           "edge-unresolved-ext",
		RepositoryID: "repo-A",
		SourceID:     symNodeCallee.ID,
		TargetID:     nodeExtModule.ID,
		TargetKind:   models.TargetKindExternal,
		Kind:         models.EdgeKindCalls,
		Status:       models.RelStatusUnresolved,
		FileID:       "file-2",
	}

	edgesA := []*models.Edge{edgeCalls, edgeImports, edgeUnresolvedExt}
	_ = store.SaveGraph(context.Background(), "repo-A", nodesA, edgesA)

	// Repo B node & edge for isolation test
	nodeB := &models.Node{
		ID:           graph.FormatNodeID(models.NodeKindSymbol, "repo-B", "sym-b"),
		RepositoryID: "repo-B",
		Kind:         models.NodeKindSymbol,
		Label:        "SecretB",
	}
	_ = store.SaveGraph(context.Background(), "repo-B", []*models.Node{nodeB}, nil)

	return store
}

func TestGraphRetriever_DirectionSemantics(t *testing.T) {
	store := setupTestGraphStorage(t)
	defer store.Close()

	retriever := retrieval.NewGraphRetriever(store)
	scope, _ := models.NewRepositoryScope("repo-A")

	// Query: CALLER_QUERY for "Authenticate" -> MUST return caller "LoginHandler"
	callers, err := retriever.Retrieve(context.Background(), scope, "Authenticate", retrieval.IntentCallerQuery, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(callers) == 0 {
		t.Fatalf("expected caller evidence for 'Authenticate'")
	}
	if callers[0].Metadata["source_name"] != "auth.LoginHandler" {
		t.Errorf("expected caller source_name = 'auth.LoginHandler', got %s", callers[0].Metadata["source_name"])
	}

	// Query: CALLEE_QUERY for "LoginHandler" -> MUST return callee "Authenticate"
	callees, err := retriever.Retrieve(context.Background(), scope, "LoginHandler", retrieval.IntentCalleeQuery, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(callees) == 0 {
		t.Fatalf("expected callee evidence for 'LoginHandler'")
	}
	if callees[0].Metadata["target_name"] != "service.Authenticate" {
		t.Errorf("expected callee target_name = 'service.Authenticate', got %s", callees[0].Metadata["target_name"])
	}

	// Query: DEPENDENCY_QUERY for "auth/login.go" -> MUST return imported file "service/auth.go" and dependent
	deps, err := retriever.Retrieve(context.Background(), scope, "auth/login.go", retrieval.IntentDependencyQuery, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) == 0 {
		t.Fatalf("expected dependency evidence for 'auth/login.go'")
	}
}

func TestGraphRetriever_InternalVsExternalTargets(t *testing.T) {
	store := setupTestGraphStorage(t)
	defer store.Close()

	retriever := retrieval.NewGraphRetriever(store)
	scope, _ := models.NewRepositoryScope("repo-A")

	// Query: CALLEE_QUERY for "Authenticate" -> returns calls to external module crypto
	callees, err := retriever.Retrieve(context.Background(), scope, "Authenticate", retrieval.IntentCalleeQuery, 10)
	if err != nil || len(callees) == 0 {
		t.Fatalf("expected callee evidence: err=%v, len=%d", err, len(callees))
	}

	foundExternal := false
	for _, item := range callees {
		if item.Metadata["target_kind"] == string(models.TargetKindExternal) {
			foundExternal = true
			if item.ResolutionStatus != models.RelStatusUnresolved {
				t.Errorf("expected unresolved status for external call edge, got %s", item.ResolutionStatus)
			}
		}
	}
	if !foundExternal {
		t.Errorf("expected external target kind metadata to be preserved without fabrication")
	}
}

func TestGraphRetriever_RepositoryIsolation(t *testing.T) {
	store := setupTestGraphStorage(t)
	defer store.Close()

	retriever := retrieval.NewGraphRetriever(store)
	scopeA, _ := models.NewRepositoryScope("repo-A")

	items, err := retriever.Retrieve(context.Background(), scopeA, "SecretB", retrieval.IntentSymbolLookup, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// repo-A query for 'SecretB' (which exists only in repo-B) MUST return 0 items
	for _, item := range items {
		if item.RepositoryID != "repo-A" {
			t.Errorf("repository isolation violated! returned item from repo %s", item.RepositoryID)
		}
	}
}

func TestGraphRetriever_DeterministicOrdering(t *testing.T) {
	store := setupTestGraphStorage(t)
	defer store.Close()

	retriever := retrieval.NewGraphRetriever(store)
	scope, _ := models.NewRepositoryScope("repo-A")

	run1, _ := retriever.Retrieve(context.Background(), scope, "LoginHandler", retrieval.IntentCalleeQuery, 10)
	run2, _ := retriever.Retrieve(context.Background(), scope, "LoginHandler", retrieval.IntentCalleeQuery, 10)

	if len(run1) != len(run2) {
		t.Fatalf("deterministic ordering test length mismatch: %d vs %d", len(run1), len(run2))
	}

	for i := range run1 {
		if run1[i].StableID != run2[i].StableID {
			t.Errorf("StableID mismatch at index %d: %s vs %s", i, run1[i].StableID, run2[i].StableID)
		}
		if run1[i].Rank != run2[i].Rank {
			t.Errorf("Rank mismatch at index %d: %d vs %d", i, run1[i].Rank, run2[i].Rank)
		}
	}
}

func TestGraphRetriever_TargetResolutionStatuses(t *testing.T) {
	tempDir := t.TempDir()
	store, _ := storage.NewSQLiteStorage(filepath.Join(tempDir, "target_res_test.db"))
	defer store.Close()

	repo := &models.Repository{
		ID:        "repo-target-res",
		Name:      "Target Res Repo",
		LocalPath: tempDir,
		Status:    models.RepoStatusIndexed,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_ = store.CreateRepository(context.Background(), repo)

	// Symbol 1: user.Login (calls UserDB)
	userLogin := &models.Node{
		ID:            graph.FormatNodeID(models.NodeKindSymbol, "repo-target-res", "sym-user-login"),
		RepositoryID:  "repo-target-res",
		Kind:          models.NodeKindSymbol,
		Label:         "Login",
		QualifiedName: "user.Login",
		RelativePath:  "user/auth.go",
		Location:      models.Location{StartLine: 10, EndLine: 20},
	}
	userDB := &models.Node{
		ID:            graph.FormatNodeID(models.NodeKindSymbol, "repo-target-res", "sym-user-db"),
		RepositoryID:  "repo-target-res",
		Kind:          models.NodeKindSymbol,
		Label:         "UserDB",
		QualifiedName: "db.UserDB",
	}

	// Symbol 2: admin.Login (calls AdminDB)
	adminLogin := &models.Node{
		ID:            graph.FormatNodeID(models.NodeKindSymbol, "repo-target-res", "sym-admin-login"),
		RepositoryID:  "repo-target-res",
		Kind:          models.NodeKindSymbol,
		Label:         "Login",
		QualifiedName: "admin.Login",
		RelativePath:  "admin/auth.go",
		Location:      models.Location{StartLine: 15, EndLine: 30},
	}
	adminDB := &models.Node{
		ID:            graph.FormatNodeID(models.NodeKindSymbol, "repo-target-res", "sym-admin-db"),
		RepositoryID:  "repo-target-res",
		Kind:          models.NodeKindSymbol,
		Label:         "AdminDB",
		QualifiedName: "db.AdminDB",
	}

	edgeUser := &models.Edge{
		ID:           "edge-user-calls",
		RepositoryID: "repo-target-res",
		SourceID:     userLogin.ID,
		TargetID:     userDB.ID,
		TargetKind:   models.TargetKindInternal,
		Kind:         models.EdgeKindCalls,
		Status:       models.RelStatusResolved,
		FileID:       "file-user",
	}
	edgeAdmin := &models.Edge{
		ID:           "edge-admin-calls",
		RepositoryID: "repo-target-res",
		SourceID:     adminLogin.ID,
		TargetID:     adminDB.ID,
		TargetKind:   models.TargetKindInternal,
		Kind:         models.EdgeKindCalls,
		Status:       models.RelStatusResolved,
		FileID:       "file-admin",
	}

	_ = store.SaveGraph(context.Background(), "repo-target-res",
		[]*models.Node{userLogin, userDB, adminLogin, adminDB},
		[]*models.Edge{edgeUser, edgeAdmin},
	)

	retriever := retrieval.NewGraphRetriever(store)
	scope, _ := models.NewRepositoryScope("repo-target-res")

	// Test 1: Ambiguous target resolution (query "Login" matches both user.Login and admin.Login)
	ambigItems, err := retriever.Retrieve(context.Background(), scope, "Login", retrieval.IntentCalleeQuery, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ambigItems) < 2 {
		t.Fatalf("expected 2 items for ambiguous target query 'Login', got %d", len(ambigItems))
	}
	for _, item := range ambigItems {
		if item.TargetResolution != models.TargetAmbiguous {
			t.Errorf("expected TargetResolution = TARGET_AMBIGUOUS, got %s", item.TargetResolution)
		}
		if item.Metadata["candidate_count"] != "2" {
			t.Errorf("expected candidate_count = 2, got %s", item.Metadata["candidate_count"])
		}
	}

	// Test 2: Qualified target resolution (query "user.Login" uniquely matches user.Login)
	qualItems, err := retriever.Retrieve(context.Background(), scope, "user.Login", retrieval.IntentCalleeQuery, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(qualItems) != 1 {
		t.Fatalf("expected exactly 1 item for qualified query 'user.Login', got %d", len(qualItems))
	}
	if qualItems[0].TargetResolution != models.TargetResolved {
		t.Errorf("expected TargetResolution = TARGET_RESOLVED for qualified query, got %s", qualItems[0].TargetResolution)
	}
	if qualItems[0].Metadata["target_name"] != "db.UserDB" {
		t.Errorf("expected target_name = 'db.UserDB', got %s", qualItems[0].Metadata["target_name"])
	}

	// Test 3: No target found (query "NonExistentFunction" yields TargetNotFound)
	notFoundItems, err := retriever.Retrieve(context.Background(), scope, "NonExistentFunction", retrieval.IntentCalleeQuery, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notFoundItems) != 0 {
		t.Errorf("expected 0 items for non-existent target, got %d", len(notFoundItems))
	}
}

func BenchmarkGraphRetriever_ColdPath_SQLiteLoadAndTraversal(b *testing.B) {
	tempDir := b.TempDir()
	store, err := storage.NewSQLiteStorage(filepath.Join(tempDir, "bench_cold.db"))
	if err != nil {
		b.Fatalf("failed to create bench storage: %v", err)
	}
	defer store.Close()

	repo := &models.Repository{
		ID:        "repo-cold",
		Name:      "Cold Repo",
		LocalPath: tempDir,
		Status:    models.RepoStatusIndexed,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_ = store.CreateRepository(context.Background(), repo)

	var nodes []*models.Node
	var edges []*models.Edge

	for i := 0; i < 1000; i++ {
		n := &models.Node{
			ID:            graph.FormatNodeID(models.NodeKindSymbol, "repo-cold", fmt.Sprintf("sym-%d", i)),
			RepositoryID:  "repo-cold",
			Kind:          models.NodeKindSymbol,
			Label:         fmt.Sprintf("SymbolFunction_%d", i),
			QualifiedName: fmt.Sprintf("pkg.SymbolFunction_%d", i),
		}
		nodes = append(nodes, n)
	}

	for i := 0; i < 999; i++ {
		e := &models.Edge{
			ID:           fmt.Sprintf("edge-%d", i),
			RepositoryID: "repo-cold",
			SourceID:     nodes[i].ID,
			TargetID:     nodes[i+1].ID,
			TargetKind:   models.TargetKindInternal,
			Kind:         models.EdgeKindCalls,
			Status:       models.RelStatusResolved,
		}
		edges = append(edges, e)
	}

	_ = store.SaveGraph(context.Background(), "repo-cold", nodes, edges)

	retriever := retrieval.NewGraphRetriever(store)
	scope, _ := models.NewRepositoryScope("repo-cold")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = retriever.Retrieve(context.Background(), scope, "SymbolFunction_500", retrieval.IntentCallerQuery, 10)
	}
}

func BenchmarkGraphRetriever_WarmPath_EngineTraversalOnly(b *testing.B) {
	var nodes []*models.Node
	var edges []*models.Edge

	for i := 0; i < 1000; i++ {
		n := &models.Node{
			ID:            graph.FormatNodeID(models.NodeKindSymbol, "repo-warm", fmt.Sprintf("sym-%d", i)),
			RepositoryID:  "repo-warm",
			Kind:          models.NodeKindSymbol,
			Label:         fmt.Sprintf("SymbolFunction_%d", i),
			QualifiedName: fmt.Sprintf("pkg.SymbolFunction_%d", i),
		}
		nodes = append(nodes, n)
	}

	for i := 0; i < 999; i++ {
		e := &models.Edge{
			ID:           fmt.Sprintf("edge-%d", i),
			RepositoryID: "repo-warm",
			SourceID:     nodes[i].ID,
			TargetID:     nodes[i+1].ID,
			TargetKind:   models.TargetKindInternal,
			Kind:         models.EdgeKindCalls,
			Status:       models.RelStatusResolved,
		}
		edges = append(edges, e)
	}

	engine := graph.NewEngine("repo-warm")
	engine.LoadGraph(nodes, edges)

	targetID := graph.FormatNodeID(models.NodeKindSymbol, "repo-warm", "sym-500")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		callSites := engine.GetCallers("sym-500")
		_ = callSites
		_ = targetID
	}
}

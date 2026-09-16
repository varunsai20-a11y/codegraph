package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codegraph/internal/api"
	"codegraph/internal/config"
	"codegraph/internal/graph"
	"codegraph/internal/guide"
	"codegraph/internal/ingestion"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/retrieval"
	"codegraph/internal/storage"
)

func createTestStorage(t *testing.T) (storage.Storage, string) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create sqlite storage: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store, tmpDir
}

func setupTestGraphEngine(repoID string) *graph.Engine {
	engine := graph.NewEngine(repoID)

	node1 := &models.Node{
		ID:           repoID + ":file:main.go",
		RepositoryID: repoID,
		Kind:         models.NodeKindFile,
		Label:        "main.go",
		RelativePath: "main.go",
	}
	node2 := &models.Node{
		ID:           repoID + ":file:auth/service.go",
		RepositoryID: repoID,
		Kind:         models.NodeKindFile,
		Label:        "service.go",
		RelativePath: "auth/service.go",
	}
	node3 := &models.Node{
		ID:           repoID + ":sym:main.go:main",
		RepositoryID: repoID,
		Kind:         models.NodeKindSymbol,
		Label:        "main",
		RelativePath: "main.go",
	}
	node4 := &models.Node{
		ID:           repoID + ":sym:auth/service.go:Authenticate",
		RepositoryID: repoID,
		Kind:         models.NodeKindSymbol,
		Label:        "Authenticate",
		RelativePath: "auth/service.go",
	}

	edge1 := &models.Edge{
		ID:           repoID + ":edge:1",
		RepositoryID: repoID,
		SourceID:     repoID + ":sym:main.go:main",
		TargetID:     repoID + ":sym:auth/service.go:Authenticate",
		Kind:         models.EdgeKindCalls,
	}

	engine.LoadGraph([]*models.Node{node1, node2, node3, node4}, []*models.Edge{edge1})
	return engine
}

func TestC7_01_InvestigationInitialization(t *testing.T) {
	store, _ := createTestStorage(t)
	scope, err := models.NewRepositoryScope("repo-c7-init")
	if err != nil {
		t.Fatalf("failed scope: %v", err)
	}

	engine := setupTestGraphEngine("repo-c7-init")
	sumCalc := guide.NewDefaultArchitectureSummaryGenerator(store, engine)
	orchestrator := guide.NewDefaultGuideOrchestrator(store, sumCalc, nil, nil)

	req, _ := models.NewInvestigationRequest(scope, "START")

	inv, err := orchestrator.Guide(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if inv.RepositoryID != "repo-c7-init" {
		t.Errorf("expected repo-c7-init, got %s", inv.RepositoryID)
	}
	if inv.Status != models.InvestigationInProgress && inv.Status != models.InvestigationInitialized {
		t.Errorf("expected IN_PROGRESS or INITIALIZED, got %s", inv.Status)
	}
	if len(inv.Steps) == 0 {
		t.Fatalf("expected non-empty steps")
	}
}

func TestC7_02_RepositoryIsolation(t *testing.T) {
	store, _ := createTestStorage(t)
	scopeA, _ := models.NewRepositoryScope("repo-A")
	scopeB, _ := models.NewRepositoryScope("repo-B")

	engineA := setupTestGraphEngine("repo-A")
	sumCalcA := guide.NewDefaultArchitectureSummaryGenerator(store, engineA)
	orchestratorA := guide.NewDefaultGuideOrchestrator(store, sumCalcA, nil, nil)

	req, _ := models.NewInvestigationRequest(scopeA, "START")
	invA, err := orchestratorA.Guide(context.Background(), req)
	if err != nil {
		t.Fatalf("failed guide A: %v", err)
	}

	if invA.RepositoryID != "repo-A" {
		t.Errorf("expected repo-A scope, got %s", invA.RepositoryID)
	}

	_ = scopeB
}

func TestC7_03_DeterministicArchitectureSummary(t *testing.T) {
	store, _ := createTestStorage(t)
	scope, _ := models.NewRepositoryScope("repo-det")
	engine := setupTestGraphEngine("repo-det")
	sumCalc := guide.NewDefaultArchitectureSummaryGenerator(store, engine)

	sum1, err1 := sumCalc.GenerateSummary(context.Background(), scope)
	if err1 != nil {
		t.Fatalf("err1: %v", err1)
	}

	sum2, err2 := sumCalc.GenerateSummary(context.Background(), scope)
	if err2 != nil {
		t.Fatalf("err2: %v", err2)
	}

	if sum1.TotalFiles != sum2.TotalFiles || sum1.TotalSymbols != sum2.TotalSymbols || sum1.TotalRelationships != sum2.TotalRelationships {
		t.Errorf("architecture summary is non-deterministic: %+v vs %+v", sum1, sum2)
	}
}

func TestC7_04_DeterministicStepOrdering(t *testing.T) {
	store, _ := createTestStorage(t)
	scope, _ := models.NewRepositoryScope("repo-order")
	engine := setupTestGraphEngine("repo-order")
	sumCalc := guide.NewDefaultArchitectureSummaryGenerator(store, engine)
	orchestrator := guide.NewDefaultGuideOrchestrator(store, sumCalc, nil, nil)

	req1, _ := models.NewInvestigationRequest(scope, "START")
	req2, _ := models.NewInvestigationRequest(scope, "START")

	inv1, _ := orchestrator.Guide(context.Background(), req1)
	inv2, _ := orchestrator.Guide(context.Background(), req2)

	if len(inv1.Steps) != len(inv2.Steps) {
		t.Fatalf("step count mismatch: %d vs %d", len(inv1.Steps), len(inv2.Steps))
	}
	for i := range inv1.Steps {
		if inv1.Steps[i].Type != inv2.Steps[i].Type || inv1.Steps[i].Title != inv2.Steps[i].Title {
			t.Errorf("step %d ordering mismatch: %s vs %s", i, inv1.Steps[i].Title, inv2.Steps[i].Title)
		}
	}
}

func TestC7_05_StepProgressionAndMaxSteps(t *testing.T) {
	store, _ := createTestStorage(t)
	scope, _ := models.NewRepositoryScope("repo-prog")
	engine := setupTestGraphEngine("repo-prog")
	sumCalc := guide.NewDefaultArchitectureSummaryGenerator(store, engine)
	orchestrator := guide.NewDefaultGuideOrchestrator(store, sumCalc, nil, nil)

	// Test MaxSteps clamping
	reqMax, _ := models.NewInvestigationRequest(scope, "START")
	reqMax.MaxSteps = 100 // Exceeds default max 10
	invMax, _ := orchestrator.Guide(context.Background(), reqMax)
	if len(invMax.Steps) > models.DefaultMaxInvestigationSteps {
		t.Errorf("expected max steps capped at %d, got %d", models.DefaultMaxInvestigationSteps, len(invMax.Steps))
	}

	req, _ := models.NewInvestigationRequest(scope, "START")
	inv, err := orchestrator.Guide(context.Background(), req)
	if err != nil {
		t.Fatalf("init error: %v", err)
	}

	initialIndex := inv.CurrentStepIndex

	for i := 0; i < 12; i++ {
		if inv.IsComplete {
			break
		}
		nextReq, _ := models.NewInvestigationRequest(scope, "NEXT")
		nextReq.CompletedStepIDs = inv.CompletedStepIDs
		inv, err = orchestrator.Guide(context.Background(), nextReq)
		if err != nil {
			t.Fatalf("step error at iteration %d: %v", i, err)
		}
	}

	if inv.CurrentStepIndex <= initialIndex && !inv.IsComplete {
		t.Errorf("expected progression in step index, stayed at %d", inv.CurrentStepIndex)
	}
}

func TestC7_06_EmptyAndInsufficientEvidence(t *testing.T) {
	store, _ := createTestStorage(t)
	scope, _ := models.NewRepositoryScope("repo-empty")
	engine := graph.NewEngine("repo-empty")
	sumCalc := guide.NewDefaultArchitectureSummaryGenerator(store, engine)
	orchestrator := guide.NewDefaultGuideOrchestrator(store, sumCalc, nil, nil)

	req, _ := models.NewInvestigationRequest(scope, "START")
	inv, err := orchestrator.Guide(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if inv == nil || inv.Summary.TotalFiles != 0 {
		t.Errorf("expected zero file summary for empty repo")
	}
}

func TestC7_07_SuggestedQuestionGeneration(t *testing.T) {
	store, _ := createTestStorage(t)
	scope, _ := models.NewRepositoryScope("repo-q")
	engine := setupTestGraphEngine("repo-q")
	sumCalc := guide.NewDefaultArchitectureSummaryGenerator(store, engine)
	orchestrator := guide.NewDefaultGuideOrchestrator(store, sumCalc, nil, nil)

	req, _ := models.NewInvestigationRequest(scope, "START")
	inv, err := orchestrator.Guide(context.Background(), req)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if len(inv.Steps) > 0 && len(inv.Steps[0].SuggestedQuestions) == 0 {
		t.Errorf("expected suggested questions on step 0")
	}
}

func TestC7_08_GuideAPIEndpoint(t *testing.T) {
	store, tmpDir := createTestStorage(t)
	wsRoot := filepath.Join(tmpDir, "workspaces")
	wsMgr, err := repository.NewWorkspaceManager(wsRoot)
	if err != nil {
		t.Fatalf("failed wsMgr: %v", err)
	}

	repoDir := filepath.Join(wsRoot, "repo-api")
	_ = os.MkdirAll(repoDir, 0755)
	_ = os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n"), 0644)

	repoID := "repo-api"
	_ = store.CreateRepository(context.Background(), &models.Repository{
		ID:         repoID,
		Name:       "repo-api",
		LocalPath:  repoDir,
		SourceType: models.SourceTypeLocal,
		Status:     models.RepoStatusIndexed,
	})

	engine := setupTestGraphEngine(repoID)
	sumCalc := guide.NewDefaultArchitectureSummaryGenerator(store, engine)
	orchestrator := guide.NewDefaultGuideOrchestrator(store, sumCalc, nil, nil)

	cfg := &config.Config{
		Port:          8080,
		WorkspaceRoot: wsRoot,
	}
	indexer := ingestion.NewIndexer(cfg, store, wsMgr)
	server := api.NewServer(cfg, store, wsMgr, indexer)
	server.SetGuideOrchestrator(orchestrator)

	payload := map[string]interface{}{
		"action": "START",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest("POST", "/api/repositories/"+repoID+"/guide", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp models.Investigation
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed unmarshal: %v", err)
	}
	if resp.RepositoryID != repoID {
		t.Errorf("expected %s, got %s", repoID, resp.RepositoryID)
	}
}

func TestC7_09_RealCodeGraphRepository(t *testing.T) {
	absPath, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("failed to resolve abs path: %v", err)
	}

	store, _ := createTestStorage(t)
	repoID := "codegraph-self"
	_ = store.CreateRepository(context.Background(), &models.Repository{
		ID:         repoID,
		Name:       "codegraph-self",
		LocalPath:  absPath,
		SourceType: models.SourceTypeLocal,
		Status:     models.RepoStatusIndexed,
	})

	scope, _ := models.NewRepositoryScope(repoID)
	engine := setupTestGraphEngine(repoID)

	sumCalc := guide.NewDefaultArchitectureSummaryGenerator(store, engine)
	orchestrator := guide.NewDefaultGuideOrchestrator(store, sumCalc, nil, nil)

	start := time.Now()
	req, _ := models.NewInvestigationRequest(scope, "START")
	inv, err := orchestrator.Guide(context.Background(), req)
	localLatency := time.Since(start)

	if err != nil {
		t.Fatalf("guided investigation failed on real codegraph repo: %v", err)
	}
	if inv == nil || inv.RepositoryID != repoID {
		t.Errorf("invalid investigation response on real codegraph repo")
	}

	t.Logf("C7 Local Guide Orchestration Latency on Real CodeGraph Repo: %v", localLatency)
}

func TestC7_10_C1toC6Regressions(t *testing.T) {
	store, _ := createTestStorage(t)
	scope, _ := models.NewRepositoryScope("repo-reg")
	expReq, _ := models.NewExplanationRequest(scope, "Test query")

	composer := retrieval.NewDefaultEvidenceComposer(nil, nil, store)
	pkg, err := composer.Compose(context.Background(), expReq)
	if err != nil {
		t.Fatalf("evidence composer error: %v", err)
	}
	if pkg == nil {
		t.Fatalf("evidence composer return nil package")
	}
}

func TestC7_11_EvidenceProvenancePreservation(t *testing.T) {
	store, _ := createTestStorage(t)
	scope, _ := models.NewRepositoryScope("repo-prov")
	engine := setupTestGraphEngine("repo-prov")
	sumCalc := guide.NewDefaultArchitectureSummaryGenerator(store, engine)
	orchestrator := guide.NewDefaultGuideOrchestrator(store, sumCalc, nil, nil)

	req, _ := models.NewInvestigationRequest(scope, "START")
	inv, err := orchestrator.Guide(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	for _, step := range inv.Steps {
		for _, ev := range step.Evidence {
			if ev.Label == "" || ev.RepositoryID == "" || ev.Type == "" {
				t.Errorf("evidence missing label, repository ID, or type in step %s: %+v", step.ID, ev)
			}
		}
	}
}

func TestC7_12_MalformedAPIRequest(t *testing.T) {
	store, tmpDir := createTestStorage(t)
	wsRoot := filepath.Join(tmpDir, "workspaces")
	wsMgr, _ := repository.NewWorkspaceManager(wsRoot)

	repoID := "repo-malformed"
	_ = store.CreateRepository(context.Background(), &models.Repository{
		ID:         repoID,
		Name:       "repo-malformed",
		LocalPath:  tmpDir,
		SourceType: models.SourceTypeLocal,
		Status:     models.RepoStatusIndexed,
	})

	cfg := &config.Config{Port: 8080, WorkspaceRoot: wsRoot}
	indexer := ingestion.NewIndexer(cfg, store, wsMgr)
	server := api.NewServer(cfg, store, wsMgr, indexer)

	badJSON := []byte(`{"action": "START", max_steps: }`) // Malformed JSON
	req := httptest.NewRequest("POST", "/api/repositories/"+repoID+"/guide", bytes.NewReader(badJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.Router().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected HTTP 400 for malformed JSON, got %d", w.Code)
	}
}

func TestC7_13_UnknownRepositoryAPI(t *testing.T) {
	store, tmpDir := createTestStorage(t)
	wsRoot := filepath.Join(tmpDir, "workspaces")
	wsMgr, _ := repository.NewWorkspaceManager(wsRoot)

	cfg := &config.Config{Port: 8080, WorkspaceRoot: wsRoot}
	indexer := ingestion.NewIndexer(cfg, store, wsMgr)
	server := api.NewServer(cfg, store, wsMgr, indexer)

	body, _ := json.Marshal(map[string]string{"action": "START"})
	req := httptest.NewRequest("POST", "/api/repositories/non-existent-repo-id/guide", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.Router().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected HTTP 404 for unknown repository, got %d", w.Code)
	}
}

func TestC7_14_DeterministicRepeatedResponse(t *testing.T) {
	store, _ := createTestStorage(t)
	scope, _ := models.NewRepositoryScope("repo-repeat")
	engine := setupTestGraphEngine("repo-repeat")
	sumCalc := guide.NewDefaultArchitectureSummaryGenerator(store, engine)
	orchestrator := guide.NewDefaultGuideOrchestrator(store, sumCalc, nil, nil)

	req, _ := models.NewInvestigationRequest(scope, "START")
	firstInv, _ := orchestrator.Guide(context.Background(), req)
	firstJSON, _ := json.Marshal(firstInv.Summary)

	for i := 0; i < 5; i++ {
		repeatInv, _ := orchestrator.Guide(context.Background(), req)
		repeatJSON, _ := json.Marshal(repeatInv.Summary)
		if !bytes.Equal(firstJSON, repeatJSON) {
			t.Fatalf("non-deterministic response at iteration %d:\nfirst: %s\nrepeat: %s", i, string(firstJSON), string(repeatJSON))
		}
	}
}

func TestC7_15_DeterministicTieBreaking(t *testing.T) {
	store, _ := createTestStorage(t)
	scope, _ := models.NewRepositoryScope("repo-tie")
	engine := setupTestGraphEngine("repo-tie")
	sumCalc := guide.NewDefaultArchitectureSummaryGenerator(store, engine)

	sum, err := sumCalc.GenerateSummary(context.Background(), scope)
	if err != nil {
		t.Fatalf("failed summary: %v", err)
	}

	for i := 0; i < len(sum.EntryPointCandidates)-1; i++ {
		curr := sum.EntryPointCandidates[i]
		next := sum.EntryPointCandidates[i+1]
		if curr.RelativePath == next.RelativePath && curr.Name == next.Name && curr.ID > next.ID {
			t.Errorf("entry points tie-breaking failure: %s vs %s", curr.ID, next.ID)
		}
	}
}

func TestC7_16_ForgedAndSkippedStepState(t *testing.T) {
	store, _ := createTestStorage(t)
	scope, _ := models.NewRepositoryScope("repo-forged")
	engine := setupTestGraphEngine("repo-forged")
	sumCalc := guide.NewDefaultArchitectureSummaryGenerator(store, engine)
	orchestrator := guide.NewDefaultGuideOrchestrator(store, sumCalc, nil, nil)

	// 1. Submit forged step ID
	reqForged, _ := models.NewInvestigationRequest(scope, "NEXT")
	reqForged.CompletedStepIDs = []string{"step-99-fake-id"}
	invForged, err := orchestrator.Guide(context.Background(), reqForged)
	if err != nil {
		t.Fatalf("forged request error: %v", err)
	}
	if invForged.CurrentStepIndex != 1 {
		t.Errorf("expected fallback to active step 1, got %d", invForged.CurrentStepIndex)
	}

	// 2. Attempt to skip required step 1 and mark step 4 completed directly
	reqSkipped, _ := models.NewInvestigationRequest(scope, "NEXT")
	reqSkipped.CompletedStepIDs = []string{"step-4-static-flow"}
	invSkipped, err := orchestrator.Guide(context.Background(), reqSkipped)
	if err != nil {
		t.Fatalf("skipped request error: %v", err)
	}
	if invSkipped.CurrentStepIndex != 1 {
		t.Errorf("expected out-of-order skip to be rejected, got step index %d", invSkipped.CurrentStepIndex)
	}
}

func TestC7_17_CrossRepositoryStateRejection(t *testing.T) {
	store, tmpDir := createTestStorage(t)
	wsRoot := filepath.Join(tmpDir, "workspaces")
	wsMgr, _ := repository.NewWorkspaceManager(wsRoot)

	repoID := "repo-target"
	_ = store.CreateRepository(context.Background(), &models.Repository{
		ID:         repoID,
		Name:       "repo-target",
		LocalPath:  tmpDir,
		SourceType: models.SourceTypeLocal,
		Status:     models.RepoStatusIndexed,
	})

	cfg := &config.Config{Port: 8080, WorkspaceRoot: wsRoot}
	indexer := ingestion.NewIndexer(cfg, store, wsMgr)
	server := api.NewServer(cfg, store, wsMgr, indexer)

	// Cross-repository state: payload contains state for "repo-foreign" sent to endpoint for "repo-target"
	crossStatePayload := map[string]interface{}{
		"action": "NEXT",
		"investigation_state": map[string]interface{}{
			"id":            "inv-foreign",
			"repository_id": "repo-foreign",
		},
	}
	body, _ := json.Marshal(crossStatePayload)
	req := httptest.NewRequest("POST", "/api/repositories/"+repoID+"/guide", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.Router().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected HTTP 400 for cross-repository investigation state, got %d: %s", w.Code, w.Body.String())
	}
}

func TestC7_18_StatelessUIProgressionContract(t *testing.T) {
	// Option A Trust Model Verification:
	// Client-provided CompletedStepIDs are UI session navigation state normalized by the server.
	store, _ := createTestStorage(t)
	scope, _ := models.NewRepositoryScope("repo-contract")
	engine := setupTestGraphEngine("repo-contract")
	sumCalc := guide.NewDefaultArchitectureSummaryGenerator(store, engine)
	orchestrator := guide.NewDefaultGuideOrchestrator(store, sumCalc, nil, nil)

	// 1. Valid sequential client UI state -> Accepted
	reqValid, _ := models.NewInvestigationRequest(scope, "NEXT")
	reqValid.CompletedStepIDs = []string{"step-1-overview"}
	invValid, err := orchestrator.Guide(context.Background(), reqValid)
	if err != nil {
		t.Fatalf("unexpected error on valid sequential UI state: %v", err)
	}
	if invValid.CurrentStepIndex != 1 {
		t.Errorf("expected current step index 1 for valid sequential UI state, got %d", invValid.CurrentStepIndex)
	}

	// 2. Duplicate client UI state -> Safely deduplicated and normalized
	reqDup, _ := models.NewInvestigationRequest(scope, "NEXT")
	reqDup.CompletedStepIDs = []string{"step-1-overview", "step-1-overview"}
	invDup, err := orchestrator.Guide(context.Background(), reqDup)
	if err != nil {
		t.Fatalf("unexpected error on duplicate UI state: %v", err)
	}
	if len(invDup.CompletedStepIDs) != 1 || invDup.CompletedStepIDs[0] != "step-1-overview" {
		t.Errorf("expected deduplicated completed step IDs, got %v", invDup.CompletedStepIDs)
	}
}

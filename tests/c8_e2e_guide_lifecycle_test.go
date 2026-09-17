package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"codegraph/internal/api"
	"codegraph/internal/config"
	"codegraph/internal/guide"
	"codegraph/internal/ingestion"
	"codegraph/internal/llm"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/retrieval"
	"codegraph/internal/storage"
)

func TestE2E_GuideLifecycleWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "guide.db")
	wsRoot := filepath.Join(tmpDir, "workspaces")

	cfg := &config.Config{
		Port:               8080,
		MaxFileSize:        2 * 1024 * 1024,
		WorkspaceRoot:      wsRoot,
		DatabasePath:       dbPath,
		DefaultExclusions:  []string{".git"},
		SupportedLanguages: []string{"Go", "TypeScript", "Java"},
	}

	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	wsMgr, err := repository.NewWorkspaceManager(wsRoot)
	if err != nil {
		t.Fatalf("failed to init workspace manager: %v", err)
	}

	indexer := ingestion.NewIndexer(cfg, store, wsMgr)
	server := api.NewServer(cfg, store, wsMgr, indexer)

	fixturePath, err := filepath.Abs(filepath.Join("fixtures", "sample-repository"))
	if err != nil {
		t.Fatalf("failed to resolve fixture path: %v", err)
	}

	// Register & Index Repository
	regBody, _ := json.Marshal(api.RegisterRepoRequest{
		Name:       "Guide E2E Repo",
		SourceType: models.SourceTypeLocal,
		LocalPath:  fixturePath,
	})
	req := httptest.NewRequest("POST", "/api/repositories", bytes.NewReader(regBody))
	rec := httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register status = %d; want 201", rec.Code)
	}

	var repo models.Repository
	_ = json.Unmarshal(rec.Body.Bytes(), &repo)

	ctx := context.Background()
	job := &models.IndexJob{ID: "job-guide-1", RepositoryID: repo.ID, Status: models.JobStatusPending, StartedAt: time.Now()}
	_ = store.CreateIndexJob(ctx, job)
	if err := indexer.RunIndex(ctx, job, &repo); err != nil {
		t.Fatalf("indexing failed: %v", err)
	}

	engine, _ := indexer.GetGraphEngine(repo.ID)
	sumGen := guide.NewDefaultArchitectureSummaryGenerator(store, engine)
	composer := retrieval.NewDefaultEvidenceComposer(nil, nil, store)
	expSvc := llm.NewGroundedExplanationService(nil, nil, composer, nil)
	orchestrator := guide.NewDefaultGuideOrchestrator(store, sumGen, composer, expSvc)
	server.SetGuideOrchestrator(orchestrator)

	// Step 1: START action
	bodyStart, _ := json.Marshal(map[string]interface{}{"action": "START"})
	reqStart := httptest.NewRequest("POST", "/api/repositories/"+repo.ID+"/guide", bytes.NewReader(bodyStart))
	reqStart.Header.Set("Content-Type", "application/json")
	recStart := httptest.NewRecorder()
	server.Router().ServeHTTP(recStart, reqStart)

	if recStart.Code != http.StatusOK {
		t.Fatalf("Guide START status = %d; want 200. Body: %s", recStart.Code, recStart.Body.String())
	}

	var inv1 models.Investigation
	if err := json.Unmarshal(recStart.Body.Bytes(), &inv1); err != nil {
		t.Fatalf("failed to parse Investigation: %v", err)
	}

	if inv1.RepositoryID != repo.ID {
		t.Errorf("expected RepositoryID %s, got %s", repo.ID, inv1.RepositoryID)
	}
	if len(inv1.Steps) < 5 {
		t.Fatalf("expected 5 investigation steps, got %d", len(inv1.Steps))
	}
	if inv1.Steps[0].Status != models.StepActive {
		t.Errorf("expected Step 1 Status == STEP_ACTIVE, got: %s", inv1.Steps[0].Status)
	}

	// Verify Step 4 (static flow) presence
	step4 := inv1.Steps[3]
	if step4.ID != "step-4-static-flow" {
		t.Errorf("expected step 4 ID 'step-4-static-flow', got '%s'", step4.ID)
	}

	// Step 2: NEXT action with valid step completed
	bodyNext1, _ := json.Marshal(map[string]interface{}{
		"action":             "NEXT",
		"current_step_index": 0,
		"completed_step_ids": []string{"step-1-overview"},
	})
	reqNext1 := httptest.NewRequest("POST", "/api/repositories/"+repo.ID+"/guide", bytes.NewReader(bodyNext1))
	reqNext1.Header.Set("Content-Type", "application/json")
	recNext1 := httptest.NewRecorder()
	server.Router().ServeHTTP(recNext1, reqNext1)

	if recNext1.Code != http.StatusOK {
		t.Fatalf("Guide NEXT status = %d; want 200", recNext1.Code)
	}

	var inv2 models.Investigation
	_ = json.Unmarshal(recNext1.Body.Bytes(), &inv2)

	if inv2.Steps[0].Status != models.StepCompleted {
		t.Errorf("expected Step 1 Status == STEP_COMPLETED, got: %s", inv2.Steps[0].Status)
	}
	if inv2.Steps[1].Status != models.StepActive {
		t.Errorf("expected Step 2 Status == STEP_ACTIVE, got: %s", inv2.Steps[1].Status)
	}

	// Step 3: Forged / Out-of-Order Step IDs test
	bodyForged, _ := json.Marshal(map[string]interface{}{
		"action":             "NEXT",
		"current_step_index": 1,
		"completed_step_ids": []string{"step-1-overview", "step-99-forged-fake-id"},
	})
	reqForged := httptest.NewRequest("POST", "/api/repositories/"+repo.ID+"/guide", bytes.NewReader(bodyForged))
	reqForged.Header.Set("Content-Type", "application/json")
	recForged := httptest.NewRecorder()
	server.Router().ServeHTTP(recForged, reqForged)

	if recForged.Code != http.StatusOK {
		t.Fatalf("Guide Forged status = %d; want 200", recForged.Code)
	}

	var invForged models.Investigation
	_ = json.Unmarshal(recForged.Body.Bytes(), &invForged)

	// Forged ID must be safely filtered out
	for _, cid := range invForged.CompletedStepIDs {
		if cid == "step-99-forged-fake-id" {
			t.Errorf("expected forged step ID to be normalized/filtered out by server")
		}
	}

	// Step 4: Progress all steps to reach COMPLETED state
	allStepIDs := []string{
		"step-1-overview",
		"step-2-module-structure",
		"step-3-important-symbols",
		"step-4-static-flow",
		"step-5-explanation",
	}

	bodyFinal, _ := json.Marshal(map[string]interface{}{
		"action":             "NEXT",
		"current_step_index": 4,
		"completed_step_ids": allStepIDs,
	})
	reqFinal := httptest.NewRequest("POST", "/api/repositories/"+repo.ID+"/guide", bytes.NewReader(bodyFinal))
	reqFinal.Header.Set("Content-Type", "application/json")
	recFinal := httptest.NewRecorder()
	server.Router().ServeHTTP(recFinal, reqFinal)

	if recFinal.Code != http.StatusOK {
		t.Fatalf("Guide Final status = %d; want 200", recFinal.Code)
	}

	var invFinal models.Investigation
	_ = json.Unmarshal(recFinal.Body.Bytes(), &invFinal)

	if !invFinal.IsComplete {
		t.Errorf("expected invFinal.IsComplete == true, got false")
	}
	if invFinal.Status != models.InvestigationCompleted {
		t.Errorf("expected invFinal.Status == INVESTIGATION_COMPLETED, got: %s", invFinal.Status)
	}
}

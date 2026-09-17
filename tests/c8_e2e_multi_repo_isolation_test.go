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
	"codegraph/internal/guide"
	"codegraph/internal/ingestion"
	"codegraph/internal/llm"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/retrieval"
	"codegraph/internal/storage"
)

func TestE2E_MultiRepositoryIsolationWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "multirepo.db")
	wsRoot := filepath.Join(tmpDir, "workspaces")

	cfg := &config.Config{
		Port:               8080,
		MaxFileSize:        2 * 1024 * 1024,
		WorkspaceRoot:      wsRoot,
		DatabasePath:       dbPath,
		DefaultExclusions:  []string{".git"},
		SupportedLanguages: []string{"Go"},
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

	// Create source directory for Repo A
	dirA := filepath.Join(tmpDir, "repo-A")
	_ = os.MkdirAll(dirA, 0755)
	_ = os.WriteFile(filepath.Join(dirA, "main.go"), []byte("package main\nfunc SharedName() {}\n"), 0644)

	// Create source directory for Repo B
	dirB := filepath.Join(tmpDir, "repo-B")
	_ = os.MkdirAll(dirB, 0755)
	_ = os.WriteFile(filepath.Join(dirB, "main.go"), []byte("package main\nfunc SharedName() {}\n"), 0644)

	ctx := context.Background()

	// Register & Index Repo A
	regBodyA, _ := json.Marshal(api.RegisterRepoRequest{Name: "Repo A", SourceType: models.SourceTypeLocal, LocalPath: dirA})
	reqA := httptest.NewRequest("POST", "/api/repositories", bytes.NewReader(regBodyA))
	recA := httptest.NewRecorder()
	server.Router().ServeHTTP(recA, reqA)
	if recA.Code != http.StatusCreated {
		t.Fatalf("register Repo A status = %d; want 201", recA.Code)
	}
	var repoA models.Repository
	_ = json.Unmarshal(recA.Body.Bytes(), &repoA)

	jobA := &models.IndexJob{ID: "job-iso-A", RepositoryID: repoA.ID, Status: models.JobStatusPending, StartedAt: time.Now()}
	_ = store.CreateIndexJob(ctx, jobA)
	if err := indexer.RunIndex(ctx, jobA, &repoA); err != nil {
		t.Fatalf("indexing Repo A failed: %v", err)
	}

	// Register & Index Repo B
	regBodyB, _ := json.Marshal(api.RegisterRepoRequest{Name: "Repo B", SourceType: models.SourceTypeLocal, LocalPath: dirB})
	reqB := httptest.NewRequest("POST", "/api/repositories", bytes.NewReader(regBodyB))
	recB := httptest.NewRecorder()
	server.Router().ServeHTTP(recB, reqB)
	var repoB models.Repository
	_ = json.Unmarshal(recB.Body.Bytes(), &repoB)

	jobB := &models.IndexJob{ID: "job-iso-B", RepositoryID: repoB.ID, Status: models.JobStatusPending, StartedAt: time.Now()}
	_ = store.CreateIndexJob(ctx, jobB)
	if err := indexer.RunIndex(ctx, jobB, &repoB); err != nil {
		t.Fatalf("indexing Repo B failed: %v", err)
	}

	// 1. Verify Symbols Isolation for Repo A
	reqSymA := httptest.NewRequest("GET", "/api/repositories/"+repoA.ID+"/symbols", nil)
	recSymA := httptest.NewRecorder()
	server.Router().ServeHTTP(recSymA, reqSymA)

	var symbolsA []*models.Symbol
	_ = json.Unmarshal(recSymA.Body.Bytes(), &symbolsA)

	for _, sym := range symbolsA {
		if sym.RepositoryID != repoA.ID {
			t.Errorf("SECURITY VIOLATION: Repo A symbols endpoint returned symbol belonging to repo %s", sym.RepositoryID)
		}
	}

	// 2. Verify Graph Isolation for Repo A
	reqGraphA := httptest.NewRequest("GET", "/api/repositories/"+repoA.ID+"/graph", nil)
	recGraphA := httptest.NewRecorder()
	server.Router().ServeHTTP(recGraphA, reqGraphA)

	var graphResA map[string]interface{}
	_ = json.Unmarshal(recGraphA.Body.Bytes(), &graphResA)

	if graphResA["repository_id"].(string) != repoA.ID {
		t.Errorf("expected graph payload repository_id %s, got %s", repoA.ID, graphResA["repository_id"])
	}

	// 3. Verify Grounded Explanation Isolation for Repo A
	mockProv := llm.NewMockLLMProvider("SharedName function in Repo A [E1].", nil)
	lexRetriever := retrieval.NewLexicalRetriever(store)
	composer := retrieval.NewDefaultEvidenceComposer(lexRetriever, nil, store)
	expSvc := llm.NewGroundedExplanationService(mockProv, nil, composer, nil)
	server.SetExplanationService(expSvc)

	bodyExp, _ := json.Marshal(map[string]interface{}{"query": "SharedName"})
	reqExpA := httptest.NewRequest("POST", "/api/repositories/"+repoA.ID+"/explain", bytes.NewReader(bodyExp))
	reqExpA.Header.Set("Content-Type", "application/json")
	recExpA := httptest.NewRecorder()
	server.Router().ServeHTTP(recExpA, reqExpA)

	var expRespA models.ExplanationResponse
	_ = json.Unmarshal(recExpA.Body.Bytes(), &expRespA)

	if expRespA.RepositoryID != repoA.ID {
		t.Errorf("SECURITY VIOLATION: ExplanationResponse repository_id %s != target repo %s", expRespA.RepositoryID, repoA.ID)
	}
	for _, ev := range expRespA.Evidence {
		if ev.RepositoryID != repoA.ID {
			t.Errorf("SECURITY VIOLATION: Evidence item %s belongs to repo %s, expected target repo %s", ev.StableID, ev.RepositoryID, repoA.ID)
		}
	}

	// 4. Verify Callers, Callees, and Flow Scoping for Repo A
	var symIDA string
	if len(symbolsA) > 0 {
		symIDA = symbolsA[0].ID
	}

	reqCallersA := httptest.NewRequest("GET", "/api/repositories/"+repoA.ID+"/graph/callers?symbol_id="+symIDA, nil)
	recCallersA := httptest.NewRecorder()
	server.Router().ServeHTTP(recCallersA, reqCallersA)
	if recCallersA.Code != http.StatusOK {
		t.Fatalf("callers API status = %d; want 200", recCallersA.Code)
	}

	var callersA []*models.CallSite
	_ = json.Unmarshal(recCallersA.Body.Bytes(), &callersA)
	for _, cs := range callersA {
		if cs.CallerNode != nil && cs.CallerNode.RepositoryID != repoA.ID {
			t.Errorf("SECURITY VIOLATION: Caller node belongs to repo %s, expected target repo %s", cs.CallerNode.RepositoryID, repoA.ID)
		}
		if cs.CalleeNode != nil && cs.CalleeNode.RepositoryID != repoA.ID {
			t.Errorf("SECURITY VIOLATION: Callee node belongs to repo %s, expected target repo %s", cs.CalleeNode.RepositoryID, repoA.ID)
		}
	}

	reqCalleesA := httptest.NewRequest("GET", "/api/repositories/"+repoA.ID+"/graph/callees?symbol_id="+symIDA, nil)
	recCalleesA := httptest.NewRecorder()
	server.Router().ServeHTTP(recCalleesA, reqCalleesA)
	if recCalleesA.Code != http.StatusOK {
		t.Fatalf("callees API status = %d; want 200", recCalleesA.Code)
	}

	var calleesA []*models.CallSite
	_ = json.Unmarshal(recCalleesA.Body.Bytes(), &calleesA)
	for _, cs := range calleesA {
		if cs.CallerNode != nil && cs.CallerNode.RepositoryID != repoA.ID {
			t.Errorf("SECURITY VIOLATION: Callee endpoint caller node belongs to repo %s, expected target repo %s", cs.CallerNode.RepositoryID, repoA.ID)
		}
	}

	reqFlowA := httptest.NewRequest("GET", "/api/repositories/"+repoA.ID+"/flow?root="+symIDA, nil)
	recFlowA := httptest.NewRecorder()
	server.Router().ServeHTTP(recFlowA, reqFlowA)
	if recFlowA.Code != http.StatusOK {
		t.Fatalf("flow API status = %d; want 200", recFlowA.Code)
	}

	var flowResA models.StaticFlowResult
	_ = json.Unmarshal(recFlowA.Body.Bytes(), &flowResA)
	if flowResA.RepositoryID != repoA.ID {
		t.Errorf("SECURITY VIOLATION: FlowResult repository_id %s != target repo %s", flowResA.RepositoryID, repoA.ID)
	}

	// 5. Verify Guide Isolation for Repo A
	sumGen := guide.NewDefaultArchitectureSummaryGenerator(store, nil)
	orchestrator := guide.NewDefaultGuideOrchestrator(store, sumGen, composer, expSvc)
	server.SetGuideOrchestrator(orchestrator)

	bodyGuide, _ := json.Marshal(map[string]interface{}{"action": "START"})
	reqGuideA := httptest.NewRequest("POST", "/api/repositories/"+repoA.ID+"/guide", bytes.NewReader(bodyGuide))
	reqGuideA.Header.Set("Content-Type", "application/json")
	recGuideA := httptest.NewRecorder()
	server.Router().ServeHTTP(recGuideA, reqGuideA)

	var invA models.Investigation
	_ = json.Unmarshal(recGuideA.Body.Bytes(), &invA)

	if invA.RepositoryID != repoA.ID {
		t.Errorf("SECURITY VIOLATION: Investigation repository_id %s != target repo %s", invA.RepositoryID, repoA.ID)
	}
}

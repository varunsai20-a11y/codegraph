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

	"codegraph/internal/guide"
	"codegraph/internal/ingestion"
	"codegraph/internal/llm"
	"codegraph/internal/models"
	"codegraph/internal/retrieval"
	"codegraph/internal/storage"

	"github.com/google/uuid"
)

// 1. Symlink Directory Escape
func TestSecurity_SymlinkDirectoryEscape(t *testing.T) {
	tmpDir := t.TempDir()
	workspace := filepath.Join(tmpDir, "repo")
	outsideDir := filepath.Join(tmpDir, "outside_dir")

	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatalf("failed to create outside dir: %v", err)
	}

	// Secret file outside workspace
	outsideFile := filepath.Join(outsideDir, "secret_outside.txt")
	if err := os.WriteFile(outsideFile, []byte("SENSITIVE OUTSIDE CONTENT"), 0644); err != nil {
		t.Fatalf("failed to write outside file: %v", err)
	}

	// Symlink directory inside workspace pointing to outsideDir
	symlinkDir := filepath.Join(workspace, "ext_dir")
	if err := os.Symlink(outsideDir, symlinkDir); err != nil {
		t.Skipf("skipping symlink directory escape test: platform lacks symlink privileges: %v", err)
		return
	}

	filter := ingestion.NewFileFilter([]string{".git"}, 2*1024*1024)
	scanner := ingestion.NewDiscoveryScanner(filter)

	discovered, err := scanner.DiscoverFiles(workspace)
	if err != nil {
		t.Fatalf("DiscoverFiles failed: %v", err)
	}

	for _, df := range discovered {
		if df.RelativePath == "ext_dir/secret_outside.txt" || filepath.Base(df.AbsolutePath) == "secret_outside.txt" {
			t.Errorf("SECURITY VIOLATION: DiscoveryScanner indexed file from outside workspace via directory symlink: %s", df.AbsolutePath)
		}
	}
}

// 2. Circular Symlink Boundedness
func TestSecurity_CircularSymlinkBoundedness(t *testing.T) {
	tmpDir := t.TempDir()
	workspace := filepath.Join(tmpDir, "repo")
	subDir := filepath.Join(workspace, "sub")

	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create sub directory: %v", err)
	}

	// Create circular symlink inside subDir pointing back to subDir
	circularLink := filepath.Join(subDir, "loop")
	if err := os.Symlink(subDir, circularLink); err != nil {
		t.Skipf("skipping circular symlink test: platform lacks symlink privileges: %v", err)
		return
	}

	filter := ingestion.NewFileFilter([]string{".git"}, 2*1024*1024)
	scanner := ingestion.NewDiscoveryScanner(filter)

	doneChan := make(chan error, 1)
	var discovered []ingestion.DiscoveredFile

	go func() {
		var err error
		discovered, err = scanner.DiscoverFiles(workspace)
		doneChan <- err
	}()

	select {
	case err := <-doneChan:
		if err != nil {
			t.Fatalf("DiscoverFiles failed on circular symlink: %v", err)
		}
		t.Logf("Discovery scanner safely bounded circular symlinks; found %d total files", len(discovered))
	case <-time.After(2 * time.Second):
		t.Fatalf("SECURITY VIOLATION: DiscoveryScanner hung or entered infinite loop on circular symlink")
	}
}

// 3. Unknown Repository ID Endpoint Contracts
func TestSecurity_UnknownRepoID_EndpointContracts(t *testing.T) {
	srv, store, _ := setupTestServer(t)
	_ = store

	unknownID := "unknown-repo-uuid-9999"

	// Entity check endpoint -> 404 Not Found
	reqRepo := httptest.NewRequest("GET", "/api/repositories/"+unknownID, nil)
	recRepo := httptest.NewRecorder()
	srv.Router().ServeHTTP(recRepo, reqRepo)
	if recRepo.Code != http.StatusNotFound {
		t.Errorf("GET /repositories/{unknown} status = %d; want 404", recRepo.Code)
	}

	// Store collection endpoint -> 200 OK with empty slice []
	reqSym := httptest.NewRequest("GET", "/api/repositories/"+unknownID+"/symbols", nil)
	recSym := httptest.NewRecorder()
	srv.Router().ServeHTTP(recSym, reqSym)
	if recSym.Code != http.StatusOK {
		t.Errorf("GET /repositories/{unknown}/symbols status = %d; want 200", recSym.Code)
	}
	var symbols []*models.Symbol
	_ = json.Unmarshal(recSym.Body.Bytes(), &symbols)
	if len(symbols) != 0 {
		t.Errorf("expected 0 symbols for unknown repo, got %d", len(symbols))
	}

	// Sub-graph navigational endpoint -> 404 Not Found
	reqCallers := httptest.NewRequest("GET", "/api/repositories/"+unknownID+"/graph/callers?symbol_id=sym1", nil)
	recCallers := httptest.NewRecorder()
	srv.Router().ServeHTTP(recCallers, reqCallers)
	if recCallers.Code != http.StatusNotFound {
		t.Errorf("GET /graph/callers status = %d; want 404", recCallers.Code)
	}
}

// 4. Cross-Repository Real Symbol IDs
func TestSecurity_CrossRepoRealSymbolIDs(t *testing.T) {
	srv, store, tmpDir := setupTestServer(t)

	// Setup Repo A
	dirA := filepath.Join(tmpDir, "repo-A")
	_ = os.MkdirAll(dirA, 0755)
	_ = os.WriteFile(filepath.Join(dirA, "main.go"), []byte("package main\nfunc FunctionA() {}\n"), 0644)
	repoA := &models.Repository{ID: "repo-A-id", Name: "Repo A", LocalPath: dirA, SourceType: models.SourceTypeLocal, Status: models.RepoStatusIndexed}
	_ = store.CreateRepository(context.Background(), repoA)

	symA := &models.Symbol{ID: "sym-A-1", RepositoryID: "repo-A-id", FileID: "fA", RelativePath: "main.go", Name: "FunctionA", Kind: models.SymbolKindFunction}
	_ = store.SaveSymbols(context.Background(), repoA.ID, []*models.Symbol{symA})
	nodeA := &models.Node{ID: symA.ID, RepositoryID: repoA.ID, Kind: models.NodeKindSymbol, Label: "FunctionA", QualifiedName: "main.FunctionA"}
	_ = store.SaveGraph(context.Background(), repoA.ID, []*models.Node{nodeA}, nil)

	// Setup Repo B
	dirB := filepath.Join(tmpDir, "repo-B")
	_ = os.MkdirAll(dirB, 0755)
	_ = os.WriteFile(filepath.Join(dirB, "main.go"), []byte("package main\nfunc FunctionB() {}\n"), 0644)
	repoB := &models.Repository{ID: "repo-B-id", Name: "Repo B", LocalPath: dirB, SourceType: models.SourceTypeLocal, Status: models.RepoStatusIndexed}
	_ = store.CreateRepository(context.Background(), repoB)

	symB := &models.Symbol{ID: "sym-B-1", RepositoryID: "repo-B-id", FileID: "fB", RelativePath: "main.go", Name: "FunctionB", Kind: models.SymbolKindFunction}
	_ = store.SaveSymbols(context.Background(), repoB.ID, []*models.Symbol{symB})
	nodeB := &models.Node{ID: symB.ID, RepositoryID: repoB.ID, Kind: models.NodeKindSymbol, Label: "FunctionB", QualifiedName: "main.FunctionB"}
	_ = store.SaveGraph(context.Background(), repoB.ID, []*models.Node{nodeB}, nil)

	// 1. GET /graph/callers for Repo A with Repo B symbol ID
	reqCallers := httptest.NewRequest("GET", "/api/repositories/"+repoA.ID+"/graph/callers?symbol_id="+symB.ID, nil)
	recCallers := httptest.NewRecorder()
	srv.Router().ServeHTTP(recCallers, reqCallers)

	if recCallers.Code != http.StatusOK {
		t.Errorf("callers status = %d; want 200", recCallers.Code)
	}
	var callers []*models.CallSite
	_ = json.Unmarshal(recCallers.Body.Bytes(), &callers)
	if len(callers) != 0 {
		t.Errorf("SECURITY VIOLATION: Repo A callers endpoint returned %d callers for Repo B symbol ID", len(callers))
	}

	// 2. GET /flow for Repo A with Repo B symbol ID
	reqFlow := httptest.NewRequest("GET", "/api/repositories/"+repoA.ID+"/flow?root="+symB.ID, nil)
	recFlow := httptest.NewRecorder()
	srv.Router().ServeHTTP(recFlow, reqFlow)

	if recFlow.Code != http.StatusOK {
		t.Errorf("flow status = %d; want 200", recFlow.Code)
	}
	var flowRes models.StaticFlowResult
	_ = json.Unmarshal(recFlow.Body.Bytes(), &flowRes)
	if flowRes.TerminationReason != models.FlowReasonInvalidRoot {
		t.Errorf("expected TerminationReason == INVALID_ROOT for foreign symbol ID, got: %s", flowRes.TerminationReason)
	}

	// 3. POST /explain for Repo A with Repo B symbol ID
	composer := retrieval.NewDefaultEvidenceComposer(nil, nil, store)
	expSvc := llm.NewGroundedExplanationService(nil, nil, composer, nil)
	srv.SetExplanationService(expSvc)

	bodyExp, _ := json.Marshal(map[string]interface{}{
		"query":     "Explain function B",
		"symbol_id": symB.ID,
	})
	reqExp := httptest.NewRequest("POST", "/api/repositories/"+repoA.ID+"/explain", bytes.NewReader(bodyExp))
	reqExp.Header.Set("Content-Type", "application/json")
	recExp := httptest.NewRecorder()
	srv.Router().ServeHTTP(recExp, reqExp)

	if recExp.Code != http.StatusOK {
		t.Errorf("explain status = %d; want 200", recExp.Code)
	}
	var expRes models.ExplanationResponse
	_ = json.Unmarshal(recExp.Body.Bytes(), &expRes)
	if !expRes.IsInsufficientEvidence {
		t.Errorf("expected IsInsufficientEvidence == true when foreign symbol ID is queried, got false")
	}
}

// 5. Guide Invalid Step Index Normalization
func TestSecurity_GuideInvalidStepIndex(t *testing.T) {
	srv, store, tmpDir := setupTestServer(t)

	repoDir := filepath.Join(tmpDir, "repo-guide")
	_ = os.MkdirAll(repoDir, 0755)
	repoID := uuid.New().String()
	_ = store.CreateRepository(context.Background(), &models.Repository{
		ID: repoID, Name: "repo-guide", LocalPath: repoDir, SourceType: models.SourceTypeLocal, Status: models.RepoStatusIndexed,
	})

	sumGen := guide.NewDefaultArchitectureSummaryGenerator(store, nil)
	composer := retrieval.NewDefaultEvidenceComposer(nil, nil, store)
	expSvc := llm.NewGroundedExplanationService(nil, nil, composer, nil)
	orchestrator := guide.NewDefaultGuideOrchestrator(store, sumGen, composer, expSvc)
	srv.SetGuideOrchestrator(orchestrator)

	// Test negative index
	bodyNeg, _ := json.Marshal(map[string]interface{}{
		"action":             "NEXT",
		"current_step_index": -1,
	})
	reqNeg := httptest.NewRequest("POST", "/api/repositories/"+repoID+"/guide", bytes.NewReader(bodyNeg))
	reqNeg.Header.Set("Content-Type", "application/json")
	recNeg := httptest.NewRecorder()
	srv.Router().ServeHTTP(recNeg, reqNeg)

	if recNeg.Code != http.StatusOK {
		t.Fatalf("Guide neg status = %d; want 200", recNeg.Code)
	}
	var invNeg models.Investigation
	_ = json.Unmarshal(recNeg.Body.Bytes(), &invNeg)
	if invNeg.CurrentStepIndex < 0 || invNeg.CurrentStepIndex >= len(invNeg.Steps) {
		t.Errorf("out-of-bounds CurrentStepIndex returned for negative input: %d", invNeg.CurrentStepIndex)
	}

	// Test oversized index
	bodyOver, _ := json.Marshal(map[string]interface{}{
		"action":             "NEXT",
		"current_step_index": 9999,
	})
	reqOver := httptest.NewRequest("POST", "/api/repositories/"+repoID+"/guide", bytes.NewReader(bodyOver))
	reqOver.Header.Set("Content-Type", "application/json")
	recOver := httptest.NewRecorder()
	srv.Router().ServeHTTP(recOver, reqOver)

	if recOver.Code != http.StatusOK {
		t.Fatalf("Guide over status = %d; want 200", recOver.Code)
	}
	var invOver models.Investigation
	_ = json.Unmarshal(recOver.Body.Bytes(), &invOver)
	if invOver.CurrentStepIndex < 0 || invOver.CurrentStepIndex >= len(invOver.Steps) {
		t.Errorf("out-of-bounds CurrentStepIndex returned for oversized input: %d", invOver.CurrentStepIndex)
	}
}

// 6. Flow Invalid max_depth Fallback
func TestSecurity_FlowInvalidMaxDepth(t *testing.T) {
	srv, store, tmpDir := setupTestServer(t)

	repoDir := filepath.Join(tmpDir, "repo-flow")
	_ = os.MkdirAll(repoDir, 0755)
	repoID := uuid.New().String()
	_ = store.CreateRepository(context.Background(), &models.Repository{
		ID: repoID, Name: "repo-flow", LocalPath: repoDir, SourceType: models.SourceTypeLocal, Status: models.RepoStatusIndexed,
	})

	// Test negative max_depth
	reqNeg := httptest.NewRequest("GET", "/api/repositories/"+repoID+"/flow?root=sym1&max_depth=-10", nil)
	recNeg := httptest.NewRecorder()
	srv.Router().ServeHTTP(recNeg, reqNeg)

	if recNeg.Code != http.StatusOK {
		t.Fatalf("flow status = %d; want 200", recNeg.Code)
	}
	var flowNeg models.StaticFlowResult
	_ = json.Unmarshal(recNeg.Body.Bytes(), &flowNeg)
	if flowNeg.MaxDepth != 10 {
		t.Errorf("expected default MaxDepth == 10 for negative input, got: %d", flowNeg.MaxDepth)
	}

	// Test non-numeric max_depth
	reqAlpha := httptest.NewRequest("GET", "/api/repositories/"+repoID+"/flow?root=sym1&max_depth=invalid_abc", nil)
	recAlpha := httptest.NewRecorder()
	srv.Router().ServeHTTP(recAlpha, reqAlpha)

	if recAlpha.Code != http.StatusOK {
		t.Fatalf("flow status = %d; want 200", recAlpha.Code)
	}
	var flowAlpha models.StaticFlowResult
	_ = json.Unmarshal(recAlpha.Body.Bytes(), &flowAlpha)
	if flowAlpha.MaxDepth != 10 {
		t.Errorf("expected default MaxDepth == 10 for non-numeric input, got: %d", flowAlpha.MaxDepth)
	}
}

// 7. Citation Grounding Validation
func TestSecurity_CitationGroundingValidation(t *testing.T) {
	validator := llm.NewCitationValidator()

	pkg := &models.EvidencePackage{
		Items: []*models.EvidenceItem{
			{Label: "E1", StableID: "sym-1", RelativePath: "main.go"},
			{Label: "E2", StableID: "sym-2", RelativePath: "app.go"},
		},
	}

	// Content contains valid [E1], ungrounded [E999], and malformed [E-1]
	content := "Function initializes entry point [E1], grants admin privilege [E999], and logs status [E-1]."
	report := validator.Validate(content, pkg)

	if report.CitationValidationStatus != "INVALID_CITATIONS_FOUND" {
		t.Errorf("expected CitationValidationStatus == INVALID_CITATIONS_FOUND, got: %s", report.CitationValidationStatus)
	}

	if len(report.InvalidCitations) != 1 || report.InvalidCitations[0] != "[E999]" {
		t.Errorf("expected InvalidCitations == [[E999]], got: %v", report.InvalidCitations)
	}

	if len(report.MalformedCitations) != 1 || report.MalformedCitations[0] != "[E-1]" {
		t.Errorf("expected MalformedCitations == [[E-1]], got: %v", report.MalformedCitations)
	}

	if report.ValidCount != 1 || report.InvalidCount != 2 {
		t.Errorf("expected validCount=1, invalidCount=2; got valid=%d, invalid=%d", report.ValidCount, report.InvalidCount)
	}
}

// 8. Evidence Repository Scope Enforcement
func TestSecurity_EvidenceRepoScopeEnforcement(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "scope.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}
	defer store.Close()

	composer := retrieval.NewDefaultEvidenceComposer(nil, nil, store)

	scopeA, err := models.NewRepositoryScope("repo-A-id")
	if err != nil {
		t.Fatalf("failed to create scope: %v", err)
	}

	itemA := &models.EvidenceItem{StableID: "sym-1", RepositoryID: "repo-A-id", Label: "E1", Content: "Repo A content"}
	itemB := &models.EvidenceItem{StableID: "sym-2", RepositoryID: "repo-B-id", Label: "E2", Content: "Repo B content"}

	// Test direct scope validation on item
	if err := scopeA.ValidateItem(itemA); err != nil {
		t.Errorf("expected itemA to be valid under scopeA: %v", err)
	}
	if err := scopeA.ValidateItem(itemB); err == nil {
		t.Errorf("SECURITY VIOLATION: itemB with foreign repo ID passed scopeA validation")
	}

	// Verify composer filtering behavior
	req, _ := models.NewExplanationRequest(scopeA, "How does initialization work?")
	req.StaticFlow = &models.StaticFlowResult{
		RepositoryID: "repo-A-id",
		Path: &models.FlowPath{
			PathID: "flow-1",
			Steps: []*models.FlowStep{
				{Sequence: 0, NodeID: "n1", Node: &models.Node{ID: "n1", RepositoryID: "repo-B-id", Label: "ForeignNode"}}, // Foreign repo ID step
			},
		},
	}

	pkg, err := composer.Compose(context.Background(), req)
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

	for _, item := range pkg.Items {
		if item.RepositoryID != "repo-A-id" {
			t.Errorf("SECURITY VIOLATION: Composed EvidencePackage contained item belonging to repo %s under repo-A scope", item.RepositoryID)
		}
	}
}

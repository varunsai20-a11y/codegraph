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
	"codegraph/internal/ingestion"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/storage"
)

func TestFullPipelineIntegration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "e2e_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	wsRoot := filepath.Join(tmpDir, "workspaces")

	cfg := &config.Config{
		Port:               8080,
		MaxFileSize:        2 * 1024 * 1024,
		WorkspaceRoot:      wsRoot,
		DatabasePath:       dbPath,
		DefaultExclusions:  []string{".git", "node_modules", "dist"},
		SupportedLanguages: []string{"TypeScript", "JavaScript", "Python", "Go", "Java"},
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
	server := api.NewServer(cfg, store, wsMgr, indexer)

	fixturePath, err := filepath.Abs(filepath.Join("fixtures", "sample-repository"))
	if err != nil {
		t.Fatalf("failed to resolve fixture path: %v", err)
	}

	// 1. API: Register Repository
	regBody, _ := json.Marshal(api.RegisterRepoRequest{
		Name:       "Sample Fixture",
		SourceType: models.SourceTypeLocal,
		LocalPath:  fixturePath,
	})

	req := httptest.NewRequest("POST", "/api/repositories", bytes.NewReader(regBody))
	rec := httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("register API status = %d; want 201. Body: %s", rec.Code, rec.Body.String())
	}

	var repo models.Repository
	if err := json.Unmarshal(rec.Body.Bytes(), &repo); err != nil {
		t.Fatal(err)
	}

	// 2. Direct Pipeline Ingestion Run
	ctx := context.Background()
	job := &models.IndexJob{
		ID:           "job-e2e-1",
		RepositoryID: repo.ID,
		Status:       models.JobStatusPending,
		StartedAt:    time.Now(),
	}
	if err := store.CreateIndexJob(ctx, job); err != nil {
		t.Fatalf("failed to create job: %v", err)
	}

	if err := indexer.RunIndex(ctx, job, &repo); err != nil {
		t.Fatalf("indexing failed: %v", err)
	}

	// 3. Verify Symbols API
	symReq := httptest.NewRequest("GET", "/api/repositories/"+repo.ID+"/symbols", nil)
	symRec := httptest.NewRecorder()
	server.Router().ServeHTTP(symRec, symReq)

	if symRec.Code != http.StatusOK {
		t.Fatalf("symbols API status = %d; want 200", symRec.Code)
	}

	var symbols []*models.Symbol
	if err := json.Unmarshal(symRec.Body.Bytes(), &symbols); err != nil {
		t.Fatal(err)
	}

	if len(symbols) == 0 {
		t.Fatalf("expected extracted symbols > 0")
	}

	// 4. Verify Relationships API
	relReq := httptest.NewRequest("GET", "/api/repositories/"+repo.ID+"/relationships", nil)
	relRec := httptest.NewRecorder()
	server.Router().ServeHTTP(relRec, relReq)

	if relRec.Code != http.StatusOK {
		t.Fatalf("relationships API status = %d; want 200", relRec.Code)
	}

	var rels []*models.Relationship
	if err := json.Unmarshal(relRec.Body.Bytes(), &rels); err != nil {
		t.Fatal(err)
	}

	if len(rels) == 0 {
		t.Fatalf("expected extracted relationships > 0")
	}

	// 5. Phase 3 API: Verify Graph Payload API
	graphReq := httptest.NewRequest("GET", "/api/repositories/"+repo.ID+"/graph", nil)
	graphRec := httptest.NewRecorder()
	server.Router().ServeHTTP(graphRec, graphReq)

	if graphRec.Code != http.StatusOK {
		t.Fatalf("graph API status = %d; want 200", graphRec.Code)
	}

	var graphRes map[string]interface{}
	if err := json.Unmarshal(graphRec.Body.Bytes(), &graphRes); err != nil {
		t.Fatal(err)
	}

	if graphRes["node_count"].(float64) == 0 || graphRes["edge_count"].(float64) == 0 {
		t.Fatalf("expected graph nodes and edges > 0, got %+v", graphRes)
	}
}

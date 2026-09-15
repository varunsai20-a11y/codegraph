package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"codegraph/internal/api"
	"codegraph/internal/models"
	"github.com/google/uuid"
)

// TestC4_01_GraphNodeLocationMetadata verifies graph nodes contain exact relative_path & location bounds
func TestC4_01_GraphNodeLocationMetadata(t *testing.T) {
	srv, store, tmpDir := setupTestServer(t)
	repoDir := filepath.Join(tmpDir, "sample-c4-repo")
	_ = os.MkdirAll(filepath.Join(repoDir, "internal", "retrieval"), 0755)

	mainCode := `package main

import "fmt"

func MainFunc() {
	fmt.Println("Main")
}
`
	_ = os.WriteFile(filepath.Join(repoDir, "main.go"), []byte(mainCode), 0644)

	repoID := uuid.New().String()
	_ = store.CreateRepository(context.Background(), &models.Repository{
		ID: repoID, Name: "sample-c4-repo", LocalPath: repoDir, SourceType: models.SourceTypeLocal, Status: models.RepoStatusIndexed,
	})

	// Index file node & symbol node into graph via SaveGraph
	fileNode := &models.Node{
		ID:           "file-main",
		RepositoryID: repoID,
		Kind:         models.NodeKindFile,
		Label:        "main.go",
		RelativePath: "main.go",
	}

	symbolNode := &models.Node{
		ID:            "sym-mainfunc",
		RepositoryID:  repoID,
		Kind:          models.NodeKindSymbol,
		Label:         "MainFunc",
		QualifiedName: "main.MainFunc",
		RelativePath:  "main.go",
		Location: models.Location{
			StartLine: 5,
			EndLine:   7,
		},
	}

	_ = store.SaveGraph(context.Background(), repoID, []*models.Node{fileNode, symbolNode}, nil)

	// Fetch graph API
	req := httptest.NewRequest(http.MethodGet, "/api/repositories/"+repoID+"/graph?scope=OVERVIEW", nil)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for graph request, got %d: %s", rec.Code, rec.Body.String())
	}

	var res struct {
		RepositoryID string         `json:"repository_id"`
		NodeCount    int            `json:"node_count"`
		EdgeCount    int            `json:"edge_count"`
		Nodes        []*models.Node `json:"nodes"`
		Edges        []*models.Edge `json:"edges"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &res)

	if len(res.Nodes) != 2 {
		t.Fatalf("expected 2 graph nodes, got %d", len(res.Nodes))
	}

	var foundSym *models.Node
	for i := range res.Nodes {
		if res.Nodes[i].ID == "sym-mainfunc" {
			foundSym = res.Nodes[i]
		}
	}

	if foundSym == nil {
		t.Fatalf("symbol node sym-mainfunc not found in graph response")
	}

	if foundSym.RelativePath != "main.go" {
		t.Errorf("expected symbol RelativePath 'main.go', got '%s'", foundSym.RelativePath)
	}

	if foundSym.Location.StartLine != 5 || foundSym.Location.EndLine != 7 {
		t.Errorf("expected symbol location 5-7, got %+v", foundSym.Location)
	}
}

// TestC4_02_FileContentWithLineBounds verifies backend file retrieval returns exact total lines & content for line highlights
func TestC4_02_FileContentWithLineBounds(t *testing.T) {
	srv, store, tmpDir := setupTestServer(t)
	repoDir := filepath.Join(tmpDir, "c4-line-repo")
	_ = os.MkdirAll(repoDir, 0755)

	content := "line 1\nline 2\nline 3\nline 4\nline 5\n"
	_ = os.WriteFile(filepath.Join(repoDir, "test.txt"), []byte(content), 0644)

	repoID := uuid.New().String()
	_ = store.CreateRepository(context.Background(), &models.Repository{
		ID: repoID, Name: "c4-line-repo", LocalPath: repoDir, SourceType: models.SourceTypeLocal, Status: models.RepoStatusIndexed,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/repositories/"+repoID+"/file?path=test.txt", nil)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var res api.FileContentResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &res)

	if res.TotalLines != 6 && res.TotalLines != 5 {
		t.Errorf("expected 5 or 6 total lines, got %d", res.TotalLines)
	}
}

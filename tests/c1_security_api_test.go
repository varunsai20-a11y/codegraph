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
	"codegraph/internal/config"
	"codegraph/internal/ingestion"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/storage"

	"github.com/google/uuid"
)

func setupTestServer(t *testing.T) (*api.Server, storage.Storage, string) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	wsRoot := filepath.Join(tmpDir, "workspaces")

	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create sqlite storage: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	wsMgr, err := repository.NewWorkspaceManager(wsRoot)
	if err != nil {
		t.Fatalf("failed to create workspace manager: %v", err)
	}

	cfg := &config.Config{
		Port:           8080,
		WorkspaceRoot:  wsRoot,
		DatabasePath:   dbPath,
		AllowedOrigins: []string{"http://localhost:3000", "http://127.0.0.1:3000"},
	}

	indexer := ingestion.NewIndexer(cfg, store, wsMgr)
	srv := api.NewServer(cfg, store, wsMgr, indexer)

	return srv, store, tmpDir
}

// 1. Valid Repository File (200 OK with content, total_lines, language)
func TestC1_01_ValidRepositoryFile(t *testing.T) {
	srv, store, tmpDir := setupTestServer(t)
	repoDir := filepath.Join(tmpDir, "sample-repo")
	_ = os.MkdirAll(repoDir, 0755)
	_ = os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n\nfunc main() {\n\tprintln(\"Hello\")\n}\n"), 0644)

	repoID := uuid.New().String()
	_ = store.CreateRepository(context.Background(), &models.Repository{
		ID: repoID, Name: "sample-repo", LocalPath: repoDir, SourceType: models.SourceTypeLocal, Status: models.RepoStatusIndexed,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/repositories/"+repoID+"/file?path=main.go", nil)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var res api.FileContentResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res.RepositoryID != repoID || res.RelativePath != "main.go" || res.Language != "GO" || res.TotalLines != 5 {
		t.Errorf("unexpected response: %+v", res)
	}
}

// 2. Nonexistent File (404 Not Found)
func TestC1_02_NonexistentFile(t *testing.T) {
	srv, store, tmpDir := setupTestServer(t)
	repoDir := filepath.Join(tmpDir, "sample-repo")
	_ = os.MkdirAll(repoDir, 0755)

	repoID := uuid.New().String()
	_ = store.CreateRepository(context.Background(), &models.Repository{
		ID: repoID, Name: "sample-repo", LocalPath: repoDir, SourceType: models.SourceTypeLocal, Status: models.RepoStatusIndexed,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/repositories/"+repoID+"/file?path=missing.go", nil)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found for missing file, got %d", rec.Code)
	}
}

// 3. Missing Repository (404 Not Found)
func TestC1_03_MissingRepository(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/repositories/non-existent-id/file?path=main.go", nil)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found for non-existent repo ID, got %d", rec.Code)
	}
}

// 4. Dot-Dot Traversal (400 Bad Request)
func TestC1_04_PathTraversalDotDot(t *testing.T) {
	srv, store, tmpDir := setupTestServer(t)
	repoDir := filepath.Join(tmpDir, "sample-repo")
	_ = os.MkdirAll(repoDir, 0755)

	repoID := uuid.New().String()
	_ = store.CreateRepository(context.Background(), &models.Repository{
		ID: repoID, Name: "sample-repo", LocalPath: repoDir, SourceType: models.SourceTypeLocal, Status: models.RepoStatusIndexed,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/repositories/"+repoID+"/file?path=../etc/passwd", nil)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for path traversal, got %d", rec.Code)
	}
}

// 5. Nested Path Traversal (400 Bad Request)
func TestC1_05_NestedPathTraversal(t *testing.T) {
	srv, store, tmpDir := setupTestServer(t)
	repoDir := filepath.Join(tmpDir, "sample-repo")
	_ = os.MkdirAll(repoDir, 0755)

	repoID := uuid.New().String()
	_ = store.CreateRepository(context.Background(), &models.Repository{
		ID: repoID, Name: "sample-repo", LocalPath: repoDir, SourceType: models.SourceTypeLocal, Status: models.RepoStatusIndexed,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/repositories/"+repoID+"/file?path=sub/../../secret.txt", nil)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for nested path traversal, got %d", rec.Code)
	}
}

// 6. Unix Absolute Path (400 Bad Request)
func TestC1_06_UnixAbsolutePath(t *testing.T) {
	srv, store, tmpDir := setupTestServer(t)
	repoDir := filepath.Join(tmpDir, "sample-repo")
	_ = os.MkdirAll(repoDir, 0755)

	repoID := uuid.New().String()
	_ = store.CreateRepository(context.Background(), &models.Repository{
		ID: repoID, Name: "sample-repo", LocalPath: repoDir, SourceType: models.SourceTypeLocal, Status: models.RepoStatusIndexed,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/repositories/"+repoID+"/file?path=/etc/passwd", nil)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for Unix absolute path, got %d", rec.Code)
	}
}

// 7. Windows Absolute Path (400 Bad Request)
func TestC1_07_WindowsAbsolutePath(t *testing.T) {
	srv, store, tmpDir := setupTestServer(t)
	repoDir := filepath.Join(tmpDir, "sample-repo")
	_ = os.MkdirAll(repoDir, 0755)

	repoID := uuid.New().String()
	_ = store.CreateRepository(context.Background(), &models.Repository{
		ID: repoID, Name: "sample-repo", LocalPath: repoDir, SourceType: models.SourceTypeLocal, Status: models.RepoStatusIndexed,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/repositories/"+repoID+"/file?path=C:\\Windows\\System32\\config", nil)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for Windows absolute path, got %d", rec.Code)
	}
}

// 8. UNC Path Traversal (400 Bad Request)
func TestC1_08_UNCPathTraversal(t *testing.T) {
	srv, store, tmpDir := setupTestServer(t)
	repoDir := filepath.Join(tmpDir, "sample-repo")
	_ = os.MkdirAll(repoDir, 0755)

	repoID := uuid.New().String()
	_ = store.CreateRepository(context.Background(), &models.Repository{
		ID: repoID, Name: "sample-repo", LocalPath: repoDir, SourceType: models.SourceTypeLocal, Status: models.RepoStatusIndexed,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/repositories/"+repoID+"/file?path=\\\\server\\share\\secret.txt", nil)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for UNC path, got %d", rec.Code)
	}
}

// 9. Sensitive / Secret File Access (403 Forbidden)
func TestC1_09_SensitiveSecretFiles(t *testing.T) {
	srv, store, tmpDir := setupTestServer(t)
	repoDir := filepath.Join(tmpDir, "sample-repo")
	_ = os.MkdirAll(repoDir, 0755)
	_ = os.WriteFile(filepath.Join(repoDir, ".env"), []byte("SECRET=123"), 0644)
	_ = os.WriteFile(filepath.Join(repoDir, "id_rsa"), []byte("PRIVATE KEY"), 0644)
	_ = os.WriteFile(filepath.Join(repoDir, "cert.pem"), []byte("CERTIFICATE"), 0644)

	repoID := uuid.New().String()
	_ = store.CreateRepository(context.Background(), &models.Repository{
		ID: repoID, Name: "sample-repo", LocalPath: repoDir, SourceType: models.SourceTypeLocal, Status: models.RepoStatusIndexed,
	})

	secrets := []string{".env", "id_rsa", "cert.pem"}
	for _, sec := range secrets {
		req := httptest.NewRequest(http.MethodGet, "/api/repositories/"+repoID+"/file?path="+sec, nil)
		rec := httptest.NewRecorder()
		srv.Router().ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for secret file %s, got %d", sec, rec.Code)
		}
	}
}

// 10. Symlink Escape (400 Bad Request)
func TestC1_10_SymlinkEscape(t *testing.T) {
	srv, store, tmpDir := setupTestServer(t)
	repoDir := filepath.Join(tmpDir, "sample-repo")
	_ = os.MkdirAll(repoDir, 0755)

	outsideFile := filepath.Join(tmpDir, "outside_secret.txt")
	_ = os.WriteFile(outsideFile, []byte("outside content"), 0644)

	symlinkPath := filepath.Join(repoDir, "sym_outside")
	if err := os.Symlink(outsideFile, symlinkPath); err != nil {
		t.Skipf("skipping symlink escape integration test: platform lacks symlink privileges: %v", err)
		return
	}

	repoID := uuid.New().String()
	_ = store.CreateRepository(context.Background(), &models.Repository{
		ID: repoID, Name: "sample-repo", LocalPath: repoDir, SourceType: models.SourceTypeLocal, Status: models.RepoStatusIndexed,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/repositories/"+repoID+"/file?path=sym_outside", nil)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusForbidden {
		t.Errorf("expected 400 or 403 for symlink escape attempt, got %d", rec.Code)
	}
}

// 11. Repository Isolation (404 Not Found when cross-requesting)
func TestC1_11_RepositoryIsolation(t *testing.T) {
	srv, store, tmpDir := setupTestServer(t)

	dirA := filepath.Join(tmpDir, "repo-A")
	_ = os.MkdirAll(dirA, 0755)
	_ = os.WriteFile(filepath.Join(dirA, "fileA.txt"), []byte("Content Repo A"), 0644)
	_ = store.CreateRepository(context.Background(), &models.Repository{ID: "repo-A-id", Name: "repo-A", LocalPath: dirA, SourceType: models.SourceTypeLocal, Status: models.RepoStatusIndexed})

	dirB := filepath.Join(tmpDir, "repo-B")
	_ = os.MkdirAll(dirB, 0755)
	_ = os.WriteFile(filepath.Join(dirB, "fileB.txt"), []byte("Content Repo B"), 0644)
	_ = store.CreateRepository(context.Background(), &models.Repository{ID: "repo-B-id", Name: "repo-B", LocalPath: dirB, SourceType: models.SourceTypeLocal, Status: models.RepoStatusIndexed})

	req := httptest.NewRequest(http.MethodGet, "/api/repositories/repo-A-id/file?path=fileB.txt", nil)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 when querying Repo B file from Repo A scope, got %d", rec.Code)
	}
}

// 12. Valid CORS Origin (Headers set properly)
func TestC1_12_ValidCORSOrigin(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/repositories", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Errorf("expected Access-Control-Allow-Origin = http://localhost:3000, got %s", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

// 13. Invalid CORS Origin (403 Forbidden on preflight, no CORS header)
func TestC1_13_InvalidCORSOrigin(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	req := httptest.NewRequest(http.MethodOptions, "/api/repositories", nil)
	req.Header.Set("Origin", "http://evil.example")
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for unauthorized CORS origin OPTIONS, got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("expected empty Access-Control-Allow-Origin for unauthorized origin, got %s", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

// 14. OPTIONS Preflight Handling
func TestC1_14_OPTIONSPreflight(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	req := httptest.NewRequest(http.MethodOptions, "/api/repositories", nil)
	req.Header.Set("Origin", "http://127.0.0.1:3000")
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent && rec.Code != http.StatusOK {
		t.Errorf("expected 204 or 200 for valid OPTIONS preflight, got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Errorf("expected Access-Control-Allow-Methods header to be set")
	}
}

// 15. Wildcard CORS is NEVER Emitted
func TestC1_15_WildcardCORSIsNeverEmitted(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	origins := []string{"http://localhost:3000", "http://127.0.0.1:3000", "http://unknown.com", ""}

	for _, orig := range origins {
		req := httptest.NewRequest(http.MethodGet, "/api/repositories", nil)
		if orig != "" {
			req.Header.Set("Origin", orig)
		}
		rec := httptest.NewRecorder()
		srv.Router().ServeHTTP(rec, req)

		if rec.Header().Get("Access-Control-Allow-Origin") == "*" {
			t.Errorf("SECURITY VIOLATION: Access-Control-Allow-Origin emitted '*' wildcard for origin '%s'", orig)
		}
	}
}

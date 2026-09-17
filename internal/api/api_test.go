package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"codegraph/internal/config"
	"codegraph/internal/ingestion"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/storage"
)

func setupTestServer(t *testing.T) (*Server, func()) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "api_test.db")
	wsPath := filepath.Join(tmpDir, "workspaces")

	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	wsMgr, err := repository.NewWorkspaceManager(wsPath)
	if err != nil {
		store.Close()
		t.Fatalf("failed to init workspace manager: %v", err)
	}

	cfg := config.Load()
	idx := ingestion.NewIndexer(cfg, store, wsMgr)
	srv := NewServer(cfg, store, wsMgr, idx)

	cleanup := func() {
		store.Close()
	}

	return srv, cleanup
}

func TestConcurrentRegistration_SemanticVariants(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	urls := []string{
		"https://github.com/gin-gonic/gin",
		"https://github.com/gin-gonic/gin/",
		"https://github.com/gin-gonic/gin.git",
		"https://GITHUB.COM/Gin-Gonic/Gin.git/",
		"https://github.com/GIN-GONIC/GIN",
	}

	numGoroutines := 10
	var wg sync.WaitGroup
	repoIDs := make([]string, numGoroutines)
	errors := make([]error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			urlToUse := urls[idx%len(urls)]
			payload := map[string]string{
				"source_type": "GIT",
				"source_url":  urlToUse,
			}
			bodyBytes, _ := json.Marshal(payload)

			req := httptest.NewRequest("POST", "/api/repositories", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			srv.router.ServeHTTP(w, req)

			if w.Code != http.StatusOK && w.Code != http.StatusCreated {
				errors[idx] = fmt.Errorf("unexpected status %d: %s", w.Code, w.Body.String())
				return
			}

			var repo models.Repository
			if err := json.Unmarshal(w.Body.Bytes(), &repo); err != nil {
				errors[idx] = err
				return
			}
			repoIDs[idx] = repo.ID
		}(i)
	}

	wg.Wait()

	for i, err := range errors {
		if err != nil {
			t.Fatalf("goroutine %d failed: %v", i, err)
		}
	}

	firstID := repoIDs[0]
	if firstID == "" {
		t.Fatalf("expected non-empty repository ID")
	}

	for i, id := range repoIDs {
		if id != firstID {
			t.Errorf("goroutine %d got repository ID %s, expected canonical %s", i, id, firstID)
		}
	}

	// Verify database contains exactly 1 repository row
	repos, err := srv.store.ListRepositories(context.Background())
	if err != nil {
		t.Fatalf("ListRepositories failed: %v", err)
	}
	if len(repos) != 1 {
		t.Errorf("expected exactly 1 repository in database, got %d", len(repos))
	}
}

func TestConcurrentIndexJobCreation(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Register test repository
	payload := map[string]string{
		"source_type": "GIT",
		"source_url":  "https://github.com/gin-gonic/gin",
	}
	bodyBytes, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/repositories", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	var repo models.Repository
	_ = json.Unmarshal(w.Body.Bytes(), &repo)

	// Trigger concurrent job starts for the same repo
	numGoroutines := 5
	var wg sync.WaitGroup
	jobIDs := make([]string, numGoroutines)
	errors := make([]error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			jobReq := httptest.NewRequest("POST", fmt.Sprintf("/api/repositories/%s/index", repo.ID), nil)
			jobW := httptest.NewRecorder()

			srv.router.ServeHTTP(jobW, jobReq)

			if jobW.Code != http.StatusOK && jobW.Code != http.StatusAccepted {
				errors[idx] = fmt.Errorf("unexpected status %d: %s", jobW.Code, jobW.Body.String())
				return
			}

			var job models.IndexJob
			if err := json.Unmarshal(jobW.Body.Bytes(), &job); err != nil {
				errors[idx] = err
				return
			}
			jobIDs[idx] = job.ID
		}(i)
	}

	wg.Wait()

	for i, err := range errors {
		if err != nil {
			t.Fatalf("job goroutine %d failed: %v", i, err)
		}
	}

	firstJobID := jobIDs[0]
	for i, id := range jobIDs {
		if id != firstJobID {
			t.Errorf("job goroutine %d got job ID %s, expected %s", i, id, firstJobID)
		}
	}
}

package repository

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codegraph/internal/models"
)

func TestPrepareLocalWorkspace(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ws_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	localRepoDir := filepath.Join(tmpDir, "my-repo")
	if err := os.MkdirAll(localRepoDir, 0755); err != nil {
		t.Fatal(err)
	}

	wm, err := NewWorkspaceManager(filepath.Join(tmpDir, "workspaces"))
	if err != nil {
		t.Fatal(err)
	}

	repo := &models.Repository{
		ID:         "repo-1",
		Name:       "Test Repo",
		SourceType: models.SourceTypeLocal,
		LocalPath:  localRepoDir,
	}

	ctx := context.Background()
	path, err := wm.PrepareWorkspace(ctx, repo)
	if err != nil {
		t.Fatalf("unexpected error preparing workspace: %v", err)
	}

	if path != filepath.Clean(localRepoDir) {
		t.Errorf("expected path %q, got %q", localRepoDir, path)
	}
}

func TestPrepareInvalidLocalWorkspace(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ws_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	wm, err := NewWorkspaceManager(filepath.Join(tmpDir, "workspaces"))
	if err != nil {
		t.Fatal(err)
	}

	repo := &models.Repository{
		ID:         "repo-2",
		Name:       "Missing Repo",
		SourceType: models.SourceTypeLocal,
		LocalPath:  filepath.Join(tmpDir, "nonexistent-dir"),
	}

	ctx := context.Background()
	_, err = wm.PrepareWorkspace(ctx, repo)
	if err == nil {
		t.Errorf("expected error for non-existent local directory, got nil")
	}
}

func TestPrepareGitWorkspace_ContextCancellation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ws_git_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	wm, err := NewWorkspaceManager(filepath.Join(tmpDir, "workspaces"))
	if err != nil {
		t.Fatal(err)
	}

	repo := &models.Repository{
		ID:         "repo-git-cancel",
		Name:       "Cancel Repo",
		SourceType: models.SourceTypeGit,
		SourceURL:  "https://github.com/gin-gonic/gin",
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel context immediately before execution

	_, err = wm.PrepareWorkspace(ctx, repo)
	if err == nil {
		t.Errorf("expected error for cancelled context, got nil")
	}
}

func TestPrepareGitWorkspace_ShortTimeout(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ws_git_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	wm, err := NewWorkspaceManager(filepath.Join(tmpDir, "workspaces"))
	if err != nil {
		t.Fatal(err)
	}

	repo := &models.Repository{
		ID:         "repo-git-timeout",
		Name:       "Timeout Repo",
		SourceType: models.SourceTypeGit,
		SourceURL:  "https://github.com/gin-gonic/gin",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(1 * time.Millisecond)

	_, err = wm.PrepareWorkspace(ctx, repo)
	if err == nil {
		t.Errorf("expected error for timed out context, got nil")
	}
}

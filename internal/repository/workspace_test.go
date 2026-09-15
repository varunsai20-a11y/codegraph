package repository

import (
	"os"
	"path/filepath"
	"testing"

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

	path, err := wm.PrepareWorkspace(repo)
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

	_, err = wm.PrepareWorkspace(repo)
	if err == nil {
		t.Errorf("expected error for non-existent local directory, got nil")
	}
}

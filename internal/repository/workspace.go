package repository

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"codegraph/internal/models"
	"codegraph/internal/security"
)

type WorkspaceManager struct {
	WorkspaceRoot string
}

func NewWorkspaceManager(workspaceRoot string) (*WorkspaceManager, error) {
	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for workspace root: %w", err)
	}

	if err := os.MkdirAll(absRoot, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace root directory: %w", err)
	}

	return &WorkspaceManager{WorkspaceRoot: absRoot}, nil
}

// PrepareWorkspace acquires or verifies repository path inside controlled workspace.
func (wm *WorkspaceManager) PrepareWorkspace(repo *models.Repository) (string, error) {
	switch repo.SourceType {
	case models.SourceTypeLocal:
		return wm.prepareLocalWorkspace(repo)
	case models.SourceTypeGit:
		return wm.prepareGitWorkspace(repo)
	default:
		return "", fmt.Errorf("unsupported repository source type: %s", repo.SourceType)
	}
}

func (wm *WorkspaceManager) prepareLocalWorkspace(repo *models.Repository) (string, error) {
	cleanPath := filepath.Clean(repo.LocalPath)
	info, err := os.Stat(cleanPath)
	if err != nil {
		return "", fmt.Errorf("local path does not exist or is inaccessible: %w", err)
	}

	if !info.IsDir() {
		return "", fmt.Errorf("local path is not a directory: %s", cleanPath)
	}

	return cleanPath, nil
}

func (wm *WorkspaceManager) prepareGitWorkspace(repo *models.Repository) (string, error) {
	if repo.SourceURL == "" {
		return "", fmt.Errorf("git repository source_url cannot be empty")
	}

	targetDir := filepath.Join(wm.WorkspaceRoot, repo.ID)
	if err := security.ValidatePathWithinWorkspace(wm.WorkspaceRoot, targetDir); err != nil {
		return "", err
	}

	// Check if already cloned
	if info, err := os.Stat(targetDir); err == nil && info.IsDir() {
		return targetDir, nil
	}

	// Acquire git repo without executing repo scripts
	cmd := exec.Command("git", "clone", "--depth", "1", repo.SourceURL, targetDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to clone git repository: %w, output: %s", err, string(output))
	}

	return targetDir, nil
}

package ingestion_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codegraph/internal/config"
	"codegraph/internal/ingestion"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/storage"
)

func TestLargeRepositoryIndexingCoverage(t *testing.T) {
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "large-repo")

	// Create directory layout
	dirs := []string{
		"src/components",
		"src/services",
		"src/utils",
		"api/v1",
		"docs",
		"node_modules/express",
		"dist",
		".git/objects",
		"scripts",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(repoDir, d), 0755); err != nil {
			t.Fatalf("failed to mkdir: %v", err)
		}
	}

	// Create .gitignore
	gitignoreContent := "node_modules/\ndist/\nbuild/\n.git/\n"
	_ = os.WriteFile(filepath.Join(repoDir, ".gitignore"), []byte(gitignoreContent), 0644)

	// Create meaningful source code, doc, config, and manifest files
	files := map[string]string{
		"README.md":                 "# Large Test Project\nAn end-to-end testing application.",
		"package.json":              `{"name": "large-test", "version": "1.0.0", "main": "src/index.ts"}`,
		"tsconfig.json":             `{"compilerOptions": {"target": "ES2022"}}`,
		"src/index.ts":              "import { UserService } from './services/user'; export function main() { new UserService().run(); }",
		"src/main.mts":              "export const appConfig = { port: 8080 };",
		"src/services/user.ts":      "export class UserService { run() { return true; } }",
		"src/components/Header.tsx": "export function Header() { return '<h1>Header</h1>'; }",
		"api/v1/router.go":         "package v1\nfunc RegisterRoutes() {}",
		"docs/architecture.md":     "# Architecture\nMicroservice setup.",
		"scripts/build.sh":          "#!/bin/bash\necho building...",
		"node_modules/express/a.js": "should be ignored",
		"dist/bundle.js":            "should be ignored",
	}

	for rel, content := range files {
		full := filepath.Join(repoDir, filepath.FromSlash(rel))
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file %s: %v", rel, err)
		}
	}

	dbPath := filepath.Join(tmpDir, "coverage_test.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}
	defer store.Close()

	wsMgr, err := repository.NewWorkspaceManager(filepath.Join(tmpDir, "workspaces"))
	if err != nil {
		t.Fatalf("failed to init workspace manager: %v", err)
	}

	cfg := &config.Config{
		DefaultExclusions: []string{".git", "node_modules", "dist", "build"},
		MaxFileSize:       10 * 1024 * 1024,
	}

	idx := ingestion.NewIndexer(cfg, store, wsMgr)

	ctx := context.Background()
	repo := &models.Repository{
		ID:         "repo-large-coverage",
		Name:       "large-test",
		SourceType: models.SourceTypeLocal,
		LocalPath:  repoDir,
		Status:     models.RepoStatusRegistered,
	}
	if err := store.CreateRepository(ctx, repo); err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	job := &models.IndexJob{
		ID:           "job-coverage-1",
		RepositoryID: repo.ID,
		Status:       models.JobStatusPending,
	}
	if err := store.CreateIndexJob(ctx, job); err != nil {
		t.Fatalf("failed to create job: %v", err)
	}

	if err := idx.RunIndex(ctx, job, repo); err != nil {
		t.Fatalf("RunIndex failed: %v", err)
	}

	// Verify IndexJob stats
	if job.Status != models.JobStatusCompleted {
		t.Errorf("expected job status COMPLETED, got %s", job.Status)
	}
	if job.FilesDiscovered == 0 {
		t.Errorf("expected FilesDiscovered > 0")
	}

	// Verify file manifest items in DB
	manifest, err := store.GetManifestForRepository(ctx, repo.ID)
	if err != nil {
		t.Fatalf("failed to get manifest: %v", err)
	}

	indexedPaths := make(map[string]bool)
	for _, m := range manifest {
		if m.Status == models.FileStatusIndexed {
			indexedPaths[m.RelativePath] = true
		}
	}

	// Must contain meaningful source code, configs, README
	expectedIncluded := []string{
		"README.md",
		"package.json",
		"tsconfig.json",
		"src/index.ts",
		"src/main.mts",
		"src/services/user.ts",
		"src/components/Header.tsx",
		"api/v1/router.go",
		"docs/architecture.md",
		"scripts/build.sh",
	}

	for _, exp := range expectedIncluded {
		if !indexedPaths[exp] {
			t.Errorf("expected meaningful file '%s' to be INDEXED", exp)
		}
	}

	// Must NOT contain ignored node_modules or dist files
	if indexedPaths["node_modules/express/a.js"] {
		t.Errorf("node_modules file should have been ignored")
	}
	if indexedPaths["dist/bundle.js"] {
		t.Errorf("dist file should have been ignored")
	}

	// Verify symbols were extracted
	symbols, err := store.GetSymbolsForRepository(ctx, repo.ID)
	if err != nil {
		t.Fatalf("failed to get symbols: %v", err)
	}
	if len(symbols) == 0 {
		t.Errorf("expected extracted symbols > 0")
	}
}

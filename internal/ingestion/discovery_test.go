package ingestion

import (
	"os"
	"path/filepath"
	"testing"

	"codegraph/internal/models"
)

func TestFileDiscoveryAndFiltering(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "disc_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create structure:
	// src/app.ts
	// node_modules/lib.js
	// dist/bundle.js
	// .env
	// image.png
	// large.txt
	// .gitignore (ignoring *.log)
	// test.log
	if err := os.MkdirAll(filepath.Join(tmpDir, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "node_modules"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "dist"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "src", "app.ts"), []byte("console.log('hi');"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "node_modules", "lib.js"), []byte("mod"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "dist", "bundle.js"), []byte("bundle"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("DB_PASS=secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "image.png"), []byte{0x89, 'P', 'N', 'G'}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte("*.log\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "test.log"), []byte("log entry"), 0644); err != nil {
		t.Fatal(err)
	}

	filter := NewFileFilter([]string{"node_modules", "dist"}, 1024*1024)
	scanner := NewDiscoveryScanner(filter)

	files, err := scanner.DiscoverFiles(tmpDir)
	if err != nil {
		t.Fatalf("unexpected discovery error: %v", err)
	}

	paths := make(map[string]bool)
	for _, f := range files {
		paths[f.RelativePath] = true
	}

	if !paths["src/app.ts"] {
		t.Errorf("expected 'src/app.ts' to be discovered")
	}
	if paths["node_modules/lib.js"] {
		t.Errorf("'node_modules/lib.js' should have been excluded")
	}
	if paths["dist/bundle.js"] {
		t.Errorf("'dist/bundle.js' should have been excluded")
	}
	if paths["test.log"] {
		t.Errorf("'test.log' should have been ignored by .gitignore")
	}

	// Verify evaluations
	for _, f := range files {
		status := filter.EvaluateFile(f.AbsolutePath, f.RelativePath, f.Info)
		if f.RelativePath == ".env" && status != models.FileStatusSecret {
			t.Errorf(".env status = %v; want %v", status, models.FileStatusSecret)
		}
		if f.RelativePath == "image.png" && status != models.FileStatusBinary {
			t.Errorf("image.png status = %v; want %v", status, models.FileStatusBinary)
		}
	}
}

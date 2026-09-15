package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsSecretFile(t *testing.T) {
	tests := []struct {
		path     string
		isSecret bool
	}{
		{".env", true},
		{".env.production", true},
		{"config/secrets.yml", true},
		{"keys/server.key", true},
		{"certs/cert.pem", true},
		{"ssh/id_rsa", true},
		{"src/main.ts", false},
		{"package.json", false},
	}

	for _, tt := range tests {
		got := IsSecretFile(tt.path)
		if got != tt.isSecret {
			t.Errorf("IsSecretFile(%q) = %v; want %v", tt.path, got, tt.isSecret)
		}
	}
}

func TestNormalizeRelativePath(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "sec_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseDir)

	subDir := filepath.Join(baseDir, "sub", "folder")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	rel, err := NormalizeRelativePath(baseDir, filepath.Join(subDir, "file.txt"))
	if err != nil {
		t.Fatalf("expected valid path, got err: %v", err)
	}
	if rel != "sub/folder/file.txt" {
		t.Errorf("expected 'sub/folder/file.txt', got %q", rel)
	}

	// Traversal attempt
	_, err = NormalizeRelativePath(baseDir, filepath.Join(baseDir, "..", "secret.txt"))
	if err == nil {
		t.Errorf("expected path traversal error, got nil")
	}
}

func TestSymlinkEscape(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "symlink_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	workspace := filepath.Join(tmpDir, "repo")
	outside := filepath.Join(tmpDir, "outside.txt")
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("outside"), 0644); err != nil {
		t.Fatal(err)
	}

	symlinkPath := filepath.Join(workspace, "link_to_outside.txt")
	if err := os.Symlink(outside, symlinkPath); err != nil {
		t.Skip("symlinks not supported on this OS/privilege level")
	}

	isSymlink, realPath, err := CheckSymlinkSafety(workspace, symlinkPath)
	if !isSymlink {
		t.Errorf("expected isSymlink to be true")
	}
	if err == nil {
		t.Errorf("expected SymlinkEscape error for target %q, got nil", realPath)
	}
}

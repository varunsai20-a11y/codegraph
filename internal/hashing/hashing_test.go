package hashing

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hash_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")
	file3 := filepath.Join(tmpDir, "file3.txt")

	contentA := []byte("console.log('hello world');\n")
	contentB := []byte("console.log('hello world');\n")
	contentC := []byte("console.log('different');\n")

	if err := os.WriteFile(file1, contentA, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, contentB, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file3, contentC, 0644); err != nil {
		t.Fatal(err)
	}

	hash1, err := HashFile(file1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hash2, err := HashFile(file2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hash3, err := HashFile(file3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("expected deterministic hash equality, got %s != %s", hash1, hash2)
	}
	if hash1 == hash3 {
		t.Errorf("expected different hashes for different contents, got %s == %s", hash1, hash3)
	}
}

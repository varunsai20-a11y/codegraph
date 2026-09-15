package security

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrPathTraversal = errors.New("security violation: path traversal attempt detected")
	ErrSymlinkEscape = errors.New("security violation: symlink points outside repository workspace")
)

// SecretPatterns defines common sensitive file names or extensions.
var SecretPatterns = []string{
	".env",
	".env.",
	"id_rsa",
	"id_ed25519",
	"credentials",
	"secrets",
}

var SecretExtensions = []string{
	".pem",
	".key",
	".pkcs12",
	".pfx",
	".asc",
}

// IsSecretFile checks if a file path matches known secret patterns.
func IsSecretFile(relPath string) bool {
	base := strings.ToLower(filepath.Base(relPath))
	ext := strings.ToLower(filepath.Ext(relPath))

	for _, secExt := range SecretExtensions {
		if ext == secExt {
			return true
		}
	}

	for _, pattern := range SecretPatterns {
		if base == pattern || strings.HasPrefix(base, pattern) {
			return true
		}
	}

	return false
}

// NormalizeRelativePath sanitizes a relative path to prevent traversal.
func NormalizeRelativePath(baseDir, targetPath string) (string, error) {
	cleanBase := filepath.Clean(baseDir)
	cleanTarget := filepath.Clean(targetPath)

	if !filepath.IsAbs(cleanTarget) {
		cleanTarget = filepath.Join(cleanBase, cleanTarget)
	}

	rel, err := filepath.Rel(cleanBase, cleanTarget)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrPathTraversal, err)
	}

	if strings.HasPrefix(rel, "..") || rel == ".." {
		return "", ErrPathTraversal
	}

	return filepath.ToSlash(rel), nil
}

// ValidatePathWithinWorkspace ensures the absolute path resolves strictly inside workspace.
func ValidatePathWithinWorkspace(workspaceRoot, targetPath string) error {
	absWorkspace, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return fmt.Errorf("failed to resolve workspace root: %w", err)
	}

	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("failed to resolve target path: %w", err)
	}

	rel, err := filepath.Rel(absWorkspace, absTarget)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return ErrPathTraversal
	}

	return nil
}

// CheckSymlinkSafety inspects if targetPath is a symlink and ensures it does not escape baseWorkspace.
func CheckSymlinkSafety(baseWorkspace, targetPath string) (isSymlink bool, realPath string, err error) {
	fi, err := os.Lstat(targetPath)
	if err != nil {
		return false, "", err
	}

	if fi.Mode()&os.ModeSymlink == 0 {
		return false, targetPath, nil
	}

	realPath, err = filepath.EvalSymlinks(targetPath)
	if err != nil {
		return true, "", fmt.Errorf("cannot resolve symlink target: %w", err)
	}

	if err := ValidatePathWithinWorkspace(baseWorkspace, realPath); err != nil {
		return true, realPath, ErrSymlinkEscape
	}

	return true, realPath, nil
}

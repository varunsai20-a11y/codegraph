package ingestion

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codegraph/internal/security"
)

type DiscoveredFile struct {
	AbsolutePath string
	RelativePath string
	Info         os.FileInfo
}

type DiscoveryScanner struct {
	Filter *FileFilter
}

func NewDiscoveryScanner(filter *FileFilter) *DiscoveryScanner {
	return &DiscoveryScanner{Filter: filter}
}

// DiscoverFiles recursively scans rootDir for all files while respecting exclusions and .gitignore files.
func (ds *DiscoveryScanner) DiscoverFiles(rootDir string) ([]DiscoveredFile, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute root: %w", err)
	}

	var results []DiscoveredFile
	gitignorePatterns := ds.loadGitignorePatterns(absRoot)

	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Skip unreadable files/dirs without breaking walk
			return nil
		}

		relPath, err := filepath.Rel(absRoot, path)
		if err != nil || relPath == "." {
			return nil
		}
		normalizedRel := filepath.ToSlash(relPath)

		// 1. Directory Exclusion check
		if d.IsDir() {
			dirName := d.Name()
			if ds.Filter.IsExcludedDir(dirName) || matchesAnyPattern(normalizedRel, gitignorePatterns) {
				return filepath.SkipDir
			}
			return nil
		}

		// 2. Symlink Safety check
		isSymlink, realPath, err := security.CheckSymlinkSafety(absRoot, path)
		if err != nil || (isSymlink && realPath == "") {
			// Skip unsafe symlink
			return nil
		}

		// 3. .gitignore pattern check
		if matchesAnyPattern(normalizedRel, gitignorePatterns) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		results = append(results, DiscoveredFile{
			AbsolutePath: path,
			RelativePath: normalizedRel,
			Info:         info,
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error during file discovery walk: %w", err)
	}

	return results, nil
}

func (ds *DiscoveryScanner) loadGitignorePatterns(absRoot string) []string {
	var patterns []string
	gitignorePath := filepath.Join(absRoot, ".gitignore")
	file, err := os.Open(gitignorePath)
	if err != nil {
		return patterns
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

func matchesAnyPattern(relPath string, patterns []string) bool {
	base := filepath.Base(relPath)
	for _, pattern := range patterns {
		pattern = strings.TrimPrefix(pattern, "/")
		if matched, _ := filepath.Match(pattern, relPath); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
		if strings.HasSuffix(pattern, "/") && strings.HasPrefix(relPath, strings.TrimSuffix(pattern, "/")) {
			return true
		}
	}
	return false
}

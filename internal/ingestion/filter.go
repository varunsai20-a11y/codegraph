package ingestion

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"

	"codegraph/internal/models"
	"codegraph/internal/security"
)

var BinaryExtensions = map[string]bool{
	".png":    true,
	".jpg":    true,
	".jpeg":   true,
	".gif":    true,
	".webp":   true,
	".ico":    true,
	".mp4":    true,
	".webm":   true,
	".mp3":    true,
	".zip":    true,
	".tar":    true,
	".gz":     true,
	".7z":     true,
	".exe":    true,
	".dll":    true,
	".so":     true,
	".dylib":  true,
	".pdf":    true,
	".woff":   true,
	".woff2":  true,
	".ttf":    true,
	".eot":    true,
	".pyc":    true,
	".o":      true,
	".a":      true,
	".class":  true,
	".db":     true,
	".sqlite": true,
	".bin":    true,
}

type FileFilter struct {
	DefaultExclusions map[string]bool
	MaxFileSize       int64
	GitIgnoreRules    []string
}

func NewFileFilter(exclusions []string, maxFileSize int64) *FileFilter {
	exMap := make(map[string]bool)
	for _, e := range exclusions {
		exMap[strings.ToLower(e)] = true
	}
	return &FileFilter{
		DefaultExclusions: exMap,
		MaxFileSize:       maxFileSize,
	}
}

// IsExcludedDir returns true if directory name matches default excluded directory.
func (f *FileFilter) IsExcludedDir(dirName string) bool {
	return f.DefaultExclusions[strings.ToLower(dirName)]
}

// EvaluateFile classifies a file into FileStatus.
func (f *FileFilter) EvaluateFile(absPath, relPath string, info os.FileInfo) models.FileStatus {
	// 1. Secret check
	if security.IsSecretFile(relPath) {
		return models.FileStatusSecret
	}

	// 2. Size limit check
	if f.MaxFileSize > 0 && info.Size() > f.MaxFileSize {
		return models.FileStatusOversized
	}

	// 3. Binary extension check
	ext := strings.ToLower(filepath.Ext(relPath))
	if BinaryExtensions[ext] {
		return models.FileStatusBinary
	}

	// 4. Binary null-byte content inspection check
	isBinary, err := f.isBinaryContent(absPath)
	if err != nil {
		return models.FileStatusFailed
	}
	if isBinary {
		return models.FileStatusBinary
	}

	return models.FileStatusIndexed
}

func (f *FileFilter) isBinaryContent(absPath string) (bool, error) {
	file, err := os.Open(absPath)
	if err != nil {
		return false, err
	}
	defer file.Close()

	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return false, err
	}

	if n == 0 {
		return false, nil
	}

	// Check for null bytes (0x00)
	if bytes.IndexByte(buf[:n], 0) != -1 {
		return true, nil
	}

	return false, nil
}

package ingestion

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"codegraph/internal/hashing"
	"codegraph/internal/models"
	"codegraph/internal/retrieval"
	"codegraph/internal/vector"
)

// VectorIndexer generates and persists code-aware semantic embeddings during repository ingestion.
type VectorIndexer struct {
	vectorStore vector.SemanticStore
	provider    vector.EmbeddingProvider
	astChunker  *retrieval.ASTSymbolChunker
}

func NewVectorIndexer(vStore vector.SemanticStore, provider vector.EmbeddingProvider) *VectorIndexer {
	return &VectorIndexer{
		vectorStore: vStore,
		provider:    provider,
		astChunker:  retrieval.NewASTSymbolChunker(),
	}
}

func (vi *VectorIndexer) IndexRepositoryVectors(
	ctx context.Context,
	scope models.RepositoryScope,
	workspacePath string,
	manifestItems []*models.FileManifestItem,
	symbols []*models.Symbol,
) error {
	if vi == nil || vi.vectorStore == nil || vi.provider == nil {
		return nil
	}

	// Clean-up existing embeddings for this repository to prevent stale vector buildup
	if err := vi.vectorStore.DeleteRepositoryEmbeddings(ctx, scope); err != nil {
		log.Printf("[VectorIndexer] Warning: Failed to clear previous embeddings for repo %s: %v", scope.RepositoryID, err)
	}

	var records []*vector.VectorRecord
	seenContentHashes := make(map[string]bool)

	// 1. Process AST Symbols into Code-Aware Vector Records (Functions, Methods, Classes, Structs, Variables)
	for _, sym := range symbols {
		if err := ctx.Err(); err != nil {
			return err
		}
		if sym.Kind != models.SymbolKindFunction && sym.Kind != models.SymbolKindMethod &&
			sym.Kind != models.SymbolKindClass && sym.Kind != models.SymbolKindStruct &&
			sym.Kind != models.SymbolKindInterface && sym.Kind != models.SymbolKindVariable {
			continue
		}

		item, err := vi.astChunker.CreateSymbolEvidence(scope, sym, workspacePath, 1.0)
		if err != nil || item == nil || strings.TrimSpace(item.Content) == "" {
			continue
		}

		// Expand content range for single-line variable or function headers
		content := item.Content
		loc := sym.Location
		if (loc.EndLine <= loc.StartLine || len(content) < 80) && workspacePath != "" && sym.RelativePath != "" {
			expandedContent, expandedEnd := expandSnippetRange(filepath.Join(workspacePath, filepath.FromSlash(sym.RelativePath)), loc.StartLine, 50)
			if strings.TrimSpace(expandedContent) != "" {
				content = expandedContent
				loc.EndLine = expandedEnd
			}
		}

		hash := hashing.HashBytes([]byte(content))
		if seenContentHashes[hash] {
			continue
		}
		seenContentHashes[hash] = true

		textToEmbed := fmt.Sprintf("%s %s (%s)\n%s", strings.ToLower(string(sym.Kind)), sym.QualifiedName, sym.RelativePath, content)
		if doc, ok := item.Metadata["documentation"]; ok && doc != "" {
			textToEmbed = fmt.Sprintf("Doc: %s\n%s", doc, textToEmbed)
		}

		vec, err := vi.provider.Embed(ctx, textToEmbed)
		if err != nil {
			log.Printf("[VectorIndexer] Warning: Failed to embed symbol %s in %s: %v", sym.Name, sym.RelativePath, err)
			continue
		}

		chunkID := fmt.Sprintf("sym:%s:%s", sym.FileID, sym.ID)
		record := &vector.VectorRecord{
			RepositoryID:       scope.RepositoryID,
			ChunkID:            chunkID,
			SymbolID:           sym.ID,
			RelativePath:       sym.RelativePath,
			Location:           loc,
			ContentHash:        hash,
			Content:            content,
			EmbeddingModel:     vi.provider.ModelName(),
			EmbeddingDimension: vi.provider.Dimension(),
			EmbeddingVersion:   vi.provider.Version(),
			Vector:             vec,
		}
		record.ID = vector.ComputeVectorID(scope.RepositoryID, chunkID, vi.provider.ModelName(), vi.provider.Version())
		records = append(records, record)
	}

	// 2. Process Source Code Files & Documentation in Bounded Windows
	for _, item := range manifestItems {
		if err := ctx.Err(); err != nil {
			return err
		}
		if isIgnoredPath(item.RelativePath) {
			continue
		}

		absPath := filepath.Join(workspacePath, filepath.FromSlash(item.RelativePath))
		contentBytes, err := os.ReadFile(absPath)
		if err != nil || len(contentBytes) == 0 {
			continue
		}

		windows := chunkFileIntoWindows(string(contentBytes), 60, 10, 2500)
		for idx, win := range windows {
			hash := hashing.HashBytes([]byte(win.content))
			if seenContentHashes[hash] {
				continue
			}
			seenContentHashes[hash] = true

			textToEmbed := fmt.Sprintf("File %s (lines %d-%d):\n%s", item.RelativePath, win.startLine, win.endLine, win.content)
			vec, err := vi.provider.Embed(ctx, textToEmbed)
			if err != nil {
				log.Printf("[VectorIndexer] Warning: Failed to embed file window %s [%d-%d]: %v", item.RelativePath, win.startLine, win.endLine, err)
				continue
			}

			chunkID := fmt.Sprintf("file:%s:win:%d", item.RelativePath, idx)
			record := &vector.VectorRecord{
				RepositoryID:       scope.RepositoryID,
				ChunkID:            chunkID,
				RelativePath:       item.RelativePath,
				Location:           models.Location{StartLine: win.startLine, EndLine: win.endLine},
				ContentHash:        hash,
				Content:            win.content,
				EmbeddingModel:     vi.provider.ModelName(),
				EmbeddingDimension: vi.provider.Dimension(),
				EmbeddingVersion:   vi.provider.Version(),
				Vector:             vec,
			}
			record.ID = vector.ComputeVectorID(scope.RepositoryID, chunkID, vi.provider.ModelName(), vi.provider.Version())
			records = append(records, record)
		}
	}

	if len(records) > 0 {
		if err := vi.vectorStore.SaveEmbeddings(ctx, scope, records); err != nil {
			return fmt.Errorf("failed to save vector embeddings: %w", err)
		}
		log.Printf("[VectorIndexer] Successfully generated and stored %d semantic vectors for repo %s", len(records), scope.RepositoryID)
	}

	return nil
}

type fileWindow struct {
	startLine int
	endLine   int
	content   string
}

func chunkFileIntoWindows(content string, windowLineCount, overlapLines, maxChars int) []fileWindow {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return nil
	}

	var windows []fileWindow
	step := windowLineCount - overlapLines
	if step <= 0 {
		step = windowLineCount
	}

	for i := 0; i < len(lines); i += step {
		end := i + windowLineCount
		if end > len(lines) {
			end = len(lines)
		}

		subLines := lines[i:end]
		winStr := strings.Join(subLines, "\n")
		winStr = strings.TrimSpace(winStr)
		if winStr == "" {
			continue
		}

		if len(winStr) > maxChars {
			winStr = winStr[:maxChars]
		}

		windows = append(windows, fileWindow{
			startLine: i + 1,
			endLine:   end,
			content:   winStr,
		})

		if end == len(lines) {
			break
		}
	}

	return windows
}

func expandSnippetRange(filePath string, startLine, maxLines int) (string, int) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", startLine
	}
	lines := strings.Split(string(data), "\n")
	if startLine < 1 || startLine > len(lines) {
		return "", startLine
	}

	sIdx := startLine - 1
	eIdx := sIdx + maxLines
	if eIdx > len(lines) {
		eIdx = len(lines)
	}

	for i := sIdx + 1; i < eIdx; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if (trimmed == "" || strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "class ") || strings.HasPrefix(trimmed, "func ")) && i > sIdx+5 {
			eIdx = i
			break
		}
	}

	return strings.Join(lines[sIdx:eIdx], "\n"), eIdx
}

func isIgnoredPath(relPath string) bool {
	lower := strings.ToLower(relPath)
	if strings.HasPrefix(lower, ".") || strings.Contains(lower, "/.") ||
		strings.Contains(lower, "node_modules/") || strings.Contains(lower, "vendor/") ||
		strings.Contains(lower, "__pycache__/") || strings.HasSuffix(lower, ".db") ||
		strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".jpg") ||
		strings.HasSuffix(lower, ".exe") || strings.HasSuffix(lower, ".lock") {
		return true
	}

	supportedExts := []string{".py", ".go", ".js", ".ts", ".tsx", ".jsx", ".java", ".c", ".cpp", ".h", ".cs", ".rs", ".rb", ".php", ".sh", ".ps1", ".md", ".txt", ".json", ".yaml", ".yml", ".toml", "dockerfile", "makefile", "requirements.txt", "package.json", "go.mod"}
	for _, ext := range supportedExts {
		if strings.HasSuffix(lower, ext) {
			return false
		}
	}

	return true
}

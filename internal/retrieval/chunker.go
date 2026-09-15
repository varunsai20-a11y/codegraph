package retrieval

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codegraph/internal/models"
)

// ASTSymbolChunker extracts code-aware AST snippets derived strictly from Phase 2 symbol source ranges.
// Note on AST Precision: Phase 2 static analyzers provide line-level (StartLine, EndLine) and column-level
// AST symbol locations. The core code content chunk corresponds strictly to the symbol's structural range [StartLine, EndLine].
// Preceding comments/docstrings are captured separately in Metadata["documentation"] to avoid expanding the core code range.
type ASTSymbolChunker struct{}

func NewASTSymbolChunker() *ASTSymbolChunker {
	return &ASTSymbolChunker{}
}

// CreateSymbolEvidence converts a Phase 2 Symbol into a code-aware EvidenceItem.
// The core Content is derived strictly from the symbol's exact line range [StartLine, EndLine].
func (c *ASTSymbolChunker) CreateSymbolEvidence(
	scope models.RepositoryScope,
	sym *models.Symbol,
	repoLocalPath string,
	rawScore float64,
) (*models.EvidenceItem, error) {
	if err := scope.ValidateItem(&models.EvidenceItem{RepositoryID: sym.RepositoryID}); err != nil {
		return nil, err
	}

	content := fmt.Sprintf("%s %s (%s:%d-%d)", strings.ToLower(string(sym.Kind)), sym.QualifiedName, sym.RelativePath, sym.Location.StartLine, sym.Location.EndLine)
	doc := ""

	if repoLocalPath != "" && sym.RelativePath != "" {
		fullPath := filepath.Join(repoLocalPath, filepath.FromSlash(sym.RelativePath))
		if snippet, docstring, err := c.extractExactRangeAndDoc(fullPath, sym.Location.StartLine, sym.Location.EndLine); err == nil && snippet != "" {
			content = snippet
			doc = docstring
		}
	}

	meta := map[string]string{
		"symbol_id":      sym.ID,
		"symbol_name":    sym.Name,
		"qualified_name": sym.QualifiedName,
		"symbol_kind":    string(sym.Kind),
	}
	if doc != "" {
		meta["documentation"] = doc
	}

	item := &models.EvidenceItem{
		RepositoryID:  sym.RepositoryID,
		Type:          models.EvidenceTypeSymbol,
		FileID:        sym.FileID,
		RelativePath:  sym.RelativePath,
		Location:      sym.Location,
		Content:       content,
		RetrieverType: "LEXICAL",
		RawScore:      rawScore,
		Metadata:      meta,
	}
	item.StableID = item.ComputeStableID()
	return item, nil
}

func (c *ASTSymbolChunker) extractExactRangeAndDoc(filePath string, startLine, endLine int) (string, string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", "", err
	}

	lines := strings.Split(string(data), "\n")
	if startLine < 1 || startLine > len(lines) {
		return "", "", fmt.Errorf("line out of bounds: %d", startLine)
	}

	// 1. Exact AST symbol code content strictly from startLine to endLine
	sIdx := startLine - 1
	eIdx := endLine
	if eIdx > len(lines) {
		eIdx = len(lines)
	}
	if eIdx < sIdx {
		eIdx = sIdx + 1
	}

	exactSnippet := strings.Join(lines[sIdx:eIdx], "\n")

	// 2. Preceding docstrings/comments captured separately as metadata
	docStart := sIdx
	for docStart > 0 && docStart >= startLine-3 {
		prevLine := strings.TrimSpace(lines[docStart-1])
		if strings.HasPrefix(prevLine, "//") || strings.HasPrefix(prevLine, "/*") || strings.HasPrefix(prevLine, "*") || strings.HasPrefix(prevLine, "#") || strings.HasPrefix(prevLine, "\"\"\"") {
			docStart--
		} else {
			break
		}
	}

	doc := ""
	if docStart < sIdx {
		doc = strings.Join(lines[docStart:sIdx], "\n")
	}

	return exactSnippet, doc, nil
}

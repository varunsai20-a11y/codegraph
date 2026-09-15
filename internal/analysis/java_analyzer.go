package analysis

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"time"

	"codegraph/internal/models"
)

type JavaAnalyzer struct{}

func NewJavaAnalyzer() *JavaAnalyzer {
	return &JavaAnalyzer{}
}

func (ja *JavaAnalyzer) Language() string {
	return "Java"
}

func (ja *JavaAnalyzer) Analyze(ctx context.Context, repoID, fileID, relPath, content string) (*models.FileAnalysis, error) {
	analysis := &models.FileAnalysis{
		FileID:       fileID,
		RelativePath: relPath,
		Language:     ja.Language(),
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0
	symbolMap := make(map[string]*models.Symbol)
	currentClass := ""
	currentClassID := ""

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			continue
		}

		// 1. Imports (Internal vs External)
		if strings.HasPrefix(trimmed, "import ") {
			impPath := strings.TrimSuffix(strings.TrimPrefix(trimmed, "import "), ";")
			impPath = strings.TrimSpace(impPath)
			if impPath != "" {
				loc := models.Location{StartLine: lineNum, StartColumn: 1, EndLine: lineNum, EndColumn: len(line)}
				targetKind := models.TargetKindExternal
				if !strings.HasPrefix(impPath, "java.") && !strings.HasPrefix(impPath, "javax.") {
					targetKind = models.TargetKindInternal
				}

				impRel := &models.Relationship{
					ID:           FormatRelationshipID(repoID, fileID, impPath, models.RelTypeImports, lineNum),
					RepositoryID: repoID,
					SourceID:     fileID,
					TargetID:     impPath,
					TargetKind:   targetKind,
					Type:         models.RelTypeImports,
					Status:       models.RelStatusResolved,
					FileID:       fileID,
					Location:     loc,
					UpdatedAt:    time.Now(),
				}
				analysis.Relationships = append(analysis.Relationships, impRel)
			}
			continue
		}

		// 2. Class / Interface / Enum declaration
		if strings.Contains(trimmed, "class ") || strings.Contains(trimmed, "interface ") || strings.Contains(trimmed, "enum ") {
			kind := models.SymbolKindClass
			if strings.Contains(trimmed, "interface ") {
				kind = models.SymbolKindInterface
			} else if strings.Contains(trimmed, "enum ") {
				kind = models.SymbolKindEnum
			}

			name := parseJavaClassName(trimmed)
			if name != "" {
				loc := models.Location{StartLine: lineNum, StartColumn: 1, EndLine: lineNum, EndColumn: len(line)}
				symID := FormatSymbolID(repoID, relPath, name, lineNum)
				sym := &models.Symbol{
					ID:            symID,
					RepositoryID:  repoID,
					FileID:        fileID,
					RelativePath:  relPath,
					Name:          name,
					QualifiedName: name,
					Kind:          kind,
					Location:      loc,
					UpdatedAt:     time.Now(),
				}
				analysis.Symbols = append(analysis.Symbols, sym)
				symbolMap[name] = sym
				currentClass = name
				currentClassID = symID

				// Extends check
				if strings.Contains(trimmed, "extends ") {
					parent := extractTargetAfterWord(trimmed, "extends")
					if parent != "" {
						extRel := &models.Relationship{
							ID:           FormatRelationshipID(repoID, symID, parent, models.RelTypeExtends, lineNum),
							RepositoryID: repoID,
							SourceID:     symID,
							TargetID:     parent,
							TargetKind:   models.TargetKindInternal,
							Type:         models.RelTypeExtends,
							Status:       models.RelStatusPartial,
							FileID:       fileID,
							Location:     loc,
							UpdatedAt:    time.Now(),
						}
						analysis.Relationships = append(analysis.Relationships, extRel)
					}
				}

				// Implements check
				if strings.Contains(trimmed, "implements ") {
					ifaces := extractTargetAfterWord(trimmed, "implements")
					if ifaces != "" {
						for _, iface := range strings.Split(ifaces, ",") {
							iface = strings.TrimSpace(iface)
							if iface != "" {
								implRel := &models.Relationship{
									ID:           FormatRelationshipID(repoID, symID, iface, models.RelTypeImplements, lineNum),
									RepositoryID: repoID,
									SourceID:     symID,
									TargetID:     iface,
									TargetKind:   models.TargetKindInternal,
									Type:         models.RelTypeImplements,
									Status:       models.RelStatusPartial,
									FileID:       fileID,
									Location:     loc,
									UpdatedAt:    time.Now(),
								}
								analysis.Relationships = append(analysis.Relationships, implRel)
							}
						}
					}
				}
			}
			continue
		}

		// 3. Method / Constructor declaration
		if strings.Contains(trimmed, "(") && strings.Contains(trimmed, ")") && (strings.Contains(trimmed, "public ") || strings.Contains(trimmed, "private ") || strings.Contains(trimmed, "protected ")) {
			mName := parseJavaMethodName(trimmed)
			if mName != "" && mName != currentClass {
				loc := models.Location{StartLine: lineNum, StartColumn: 1, EndLine: lineNum, EndColumn: len(line)}
				qualName := fmt.Sprintf("%s.%s", currentClass, mName)
				symID := FormatSymbolID(repoID, relPath, qualName, lineNum)
				sym := &models.Symbol{
					ID:            symID,
					RepositoryID:  repoID,
					FileID:        fileID,
					RelativePath:  relPath,
					Name:          mName,
					QualifiedName: qualName,
					Kind:          models.SymbolKindMethod,
					ParentID:      currentClassID,
					Location:      loc,
					UpdatedAt:     time.Now(),
				}
				analysis.Symbols = append(analysis.Symbols, sym)
				symbolMap[mName] = sym
			}
		}
	}

	return analysis, nil
}

func parseJavaClassName(line string) string {
	words := strings.Fields(line)
	for i, w := range words {
		if (w == "class" || w == "interface" || w == "enum") && i+1 < len(words) {
			target := words[i+1]
			target = strings.Split(target, "<")[0]
			target = strings.TrimSuffix(target, "{")
			return strings.TrimSpace(target)
		}
	}
	return ""
}

func parseJavaMethodName(line string) string {
	idx := strings.Index(line, "(")
	if idx <= 0 {
		return ""
	}
	beforeParen := strings.TrimSpace(line[:idx])
	words := strings.Fields(beforeParen)
	if len(words) >= 2 {
		return words[len(words)-1]
	}
	return ""
}

func extractTargetAfterWord(line, word string) string {
	idx := strings.Index(line, word+" ")
	if idx == -1 {
		return ""
	}
	after := line[idx+len(word)+1:]
	after = strings.Split(after, "{")[0]
	after = strings.Split(after, "implements")[0]
	return strings.TrimSpace(after)
}

package analysis

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"time"

	"codegraph/internal/models"
)

type PythonAnalyzer struct{}

func NewPythonAnalyzer() *PythonAnalyzer {
	return &PythonAnalyzer{}
}

func (pa *PythonAnalyzer) Language() string {
	return "Python"
}

func (pa *PythonAnalyzer) Analyze(ctx context.Context, repoID, fileID, relPath, content string) (*models.FileAnalysis, error) {
	analysis := &models.FileAnalysis{
		FileID:       fileID,
		RelativePath: relPath,
		Language:     pa.Language(),
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

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// 1. Class definition
		if strings.HasPrefix(trimmed, "class ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				className := strings.Split(parts[1], "(")[0]
				className = strings.TrimSuffix(className, ":")

				loc := models.Location{StartLine: lineNum, StartColumn: 1, EndLine: lineNum, EndColumn: len(line)}
				symID := FormatSymbolID(repoID, relPath, className, lineNum)
				sym := &models.Symbol{
					ID:            symID,
					RepositoryID:  repoID,
					FileID:        fileID,
					RelativePath:  relPath,
					Name:          className,
					QualifiedName: className,
					Kind:          models.SymbolKindClass,
					Location:      loc,
					UpdatedAt:     time.Now(),
				}
				analysis.Symbols = append(analysis.Symbols, sym)
				symbolMap[className] = sym
				currentClass = className
				currentClassID = symID

				// Inheritance check e.g., class Child(Parent):
				if strings.Contains(parts[1], "(") && strings.Contains(parts[1], ")") {
					parentName := extractBetween(parts[1], "(", ")")
					if parentName != "" && parentName != "object" {
						extRel := &models.Relationship{
							ID:           FormatRelationshipID(repoID, symID, parentName, models.RelTypeExtends, lineNum),
							RepositoryID: repoID,
							SourceID:     symID,
							TargetID:     parentName,
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
			}
			continue
		}

		// 2. Function / Method definition
		if strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "async def ") {
			funcStr := strings.TrimPrefix(trimmed, "async ")
			funcStr = strings.TrimPrefix(funcStr, "def ")
			funcName := strings.Split(funcStr, "(")[0]

			kind := models.SymbolKindFunction
			parentID := ""
			qualName := funcName

			// Method check (indented inside class or contains self/cls)
			if currentClass != "" && (strings.HasPrefix(line, "    def ") || strings.HasPrefix(line, "\tdef ")) {
				kind = models.SymbolKindMethod
				parentID = currentClassID
				qualName = fmt.Sprintf("%s.%s", currentClass, funcName)
			}

			loc := models.Location{StartLine: lineNum, StartColumn: 1, EndLine: lineNum, EndColumn: len(line)}
			symID := FormatSymbolID(repoID, relPath, qualName, lineNum)
			sym := &models.Symbol{
				ID:            symID,
				RepositoryID:  repoID,
				FileID:        fileID,
				RelativePath:  relPath,
				Name:          funcName,
				QualifiedName: qualName,
				Kind:          kind,
				ParentID:      parentID,
				Location:      loc,
				UpdatedAt:     time.Now(),
			}
			analysis.Symbols = append(analysis.Symbols, sym)
			symbolMap[funcName] = sym
			continue
		}

		// 3. Imports (Internal vs External)
		if strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "from ") {
			modName := parsePythonImport(trimmed)
			if modName != "" {
				loc := models.Location{StartLine: lineNum, StartColumn: 1, EndLine: lineNum, EndColumn: len(line)}
				targetKind := models.TargetKindExternal
				if strings.HasPrefix(modName, ".") {
					targetKind = models.TargetKindInternal
				}

				impRel := &models.Relationship{
					ID:           FormatRelationshipID(repoID, fileID, modName, models.RelTypeImports, lineNum),
					RepositoryID: repoID,
					SourceID:     fileID,
					TargetID:     modName,
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

		// 4. Calls
		if idx := strings.Index(trimmed, "("); idx > 0 {
			candidate := trimmed[:idx]
			if isIdentifier(candidate) {
				loc := models.Location{StartLine: lineNum, StartColumn: 1, EndLine: lineNum, EndColumn: len(line)}
				status := models.RelStatusUnresolved
				target := candidate

				if sym, found := symbolMap[candidate]; found {
					target = sym.ID
					status = models.RelStatusResolved
				} else {
					status = models.RelStatusPartial
				}

				callRel := &models.Relationship{
					ID:           FormatRelationshipID(repoID, fileID, target, models.RelTypeCalls, lineNum),
					RepositoryID: repoID,
					SourceID:     fileID,
					TargetID:     target,
					TargetKind:   models.TargetKindInternal,
					Type:         models.RelTypeCalls,
					Status:       status,
					FileID:       fileID,
					Location:     loc,
					UpdatedAt:    time.Now(),
				}
				analysis.Relationships = append(analysis.Relationships, callRel)
			}
		}
	}

	return analysis, nil
}

func parsePythonImport(line string) string {
	fields := strings.Fields(line)
	if len(fields) >= 2 && fields[0] == "import" {
		return fields[1]
	}
	if len(fields) >= 2 && fields[0] == "from" {
		return fields[1]
	}
	return ""
}

func extractBetween(s, start, end string) string {
	sIdx := strings.Index(s, start)
	eIdx := strings.Index(s, end)
	if sIdx != -1 && eIdx != -1 && eIdx > sIdx {
		return strings.TrimSpace(s[sIdx+len(start) : eIdx])
	}
	return ""
}

func isIdentifier(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, " =+-*/%&|^~!@#$%^&*()") {
		return false
	}
	return true
}

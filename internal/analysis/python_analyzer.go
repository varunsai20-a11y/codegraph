package analysis

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
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

var (
	pyBuiltins = map[string]bool{
		"True": true, "False": true, "None": true, "str": true, "int": true, "float": true,
		"bool": true, "list": true, "dict": true, "set": true, "tuple": true, "self": true,
		"cls": true, "print": true, "len": true, "range": true, "os": true, "sys": true,
		"super": true, "open": true, "logging": true, "logger": true, "argparse": true,
		"type": true, "isinstance": true, "def": true, "class": true, "import": true,
		"from": true, "as": true, "if": true, "else": true, "return": true, "with": true,
		"for": true, "in": true, "try": true, "except": true, "raise": true, "finally": true,
		"pass": true, "break": true, "continue": true, "not": true, "and": true, "or": true,
		"is": true, "lambda": true, "object": true, "exec": true, "eval": true,
	}
	pyIdentRegex         = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\b`)
	pyStringLiteralRegex = regexp.MustCompile(`(?s)"""(.*?)"""|(?s)'''(.*?)'''|"[^"\\]*(?:\\.[^"\\]*)*"|'[^'\\]*(?:\\.[^'\\]*)*'`)
)

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
	currentFuncID := ""
	inFunction := false
	inMainGuard := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Main guard detection
		if strings.Contains(trimmed, "__name__") && strings.Contains(trimmed, "__main__") {
			inMainGuard = true
		}

		// Reset class or function scope when an unindented line is encountered
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			if !strings.HasPrefix(trimmed, "def ") && !strings.HasPrefix(trimmed, "async def ") && !strings.HasPrefix(trimmed, "class ") {
				inFunction = false
				currentFuncID = ""
			}
			if !strings.HasPrefix(trimmed, "class ") && !strings.HasPrefix(trimmed, "def ") && !strings.HasPrefix(trimmed, "async def ") {
				currentClass = ""
				currentClassID = ""
			}
		}

		// 1. Class definition
		if strings.HasPrefix(trimmed, "class ") {
			inFunction = false
			currentFuncID = ""
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
			inFunction = true
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
			currentFuncID = symID
			continue
		}

		// 3. Imports (Internal vs External & Alias Support)
		if strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "from ") {
			modName, importedSyms, alias := parsePythonImportDetails(trimmed)
			if modName != "" {
				loc := models.Location{StartLine: lineNum, StartColumn: 1, EndLine: lineNum, EndColumn: len(line)}
				targetKind := models.TargetKindInternal
				if !strings.HasPrefix(modName, ".") && isPyStdlibOrThirdParty(modName) {
					targetKind = models.TargetKindExternal
				}

				targetID := modName
				if alias != "" {
					targetID = fmt.Sprintf("%s as %s", modName, alias)
				}
				if len(importedSyms) > 0 {
					targetID = fmt.Sprintf("%s:%s", modName, strings.Join(importedSyms, ","))
				}

				impRel := &models.Relationship{
					ID:           FormatRelationshipID(repoID, fileID, targetID, models.RelTypeImports, lineNum),
					RepositoryID: repoID,
					SourceID:     fileID,
					TargetID:     targetID,
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

		// 4. Assignments & Symbol References (Module-level or Inside Function)
		if strings.Contains(trimmed, "=") && !strings.HasPrefix(trimmed, "if ") && !strings.HasPrefix(trimmed, "while ") {
			eqIdx := strings.Index(trimmed, "=")
			varCandidate := strings.TrimSpace(trimmed[:eqIdx])

			startL := lineNum
			// Extract module-level variable symbol if applicable
			if !inFunction && currentClass == "" && isIdentifier(varCandidate) && !strings.HasPrefix(varCandidate, "class ") && !strings.HasPrefix(varCandidate, "def ") && !strings.HasPrefix(varCandidate, "import ") && !strings.HasPrefix(varCandidate, "from ") {
				loc := models.Location{StartLine: startL, StartColumn: 1, EndLine: lineNum, EndColumn: len(line)}
				symID := FormatSymbolID(repoID, relPath, varCandidate, startL)
				sym := &models.Symbol{
					ID:            symID,
					RepositoryID:  repoID,
					FileID:        fileID,
					RelativePath:  relPath,
					Name:          varCandidate,
					QualifiedName: varCandidate,
					Kind:          models.SymbolKindVariable,
					Location:      loc,
					UpdatedAt:     time.Now(),
				}
				analysis.Symbols = append(analysis.Symbols, sym)
				symbolMap[varCandidate] = sym
				currentFuncID = symID
			}

			rhs := strings.TrimSpace(trimmed[eqIdx+1:])
			openCount := strings.Count(trimmed, "(") + strings.Count(trimmed, "[") + strings.Count(trimmed, "{")
			closeCount := strings.Count(trimmed, ")") + strings.Count(trimmed, "]") + strings.Count(trimmed, "}")

			fullRHS := rhs
			for openCount > closeCount && scanner.Scan() {
				lineNum++
				nextL := scanner.Text()
				nextTrim := strings.TrimSpace(nextL)
				openCount += strings.Count(nextTrim, "(") + strings.Count(nextTrim, "[") + strings.Count(nextTrim, "{")
				closeCount += strings.Count(nextTrim, ")") + strings.Count(nextTrim, "]") + strings.Count(nextTrim, "}")
				fullRHS += "\n" + nextTrim
			}

			activeSource := currentFuncID
			if activeSource == "" {
				activeSource = fileID
			}

			extractPythonRHSReferences(repoID, fileID, activeSource, fullRHS, startL, symbolMap, analysis)
			continue
		}

		// 5. Calls & References in standard statement lines
		if strings.Contains(trimmed, "(") {
			activeSource := currentFuncID
			if activeSource == "" {
				activeSource = fileID
			}
			extractPythonRHSReferences(repoID, fileID, activeSource, trimmed, lineNum, symbolMap, analysis)
		}
	}

	if inMainGuard {
		analysis.Warnings = append(analysis.Warnings, "MAIN_GUARD_DETECTED")
	}

	return analysis, nil
}

func parsePythonImportDetails(line string) (modName string, importedSyms []string, alias string) {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "import ") {
		rest := strings.TrimPrefix(line, "import ")
		if strings.Contains(rest, " as ") {
			parts := strings.Split(rest, " as ")
			return strings.TrimSpace(parts[0]), nil, strings.TrimSpace(parts[1])
		}
		return strings.TrimSpace(rest), nil, ""
	}
	if strings.HasPrefix(line, "from ") {
		rest := strings.TrimPrefix(line, "from ")
		fromParts := strings.Split(rest, " import ")
		if len(fromParts) == 2 {
			modName = strings.TrimSpace(fromParts[0])
			impPart := strings.TrimSpace(fromParts[1])
			if strings.Contains(impPart, " as ") {
				asParts := strings.Split(impPart, " as ")
				importedSyms = []string{strings.TrimSpace(asParts[0])}
				alias = strings.TrimSpace(asParts[1])
			} else {
				syms := strings.Split(impPart, ",")
				for _, s := range syms {
					sClean := strings.TrimSpace(s)
					if sClean != "" {
						importedSyms = append(importedSyms, sClean)
					}
				}
			}
			return modName, importedSyms, alias
		}
	}
	return "", nil, ""
}

func isPyStdlibOrThirdParty(mod string) bool {
	base := strings.Split(mod, ".")[0]
	switch base {
	case "os", "sys", "math", "time", "json", "re", "typing", "datetime", "pathlib", "bufio", "io", "subprocess", "logging", "argparse", "functools", "collections", "random", "httpx", "requests", "dotenv", "crewai", "crewai_tools", "langchain", "pydantic", "fastapi", "flask", "numpy", "pandas":
		return true
	default:
		return false
	}
}

func extractPythonRHSReferences(repoID, fileID, sourceID, expr string, lineNum int, symbolMap map[string]*models.Symbol, analysis *models.FileAnalysis) {
	codeOnly := pyStringLiteralRegex.ReplaceAllString(expr, `""`)
	tokens := pyIdentRegex.FindAllString(codeOnly, -1)
	if len(tokens) == 0 {
		return
	}

	seen := make(map[string]bool)
	loc := models.Location{StartLine: lineNum, StartColumn: 1, EndLine: lineNum, EndColumn: len(expr)}

	// 1. Direct call target check (e.g. research_crew.kickoff or Task)
	if idx := strings.Index(expr, "("); idx > 0 {
		prefix := strings.TrimSpace(expr[:idx])
		if lastDot := strings.LastIndex(prefix, "."); lastDot > 0 {
			target := prefix
			if !seen[target] {
				seen[target] = true
				rel := &models.Relationship{
					ID:           FormatRelationshipID(repoID, sourceID, target, models.RelTypeCalls, lineNum),
					RepositoryID: repoID,
					SourceID:     sourceID,
					TargetID:     target,
					TargetKind:   models.TargetKindInternal,
					Type:         models.RelTypeCalls,
					Status:       models.RelStatusPartial,
					FileID:       fileID,
					Location:     loc,
					UpdatedAt:    time.Now(),
				}
				analysis.Relationships = append(analysis.Relationships, rel)
			}
		}
	}

	// 2. Identifier references in arguments (e.g. agent=research_agent, tools=[search_tool])
	for _, tok := range tokens {
		if pyBuiltins[tok] || seen[tok] || len(tok) < 2 {
			continue
		}
		seen[tok] = true

		target := tok
		status := models.RelStatusPartial
		if sym, found := symbolMap[tok]; found {
			target = sym.ID
			status = models.RelStatusResolved
		}

		rel := &models.Relationship{
			ID:           FormatRelationshipID(repoID, sourceID, target, models.RelTypeCalls, lineNum),
			RepositoryID: repoID,
			SourceID:     sourceID,
			TargetID:     target,
			TargetKind:   models.TargetKindInternal,
			Type:         models.RelTypeCalls,
			Status:       status,
			FileID:       fileID,
			Location:     loc,
			UpdatedAt:    time.Now(),
		}
		analysis.Relationships = append(analysis.Relationships, rel)
	}
}

func parsePythonImport(line string) string {
	mod, _, _ := parsePythonImportDetails(line)
	return mod
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


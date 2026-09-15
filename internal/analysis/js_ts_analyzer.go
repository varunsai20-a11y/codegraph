package analysis

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codegraph/internal/models"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/js"
)

type JSTSAnalyzer struct {
	lang string
}

func NewJSTSAnalyzer(lang string) *JSTSAnalyzer {
	return &JSTSAnalyzer{lang: lang}
}

func (j *JSTSAnalyzer) Language() string {
	return j.lang
}

func (j *JSTSAnalyzer) Analyze(ctx context.Context, repoID, fileID, relPath, content string) (*models.FileAnalysis, error) {
	analysis := &models.FileAnalysis{
		FileID:       fileID,
		RelativePath: relPath,
		Language:     j.lang,
	}

	lexer := js.NewLexer(parse.NewInputString(content))
	lineOffsets := calculateLineOffsets(content)
	symbolMap := make(map[string]*models.Symbol)

	var prevToken js.TokenType
	var prevText string
	pos := 0

	for {
		tt, textBytes := lexer.Next()
		text := string(textBytes)
		if tt == js.ErrorToken {
			break
		}

		startPos := pos
		endPos := pos + len(textBytes)
		pos = endPos
		loc := getLoc(uint32(startPos), uint32(endPos), lineOffsets)

		// 1. Function declaration: function foo()
		if tt == js.IdentifierToken && (prevToken == js.FunctionToken || prevText == "function") {
			funcName := text
			symID := FormatSymbolID(repoID, relPath, funcName, loc.StartLine)
			sym := &models.Symbol{
				ID:            symID,
				RepositoryID:  repoID,
				FileID:        fileID,
				RelativePath:  relPath,
				Name:          funcName,
				QualifiedName: funcName,
				Kind:          models.SymbolKindFunction,
				Location:      loc,
				UpdatedAt:     time.Now(),
			}
			analysis.Symbols = append(analysis.Symbols, sym)
			symbolMap[funcName] = sym
		}

		// 2. Class declaration: class Foo
		if tt == js.IdentifierToken && (prevToken == js.ClassToken || prevText == "class") {
			className := text
			symID := FormatSymbolID(repoID, relPath, className, loc.StartLine)
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
		}

		// 3. Interface or Type declaration: interface Foo / type Foo
		if tt == js.IdentifierToken && (prevText == "interface" || prevText == "type" || prevText == "enum") {
			kind := models.SymbolKindType
			if prevText == "interface" {
				kind = models.SymbolKindInterface
			} else if prevText == "enum" {
				kind = models.SymbolKindEnum
			}

			symID := FormatSymbolID(repoID, relPath, text, loc.StartLine)
			sym := &models.Symbol{
				ID:            symID,
				RepositoryID:  repoID,
				FileID:        fileID,
				RelativePath:  relPath,
				Name:          text,
				QualifiedName: text,
				Kind:          kind,
				Location:      loc,
				UpdatedAt:     time.Now(),
			}
			analysis.Symbols = append(analysis.Symbols, sym)
			symbolMap[text] = sym
		}

		// 4. Import statements: import ... from 'module'
		if prevText == "from" && (tt == js.StringToken || strings.HasPrefix(text, "'") || strings.HasPrefix(text, `"`)) {
			impModule := strings.Trim(text, `"'`)
			if impModule != "" {
				targetKind := models.TargetKindExternal
				if strings.HasPrefix(impModule, ".") || strings.HasPrefix(impModule, "/") {
					targetKind = models.TargetKindInternal
				}

				impRel := &models.Relationship{
					ID:           FormatRelationshipID(repoID, fileID, impModule, models.RelTypeImports, loc.StartLine),
					RepositoryID: repoID,
					SourceID:     fileID,
					TargetID:     impModule,
					TargetKind:   targetKind,
					Type:         models.RelTypeImports,
					Status:       models.RelStatusResolved,
					FileID:       fileID,
					Location:     loc,
					UpdatedAt:    time.Now(),
				}
				analysis.Relationships = append(analysis.Relationships, impRel)
			}
		}

		// 5. Calls: foo(...)
		if tt == js.OpenParenToken && prevToken == js.IdentifierToken && prevText != "" {
			callTarget := prevText
			status := models.RelStatusUnresolved

			if sym, found := symbolMap[callTarget]; found {
				callTarget = sym.ID
				status = models.RelStatusResolved
			} else {
				status = models.RelStatusPartial
			}

			if isIdentifier(callTarget) && !isKeyword(callTarget) {
				callRel := &models.Relationship{
					ID:           FormatRelationshipID(repoID, fileID, callTarget, models.RelTypeCalls, loc.StartLine),
					RepositoryID: repoID,
					SourceID:     fileID,
					TargetID:     callTarget,
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

		if tt != js.WhitespaceToken && tt != js.CommentToken && tt != js.LineTerminatorToken {
			prevToken = tt
			prevText = text
		}
	}

	return analysis, nil
}

func isKeyword(s string) bool {
	switch s {
	case "if", "for", "while", "switch", "catch", "function", "return", "import", "export", "typeof":
		return true
	}
	return false
}

func calculateLineOffsets(content string) []int {
	offsets := []int{0}
	for i, r := range content {
		if r == '\n' {
			offsets = append(offsets, i+1)
		}
	}
	return offsets
}

func getLoc(startPos, endPos uint32, offsets []int) models.Location {
	startLine, startCol := getLineCol(int(startPos), offsets)
	endLine, endCol := getLineCol(int(endPos), offsets)
	return models.Location{
		StartLine:   startLine,
		StartColumn: startCol,
		EndLine:     endLine,
		EndColumn:   endCol,
	}
}

func getLineCol(pos int, offsets []int) (int, int) {
	if pos <= 0 {
		return 1, 1
	}
	line := 1
	for i, offset := range offsets {
		if pos < offset {
			break
		}
		line = i + 1
	}
	col := pos - offsets[line-1] + 1
	return line, col
}

var _ = fmt.Sprintf

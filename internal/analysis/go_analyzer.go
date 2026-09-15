package analysis

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"time"

	"codegraph/internal/models"
)

type GoAnalyzer struct{}

func NewGoAnalyzer() *GoAnalyzer {
	return &GoAnalyzer{}
}

func (ga *GoAnalyzer) Language() string {
	return "Go"
}

func (ga *GoAnalyzer) Analyze(ctx context.Context, repoID, fileID, relPath, content string) (*models.FileAnalysis, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, relPath, content, parser.ParseComments)
	if err != nil {
		// Resilient return on syntax errors
		return &models.FileAnalysis{
			FileID:       fileID,
			RelativePath: relPath,
			Language:     ga.Language(),
			Warnings:     []string{fmt.Sprintf("go syntax parsing warning: %v", err)},
		}, nil
	}

	analysis := &models.FileAnalysis{
		FileID:       fileID,
		RelativePath: relPath,
		Language:     ga.Language(),
	}

	symbolMap := make(map[string]*models.Symbol)

	// 1. Extract Package & Symbols
	ast.Inspect(node, func(n ast.Node) bool {
		if n == nil {
			return true
		}

		pos := fset.Position(n.Pos())
		endPos := fset.Position(n.End())

		loc := models.Location{
			StartLine:   pos.Line,
			StartColumn: pos.Column,
			EndLine:     endPos.Line,
			EndColumn:   endPos.Column,
		}

		switch decl := n.(type) {
		case *ast.FuncDecl:
			kind := models.SymbolKindFunction
			parentID := ""
			name := decl.Name.Name
			qualName := fmt.Sprintf("%s.%s", node.Name.Name, name)

			if decl.Recv != nil && len(decl.Recv.List) > 0 {
				kind = models.SymbolKindMethod
				recvType := ga.formatType(decl.Recv.List[0].Type)
				qualName = fmt.Sprintf("%s.%s.%s", node.Name.Name, recvType, name)
				parentID = FormatSymbolID(repoID, relPath, recvType, 0)
			}

			symID := FormatSymbolID(repoID, relPath, name, loc.StartLine)
			sym := &models.Symbol{
				ID:            symID,
				RepositoryID:  repoID,
				FileID:        fileID,
				RelativePath:  relPath,
				Name:          name,
				QualifiedName: qualName,
				Kind:          kind,
				ParentID:      parentID,
				Location:      loc,
				UpdatedAt:     time.Now(),
			}
			analysis.Symbols = append(analysis.Symbols, sym)
			symbolMap[name] = sym

			// Record CONTAINS relationship
			containsRel := &models.Relationship{
				ID:           FormatRelationshipID(repoID, fileID, symID, models.RelTypeContains, loc.StartLine),
				RepositoryID: repoID,
				SourceID:     fileID,
				TargetID:     symID,
				TargetKind:   models.TargetKindInternal,
				Type:         models.RelTypeContains,
				Status:       models.RelStatusResolved,
				FileID:       fileID,
				Location:     loc,
				UpdatedAt:    time.Now(),
			}
			analysis.Relationships = append(analysis.Relationships, containsRel)

		case *ast.TypeSpec:
			name := decl.Name.Name
			kind := models.SymbolKindType
			switch decl.Type.(type) {
			case *ast.StructType:
				kind = models.SymbolKindStruct
			case *ast.InterfaceType:
				kind = models.SymbolKindInterface
			}

			symID := FormatSymbolID(repoID, relPath, name, loc.StartLine)
			sym := &models.Symbol{
				ID:            symID,
				RepositoryID:  repoID,
				FileID:        fileID,
				RelativePath:  relPath,
				Name:          name,
				QualifiedName: fmt.Sprintf("%s.%s", node.Name.Name, name),
				Kind:          kind,
				Location:      loc,
				UpdatedAt:     time.Now(),
			}
			analysis.Symbols = append(analysis.Symbols, sym)
			symbolMap[name] = sym
		}

		return true
	})

	// 2. Extract Imports (Distinguishing Internal vs External)
	for _, imp := range node.Imports {
		impPath := strings.Trim(imp.Path.Value, `"`)
		pos := fset.Position(imp.Pos())
		endPos := fset.Position(imp.End())
		loc := models.Location{
			StartLine:   pos.Line,
			StartColumn: pos.Column,
			EndLine:     endPos.Line,
			EndColumn:   endPos.Column,
		}

		targetKind := models.TargetKindExternal
		if !strings.Contains(impPath, ".") || strings.HasPrefix(impPath, "codegraph/") {
			targetKind = models.TargetKindInternal
		}

		impRel := &models.Relationship{
			ID:           FormatRelationshipID(repoID, fileID, impPath, models.RelTypeImports, loc.StartLine),
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

	// 3. Extract Function Calls with Honest Static Resolution Status
	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		pos := fset.Position(call.Pos())
		endPos := fset.Position(call.End())
		loc := models.Location{
			StartLine:   pos.Line,
			StartColumn: pos.Column,
			EndLine:     endPos.Line,
			EndColumn:   endPos.Column,
		}

		callTarget, status := ga.resolveCallExpr(call, symbolMap)
		if callTarget != "" {
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

		return true
	})

	return analysis, nil
}

func (ga *GoAnalyzer) formatType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return ga.formatType(t.X)
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", ga.formatType(t.X), t.Sel.Name)
	default:
		return filepath.Base(fmt.Sprintf("%T", expr))
	}
}

func (ga *GoAnalyzer) resolveCallExpr(call *ast.CallExpr, localSymbols map[string]*models.Symbol) (string, models.RelationStatus) {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		// Direct function call in same file/package
		if sym, found := localSymbols[fun.Name]; found {
			return sym.ID, models.RelStatusResolved
		}
		return fun.Name, models.RelStatusPartial
	case *ast.SelectorExpr:
		// Method or package call e.g., store.GetRepository or fmt.Sprintf
		recv := ga.formatType(fun.X)
		target := fmt.Sprintf("%s.%s", recv, fun.Sel.Name)
		return target, models.RelStatusUnresolved
	default:
		return "", models.RelStatusUnresolved
	}
}

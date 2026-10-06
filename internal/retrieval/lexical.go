package retrieval

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/storage"
)

type LexicalRetriever struct {
	store   storage.Storage
	wsMgr   *repository.WorkspaceManager
	chunker *ASTSymbolChunker
}

func NewLexicalRetriever(store storage.Storage, wsMgr *repository.WorkspaceManager) *LexicalRetriever {
	return &LexicalRetriever{
		store:   store,
		wsMgr:   wsMgr,
		chunker: NewASTSymbolChunker(),
	}
}

func (r *LexicalRetriever) Retrieve(
	ctx context.Context,
	scope models.RepositoryScope,
	query string,
	intent string,
	limit int,
) ([]*models.EvidenceItem, error) {
	if scope.RepositoryID == "" {
		return nil, models.ErrInvalidRepositoryScope
	}
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return []*models.EvidenceItem{}, nil
	}
	if limit <= 0 {
		limit = 10
	}

	// 1. Fetch static intelligence symbols for target repository ONLY
	symbols, err := r.store.GetSymbolsForRepository(ctx, scope.RepositoryID)
	if err != nil {
		return nil, err
	}

	// Fetch repository details & resolve actual workspace disk path
	repo, _ := r.store.GetRepository(ctx, scope.RepositoryID)
	repoLocalPath := ""
	if repo != nil {
		if r.wsMgr != nil {
			if path, err := r.wsMgr.PrepareWorkspace(ctx, repo); err == nil && path != "" {
				repoLocalPath = path
			}
		}
		if repoLocalPath == "" {
			repoLocalPath = repo.LocalPath
		}
	}

	effectiveIntent := intent
	if effectiveIntent == "" || effectiveIntent == "EXPLANATION" || effectiveIntent == IntentExplanation {
		classifier := NewRuleBasedIntentClassifier()
		effectiveIntent = classifier.Classify(query)
	}

	lowerQuery := strings.ToLower(trimmedQuery)
	var candidates []*models.EvidenceItem

	// 2. Score symbols against query and query intent
	for _, sym := range symbols {
		rawScore := r.calculateSymbolScore(sym, trimmedQuery, lowerQuery, effectiveIntent)
		if rawScore <= 0 {
			continue
		}

		item, err := r.chunker.CreateSymbolEvidence(scope, sym, repoLocalPath, rawScore)
		if err != nil {
			continue
		}
		candidates = append(candidates, item)
	}

	// 3. Score file manifest paths and source content against query and query intent
	manifestItems, _ := r.store.GetManifestForRepository(ctx, scope.RepositoryID)
	for _, f := range manifestItems {
		if f.Status == models.FileStatusSecret || f.Status == models.FileStatusBinary || f.Status == models.FileStatusOversized || f.Status == models.FileStatusIgnored {
			continue
		}

		pathScore := r.calculateFileScore(f, trimmedQuery, lowerQuery, effectiveIntent)

		content := "file " + f.RelativePath + " (" + f.Language + ")"
		evidenceType := models.EvidenceTypeDependency
		startLine, endLine := 1, 1
		contentScore := 0.0

		if repoLocalPath != "" && f.RelativePath != "" {
			fullPath := filepath.Join(repoLocalPath, filepath.FromSlash(f.RelativePath))
			if info, err := os.Stat(fullPath); err == nil && !info.IsDir() && info.Size() <= 2*1024*1024 {
				if data, err := os.ReadFile(fullPath); err == nil && len(data) > 0 {
					lines := strings.Split(string(data), "\n")
					totalLines := len(lines)

					// Extract non-stopword query terms for content search
					queryWords := strings.FieldsFunc(lowerQuery, func(r rune) bool {
						return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-')
					})
					var nonStopTerms []string
					for _, qw := range queryWords {
						if len(qw) >= 3 && !isStopWord(qw) {
							nonStopTerms = append(nonStopTerms, qw)
						}
					}

					bestLineIdx := 0
					maxTermMatches := 0
					matchedTermsSet := make(map[string]bool)

					for lIdx, lineStr := range lines {
						lowerLine := strings.ToLower(lineStr)
						matches := 0
						for _, qw := range nonStopTerms {
							if strings.Contains(lowerLine, qw) {
								matches++
								matchedTermsSet[qw] = true
							}
						}
						if matches > maxTermMatches {
							maxTermMatches = matches
							bestLineIdx = lIdx
						}
					}

					// Fallback to all queryWords >= 3 if no non-stop terms matched
					if len(matchedTermsSet) == 0 {
						for lIdx, lineStr := range lines {
							lowerLine := strings.ToLower(lineStr)
							matches := 0
							for _, qw := range queryWords {
								if len(qw) >= 3 && strings.Contains(lowerLine, qw) {
									matches++
									matchedTermsSet[qw] = true
								}
							}
							if matches > maxTermMatches {
								maxTermMatches = matches
								bestLineIdx = lIdx
							}
						}
					}

					if len(matchedTermsSet) > 0 {
						contentScore = 30.0 + float64(len(matchedTermsSet))*15.0
						if maxTermMatches >= 2 {
							contentScore += float64(maxTermMatches) * 5.0
						}
						for _, lineStr := range lines {
							if len(trimmedQuery) >= 5 && strings.Contains(strings.ToLower(lineStr), lowerQuery) {
								contentScore += 25.0
								break
							}
						}
					}

					// Slicing window around best matching line
					windowSize := 60
					halfWin := windowSize / 2
					sIdx := bestLineIdx - halfWin
					if sIdx < 0 {
						sIdx = 0
					}
					eIdx := sIdx + windowSize
					if eIdx > totalLines {
						eIdx = totalLines
						sIdx = eIdx - windowSize
						if sIdx < 0 {
							sIdx = 0
						}
					}

					startLine = sIdx + 1
					endLine = eIdx
					if endLine < startLine {
						endLine = startLine
					}

					snippetLines := lines[sIdx:eIdx]
					content = strings.Join(snippetLines, "\n")

					ext := strings.ToLower(filepath.Ext(f.RelativePath))
					base := strings.ToLower(filepath.Base(f.RelativePath))
					if ext == ".md" || ext == ".rst" || ext == ".txt" {
						evidenceType = models.EvidenceTypeDocumentation
					} else if ext == ".yml" || ext == ".yaml" || ext == ".json" || base == "requirements.txt" || base == ".env.example" {
						evidenceType = models.EvidenceTypeDependency
					} else {
						evidenceType = models.EvidenceTypeCodeSnippet
					}
				}
			}
		}

		rawScore := pathScore
		if contentScore > 0 {
			if pathScore > 0 {
				rawScore = pathScore + contentScore
			} else {
				rawScore = contentScore
			}
		}

		if rawScore <= 0 {
			continue
		}

		item := &models.EvidenceItem{
			RepositoryID:  scope.RepositoryID,
			Type:          evidenceType,
			FileID:        f.ID,
			RelativePath:  f.RelativePath,
			Location:      models.Location{StartLine: startLine, EndLine: endLine},
			Content:       content,
			RetrieverType: "LEXICAL",
			RawScore:      rawScore,
			Metadata: map[string]string{
				"file_id":       f.ID,
				"language":      f.Language,
				"relative_path": f.RelativePath,
			},
		}
		item.StableID = item.ComputeStableID()
		candidates = append(candidates, item)
	}

	// 4. Deterministic Sort: Primary = RawScore desc, Secondary = StableID asc
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].RawScore != candidates[j].RawScore {
			return candidates[i].RawScore > candidates[j].RawScore
		}
		return candidates[i].StableID < candidates[j].StableID
	})

	// 5. Assign sequential Rank and limit output
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	for i, item := range candidates {
		item.Rank = i + 1
	}

	return candidates, nil
}

// calculateSymbolScore evaluates a static symbol against query terms and intent heuristics.
func (r *LexicalRetriever) calculateSymbolScore(sym *models.Symbol, query, lowerQuery, intent string) float64 {
	lowerName := strings.ToLower(sym.Name)
	lowerQual := strings.ToLower(sym.QualifiedName)
	score := 0.0

	if query == sym.QualifiedName || lowerQuery == lowerQual {
		score = 100.0
	} else if query == sym.Name || lowerQuery == lowerName {
		score = 80.0
	} else if strings.HasPrefix(lowerName, lowerQuery) {
		score = 60.0
	} else if strings.Contains(lowerQual, lowerQuery) || strings.Contains(lowerName, lowerQuery) {
		score = 40.0
	}

	if score == 0.0 {
		words := strings.FieldsFunc(lowerQuery, func(r rune) bool {
			return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-')
		})
		for _, w := range words {
			if len(w) < 3 {
				continue
			}
			if w == lowerName || w == lowerQual {
				score = 70.0
				break
			} else if strings.Contains(lowerName, w) || strings.Contains(lowerQual, w) {
				score = 40.0
				break
			}
		}
	}

	if score == 0.0 {
		return 0.0
	}

	// Intent & Execution-Flow rank adjustments
	if isStartupQuery(lowerQuery) || isExecutionFlowQuery(lowerQuery) || intent == IntentRepoOverview || intent == IntentArchitectureQuery || intent == IntentTrace {
		relLower := strings.ToLower(sym.RelativePath)
		nameLower := strings.ToLower(sym.Name)
		if nameLower == "main" || nameLower == "ensure_environment" || nameLower == "check_ollama_server" || nameLower == "init" || nameLower == "run" || relLower == "run.py" || relLower == "main.go" || relLower == "main.py" || relLower == "app.py" {
			score += 45.0
		}
	}

	switch intent {
	case IntentSymbolLookup:
		if score >= 80.0 {
			score += 30.0
		}
	case IntentCallerQuery, IntentCalleeQuery:
		if sym.Kind == models.SymbolKindFunction || sym.Kind == models.SymbolKindMethod {
			score += 25.0
		}
	case IntentFeatureSearch:
		if score <= 60.0 {
			score += 15.0
		}
	}

	return score
}

// calculateFileScore evaluates a file path against query terms and intent heuristics.
func (r *LexicalRetriever) calculateFileScore(f *models.FileManifestItem, query, lowerQuery, intent string) float64 {
	lowerPath := strings.ToLower(f.RelativePath)
	baseName := strings.ToLower(filepath.Base(f.RelativePath))
	ext := strings.ToLower(filepath.Ext(f.RelativePath))

	targetFile := ExtractTargetFile(query)
	if targetFile != "" {
		cleanTarget := strings.ToLower(targetFile)
		if lowerPath == cleanTarget || baseName == cleanTarget || strings.HasSuffix(lowerPath, "/"+cleanTarget) {
			return 150.0
		}
	}

	score := 0.0

	if query == f.RelativePath || lowerQuery == lowerPath || lowerQuery == baseName {
		score = 60.0
	} else if strings.Contains(lowerPath, lowerQuery) {
		score = 40.0
	}

	// Term matching against file path
	words := strings.FieldsFunc(lowerQuery, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '/' || r == '.')
	})
	matchedTerms := 0
	for _, w := range words {
		cleanW := CleanToken(w)
		if len(cleanW) < 3 || isStopWord(cleanW) {
			continue
		}
		if baseName == cleanW || strings.TrimSuffix(baseName, ext) == cleanW {
			score += 45.0
			matchedTerms++
		} else if strings.Contains(baseName, cleanW) || strings.Contains(lowerPath, cleanW) {
			score += 25.0
			matchedTerms++
		}
	}

	// Intent-aware repository-level purpose/overview query matching
	if intent == IntentArchitectureQuery {
		if baseName == "readme.md" || baseName == "architecture.md" {
			score += 35.0
			matchedTerms++
		} else if baseName == "main.go" || baseName == "run.py" || baseName == "app.py" || baseName == "app.java" || baseName == "index.ts" {
			score += 25.0
			matchedTerms++
		}
	}

	// CRITICAL: If no query terms matched the file path or repository overview criteria, return 0.0
	if matchedTerms == 0 && score == 0.0 {
		return 0.0
	}

	// Executable source code boost for code implementation queries (ONLY if file already matched terms!)
	isSourceCode := ext == ".py" || ext == ".ts" || ext == ".js" || ext == ".go" || ext == ".java"
	wantsSource := strings.Contains(lowerQuery, "source") || strings.Contains(lowerQuery, "function") || strings.Contains(lowerQuery, "class") || strings.Contains(lowerQuery, "call") || strings.Contains(lowerQuery, "responsibil") || strings.Contains(lowerQuery, "implement") || strings.Contains(lowerQuery, "agent") || strings.Contains(lowerQuery, "workflow")

	if isSourceCode && wantsSource && score > 0.0 {
		score += 20.0
	}

	switch intent {
	case IntentDependencyQuery, IntentArchitectureQuery:
		if score > 0.0 {
			score += 15.0
		}
	}

	return score
}

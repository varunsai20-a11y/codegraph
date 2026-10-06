package llm

import (
	"fmt"
	"strings"

	"codegraph/internal/models"
	"codegraph/internal/retrieval"
)

// AnalysisContext provides a query-aware structured representation of repository intelligence
// for consumption by LLMs (Gemini, Groq, or local Ollama qwen2.5:3b).
type AnalysisContext struct {
	RepositoryID         string                 `json:"repository_id"`
	RepositoryName       string                 `json:"repository_name,omitempty"`
	RepositoryStats      models.RepositoryStats `json:"repository_stats"`
	UserQuery            string                 `json:"user_query"`
	QueryIntent          string                 `json:"query_intent"`
	PurposeEvidence      string                 `json:"purpose_evidence,omitempty"`
	ArchitectureEvidence string                 `json:"architecture_evidence,omitempty"`
	RelevantFiles        []string               `json:"relevant_files,omitempty"`
	RelevantSymbols      []string               `json:"relevant_symbols,omitempty"`
	Relationships        []string               `json:"relationships,omitempty"`
	ExecutionFlow        string                 `json:"execution_flow,omitempty"`
	DependencyEvidence   string                 `json:"dependency_evidence,omitempty"`
	Snippets             []string               `json:"snippets,omitempty"`
	UnverifiedItems      []string               `json:"unverified_items,omitempty"`
}

// BuildAnalysisContext converts an EvidencePackage into a query-aware AnalysisContext.
func BuildAnalysisContext(pkg *models.EvidencePackage) *AnalysisContext {
	if pkg == nil {
		return &AnalysisContext{}
	}

	classifier := retrieval.NewRuleBasedIntentClassifier()
	intent := classifier.Classify(pkg.Question)

	ac := &AnalysisContext{
		RepositoryID: pkg.RepositoryID,
		UserQuery:    pkg.Question,
		QueryIntent:  string(intent),
	}

	fileSet := make(map[string]bool)
	symbolSet := make(map[string]bool)

	maxSnippets := 10
	for i, item := range pkg.Items {
		if item.RelativePath != "" {
			fileSet[item.RelativePath] = true
		}
		if symName, ok := item.Metadata["symbol_name"]; ok && symName != "" {
			symbolSet[symName] = true
		} else if sym, ok := item.Metadata["symbol"]; ok && sym != "" {
			symbolSet[sym] = true
		} else if item.Label != "" {
			symbolSet[item.Label] = true
		}

		if i < maxSnippets {
			content := item.Content
			if len(content) > 800 {
				content = content[:800] + "\n... [truncated for context size]"
			}
			snipStr := fmt.Sprintf("[%s] %s (L%d-L%d)\n%s", item.Label, item.RelativePath, item.Location.StartLine, item.Location.EndLine, content)
			ac.Snippets = append(ac.Snippets, snipStr)
		}

		lowerPath := strings.ToLower(item.RelativePath)
		if strings.HasSuffix(lowerPath, "readme.md") {
			ac.PurposeEvidence = item.Content
		} else if strings.HasSuffix(lowerPath, "package.json") || strings.HasSuffix(lowerPath, "go.mod") || strings.HasSuffix(lowerPath, "cargo.toml") {
			ac.DependencyEvidence = item.Content
		}
	}

	for f := range fileSet {
		ac.RelevantFiles = append(ac.RelevantFiles, f)
	}
	for s := range symbolSet {
		ac.RelevantSymbols = append(ac.RelevantSymbols, s)
	}

	for _, cit := range pkg.Citations {
		ac.Relationships = append(ac.Relationships, fmt.Sprintf("%s (%s)", cit.EvidenceID, cit.RelativePath))
	}

	if pkg.Sufficiency.Status == models.SufficiencyPartial || len(pkg.Items) == 0 {
		ac.UnverifiedItems = append(ac.UnverifiedItems, "Some internal call paths or private implementations could not be fully verified from indexed evidence.")
	}

	return ac
}

func (ac *AnalysisContext) String() string {
	var sb strings.Builder

	sb.WriteString("=== REPOSITORY ANALYSIS CONTEXT ===\n")
	if ac.RepositoryID != "" {
		sb.WriteString(fmt.Sprintf("Repository ID: %s\n", ac.RepositoryID))
	}
	if ac.QueryIntent != "" {
		sb.WriteString(fmt.Sprintf("Query Intent: %s\n", ac.QueryIntent))
	}
	sb.WriteString("\n")

	if len(ac.RelevantFiles) > 0 {
		sb.WriteString("--- RELEVANT FILES ---\n")
		for _, f := range ac.RelevantFiles {
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
		sb.WriteString("\n")
	}

	if len(ac.RelevantSymbols) > 0 {
		sb.WriteString("--- RELEVANT SYMBOLS ---\n")
		for _, sym := range ac.RelevantSymbols {
			sb.WriteString(fmt.Sprintf("- %s\n", sym))
		}
		sb.WriteString("\n")
	}

	if len(ac.Snippets) > 0 {
		sb.WriteString("--- GROUNDED CODE EVIDENCE SNIPPETS ---\n")
		for _, snip := range ac.Snippets {
			sb.WriteString(snip)
			sb.WriteString("\n\n")
		}
	}

	if len(ac.UnverifiedItems) > 0 {
		sb.WriteString("--- UNVERIFIED / UNCERTAIN ITEMS ---\n")
		for _, u := range ac.UnverifiedItems {
			sb.WriteString(fmt.Sprintf("- %s\n", u))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("=== END REPOSITORY ANALYSIS CONTEXT ===")
	return sb.String()
}

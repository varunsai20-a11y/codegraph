package llm

import (
	"regexp"
	"sort"
	"strings"

	"codegraph/internal/models"
)

var (
	// codeGraphCandidatePattern extracts only CodeGraph citation candidates starting with E or Evidence inside brackets.
	// Ordinary bracketed prose/code tokens like [GET], [10], [], [array], [i] do NOT match this pattern and are ignored.
	codeGraphCandidatePattern = regexp.MustCompile(`\[(E\d+|E[_-]\d+|E[A-Za-z0-9_-]+|Evidence\d+)\]`)
	standardEPattern          = regexp.MustCompile(`^E\d+$`)
)

// CitationValidationReport holds the output of deterministic citation validation against an EvidencePackage.
type CitationValidationReport struct {
	RawCitations             []string          `json:"raw_citations"`
	ValidatedCitations       []models.Citation `json:"validated_citations"`
	InvalidCitations         []string          `json:"invalid_citations"`
	MalformedCitations       []string          `json:"malformed_citations"`
	CitationValidationStatus string            `json:"citation_validation_status"`
	TotalCitations           int               `json:"total_citations"`
	ValidCount               int               `json:"valid_count"`
	InvalidCount             int               `json:"invalid_count"`
	CitationCoverage         float64           `json:"citation_coverage"`
}

// CitationValidator deterministically extracts and validates citations from LLM output.
type CitationValidator struct{}

func NewCitationValidator() *CitationValidator {
	return &CitationValidator{}
}

func (v *CitationValidator) Validate(content string, pkg *models.EvidencePackage) CitationValidationReport {
	if pkg == nil {
		return CitationValidationReport{
			CitationValidationStatus: "NO_CITATIONS_PRESENT",
			CitationCoverage:         1.0,
		}
	}

	// Index package evidence items by Label and StableID for fast deterministic lookup
	knownItemsByLabel := make(map[string]*models.EvidenceItem)
	knownItemsByStableID := make(map[string]*models.EvidenceItem)

	for _, item := range pkg.Items {
		if item.Label != "" {
			knownItemsByLabel[item.Label] = item
		}
		if item.StableID != "" {
			knownItemsByStableID[item.StableID] = item
		}
	}

	// Extract all CodeGraph citation candidates in text order
	matches := codeGraphCandidatePattern.FindAllStringSubmatch(content, -1)

	var rawCitations []string
	var invalidCitations []string
	var malformedCitations []string

	validatedMap := make(map[string]models.Citation)
	var validatedList []models.Citation

	validCount := 0
	invalidCount := 0

	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		rawToken := m[0]   // e.g. "[E1]"
		tokenValue := m[1] // e.g. "E1"
		rawCitations = append(rawCitations, rawToken)

		// Check if matches standard E<number> pattern (e.g. E1, E2, E99)
		if standardEPattern.MatchString(tokenValue) {
			item, found := knownItemsByLabel[tokenValue]
			if !found {
				// Check if StableID match
				item, found = knownItemsByStableID[tokenValue]
			}

			if found {
				validCount++
				if _, exists := validatedMap[item.StableID]; !exists {
					cit := models.Citation{
						EvidenceID:   item.Label,
						StableID:     item.StableID,
						RelativePath: item.RelativePath,
						Location:     item.Location,
						IsValid:      true,
					}
					validatedMap[item.StableID] = cit
					validatedList = append(validatedList, cit)
				}
			} else {
				invalidCount++
				invalidCitations = append(invalidCitations, rawToken)
			}
		} else {
			// Non-standard CodeGraph citation format e.g. [E-1], [Evidence1], [E_1]
			invalidCount++
			malformedCitations = append(malformedCitations, rawToken)
		}
	}

	// Sort validated citations deterministically by EvidenceID (E1, E2, ...)
	sort.Slice(validatedList, func(i, j int) bool {
		return validatedList[i].EvidenceID < validatedList[j].EvidenceID
	})

	totalCitations := len(rawCitations)
	coverage := 1.0
	if totalCitations > 0 {
		coverage = float64(validCount) / float64(totalCitations)
	}

	status := "VALIDATION_PASSED"
	if totalCitations == 0 {
		status = "NO_CITATIONS_PRESENT"
	} else if invalidCount > 0 || len(malformedCitations) > 0 {
		status = "INVALID_CITATIONS_FOUND"
	}

	return CitationValidationReport{
		RawCitations:             rawCitations,
		ValidatedCitations:       validatedList,
		InvalidCitations:         invalidCitations,
		MalformedCitations:       malformedCitations,
		CitationValidationStatus: status,
		TotalCitations:           totalCitations,
		ValidCount:               validCount,
		InvalidCount:             invalidCount,
		CitationCoverage:         coverage,
	}
}

var _ = strings.TrimSpace

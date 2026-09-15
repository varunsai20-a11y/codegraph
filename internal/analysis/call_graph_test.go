package analysis

import (
	"context"
	"testing"

	"codegraph/internal/models"
)

func TestCallGraphResolutionStatuses(t *testing.T) {
	content := `package calc

func add(a, b int) int {
	return a + b
}

func calculateSum(x, y int) int {
	// 1. Direct same-file call -> RESOLVED
	res := add(x, y)
	
	// 4. Recursive call -> RESOLVED
	if res < 0 {
		return calculateSum(-x, -y)
	}
	
	return res
}

type Calculator struct{}

func (c *Calculator) Execute(v int) int {
	// 3. Method call on selector -> UNRESOLVED
	return c.Compute(v)
}

func (c *Calculator) Compute(v int) int {
	return v * 2
}

func ProcessData() {
	// 2. Cross-file / unknown function call -> PARTIAL
	unknownHelper()
}
`

	ga := NewGoAnalyzer()
	res, err := ga.Analyze(context.Background(), "repo-1", "file-calc", "calc.go", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	callMap := make(map[string]*models.Relationship)
	for _, rel := range res.Relationships {
		if rel.Type == models.RelTypeCalls {
			callMap[rel.TargetID] = rel
		}
	}

	// 1. Direct same-file function call -> RESOLVED
	addSymID := FormatSymbolID("repo-1", "calc.go", "add", 3)
	if rel, ok := callMap[addSymID]; !ok || rel.Status != models.RelStatusResolved {
		t.Errorf("expected direct same-file call to 'add' to be RESOLVED, got %+v", rel)
	}

	// 4. Recursive call -> RESOLVED
	calcSymID := FormatSymbolID("repo-1", "calc.go", "calculateSum", 7)
	if rel, ok := callMap[calcSymID]; !ok || rel.Status != models.RelStatusResolved {
		t.Errorf("expected recursive call to 'calculateSum' to be RESOLVED, got %+v", rel)
	}

	// 3. Selector method call -> UNRESOLVED
	if rel, ok := callMap["c.Compute"]; !ok || rel.Status != models.RelStatusUnresolved {
		t.Errorf("expected selector method call 'c.Compute' to be UNRESOLVED, got %+v", rel)
	}

	// 2. Cross-file / unknown identifier -> PARTIAL
	if rel, ok := callMap["unknownHelper"]; !ok || rel.Status != models.RelStatusPartial {
		t.Errorf("expected unknown function call 'unknownHelper' to be PARTIAL, got %+v", rel)
	}
}

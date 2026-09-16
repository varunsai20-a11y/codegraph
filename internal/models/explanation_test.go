package models_test

import (
	"encoding/json"
	"testing"

	"codegraph/internal/models"
)

func TestC6_1_ValidExplanationRequest(t *testing.T) {
	scope, err := models.NewRepositoryScope("repo-alpha")
	if err != nil {
		t.Fatalf("expected valid scope, got err: %v", err)
	}

	req, err := models.NewExplanationRequest(scope, "How does authentication work?")
	if err != nil {
		t.Fatalf("expected valid request, got err: %v", err)
	}

	if req.RepositoryScope.RepositoryID != "repo-alpha" {
		t.Errorf("expected repo-alpha, got %s", req.RepositoryScope.RepositoryID)
	}
	if req.Question != "How does authentication work?" {
		t.Errorf("expected question match, got %s", req.Question)
	}
	if err := req.Validate(); err != nil {
		t.Errorf("expected request validation to pass, got err: %v", err)
	}
}

func TestC6_1_RepositoryScopePreservationAndIsolation(t *testing.T) {
	scopeA, _ := models.NewRepositoryScope("repo-A")
	scopeB, _ := models.NewRepositoryScope("repo-B")

	itemA := &models.EvidenceItem{
		StableID:     "stab-100",
		Label:        "E1",
		RepositoryID: "repo-A",
		Type:         models.EvidenceTypeSymbol,
		RelativePath: "auth/service.go",
		Content:      "func Authenticate()",
	}

	// Converting itemA with scopeA should succeed
	evA, err := models.NewExplanationEvidenceFromItem(scopeA, itemA)
	if err != nil {
		t.Fatalf("expected success when scope matches evidence, got err: %v", err)
	}

	// Validating evA against scopeB must fail deterministically
	if err := evA.ValidateScope(scopeB); err == nil {
		t.Fatalf("expected error when validating repo-A evidence against repo-B scope, got nil")
	}

	// Attempting to construct evidence for scopeB from repo-A item must fail
	_, err = models.NewExplanationEvidenceFromItem(scopeB, itemA)
	if err == nil {
		t.Fatalf("expected repository mismatch error, got nil")
	}

	// Insufficient evidence response with cross-repository evidence must be rejected
	sufficiency := models.EvidenceSufficiencyResult{Status: models.SufficiencyInsufficient}
	_, err = models.NewInsufficientEvidenceResponse(scopeB, "question?", sufficiency, []*models.ExplanationEvidence{evA}, nil)
	if err == nil {
		t.Fatalf("expected repository mismatch error when attaching repo-A evidence to repo-B response, got nil")
	}
}

func TestC6_1_EvidenceProvenance(t *testing.T) {
	scope, _ := models.NewRepositoryScope("repo-core")
	item := &models.EvidenceItem{
		StableID:     "stable-proof-42",
		Label:        "E3",
		RepositoryID: "repo-core",
		Type:         models.EvidenceTypeGraphEdge,
		FileID:       "file-99",
		RelativePath: "pkg/core.go",
		Location: models.Location{
			StartLine:   10,
			StartColumn: 1,
			EndLine:     20,
			EndColumn:   5,
		},
		Content:       "call site proof",
		RetrieverType: "GRAPH",
		Metadata: map[string]string{
			"symbol_id":    "sym-auth",
			"symbol_name":  "VerifyToken",
			"edge_kind":    "EDGE_CALLS",
			"flow_path_id": "path-77",
		},
	}

	ev, err := models.NewExplanationEvidenceFromItem(scope, item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ev.StableID != "stable-proof-42" {
		t.Errorf("expected stable-proof-42, got %s", ev.StableID)
	}
	if ev.Label != "E3" {
		t.Errorf("expected E3, got %s", ev.Label)
	}
	if ev.SymbolID != "sym-auth" || ev.SymbolName != "VerifyToken" {
		t.Errorf("symbol provenance mismatch: %s / %s", ev.SymbolID, ev.SymbolName)
	}
	if ev.EdgeKind != models.EdgeKindCalls {
		t.Errorf("expected EDGE_CALLS, got %s", ev.EdgeKind)
	}
	if ev.FlowPathID != "path-77" {
		t.Errorf("expected path-77, got %s", ev.FlowPathID)
	}
}

func TestC6_1_DeterministicEvidenceReferenceIdentity(t *testing.T) {
	ref := models.EvidenceReference{
		Label:    "E1",
		StableID: "hash-stable-12345",
	}

	claim := models.ExplanationClaim{
		ID:                 "claim-1",
		Text:               "Authentication uses token verification.",
		EvidenceReferences: []models.EvidenceReference{ref},
		GroundingStatus:    models.ClaimGrounded,
		IsValid:            true,
	}

	if len(claim.EvidenceReferences) != 1 {
		t.Fatalf("expected 1 evidence reference, got %d", len(claim.EvidenceReferences))
	}

	// Prove Label vs StableID distinction is unambiguous
	gotRef := claim.EvidenceReferences[0]
	if gotRef.Label != "E1" {
		t.Errorf("expected Label 'E1', got %s", gotRef.Label)
	}
	if gotRef.StableID != "hash-stable-12345" {
		t.Errorf("expected StableID 'hash-stable-12345', got %s", gotRef.StableID)
	}
}

func TestC6_1_InsufficientEvidenceWithAvailableEvidence(t *testing.T) {
	scope, _ := models.NewRepositoryScope("repo-partial")
	item := &models.EvidenceItem{
		StableID:     "hash-partial-1",
		Label:        "E1",
		RepositoryID: "repo-partial",
		Type:         models.EvidenceTypeSymbol,
		RelativePath: "partial.go",
		Content:      "type Partial struct{}",
	}
	ev, _ := models.NewExplanationEvidenceFromItem(scope, item)

	cit := models.Citation{
		EvidenceID:   "E1",
		StableID:     "hash-partial-1",
		RelativePath: "partial.go",
		IsValid:      true,
	}

	sufficiency := models.EvidenceSufficiencyResult{
		Status:           models.SufficiencyInsufficient,
		TargetFound:      true,
		SourceAvailable:  false,
		TargetResolution: models.TargetAmbiguous,
		Reasons:          []string{"Implementation source code not available"},
	}

	resp, err := models.NewInsufficientEvidenceResponse(
		scope,
		"How is Partial implemented?",
		sufficiency,
		[]*models.ExplanationEvidence{ev},
		[]models.Citation{cit},
	)
	if err != nil {
		t.Fatalf("unexpected error creating response: %v", err)
	}

	if resp.Status != models.ExplanationStatusInsufficientEvidence {
		t.Errorf("expected status INSUFFICIENT_EVIDENCE, got %s", resp.Status)
	}
	if !resp.IsInsufficientEvidence {
		t.Errorf("expected IsInsufficientEvidence = true")
	}
	if len(resp.Evidence) != 1 || resp.Evidence[0].StableID != "hash-partial-1" {
		t.Errorf("expected 1 preserved evidence item, got %d", len(resp.Evidence))
	}
	if len(resp.Citations) != 1 || resp.Citations[0].EvidenceID != "E1" {
		t.Errorf("expected 1 preserved citation, got %d", len(resp.Citations))
	}
	if len(resp.Claims) != 0 {
		t.Errorf("expected 0 claims when evidence is insufficient, got %d", len(resp.Claims))
	}
	if err := resp.Validate(); err != nil {
		t.Errorf("expected valid response state, got err: %v", err)
	}
}

func TestC6_1_InsufficientEvidenceWithZeroEvidence(t *testing.T) {
	scope, _ := models.NewRepositoryScope("repo-empty")
	sufficiency := models.EvidenceSufficiencyResult{
		Status:           models.SufficiencyInsufficient,
		TargetFound:      false,
		SourceAvailable:  false,
		TargetResolution: models.TargetNotFound,
		Reasons:          []string{"No matching symbols found for query"},
	}

	resp, err := models.NewInsufficientEvidenceResponse(scope, "What is the secret key?", sufficiency, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error creating insufficient response: %v", err)
	}

	if resp.Status != models.ExplanationStatusInsufficientEvidence {
		t.Errorf("expected INSUFFICIENT_EVIDENCE status, got %s", resp.Status)
	}
	if !resp.IsInsufficientEvidence {
		t.Errorf("expected IsInsufficientEvidence to be true")
	}
	if len(resp.Evidence) != 0 {
		t.Errorf("expected 0 evidence items, got %d", len(resp.Evidence))
	}
	if len(resp.Citations) != 0 {
		t.Errorf("expected 0 citations, got %d", len(resp.Citations))
	}
	if len(resp.Claims) != 0 {
		t.Errorf("expected 0 claims, got %d", len(resp.Claims))
	}
	if err := resp.Validate(); err != nil {
		t.Errorf("expected valid response state, got err: %v", err)
	}
}

func TestC6_1_ContradictoryStateRejection(t *testing.T) {
	scope, _ := models.NewRepositoryScope("repo-test")

	// Case 1: Status = SUCCESS but IsInsufficientEvidence = true
	resp1 := &models.ExplanationResponse{
		RepositoryID:           scope.RepositoryID,
		Question:               "question",
		Status:                 models.ExplanationStatusSuccess,
		IsInsufficientEvidence: true,
	}
	if err := resp1.Validate(); err == nil {
		t.Errorf("expected validation error for SUCCESS with IsInsufficientEvidence=true, got nil")
	}

	// Case 2: Status = INSUFFICIENT_EVIDENCE but Grounding.Status = GROUNDING_PASSED
	resp2 := &models.ExplanationResponse{
		RepositoryID:           scope.RepositoryID,
		Question:               "question",
		Status:                 models.ExplanationStatusInsufficientEvidence,
		IsInsufficientEvidence: true,
		Grounding:              models.GroundingMetadata{Status: models.GroundingPassed},
	}
	if err := resp2.Validate(); err == nil {
		t.Errorf("expected validation error for INSUFFICIENT_EVIDENCE with GroundingStatus PASSED, got nil")
	}

	// Case 3: Status = INSUFFICIENT_EVIDENCE but Sufficiency.Status = SUFFICIENT
	resp3 := &models.ExplanationResponse{
		RepositoryID:           scope.RepositoryID,
		Question:               "question",
		Status:                 models.ExplanationStatusInsufficientEvidence,
		IsInsufficientEvidence: true,
		Sufficiency:            models.EvidenceSufficiencyResult{Status: models.SufficiencySufficient},
	}
	if err := resp3.Validate(); err == nil {
		t.Errorf("expected validation error for INSUFFICIENT_EVIDENCE with SufficiencyStatus SUFFICIENT, got nil")
	}

	// Case 4: Status = INSUFFICIENT_EVIDENCE containing claims
	resp4 := &models.ExplanationResponse{
		RepositoryID:           scope.RepositoryID,
		Question:               "question",
		Status:                 models.ExplanationStatusInsufficientEvidence,
		IsInsufficientEvidence: true,
		Claims:                 []models.ExplanationClaim{{ID: "c1", Text: "invalid claim"}},
	}
	if err := resp4.Validate(); err == nil {
		t.Errorf("expected validation error for INSUFFICIENT_EVIDENCE containing claims, got nil")
	}
}

func TestC6_1_DeterministicSerializationAndOrder(t *testing.T) {
	meta := models.GroundingMetadata{
		EvidenceCount:            2,
		CitedEvidenceCount:       2,
		CitationValidationStatus: "VALIDATION_PASSED",
		TotalClaims:              1,
		GroundedClaims:           1,
		UnsupportedClaimCount:    0,
		Status:                   models.GroundingPassed,
		Sufficiency:              models.SufficiencySufficient,
	}

	data1, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	data2, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	if string(data1) != string(data2) {
		t.Errorf("serialization not deterministic:\n1: %s\n2: %s", string(data1), string(data2))
	}
}

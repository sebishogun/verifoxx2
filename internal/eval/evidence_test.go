package eval_test

import (
	"testing"

	"github.com/sebishogun/verifoxx2/internal/ast"
	policycompile "github.com/sebishogun/verifoxx2/internal/compile"
	"github.com/sebishogun/verifoxx2/internal/eval"
	"github.com/sebishogun/verifoxx2/internal/input"
	"github.com/sebishogun/verifoxx2/internal/program"
	"github.com/sebishogun/verifoxx2/internal/schema"
)

func evidenceProgram(t *testing.T) program.Program {
	t.Helper()
	approve := ast.Resolution{
		Satisfied: ast.OutcomeApprove, False: ast.OutcomeApprove,
		Missing: ast.OutcomeApprove, Invalid: ast.OutcomeApprove,
		Stale: ast.OutcomeApprove, Unclear: ast.OutcomeApprove,
		Unverifiable: ast.OutcomeApprove, Conflict: ast.OutcomeApprove,
	}
	source := ast.Policy{
		Name: "evidence", Version: "1",
		Requirements: []ast.Requirement{{
			ID:            "R1",
			Applicability: ast.Expression{Op: ast.OperatorEqual, Field: "dataset", Value: "protected_dataset"},
			Clauses: []ast.Clause{{
				ID: "C1",
				Assertion: ast.Expression{Op: ast.OperatorEvidenceMatches, Evidence: ast.EvidencePredicate{
					Kind: "approval_record", Status: "valid", Timing: "before_execution",
					Reviewer: "designated_reviewer", TimestampState: "current", Subject: "protected_dataset",
					AttestationState: "valid", Scope: "trusted_internal_only", AdjustmentType: "above_standard_limit",
				}},
				Resolution: approve,
			}},
		}},
	}
	compiled, diagnostics := policycompile.Compile(source)
	if len(diagnostics) != 0 {
		t.Fatalf("Compile() diagnostics = %+v", diagnostics)
	}
	return compiled
}

func TestEvidenceMatchesReducesAttachedEvidenceStates(t *testing.T) {
	compiled := evidenceProgram(t)
	valid := input.Evidence{
		Type: "approval_record", Status: "valid", Timing: "before_execution",
		Reviewer: "designated_reviewer", TimestampState: "current", Subject: "protected_dataset",
		AttestationState: "valid", Scope: "trusted_internal_only", AdjustmentType: "above_standard_limit",
	}
	evidence := []input.Evidence{
		{ID: "WRONG", Type: "approval_record", Status: "pending", Timing: "before_execution", Reviewer: "designated_reviewer", TimestampState: "current"},
		{ID: "REVOKED", Type: "approval_record", Status: "revoked", Timing: "before_execution", Reviewer: "designated_reviewer", TimestampState: "current"},
		{ID: "STALE", Type: "approval_record", Status: "valid", Timing: "before_execution", Reviewer: "designated_reviewer", TimestampState: "stale"},
		{ID: "UNCLEAR", Type: "approval_record", Status: "valid", Timing: "before_execution", TimestampState: "current"},
		{ID: "CONFLICT", Type: "approval_record", Status: "conflicting", TimestampState: "conflicting"},
		{ID: "IRRELEVANT", Type: "usage_limit_adjustment", Status: "conflicting"},
	}
	valid.ID = "VALID"
	evidence = append(evidence, valid)
	for id, edit := range map[string]func(*input.Evidence){
		"EXPIRED":           func(item *input.Evidence) { item.Status = "expired" },
		"WRONG_TIMING":      func(item *input.Evidence) { item.Timing = "after_execution" },
		"WRONG_REVIEWER":    func(item *input.Evidence) { item.Reviewer = "other_reviewer" },
		"WRONG_SUBJECT":     func(item *input.Evidence) { item.Subject = "other_dataset" },
		"WRONG_ATTESTATION": func(item *input.Evidence) { item.AttestationState = "invalid" },
		"WRONG_SCOPE":       func(item *input.Evidence) { item.Scope = "external" },
		"WRONG_ADJUSTMENT":  func(item *input.Evidence) { item.AdjustmentType = "standard" },
	} {
		item := valid
		item.ID = id
		edit(&item)
		evidence = append(evidence, item)
	}
	requests := []input.Request{
		{ID: "NO_RECORD", Requester: "x", TrustLevel: "external", Action: "aggregate_analysis", OutputKind: "aggregate_counts", Dataset: "protected_dataset", Environment: "local_approved_env", UsageLimit: "standard"},
		{ID: "MISSING_REF", Requester: "x", TrustLevel: "external", Action: "aggregate_analysis", OutputKind: "aggregate_counts", Dataset: "protected_dataset", Environment: "local_approved_env", UsageLimit: "standard", EvidenceIDs: []string{"GHOST"}},
		{ID: "SATISFIED", Requester: "x", TrustLevel: "external", Action: "aggregate_analysis", OutputKind: "aggregate_counts", Dataset: "protected_dataset", Environment: "local_approved_env", UsageLimit: "standard", EvidenceIDs: []string{"VALID"}},
		{ID: "WRONG", Requester: "x", TrustLevel: "external", Action: "aggregate_analysis", OutputKind: "aggregate_counts", Dataset: "protected_dataset", Environment: "local_approved_env", UsageLimit: "standard", EvidenceIDs: []string{"WRONG"}},
		{ID: "REVOKED", Requester: "x", TrustLevel: "external", Action: "aggregate_analysis", OutputKind: "aggregate_counts", Dataset: "protected_dataset", Environment: "local_approved_env", UsageLimit: "standard", EvidenceIDs: []string{"REVOKED"}},
		{ID: "STALE", Requester: "x", TrustLevel: "external", Action: "aggregate_analysis", OutputKind: "aggregate_counts", Dataset: "protected_dataset", Environment: "local_approved_env", UsageLimit: "standard", EvidenceIDs: []string{"STALE"}},
		{ID: "UNCLEAR", Requester: "x", TrustLevel: "external", Action: "aggregate_analysis", OutputKind: "aggregate_counts", Dataset: "protected_dataset", Environment: "local_approved_env", UsageLimit: "standard", EvidenceIDs: []string{"UNCLEAR"}},
		{ID: "CONFLICT", Requester: "x", TrustLevel: "external", Action: "aggregate_analysis", OutputKind: "aggregate_counts", Dataset: "protected_dataset", Environment: "local_approved_env", UsageLimit: "standard", EvidenceIDs: []string{"CONFLICT"}},
		{ID: "VALID_STALE", Requester: "x", TrustLevel: "external", Action: "aggregate_analysis", OutputKind: "aggregate_counts", Dataset: "protected_dataset", Environment: "local_approved_env", UsageLimit: "standard", EvidenceIDs: []string{"VALID", "STALE"}},
		{ID: "VALID_REVOKED", Requester: "x", TrustLevel: "external", Action: "aggregate_analysis", OutputKind: "aggregate_counts", Dataset: "protected_dataset", Environment: "local_approved_env", UsageLimit: "standard", EvidenceIDs: []string{"VALID", "REVOKED"}},
		{ID: "VALID_WRONG", Requester: "x", TrustLevel: "external", Action: "aggregate_analysis", OutputKind: "aggregate_counts", Dataset: "protected_dataset", Environment: "local_approved_env", UsageLimit: "standard", EvidenceIDs: []string{"VALID", "WRONG"}},
		{ID: "IRRELEVANT_CONFLICT", Requester: "x", TrustLevel: "external", Action: "aggregate_analysis", OutputKind: "aggregate_counts", Dataset: "protected_dataset", Environment: "local_approved_env", UsageLimit: "standard", EvidenceIDs: []string{"VALID", "IRRELEVANT"}},
	}
	for _, id := range []string{"EXPIRED", "WRONG_TIMING", "WRONG_REVIEWER", "WRONG_SUBJECT", "WRONG_ATTESTATION", "WRONG_SCOPE", "WRONG_ADJUSTMENT"} {
		requests = append(requests, input.Request{
			ID: id, Requester: "x", TrustLevel: "external", Action: "aggregate_analysis",
			OutputKind: "aggregate_counts", Dataset: "protected_dataset", Environment: "local_approved_env",
			UsageLimit: "standard", EvidenceIDs: []string{id},
		})
	}
	want := []schema.ReasonID{
		schema.ReasonMissing, schema.ReasonMissing, schema.ReasonSatisfied,
		schema.ReasonInvalidEvidence, schema.ReasonInvalidEvidence, schema.ReasonStale,
		schema.ReasonUnclear, schema.ReasonConflict, schema.ReasonStale,
		schema.ReasonInvalidEvidence, schema.ReasonSatisfied, schema.ReasonSatisfied,
	}
	want = append(want,
		schema.ReasonInvalidEvidence,
		schema.ReasonInvalidEvidence,
		schema.ReasonInvalidEvidence,
		schema.ReasonInvalidEvidence,
		schema.ReasonInvalidEvidence,
		schema.ReasonInvalidEvidence,
		schema.ReasonInvalidEvidence,
	)

	var batch eval.Batch
	if err := eval.NewBuilder(&compiled, eval.DefaultLimits()).BuildInto(&batch, requests, evidence); err != nil {
		t.Fatal(err)
	}
	var context eval.Context
	context.Ensure(compiled, batch.Rows)
	if err := eval.EvaluateInstructions(compiled, batch, &context); err != nil {
		t.Fatalf("EvaluateInstructions() error = %v", err)
	}
	instruction := schema.InstructionID(2)
	for row, reason := range want {
		if !planeBit(context.ReasonPlane(reason, instruction), row) {
			t.Fatalf("row %d reason plane does not contain %d", row, reason)
		}
	}
	if !planeBit(context.PositivePlane(instruction), 2) || planeBit(context.NegativePlane(instruction), 2) {
		t.Fatal("satisfied evidence truth is not positive-only")
	}
	if !planeBit(context.PositivePlane(instruction), 7) || !planeBit(context.NegativePlane(instruction), 7) {
		t.Fatal("conflicting evidence truth is not conflict")
	}
}

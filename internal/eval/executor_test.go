package eval_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/sebishogun/verifoxx2/internal/adapters/jsonio"
	"github.com/sebishogun/verifoxx2/internal/ast"
	policycompile "github.com/sebishogun/verifoxx2/internal/compile"
	"github.com/sebishogun/verifoxx2/internal/eval"
	"github.com/sebishogun/verifoxx2/internal/input"
	"github.com/sebishogun/verifoxx2/internal/result"
	"github.com/sebishogun/verifoxx2/internal/schema"
)

func compiledFixture(t *testing.T) (eval.Evaluator, eval.Batch, []input.Request) {
	t.Helper()
	source, err := jsonio.LoadPolicy("../../policies/policy.json")
	if err != nil {
		t.Fatal(err)
	}
	compiled, diagnostics := policycompile.Compile(source)
	if len(diagnostics) != 0 {
		t.Fatalf("Compile() diagnostics = %+v", diagnostics)
	}
	requests, err := jsonio.LoadRequests("../../fixtures/requests.json")
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := jsonio.LoadEvidence("../../fixtures/evidence.json")
	if err != nil {
		t.Fatal(err)
	}
	var batch eval.Batch
	if err := eval.NewBuilder(&compiled, eval.DefaultLimits()).BuildInto(&batch, requests, evidence); err != nil {
		t.Fatal(err)
	}
	return eval.NewEvaluator(&compiled), batch, requests
}

func TestEvaluatorResolvesSuppliedPackIntoNumericResults(t *testing.T) {
	evaluator, batch, _ := compiledFixture(t)
	var context eval.Context
	context.Ensure(evaluator.Program(), batch.Rows)
	var got result.Batch
	got.Ensure(batch.Rows, int(batch.Rows)*3, len(batch.EvidenceRefs), int(batch.Rows)*2)
	if err := evaluator.EvaluateInto(&context, batch, &got); err != nil {
		t.Fatalf("EvaluateInto() error = %v", err)
	}
	wantOutcomes := []schema.OutcomeID{
		schema.OutcomeApprove,
		schema.OutcomeReject,
		schema.OutcomeRevise,
		schema.OutcomeEscalate,
		schema.OutcomeEscalate,
	}
	if !reflect.DeepEqual(got.OutcomeIDs, wantOutcomes) {
		t.Fatalf("OutcomeIDs = %v, want %v", got.OutcomeIDs, wantOutcomes)
	}
	wantRequirements := [][]schema.RequirementID{{1, 2}, {1, 2}, {2, 3}, {1, 2}, {2, 3}}
	for row, want := range wantRequirements {
		start, end := got.RequirementOffsets[row], got.RequirementOffsets[row+1]
		if actual := got.RequirementIDs[start:end]; !reflect.DeepEqual(actual, want) {
			t.Fatalf("row %d requirements = %v, want %v", row, actual, want)
		}
	}
	if got.DriverKinds[1] != result.DriverClause || got.DriverReasonIDs[1] != schema.ReasonFalse {
		t.Fatalf("R2 driver = kind %d clause %d reason %d", got.DriverKinds[1], got.DriverClauseIDs[1], got.DriverReasonIDs[1])
	}
	if got.DriverKinds[2] != result.DriverClause || got.DriverReasonIDs[2] != schema.ReasonMissing || len(got.RemediationIDs) != 1 {
		t.Fatalf("R3 driver/remediation = kind %d reason %d remediation %v", got.DriverKinds[2], got.DriverReasonIDs[2], got.RemediationIDs)
	}
}

func TestEvaluatorRejectsInsufficientResultCapacityWithoutMutation(t *testing.T) {
	evaluator, batch, _ := compiledFixture(t)
	var context eval.Context
	context.Ensure(evaluator.Program(), batch.Rows)
	dst := result.Batch{
		OutcomeIDs:     []schema.OutcomeID{99},
		DriverKinds:    []result.DriverKind{99},
		RequirementIDs: []schema.RequirementID{99},
	}
	want := result.Batch{
		OutcomeIDs:     append([]schema.OutcomeID(nil), dst.OutcomeIDs...),
		DriverKinds:    append([]result.DriverKind(nil), dst.DriverKinds...),
		RequirementIDs: append([]schema.RequirementID(nil), dst.RequirementIDs...),
	}
	err := evaluator.EvaluateInto(&context, batch, &dst)
	var capacityError *result.CapacityError
	if !errors.As(err, &capacityError) {
		t.Fatalf("EvaluateInto() error = %v, want CapacityError", err)
	}
	if !reflect.DeepEqual(dst, want) {
		t.Fatalf("destination mutated on capacity error:\ngot  %+v\nwant %+v", dst, want)
	}
}

func TestEvaluatorChecksEveryResultColumnBeforeMutation(t *testing.T) {
	evaluator, batch, _ := compiledFixture(t)
	compiled := evaluator.Program()
	var context eval.Context
	context.Ensure(compiled, batch.Rows)
	tests := []struct {
		column  string
		disable func(*result.Batch)
	}{
		{"OutcomeIDs", func(dst *result.Batch) { dst.OutcomeIDs = nil }},
		{"DriverKinds", func(dst *result.Batch) { dst.DriverKinds = nil }},
		{"DriverRequirementIDs", func(dst *result.Batch) { dst.DriverRequirementIDs = nil }},
		{"DriverClauseIDs", func(dst *result.Batch) { dst.DriverClauseIDs = nil }},
		{"DriverReasonIDs", func(dst *result.Batch) { dst.DriverReasonIDs = nil }},
		{"DriverFieldIDs", func(dst *result.Batch) { dst.DriverFieldIDs = nil }},
		{"DriverEvidenceEdgeIDs", func(dst *result.Batch) { dst.DriverEvidenceEdgeIDs = nil }},
		{"IssueIDs", func(dst *result.Batch) { dst.IssueIDs = nil }},
		{"RequirementOffsets", func(dst *result.Batch) { dst.RequirementOffsets = nil }},
		{"EvidenceOffsets", func(dst *result.Batch) { dst.EvidenceOffsets = nil }},
		{"RemediationOffsets", func(dst *result.Batch) { dst.RemediationOffsets = nil }},
		{"RequirementIDs", func(dst *result.Batch) { dst.RequirementIDs = nil }},
		{"EvidenceRefs", func(dst *result.Batch) { dst.EvidenceRefs = nil }},
		{"RemediationIDs", func(dst *result.Batch) { dst.RemediationIDs = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.column, func(t *testing.T) {
			var dst result.Batch
			dst.Ensure(batch.Rows, int(batch.Rows)*len(compiled.RequirementSymbols), len(batch.EvidenceRefs), int(batch.Rows)*len(compiled.Remediations))
			poisonResultBatch(&dst)
			tt.disable(&dst)
			want := cloneResultBatch(dst)
			err := evaluator.EvaluateInto(&context, batch, &dst)
			var capacityError *result.CapacityError
			if !errors.As(err, &capacityError) || capacityError.Column != tt.column {
				t.Fatalf("EvaluateInto() error = %v, want CapacityError for %s", err, tt.column)
			}
			if !reflect.DeepEqual(dst, want) {
				t.Fatalf("destination mutated after %s capacity error", tt.column)
			}
		})
	}
}

func TestEvaluatorRejectOutranksEarlierSemanticEscalation(t *testing.T) {
	source, err := jsonio.LoadPolicy("../../policies/policy.json")
	if err != nil {
		t.Fatal(err)
	}
	compiled, diagnostics := policycompile.Compile(source)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	request := input.Request{
		ID: "MIXED", Requester: "partner", TrustLevel: "external", Action: "mystery",
		OutputKind: "individual_records", Dataset: "protected_dataset", Environment: "local_approved_env",
		UsageLimit: "standard",
	}
	var batch eval.Batch
	if err := eval.NewBuilder(&compiled, eval.DefaultLimits()).BuildInto(&batch, []input.Request{request}, nil); err != nil {
		t.Fatal(err)
	}
	var context eval.Context
	context.Ensure(compiled, 1)
	var results result.Batch
	results.Ensure(1, 3, 0, 2)
	if err := eval.NewEvaluator(&compiled).EvaluateInto(&context, batch, &results); err != nil {
		t.Fatal(err)
	}
	if results.OutcomeIDs[0] != schema.OutcomeReject || results.DriverKinds[0] != result.DriverClause {
		t.Fatalf("mixed semantic/disclosure outcome = %d driver %d, want Reject clause", results.OutcomeIDs[0], results.DriverKinds[0])
	}
}

func TestEvaluatorNoApplicabilityAndEqualRankKeepDeterministicDriver(t *testing.T) {
	resolution := ast.Resolution{
		Satisfied: ast.OutcomeApprove, False: ast.OutcomeEscalate,
		Missing: ast.OutcomeApprove, Invalid: ast.OutcomeApprove,
		Stale: ast.OutcomeApprove, Unclear: ast.OutcomeApprove,
		Unverifiable: ast.OutcomeApprove, Conflict: ast.OutcomeApprove,
	}
	source := ast.Policy{
		Name: "resolution", Version: "1",
		Requirements: []ast.Requirement{{
			ID: "R1", Applicability: ast.Expression{Op: ast.OperatorEqual, Field: "requester", Value: "specific_team"},
			Clauses: []ast.Clause{
				{ID: "C1", Assertion: ast.Expression{Op: ast.OperatorEqual, Field: "action", Value: "row_level_export"}, Resolution: resolution, Explanations: ast.Explanations{False: "first"}},
				{ID: "C2", Assertion: ast.Expression{Op: ast.OperatorEqual, Field: "output_kind", Value: "individual_records"}, Resolution: resolution, Explanations: ast.Explanations{False: "second"}},
			},
		}},
	}
	compiled, diagnostics := policycompile.Compile(source)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	requests := []input.Request{
		{ID: "NONE", Requester: "other_team", TrustLevel: "external", Action: "aggregate_analysis", OutputKind: "aggregate_counts", Dataset: "protected_dataset", Environment: "local_approved_env", UsageLimit: "standard"},
		{ID: "TIE", Requester: "specific_team", TrustLevel: "external", Action: "aggregate_analysis", OutputKind: "aggregate_counts", Dataset: "protected_dataset", Environment: "local_approved_env", UsageLimit: "standard"},
	}
	var batch eval.Batch
	if err := eval.NewBuilder(&compiled, eval.DefaultLimits()).BuildInto(&batch, requests, nil); err != nil {
		t.Fatal(err)
	}
	var context eval.Context
	context.Ensure(compiled, batch.Rows)
	var numeric result.Batch
	numeric.Ensure(batch.Rows, len(requests), 0, 0)
	if err := eval.NewEvaluator(&compiled).EvaluateInto(&context, batch, &numeric); err != nil {
		t.Fatal(err)
	}
	if numeric.OutcomeIDs[0] != schema.OutcomeEscalate || numeric.DriverKinds[0] != result.DriverNoApplicableRequirement {
		t.Fatalf("no-applicability result = outcome %d driver %d", numeric.OutcomeIDs[0], numeric.DriverKinds[0])
	}
	if numeric.OutcomeIDs[1] != schema.OutcomeEscalate || numeric.DriverKinds[1] != result.DriverClause || numeric.DriverClauseIDs[1] != 1 {
		t.Fatalf("equal-rank result = outcome %d driver %d clause %d, want first clause", numeric.OutcomeIDs[1], numeric.DriverKinds[1], numeric.DriverClauseIDs[1])
	}
}

func TestEvaluatorSupportsConcurrentIndependentContexts(t *testing.T) {
	evaluator, batch, _ := compiledFixture(t)
	compiled := evaluator.Program()
	want := []schema.OutcomeID{
		schema.OutcomeApprove,
		schema.OutcomeReject,
		schema.OutcomeRevise,
		schema.OutcomeEscalate,
		schema.OutcomeEscalate,
	}
	const workers = 8
	start := make(chan struct{})
	errorsByWorker := make(chan error, workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			<-start
			var context eval.Context
			context.Ensure(compiled, batch.Rows)
			var numeric result.Batch
			numeric.Ensure(batch.Rows, int(batch.Rows)*len(compiled.RequirementSymbols), len(batch.EvidenceRefs), int(batch.Rows)*len(compiled.Remediations))
			for iteration := 0; iteration < 100; iteration++ {
				if err := evaluator.EvaluateInto(&context, batch, &numeric); err != nil {
					errorsByWorker <- err
					return
				}
				if !reflect.DeepEqual(numeric.OutcomeIDs, want) {
					errorsByWorker <- fmt.Errorf("outcomes = %v, want %v", numeric.OutcomeIDs, want)
					return
				}
			}
			errorsByWorker <- nil
		}()
	}
	close(start)
	for range workers {
		if err := <-errorsByWorker; err != nil {
			t.Fatal(err)
		}
	}
}

func poisonResultBatch(dst *result.Batch) {
	for i := range dst.OutcomeIDs {
		dst.OutcomeIDs[i] = 99
		dst.DriverKinds[i] = 99
		dst.DriverRequirementIDs[i] = 99
		dst.DriverClauseIDs[i] = 99
		dst.DriverReasonIDs[i] = 99
		dst.DriverFieldIDs[i] = 99
		dst.DriverEvidenceEdgeIDs[i] = 99
		dst.IssueIDs[i] = 99
	}
	for i := range dst.RequirementOffsets {
		dst.RequirementOffsets[i] = 99
		dst.EvidenceOffsets[i] = 99
		dst.RemediationOffsets[i] = 99
	}
	dst.RequirementIDs = append(dst.RequirementIDs, 99)
	dst.EvidenceRefs = append(dst.EvidenceRefs, 99)
	dst.RemediationIDs = append(dst.RemediationIDs, 99)
}

func cloneResultBatch(source result.Batch) result.Batch {
	clone := source
	clone.OutcomeIDs = append([]schema.OutcomeID(nil), source.OutcomeIDs...)
	clone.DriverKinds = append([]result.DriverKind(nil), source.DriverKinds...)
	clone.DriverRequirementIDs = append([]schema.RequirementID(nil), source.DriverRequirementIDs...)
	clone.DriverClauseIDs = append([]schema.ClauseID(nil), source.DriverClauseIDs...)
	clone.DriverReasonIDs = append([]schema.ReasonID(nil), source.DriverReasonIDs...)
	clone.DriverFieldIDs = append([]schema.FieldID(nil), source.DriverFieldIDs...)
	clone.DriverEvidenceEdgeIDs = append([]uint32(nil), source.DriverEvidenceEdgeIDs...)
	clone.IssueIDs = append([]schema.ReasonID(nil), source.IssueIDs...)
	clone.RequirementOffsets = append([]uint32(nil), source.RequirementOffsets...)
	clone.RequirementIDs = append([]schema.RequirementID(nil), source.RequirementIDs...)
	clone.EvidenceOffsets = append([]uint32(nil), source.EvidenceOffsets...)
	clone.EvidenceRefs = append([]uint32(nil), source.EvidenceRefs...)
	clone.RemediationOffsets = append([]uint32(nil), source.RemediationOffsets...)
	clone.RemediationIDs = append([]schema.RemediationID(nil), source.RemediationIDs...)
	return clone
}

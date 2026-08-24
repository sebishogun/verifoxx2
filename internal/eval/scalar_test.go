package eval_test

import (
	"fmt"
	"testing"

	"github.com/sebishogun/verifoxx2/internal/ast"
	policycompile "github.com/sebishogun/verifoxx2/internal/compile"
	"github.com/sebishogun/verifoxx2/internal/eval"
	"github.com/sebishogun/verifoxx2/internal/input"
	"github.com/sebishogun/verifoxx2/internal/program"
	"github.com/sebishogun/verifoxx2/internal/schema"
)

func scalarProgram(t *testing.T) program.Program {
	t.Helper()
	approve := ast.Resolution{
		Satisfied: ast.OutcomeApprove, False: ast.OutcomeApprove,
		Missing: ast.OutcomeApprove, Invalid: ast.OutcomeApprove,
		Stale: ast.OutcomeApprove, Unclear: ast.OutcomeApprove,
		Unverifiable: ast.OutcomeApprove, Conflict: ast.OutcomeApprove,
	}
	source := ast.Policy{
		Name: "scalar", Version: "1",
		Requirements: []ast.Requirement{{
			ID:            "R1",
			Applicability: ast.Expression{Op: ast.OperatorEqual, Field: "dataset", Value: "protected_dataset"},
			Clauses: []ast.Clause{{
				ID: "C1",
				Assertion: ast.Expression{Op: ast.OperatorAll, Children: []ast.Expression{
					{Op: ast.OperatorEqual, Field: "action", Value: "aggregate_analysis"},
					{Op: ast.OperatorIn, Field: "output_kind", Values: []string{"aggregate_counts"}},
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

func scalarRequest(id, action, output string) input.Request {
	return input.Request{
		ID: id, Requester: "partner", TrustLevel: "external", Action: action,
		OutputKind: output, Dataset: "protected_dataset", Environment: "local_approved_env",
		UsageLimit: "standard",
	}
}

func planeBit(plane []uint64, row int) bool {
	return plane[row/64]&(uint64(1)<<(row&63)) != 0
}

func TestScalarEvaluatesEqualInAndAllTruth(t *testing.T) {
	compiled := scalarProgram(t)
	requests := []input.Request{
		scalarRequest("true", "aggregate_analysis", "aggregate_counts"),
		scalarRequest("false", "row_level_export", "aggregate_counts"),
		scalarRequest("unknown", "mystery", "aggregate_counts"),
	}
	var batch eval.Batch
	if err := eval.NewBuilder(&compiled, eval.DefaultLimits()).BuildInto(&batch, requests, nil); err != nil {
		t.Fatal(err)
	}
	var context eval.Context
	context.Ensure(compiled, batch.Rows)
	if err := eval.EvaluateInstructions(compiled, batch, &context); err != nil {
		t.Fatalf("EvaluateInstructions() error = %v", err)
	}

	actionEqual := schema.InstructionID(2)
	assertionAll := schema.InstructionID(4)
	for row, want := range []struct{ positive, negative bool }{{true, false}, {false, true}, {false, false}} {
		if got := planeBit(context.PositivePlane(actionEqual), row); got != want.positive {
			t.Fatalf("action positive row %d = %v, want %v", row, got, want.positive)
		}
		if got := planeBit(context.NegativePlane(actionEqual), row); got != want.negative {
			t.Fatalf("action negative row %d = %v, want %v", row, got, want.negative)
		}
	}
	if !planeBit(context.ReasonPlane(schema.ReasonSatisfied, assertionAll), 0) {
		t.Fatal("all row 0 is not satisfied")
	}
	if !planeBit(context.ReasonPlane(schema.ReasonFalse, assertionAll), 1) {
		t.Fatal("all row 1 is not false")
	}
	if !planeBit(context.ReasonPlane(schema.ReasonUnclear, assertionAll), 2) {
		t.Fatal("all row 2 is not unclear")
	}
}

func TestScalarClearsTailBitsAndPoisonAcrossBoundaries(t *testing.T) {
	compiled := scalarProgram(t)
	for _, rows := range []int{0, 1, 5, 63, 64, 65} {
		t.Run(fmt.Sprintf("rows_%d", rows), func(t *testing.T) {
			requests := make([]input.Request, rows)
			for i := range requests {
				requests[i] = scalarRequest(fmt.Sprintf("R%d", i), "aggregate_analysis", "aggregate_counts")
			}
			var batch eval.Batch
			if err := eval.NewBuilder(&compiled, eval.DefaultLimits()).BuildInto(&batch, requests, nil); err != nil {
				t.Fatal(err)
			}
			var context eval.Context
			context.Ensure(compiled, batch.Rows)
			for i := range context.Positive {
				context.Positive[i] = ^uint64(0)
				context.Negative[i] = ^uint64(0)
			}
			for i := range context.Reasons {
				context.Reasons[i] = ^uint64(0)
			}
			if err := eval.EvaluateInstructions(compiled, batch, &context); err != nil {
				t.Fatal(err)
			}
			if rows == 0 {
				return
			}
			plane := context.PositivePlane(schema.InstructionID(4))
			if !planeBit(plane, rows-1) {
				t.Fatalf("last active row %d is not true", rows-1)
			}
			if tail := rows & 63; tail != 0 && plane[len(plane)-1]&^(uint64(1)<<tail-1) != 0 {
				t.Fatalf("tail bits retained poison: %064b", plane[len(plane)-1])
			}
		})
	}
}

func TestScalarRejectsInsufficientContext(t *testing.T) {
	compiled := scalarProgram(t)
	var batch eval.Batch
	if err := eval.NewBuilder(&compiled, eval.DefaultLimits()).BuildInto(&batch, []input.Request{scalarRequest("R1", "aggregate_analysis", "aggregate_counts")}, nil); err != nil {
		t.Fatal(err)
	}
	var context eval.Context
	if err := eval.EvaluateInstructions(compiled, batch, &context); err == nil {
		t.Fatal("EvaluateInstructions() accepted an unprepared context")
	}
}

package compile

import (
	"reflect"
	"strings"
	"testing"

	"github.com/sebishogun/verifoxx2/internal/ast"
	"github.com/sebishogun/verifoxx2/internal/program"
	"github.com/sebishogun/verifoxx2/internal/schema"
)

func compileTestResolution() ast.Resolution {
	return ast.Resolution{
		Satisfied:    ast.OutcomeApprove,
		False:        ast.OutcomeReject,
		Missing:      ast.OutcomeEscalate,
		Invalid:      ast.OutcomeEscalate,
		Stale:        ast.OutcomeEscalate,
		Unclear:      ast.OutcomeEscalate,
		Unverifiable: ast.OutcomeEscalate,
		Conflict:     ast.OutcomeEscalate,
	}
}

func compileTestExplanations() ast.Explanations {
	return ast.Explanations{
		False:        "rejected",
		Missing:      "missing",
		Invalid:      "invalid",
		Stale:        "stale",
		Unclear:      "unclear",
		Unverifiable: "unverifiable",
		Conflict:     "conflict",
	}
}

func compileTestPolicy() ast.Policy {
	return ast.Policy{
		Name:    "compile-test",
		Version: "1",
		Requirements: []ast.Requirement{{
			ID: "R1",
			Applicability: ast.Expression{
				Op: ast.OperatorEqual, Field: "dataset", Value: "protected_dataset",
			},
			Clauses: []ast.Clause{{
				ID: "R1_C1",
				Assertion: ast.Expression{
					Op: ast.OperatorAll,
					Children: []ast.Expression{
						{Op: ast.OperatorEqual, Field: "action", Value: "aggregate_analysis"},
						{Op: ast.OperatorIn, Field: "output_kind", Values: []string{"aggregate_counts", "individual_records"}},
						{Op: ast.OperatorEvidenceMatches, Evidence: ast.EvidencePredicate{Kind: "approval_record", Status: "valid"}},
					},
				},
				Resolution:   compileTestResolution(),
				Explanations: compileTestExplanations(),
			}},
		}},
	}
}

func TestCompileLowersPostorderAndFreezesSymbols(t *testing.T) {
	got, diagnostics := Compile(compileTestPolicy())
	if len(diagnostics) != 0 {
		t.Fatalf("Compile() diagnostics = %+v", diagnostics)
	}

	wantOpcodes := []program.Opcode{
		program.OpEqual,
		program.OpEqual,
		program.OpIn,
		program.OpEvidenceMatches,
		program.OpAll,
	}
	if !reflect.DeepEqual(got.Opcodes, wantOpcodes) {
		t.Fatalf("opcodes = %v, want %v", got.Opcodes, wantOpcodes)
	}
	if got.RequirementApplicabilityRoots[0] != schema.InstructionID(1) {
		t.Fatalf("applicability root = %d, want 1", got.RequirementApplicabilityRoots[0])
	}
	if got.ClauseAssertionRoots[0] != schema.InstructionID(5) {
		t.Fatalf("assertion root = %d, want 5", got.ClauseAssertionRoots[0])
	}
	if want := []schema.InstructionID{2, 3, 4}; !reflect.DeepEqual(got.Operands, want) {
		t.Fatalf("operands = %v, want %v", got.Operands, want)
	}
	if got.OperandStarts[4] != 0 || got.OperandCounts[4] != 3 {
		t.Fatalf("all operand range = (%d, %d), want (0, 3)", got.OperandStarts[4], got.OperandCounts[4])
	}
	if got.SetCounts[2] != 2 || len(got.SetValues) != 2 {
		t.Fatalf("in set shape = count %d, values %v", got.SetCounts[2], got.SetValues)
	}
	if len(got.EvidenceSpecs) != 1 || got.EvidenceSpecs[0].Kind != schema.EvidenceKindID(1) {
		t.Fatalf("evidence specs = %+v, want one kind 1", got.EvidenceSpecs)
	}
	if got.EvidenceSpecIndexes[3] != 1 {
		t.Fatalf("evidence instruction spec index = %d, want 1", got.EvidenceSpecIndexes[3])
	}
	if len(got.ClauseResolutionOutcomeIDs) != int(schema.ReasonCount) {
		t.Fatalf("resolution entries = %d, want %d", len(got.ClauseResolutionOutcomeIDs), schema.ReasonCount)
	}
	if got.ClauseResolutionOutcomeIDs[0] != schema.OutcomeApprove || got.ClauseResolutionOutcomeIDs[1] != schema.OutcomeReject {
		t.Fatalf("resolution prefix = %v, want Approve, Reject", got.ClauseResolutionOutcomeIDs[:2])
	}

	for _, symbol := range []string{"R1", "R1_C1", "protected_dataset", "aggregate_analysis", "approval_record", "valid"} {
		id, ok := got.LookupSymbol(symbol)
		if !ok || !id.Valid() {
			t.Fatalf("LookupSymbol(%q) = (%d, %v)", symbol, id, ok)
		}
		if got.Symbol(id) != symbol {
			t.Fatalf("Symbol(LookupSymbol(%q)) = %q", symbol, got.Symbol(id))
		}
	}

	again, diagnostics := Compile(compileTestPolicy())
	if len(diagnostics) != 0 || !reflect.DeepEqual(got, again) {
		t.Fatalf("repeated compilation is not deterministic:\nfirst=%+v\nsecond=%+v\ndiagnostics=%+v", got, again, diagnostics)
	}
}

func TestCompileRejectsInvalidSource(t *testing.T) {
	source := compileTestPolicy()
	source.Requirements[0].Clauses[0].Resolution.Conflict = ""

	got, diagnostics := Compile(source)
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics = %+v, want one", diagnostics)
	}
	if !strings.Contains(diagnostics[0].Message, "resolution.conflict") {
		t.Fatalf("diagnostic = %+v, want resolution.conflict", diagnostics[0])
	}
	if len(got.Opcodes) != 0 {
		t.Fatalf("invalid source published program %+v", got)
	}
}

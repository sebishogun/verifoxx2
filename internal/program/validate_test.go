package program_test

import (
	"strings"
	"testing"

	"github.com/sebishogun/verifoxx2/internal/ast"
	policycompile "github.com/sebishogun/verifoxx2/internal/compile"
	"github.com/sebishogun/verifoxx2/internal/program"
	"github.com/sebishogun/verifoxx2/internal/schema"
)

func validProgram(t *testing.T) program.Program {
	t.Helper()
	resolution := ast.Resolution{
		Satisfied: ast.OutcomeApprove, False: ast.OutcomeReject,
		Missing: ast.OutcomeEscalate, Invalid: ast.OutcomeEscalate,
		Stale: ast.OutcomeEscalate, Unclear: ast.OutcomeEscalate,
		Unverifiable: ast.OutcomeEscalate, Conflict: ast.OutcomeEscalate,
	}
	explanations := ast.Explanations{
		False: "false", Missing: "missing", Invalid: "invalid", Stale: "stale",
		Unclear: "unclear", Unverifiable: "unverifiable", Conflict: "conflict",
	}
	source := ast.Policy{
		Name: "test", Version: "1",
		Requirements: []ast.Requirement{{
			ID:            "R1",
			Applicability: ast.Expression{Op: ast.OperatorEqual, Field: "dataset", Value: "protected_dataset"},
			Clauses: []ast.Clause{{
				ID: "C1",
				Assertion: ast.Expression{Op: ast.OperatorAll, Children: []ast.Expression{
					{Op: ast.OperatorIn, Field: "action", Values: []string{"aggregate_analysis"}},
					{Op: ast.OperatorEvidenceMatches, Evidence: ast.EvidencePredicate{Kind: "approval_record", Status: "valid"}},
				}},
				Resolution: resolution, Explanations: explanations,
			}},
		}},
	}
	compiled, diagnostics := policycompile.Compile(source)
	if len(diagnostics) != 0 {
		t.Fatalf("Compile() diagnostics = %+v", diagnostics)
	}
	return compiled
}

func TestProgramValidateAcceptsCompilerOutput(t *testing.T) {
	got := validProgram(t)
	if err := got.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

var materializedSymbol string

func TestProgramSymbolAllocation(t *testing.T) {
	compiled := validProgram(t)
	id, ok := compiled.LookupSymbol("protected_dataset")
	if !ok {
		t.Fatal("compiled program does not contain protected_dataset")
	}
	allocations := testing.AllocsPerRun(100, func() {
		materializedSymbol = compiled.Symbol(id)
	})
	if allocations != 0 {
		t.Fatalf("Symbol allocations = %v, want 0", allocations)
	}
}

func TestProgramValidateRejectsCorruptShape(t *testing.T) {
	tests := []struct {
		name string
		edit func(*program.Program)
		want string
	}{
		{"parallel instruction lengths", func(p *program.Program) { p.Fields = p.Fields[:len(p.Fields)-1] }, "Fields length"},
		{"invalid opcode", func(p *program.Program) { p.Opcodes[0] = program.OpInvalid }, "Opcodes[0]"},
		{"invalid field", func(p *program.Program) { p.Fields[0] = schema.FieldID(99) }, "Fields[0]"},
		{"bad operand range", func(p *program.Program) { p.OperandStarts[len(p.OperandStarts)-1] = ^uint32(0) }, "operand range"},
		{"forward operand", func(p *program.Program) { p.Operands[0] = schema.InstructionID(len(p.Opcodes)) }, "must precede"},
		{"bad set range", func(p *program.Program) { p.SetCounts[1] = ^uint16(0) }, "set range"},
		{"bad evidence spec", func(p *program.Program) { p.EvidenceSpecIndexes[2] = 99 }, "EvidenceSpecIndexes[2]"},
		{"bad applicability root", func(p *program.Program) { p.RequirementApplicabilityRoots[0] = 0 }, "RequirementApplicabilityRoots[0]"},
		{"short resolution", func(p *program.Program) { p.ClauseResolutionOutcomeIDs = p.ClauseResolutionOutcomeIDs[:7] }, "ClauseResolutionOutcomeIDs length"},
		{"missing explanation", func(p *program.Program) { p.ClauseExplanationIDs[1] = 0 }, "ClauseExplanationIDs[1]"},
		{"bad clause csr", func(p *program.Program) { p.RequirementClauseStarts[0] = 99 }, "clause range"},
		{"bad symbol range", func(p *program.Program) { p.SymbolStarts[0] = uint32(len(p.SymbolText) + 1) }, "SymbolStarts[0]"},
		{"bad precedence", func(p *program.Program) { p.OutcomePrecedence[0] = 4 }, "duplicate precedence"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validProgram(t)
			tt.edit(&got)
			err := got.Validate()
			if err == nil {
				t.Fatalf("Validate() succeeded for corrupt program")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %q, want substring %q", err, tt.want)
			}
		})
	}
}

package eval_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/sebishogun/verifoxx2/internal/ast"
	policycompile "github.com/sebishogun/verifoxx2/internal/compile"
	"github.com/sebishogun/verifoxx2/internal/eval"
	"github.com/sebishogun/verifoxx2/internal/input"
	"github.com/sebishogun/verifoxx2/internal/program"
	"github.com/sebishogun/verifoxx2/internal/schema"
)

func builderProgram(t *testing.T) program.Program {
	t.Helper()
	approve := ast.Resolution{
		Satisfied: ast.OutcomeApprove, False: ast.OutcomeApprove,
		Missing: ast.OutcomeApprove, Invalid: ast.OutcomeApprove,
		Stale: ast.OutcomeApprove, Unclear: ast.OutcomeApprove,
		Unverifiable: ast.OutcomeApprove, Conflict: ast.OutcomeApprove,
	}
	source := ast.Policy{
		Name: "batch", Version: "1",
		Requirements: []ast.Requirement{{
			ID:            "R1",
			Applicability: ast.Expression{Op: ast.OperatorEqual, Field: "dataset", Value: "protected_dataset"},
			Clauses: []ast.Clause{{
				ID: "C1",
				Assertion: ast.Expression{Op: ast.OperatorAll, Children: []ast.Expression{
					{Op: ast.OperatorEqual, Field: "action", Value: "aggregate_analysis"},
					{Op: ast.OperatorEvidenceMatches, Evidence: ast.EvidencePredicate{Kind: "approval_record", Status: "valid"}},
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

func TestBuilderBuildsColumnarRequestsAndEvidenceCSR(t *testing.T) {
	compiled := builderProgram(t)
	builder := eval.NewBuilder(&compiled, eval.DefaultLimits())
	requests := []input.Request{
		{ID: "A", Requester: "partner", TrustLevel: "external", Action: "aggregate_analysis", OutputKind: "aggregate_counts", Dataset: "protected_dataset", Environment: "local_approved_env", UsageLimit: "standard", EvidenceIDs: []string{"E1", "E_GHOST"}},
		{ID: "B", Requester: "partner", TrustLevel: "external", Action: "mystery", OutputKind: "aggregate_counts", Dataset: "protected_dataset", Environment: "local_approved_env", UsageLimit: "standard", EvidenceIDs: []string{"E2"}},
	}
	evidence := []input.Evidence{
		{ID: "E1", Type: "approval_record", Status: "valid"},
		{ID: "E2", Type: "other_record", Status: "valid"},
	}

	var batch eval.Batch
	if err := builder.BuildInto(&batch, requests, evidence); err != nil {
		t.Fatalf("BuildInto() error = %v", err)
	}
	if batch.Rows != 2 || batch.Words != 1 {
		t.Fatalf("batch shape = (%d rows, %d words), want (2, 1)", batch.Rows, batch.Words)
	}
	if len(batch.Values) != int(schema.FieldCount-1)*2 {
		t.Fatalf("Values length = %d, want %d", len(batch.Values), int(schema.FieldCount-1)*2)
	}
	dataset, _ := compiled.LookupSymbol("protected_dataset")
	index := (int(schema.FieldDataset) - 1) * int(batch.Rows)
	if batch.Values[index] != dataset || batch.Values[index+1] != dataset {
		t.Fatalf("dataset column = %v, want [%d %d]", batch.Values[index:index+2], dataset, dataset)
	}
	if got, want := batch.EvidenceRefOffsets, []uint32{0, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("EvidenceRefOffsets = %v, want %v", got, want)
	}
	if got, want := batch.EvidenceRefs, []uint32{1, 0, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("EvidenceRefs = %v, want %v", got, want)
	}
	if batch.SemanticIssueMasks[0] != 0 || batch.SemanticIssueMasks[1]&(1<<(schema.FieldAction-1)) == 0 {
		t.Fatalf("SemanticIssueMasks = %08b, want only row 1 action issue", batch.SemanticIssueMasks)
	}
	if batch.EvidenceKinds[0] != 1 || batch.EvidenceKinds[1] != 0 {
		t.Fatalf("EvidenceKinds = %v, want [1 0]", batch.EvidenceKinds)
	}
}

func TestBuilderFullyOverwritesReusedBatch(t *testing.T) {
	compiled := builderProgram(t)
	builder := eval.NewBuilder(&compiled, eval.DefaultLimits())
	requests := []input.Request{{ID: "A", Requester: "x", TrustLevel: "external", Action: "aggregate_analysis", OutputKind: "aggregate_counts", Dataset: "protected_dataset", Environment: "local_approved_env", UsageLimit: "standard", EvidenceIDs: []string{"E1"}}}
	evidence := []input.Evidence{{ID: "E1", Type: "approval_record", Status: "valid"}}

	var batch eval.Batch
	if err := builder.BuildInto(&batch, append(requests, requests...), evidence); err == nil {
		t.Fatal("duplicate request IDs unexpectedly accepted")
	}
	if err := builder.BuildInto(&batch, requests, evidence); err != nil {
		t.Fatal(err)
	}
	for i := range batch.Values {
		batch.Values[i] = ^schema.SymbolID(0)
	}
	for i := range batch.Present {
		batch.Present[i] = ^uint64(0)
		batch.Valid[i] = ^uint64(0)
	}
	for i := range batch.EvidenceRefs {
		batch.EvidenceRefs[i] = ^uint32(0)
	}
	if err := builder.BuildInto(&batch, requests, evidence); err != nil {
		t.Fatal(err)
	}
	if batch.EvidenceRefs[0] != 1 || batch.Present[0]&1 == 0 || batch.Present[0]&^uint64(1) != 0 {
		t.Fatalf("reused batch retained poison: refs=%v present=%064b", batch.EvidenceRefs, batch.Present[0])
	}
}

func TestBuilderWarmPathAllocation(t *testing.T) {
	compiled := builderProgram(t)
	builder := eval.NewBuilder(&compiled, eval.DefaultLimits())
	requests := make([]input.Request, 1024)
	for i := range requests {
		requests[i] = input.Request{
			ID: fmt.Sprintf("REQ-%d", i), Requester: "partner", TrustLevel: "external",
			Action: "aggregate_analysis", OutputKind: "aggregate_counts", Dataset: "protected_dataset",
			Environment: "local_approved_env", UsageLimit: "standard", EvidenceIDs: []string{"E1"},
		}
	}
	evidence := []input.Evidence{{ID: "E1", Type: "approval_record", Status: "valid"}}

	var batch eval.Batch
	if err := builder.BuildInto(&batch, requests, evidence); err != nil {
		t.Fatal(err)
	}
	allocations := testing.AllocsPerRun(100, func() {
		if err := builder.BuildInto(&batch, requests, evidence); err != nil {
			panic(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("warm BuildInto allocations = %v, want 0", allocations)
	}
}

func TestBuilderErrorsDoNotMutateDestination(t *testing.T) {
	compiled := builderProgram(t)
	builder := eval.NewBuilder(&compiled, eval.Limits{MaxRows: 1, MaxEvidence: 1, MaxEdges: 1})
	validRequests := []input.Request{{ID: "A", Requester: "x", TrustLevel: "external", Action: "aggregate_analysis", OutputKind: "aggregate_counts", Dataset: "protected_dataset", Environment: "local_approved_env", UsageLimit: "standard", EvidenceIDs: []string{"E1"}}}
	validEvidence := []input.Evidence{{ID: "E1", Type: "approval_record", Status: "valid"}}
	var batch eval.Batch
	if err := builder.BuildInto(&batch, validRequests, validEvidence); err != nil {
		t.Fatal(err)
	}
	want := batch

	tests := []struct {
		name     string
		requests []input.Request
		evidence []input.Evidence
	}{
		{"row limit", append(append([]input.Request{}, validRequests...), input.Request{ID: "B"}), validEvidence},
		{"evidence limit", validRequests, append(append([]input.Evidence{}, validEvidence...), input.Evidence{ID: "E2"})},
		{"edge limit", []input.Request{{ID: "B", EvidenceIDs: []string{"E1", "E1"}}}, validEvidence},
		{"malformed reference", []input.Request{{ID: "B", EvidenceIDs: []string{"  "}}}, validEvidence},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := builder.BuildInto(&batch, tt.requests, tt.evidence)
			if err == nil {
				t.Fatal("BuildInto() succeeded")
			}
			if !reflect.DeepEqual(batch, want) {
				t.Fatalf("destination changed after %q error", err)
			}
		})
	}
	if err := builder.BuildInto(nil, validRequests, validEvidence); err == nil || !strings.Contains(err.Error(), "destination") {
		t.Fatalf("nil destination error = %v", err)
	}
}

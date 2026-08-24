package eval_test

import (
	"testing"

	"github.com/sebishogun/verifoxx2/internal/eval"
	"github.com/sebishogun/verifoxx2/internal/result"
)

func TestEvaluateIntoWarmPathAllocation(t *testing.T) {
	evaluator, batch, _ := compiledFixture(t)
	compiled := evaluator.Program()
	var context eval.Context
	context.Ensure(compiled, batch.Rows)
	var results result.Batch
	results.Ensure(batch.Rows, int(batch.Rows)*len(compiled.RequirementSymbols), len(batch.EvidenceRefs), int(batch.Rows)*len(compiled.Remediations))
	if err := evaluator.EvaluateInto(&context, batch, &results); err != nil {
		t.Fatal(err)
	}

	allocations := testing.AllocsPerRun(100, func() {
		if err := evaluator.EvaluateInto(&context, batch, &results); err != nil {
			panic(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("warm EvaluateInto allocations = %v, want 0", allocations)
	}
}

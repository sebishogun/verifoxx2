package jsonio

import (
	"testing"

	policycompile "github.com/sebishogun/verifoxx2/internal/compile"
	"github.com/sebishogun/verifoxx2/internal/eval"
	"github.com/sebishogun/verifoxx2/internal/result"
)

var benchmarkPack OutputPack

func BenchmarkMaterializeSuppliedPack(b *testing.B) {
	source, err := LoadPolicy("../../../policies/policy.json")
	if err != nil {
		b.Fatal(err)
	}
	compiled, diagnostics := policycompile.Compile(source)
	if len(diagnostics) != 0 {
		b.Fatal(diagnostics)
	}
	requests, err := LoadRequests("../../../fixtures/requests.json")
	if err != nil {
		b.Fatal(err)
	}
	evidence, err := LoadEvidence("../../../fixtures/evidence.json")
	if err != nil {
		b.Fatal(err)
	}
	var batch eval.Batch
	if err := eval.NewBuilder(&compiled, eval.DefaultLimits()).BuildInto(&batch, requests, evidence); err != nil {
		b.Fatal(err)
	}
	var context eval.Context
	context.Ensure(compiled, batch.Rows)
	var numeric result.Batch
	numeric.Ensure(batch.Rows, len(requests)*len(compiled.RequirementSymbols), len(batch.EvidenceRefs), len(requests)*len(compiled.Remediations))
	if err := eval.NewEvaluator(&compiled).EvaluateInto(&context, batch, &numeric); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var pack OutputPack
		if err := MaterializeInto(&pack, compiled, batch, numeric, requests, evidence); err != nil {
			b.Fatal(err)
		}
		benchmarkPack = pack
	}
}

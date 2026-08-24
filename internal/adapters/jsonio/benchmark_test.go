package jsonio

import (
	"testing"

	policycompile "github.com/sebishogun/verifoxx2/internal/compile"
	"github.com/sebishogun/verifoxx2/internal/eval"
	"github.com/sebishogun/verifoxx2/internal/input"
	"github.com/sebishogun/verifoxx2/internal/program"
	"github.com/sebishogun/verifoxx2/internal/result"
)

var benchmarkPack OutputPack

func benchmarkMaterializeInputs(b *testing.B) (program.Program, eval.Batch, result.Batch, []input.Request, []input.Evidence) {
	b.Helper()
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
	return compiled, batch, numeric, requests, evidence
}

func BenchmarkMaterializeSuppliedPack(b *testing.B) {
	compiled, batch, numeric, requests, evidence := benchmarkMaterializeInputs(b)
	var pack OutputPack
	if err := MaterializeInto(&pack, compiled, batch, numeric, requests, evidence); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := MaterializeInto(&pack, compiled, batch, numeric, requests, evidence); err != nil {
			b.Fatal(err)
		}
		benchmarkPack = pack
	}
}

func BenchmarkMaterializeColdSuppliedPack(b *testing.B) {
	compiled, batch, numeric, requests, evidence := benchmarkMaterializeInputs(b)
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

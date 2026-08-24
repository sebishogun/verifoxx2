package eval_test

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/sebishogun/verifoxx2/internal/adapters/jsonio"
	policycompile "github.com/sebishogun/verifoxx2/internal/compile"
	"github.com/sebishogun/verifoxx2/internal/eval"
	"github.com/sebishogun/verifoxx2/internal/input"
	"github.com/sebishogun/verifoxx2/internal/result"
)

var benchmarkOutcome uint64

func benchmarkInputs(b *testing.B, rows int) (eval.Evaluator, eval.Batch) {
	b.Helper()
	source, err := jsonio.LoadPolicy("../../policies/policy.json")
	if err != nil {
		b.Fatal(err)
	}
	compiled, diagnostics := policycompile.Compile(source)
	if len(diagnostics) != 0 {
		b.Fatal(diagnostics)
	}
	baseRequests, err := jsonio.LoadRequests("../../fixtures/requests.json")
	if err != nil {
		b.Fatal(err)
	}
	evidence, err := jsonio.LoadEvidence("../../fixtures/evidence.json")
	if err != nil {
		b.Fatal(err)
	}
	requests := make([]input.Request, rows)
	for i := range requests {
		requests[i] = baseRequests[i%len(baseRequests)]
		requests[i].ID = fmt.Sprintf("BENCH_%d", i)
	}
	var batch eval.Batch
	if err := eval.NewBuilder(&compiled, eval.DefaultLimits()).BuildInto(&batch, requests, evidence); err != nil {
		b.Fatal(err)
	}
	return eval.NewEvaluator(&compiled), batch
}

func benchmarkEvaluate(b *testing.B, rows int) {
	evaluator, batch := benchmarkInputs(b, rows)
	compiled := evaluator.Program()
	var context eval.Context
	context.Ensure(compiled, batch.Rows)
	var results result.Batch
	results.Ensure(batch.Rows, rows*len(compiled.RequirementSymbols), len(batch.EvidenceRefs), rows*len(compiled.Remediations))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := evaluator.EvaluateInto(&context, batch, &results); err != nil {
			b.Fatal(err)
		}
	}
	if len(results.OutcomeIDs) != 0 {
		benchmarkOutcome = uint64(results.OutcomeIDs[0])
	}
}

func BenchmarkEvaluateRows1(b *testing.B)    { benchmarkEvaluate(b, 1) }
func BenchmarkEvaluateRows5(b *testing.B)    { benchmarkEvaluate(b, 5) }
func BenchmarkEvaluateRows64(b *testing.B)   { benchmarkEvaluate(b, 64) }
func BenchmarkEvaluateRows1024(b *testing.B) { benchmarkEvaluate(b, 1024) }

func BenchmarkCompilePolicy(b *testing.B) {
	source, err := jsonio.LoadPolicy("../../policies/policy.json")
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		compiled, diagnostics := policycompile.Compile(source)
		if len(diagnostics) != 0 {
			b.Fatal(diagnostics)
		}
		benchmarkOutcome = uint64(len(compiled.Opcodes))
	}
}

func BenchmarkCompilePolicyJSON(b *testing.B) {
	data, err := os.ReadFile("../../policies/policy.json")
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		source, err := jsonio.DecodePolicy(bytes.NewReader(data), "policy.json")
		if err != nil {
			b.Fatal(err)
		}
		compiled, diagnostics := policycompile.Compile(source)
		if len(diagnostics) != 0 {
			b.Fatal(diagnostics)
		}
		benchmarkOutcome = uint64(len(compiled.Opcodes))
	}
}

func BenchmarkBuildBatchRows1024(b *testing.B) {
	source, err := jsonio.LoadPolicy("../../policies/policy.json")
	if err != nil {
		b.Fatal(err)
	}
	compiled, diagnostics := policycompile.Compile(source)
	if len(diagnostics) != 0 {
		b.Fatal(diagnostics)
	}
	base, err := jsonio.LoadRequests("../../fixtures/requests.json")
	if err != nil {
		b.Fatal(err)
	}
	evidence, err := jsonio.LoadEvidence("../../fixtures/evidence.json")
	if err != nil {
		b.Fatal(err)
	}
	requests := make([]input.Request, 1024)
	for i := range requests {
		requests[i] = base[i%len(base)]
		requests[i].ID = fmt.Sprintf("BUILD_%d", i)
	}
	builder := eval.NewBuilder(&compiled, eval.DefaultLimits())
	var batch eval.Batch
	if err := builder.BuildInto(&batch, requests, evidence); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := builder.BuildInto(&batch, requests, evidence); err != nil {
			b.Fatal(err)
		}
	}
	benchmarkOutcome = uint64(batch.Rows)
}

func BenchmarkBuildBatchColdDestinationRows1024(b *testing.B) {
	source, err := jsonio.LoadPolicy("../../policies/policy.json")
	if err != nil {
		b.Fatal(err)
	}
	compiled, diagnostics := policycompile.Compile(source)
	if len(diagnostics) != 0 {
		b.Fatal(diagnostics)
	}
	base, err := jsonio.LoadRequests("../../fixtures/requests.json")
	if err != nil {
		b.Fatal(err)
	}
	evidence, err := jsonio.LoadEvidence("../../fixtures/evidence.json")
	if err != nil {
		b.Fatal(err)
	}
	requests := make([]input.Request, 1024)
	for i := range requests {
		requests[i] = base[i%len(base)]
		requests[i].ID = fmt.Sprintf("COLD_BUILD_%d", i)
	}
	builder := eval.NewBuilder(&compiled, eval.DefaultLimits())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var batch eval.Batch
		if err := builder.BuildInto(&batch, requests, evidence); err != nil {
			b.Fatal(err)
		}
		benchmarkOutcome = uint64(batch.Rows)
	}
}

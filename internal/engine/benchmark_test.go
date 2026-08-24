package engine

import (
	"testing"

	"github.com/sebishogun/verifoxx2/internal/adapters/jsonio"
	policycompile "github.com/sebishogun/verifoxx2/internal/compile"
	"github.com/sebishogun/verifoxx2/internal/eval"
)

var benchmarkResults int

func BenchmarkSessionSuppliedPack(b *testing.B) {
	source, err := jsonio.LoadPolicy("../../policies/policy.json")
	if err != nil {
		b.Fatal(err)
	}
	compiled, diagnostics := policycompile.Compile(source)
	if len(diagnostics) != 0 {
		b.Fatal(diagnostics)
	}
	engine, err := New(compiled, eval.DefaultLimits())
	if err != nil {
		b.Fatal(err)
	}
	requests, err := jsonio.LoadRequests("../../fixtures/requests.json")
	if err != nil {
		b.Fatal(err)
	}
	evidence, err := jsonio.LoadEvidence("../../fixtures/evidence.json")
	if err != nil {
		b.Fatal(err)
	}
	session := engine.NewSession()
	if _, err := session.Evaluate(requests, evidence); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pack, err := session.Evaluate(requests, evidence)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkResults = len(pack.Results)
	}
}

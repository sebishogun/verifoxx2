package conformance

import (
	"bytes"
	"os"
	"testing"

	"github.com/sebishogun/verifoxx2/internal/adapters/jsonio"
	policycompile "github.com/sebishogun/verifoxx2/internal/compile"
	"github.com/sebishogun/verifoxx2/internal/eval"
	"github.com/sebishogun/verifoxx2/internal/result"
)

func TestCompiledPipelineMatchesTrackedGoldens(t *testing.T) {
	tests := []struct {
		name, requests, evidence, golden string
	}{
		{"supplied", "../../fixtures/requests.json", "../../fixtures/evidence.json", "../../results/requests.json"},
		{"edge", "../../fixtures/demo/requests.json", "../../fixtures/demo/evidence.json", "../../fixtures/demo/expected.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, err := jsonio.LoadPolicy("../../policies/policy.json")
			if err != nil {
				t.Fatal(err)
			}
			compiled, diagnostics := policycompile.Compile(source)
			if len(diagnostics) != 0 {
				t.Fatal(diagnostics)
			}
			requests, err := jsonio.LoadRequests(tt.requests)
			if err != nil {
				t.Fatal(err)
			}
			evidence, err := jsonio.LoadEvidence(tt.evidence)
			if err != nil {
				t.Fatal(err)
			}
			var batch eval.Batch
			if err := eval.NewBuilder(&compiled, eval.DefaultLimits()).BuildInto(&batch, requests, evidence); err != nil {
				t.Fatal(err)
			}
			var context eval.Context
			context.Ensure(compiled, batch.Rows)
			var numeric result.Batch
			numeric.Ensure(batch.Rows, len(requests)*len(compiled.RequirementSymbols), len(batch.EvidenceRefs), len(requests)*len(compiled.Remediations))
			if err := eval.NewEvaluator(&compiled).EvaluateInto(&context, batch, &numeric); err != nil {
				t.Fatal(err)
			}
			var pack jsonio.OutputPack
			if err := jsonio.MaterializeInto(&pack, compiled, batch, numeric, requests, evidence); err != nil {
				t.Fatal(err)
			}
			var got bytes.Buffer
			if err := jsonio.EncodeResults(&got, pack); err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(tt.golden)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Bytes(), want) {
				t.Fatalf("compiled output differs from %s:\n--- got ---\n%s--- want ---\n%s", tt.golden, got.Bytes(), want)
			}
		})
	}
}

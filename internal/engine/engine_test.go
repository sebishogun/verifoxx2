package engine

import (
	"bytes"
	"os"
	"testing"

	"github.com/sebishogun/verifoxx2/internal/adapters/jsonio"
	policycompile "github.com/sebishogun/verifoxx2/internal/compile"
	"github.com/sebishogun/verifoxx2/internal/eval"
	"github.com/sebishogun/verifoxx2/internal/input"
	"github.com/sebishogun/verifoxx2/internal/program"
)

func engineFixture(t *testing.T) (*Engine, []input.Request, []input.Evidence) {
	t.Helper()
	source, err := jsonio.LoadPolicy("../../policies/policy.json")
	if err != nil {
		t.Fatal(err)
	}
	compiled, diagnostics := policycompile.Compile(source)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	engine, err := New(compiled, eval.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	requests, err := jsonio.LoadRequests("../../fixtures/requests.json")
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := jsonio.LoadEvidence("../../fixtures/evidence.json")
	if err != nil {
		t.Fatal(err)
	}
	return engine, requests, evidence
}

func TestNewRejectsInvalidProgram(t *testing.T) {
	if _, err := New(program.Program{}, eval.DefaultLimits()); err == nil {
		t.Fatal("New accepted an invalid program")
	}
}

func TestCompilePublishesValidatedEngine(t *testing.T) {
	source, err := jsonio.LoadPolicy("../../policies/policy.json")
	if err != nil {
		t.Fatal(err)
	}
	runtime, diagnostics := Compile(source, eval.DefaultLimits())
	if len(diagnostics) != 0 || runtime == nil {
		t.Fatalf("Compile = (%v, %+v)", runtime, diagnostics)
	}
	if runtime.Name() != "verifoxx-policy" || runtime.Version() != "1.0.0" {
		t.Fatalf("engine metadata = %q/%q", runtime.Name(), runtime.Version())
	}
}

func TestSessionEvaluatesSuppliedPack(t *testing.T) {
	engine, requests, evidence := engineFixture(t)
	pack, err := engine.NewSession().Evaluate(requests, evidence)
	if err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	if err := jsonio.EncodeResults(&got, *pack); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../results/requests.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatalf("session output differs from golden:\n--- got ---\n%s--- want ---\n%s", got.Bytes(), want)
	}
}

func TestSessionReusesStorageAcrossPackSizes(t *testing.T) {
	engine, requests, evidence := engineFixture(t)
	session := engine.NewSession()
	pack, err := session.Evaluate(requests, evidence)
	if err != nil {
		t.Fatal(err)
	}
	resultStorage := &pack.Results[0]

	pack, err = session.Evaluate(requests[:1], evidence)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Results) != 1 || pack.Results[0].RequestID != "R1" {
		t.Fatalf("shrunk results = %+v, want only R1", pack.Results)
	}
	if &pack.Results[0] != resultStorage {
		t.Fatal("session replaced reusable result storage")
	}

	pack, err = session.Evaluate(requests, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Results) != len(requests) || pack.Results[4].RequestID != "R5" {
		t.Fatalf("regrown results = %+v, want complete supplied pack", pack.Results)
	}
}

func TestSessionReturnsAnEmptyResultArrayForAnEmptyPack(t *testing.T) {
	engine, _, _ := engineFixture(t)
	pack, err := engine.NewSession().Evaluate([]input.Request{}, []input.Evidence{})
	if err != nil {
		t.Fatal(err)
	}
	if pack.Results == nil || len(pack.Results) != 0 {
		t.Fatalf("empty pack results = %#v, want non-nil empty slice", pack.Results)
	}
}

func TestSessionClassifiesInvalidInput(t *testing.T) {
	engine, requests, evidence := engineFixture(t)
	requests[1].ID = requests[0].ID
	if _, err := engine.NewSession().Evaluate(requests, evidence); err == nil || !IsInputError(err) {
		t.Fatalf("Evaluate error = %v, want classified input error", err)
	}
}

func TestIndependentSessionsShareEngineConcurrently(t *testing.T) {
	engine, requests, evidence := engineFixture(t)
	errors := make(chan error, 2)
	for range 2 {
		go func() {
			pack, err := engine.NewSession().Evaluate(requests, evidence)
			if err == nil && (len(pack.Results) != 5 || pack.Results[4].Decision != "Escalate") {
				err = os.ErrInvalid
			}
			errors <- err
		}()
	}
	for range 2 {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}
}

package jsonio

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebishogun/verifoxx2/internal/ast"
	policycompile "github.com/sebishogun/verifoxx2/internal/compile"
	"github.com/sebishogun/verifoxx2/internal/eval"
	"github.com/sebishogun/verifoxx2/internal/input"
	"github.com/sebishogun/verifoxx2/internal/program"
	"github.com/sebishogun/verifoxx2/internal/result"
	"github.com/sebishogun/verifoxx2/internal/schema"
)

func materializeFixture(t *testing.T, requestsPath, evidencePath string) OutputPack {
	t.Helper()
	source, err := LoadPolicy("../../../policies/policy.json")
	if err != nil {
		t.Fatal(err)
	}
	compiled, diagnostics := policycompile.Compile(source)
	if len(diagnostics) != 0 {
		t.Fatalf("Compile() diagnostics = %+v", diagnostics)
	}
	requests, err := LoadRequests(requestsPath)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := LoadEvidence(evidencePath)
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
	var pack OutputPack
	if err := MaterializeInto(&pack, compiled, batch, numeric, requests, evidence); err != nil {
		t.Fatalf("MaterializeInto() error = %v", err)
	}
	return pack
}

func TestMaterializeAndEncodeMatchTrackedGoldens(t *testing.T) {
	tests := []struct {
		name         string
		requestsPath string
		evidencePath string
		goldenPath   string
	}{
		{"supplied", "../../../fixtures/requests.json", "../../../fixtures/evidence.json", "../../../results/requests.json"},
		{"edge", "../../../fixtures/demo/requests.json", "../../../fixtures/demo/evidence.json", "../../../fixtures/demo/expected.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pack := materializeFixture(t, tt.requestsPath, tt.evidencePath)
			var first bytes.Buffer
			if err := EncodeResults(&first, pack); err != nil {
				t.Fatal(err)
			}
			var second bytes.Buffer
			if err := EncodeResults(&second, pack); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(first.Bytes(), second.Bytes()) || !bytes.HasSuffix(first.Bytes(), []byte("\n")) {
				t.Fatal("encoding is not deterministic with one trailing newline")
			}
			want, err := os.ReadFile(tt.goldenPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(first.Bytes(), want) {
				t.Fatalf("encoded output differs from %s:\n--- got ---\n%s--- want ---\n%s", tt.goldenPath, first.Bytes(), want)
			}
		})
	}
}

func TestMaterializeRejectsMismatchedRows(t *testing.T) {
	pack := materializeFixture(t, "../../../fixtures/requests.json", "../../../fixtures/evidence.json")
	if len(pack.Results) == 0 {
		t.Fatal("fixture has no results")
	}
	var dst OutputPack
	if err := MaterializeInto(&dst, structProgram(), eval.Batch{}, result.Batch{}, nil, nil); err == nil {
		t.Fatal("MaterializeInto() accepted malformed empty inputs")
	}
}

func TestMaterializeCoversEveryReasonAndRemediationKind(t *testing.T) {
	explanations := ast.Explanations{
		False: "false explanation", Missing: "missing explanation", Invalid: "invalid explanation",
		Stale: "stale explanation", Unclear: "unclear explanation",
		Unverifiable: "unverifiable explanation", Conflict: "conflict explanation",
	}
	source := ast.Policy{
		Name: "reasons", Version: "1",
		Requirements: []ast.Requirement{{
			ID: "R1", Applicability: ast.Expression{Op: ast.OperatorEqual, Field: "dataset", Value: "protected_dataset"},
			Clauses: []ast.Clause{{
				ID: "C1", Assertion: ast.Expression{Op: ast.OperatorEvidenceMatches, Evidence: ast.EvidencePredicate{Kind: "approval_record", Status: "valid"}},
				Resolution: ast.Resolution{
					Satisfied: ast.OutcomeApprove, False: ast.OutcomeReject, Missing: ast.OutcomeRevise,
					Invalid: ast.OutcomeEscalate, Stale: ast.OutcomeEscalate, Unclear: ast.OutcomeEscalate,
					Unverifiable: ast.OutcomeEscalate, Conflict: ast.OutcomeEscalate,
				},
				Explanations: explanations,
				Remediations: []ast.Remediation{
					{Action: ast.RemediationAddEvidence, EvidenceKind: "approval_record", Description: "add approval"},
					{Action: ast.RemediationSetField, Field: "usage_limit", Value: "standard"},
				},
			}},
		}},
	}
	compiled, diagnostics := policycompile.Compile(source)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	const rows = uint32(schema.ReasonCount)
	batch := eval.Batch{Rows: rows, Words: 1, EvidenceRefOffsets: make([]uint32, rows+1)}
	requests := make([]input.Request, rows)
	var numeric result.Batch
	numeric.Ensure(rows, int(rows), 0, 2)
	outcomes := [...]schema.OutcomeID{
		schema.OutcomeApprove, schema.OutcomeReject, schema.OutcomeRevise, schema.OutcomeEscalate,
		schema.OutcomeEscalate, schema.OutcomeEscalate, schema.OutcomeEscalate, schema.OutcomeEscalate,
	}
	for row := uint32(0); row < rows; row++ {
		requests[row].ID = "REASON_" + string(rune('1'+row))
		numeric.OutcomeIDs[row] = outcomes[row]
		numeric.DriverReasonIDs[row] = schema.ReasonID(row + 1)
		numeric.RequirementOffsets[row] = uint32(len(numeric.RequirementIDs))
		numeric.RequirementIDs = append(numeric.RequirementIDs, 1)
		numeric.RequirementOffsets[row+1] = uint32(len(numeric.RequirementIDs))
		numeric.RemediationOffsets[row] = uint32(len(numeric.RemediationIDs))
		if schema.ReasonID(row+1) == schema.ReasonMissing {
			numeric.RemediationIDs = append(numeric.RemediationIDs, 1, 2)
		}
		numeric.RemediationOffsets[row+1] = uint32(len(numeric.RemediationIDs))
		if row != 0 {
			numeric.DriverKinds[row] = result.DriverClause
			numeric.DriverRequirementIDs[row] = 1
			numeric.DriverClauseIDs[row] = 1
			numeric.IssueIDs[row] = schema.ReasonID(row + 1)
		}
	}
	var pack OutputPack
	if err := MaterializeInto(&pack, compiled, batch, numeric, requests, nil); err != nil {
		t.Fatalf("MaterializeInto() error = %v", err)
	}
	if len(pack.Results) != int(rows) {
		t.Fatalf("result count = %d, want %d", len(pack.Results), rows)
	}
	wantRationales := []string{
		"The request satisfies all applicable requirements and supporting evidence.",
		explanations.False, explanations.Missing, explanations.Invalid, explanations.Stale,
		explanations.Unclear, explanations.Unverifiable, explanations.Conflict,
	}
	for row := range pack.Results {
		got := pack.Results[row]
		if got.Rationale != wantRationales[row] || len(got.Assumptions) != 1 {
			t.Fatalf("row %d rationale/assumptions = %q/%v", row, got.Rationale, got.Assumptions)
		}
		if row >= int(schema.ReasonMissing)-1 && (len(got.MissingOrConflictingEvidence) != 1 || len(got.UnresolvedUncertainty) != 1) {
			t.Fatalf("row %d evidence detail/uncertainty = %v/%v", row, got.MissingOrConflictingEvidence, got.UnresolvedUncertainty)
		}
	}
	remediations := pack.Results[int(schema.ReasonMissing)-1].Remediation
	if len(remediations) != 2 || remediations[0].Action != "add_evidence" || remediations[1].Action != "set_field" {
		t.Fatalf("missing-state remediations = %+v", remediations)
	}
}

// structProgram returns a non-nil-looking zero program without coupling this
// error-path test to compiler setup.
func structProgram() program.Program { return program.Program{Name: "x", Version: "1"} }

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) { return 0, errors.New("writer failed") }

func TestEncodeResultsReportsWriterFailure(t *testing.T) {
	err := EncodeResults(errorWriter{}, OutputPack{SchemaVersion: 1})
	if err == nil || !strings.Contains(err.Error(), "encode results") || !strings.Contains(err.Error(), "writer failed") {
		t.Fatalf("EncodeResults() error = %v, want encoder and writer context", err)
	}
}

func TestWriteResultsReplacesFileAndCleansRenameFailure(t *testing.T) {
	pack := materializeFixture(t, "../../../fixtures/requests.json", "../../../fixtures/evidence.json")
	directory := t.TempDir()
	destination := filepath.Join(directory, "nested", "results.json")
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteResults(destination, pack); err != nil {
		t.Fatalf("WriteResults() error = %v", err)
	}
	data, err := os.ReadFile(destination)
	if err != nil || !bytes.HasSuffix(data, []byte("\n")) || bytes.Equal(data, []byte("old")) {
		t.Fatalf("replacement result = %q, error %v", data, err)
	}

	occupied := filepath.Join(directory, "occupied")
	if err := os.Mkdir(occupied, 0o755); err != nil {
		t.Fatal(err)
	}
	err = WriteResults(occupied, pack)
	if err == nil || !strings.Contains(err.Error(), "replace result") {
		t.Fatalf("WriteResults(directory) error = %v, want replace context", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".verifoxx-") {
			t.Fatalf("temporary file %s remained after rename failure", entry.Name())
		}
	}
}

func TestWriteResultsOperationFailuresPreserveDestinationAndCleanTemporary(t *testing.T) {
	pack := materializeFixture(t, "../../../fixtures/requests.json", "../../../fixtures/evidence.json")
	tests := []struct {
		name   string
		want   string
		inject func(*outputOps)
	}{
		{
			name: "encode",
			want: "write temporary result",
			inject: func(ops *outputOps) {
				ops.encode = func(io.Writer, OutputPack) error { return errors.New("injected encode failure") }
			},
		},
		{
			name: "close",
			want: "close temporary result",
			inject: func(ops *outputOps) {
				ops.close = func(file *os.File) error {
					_ = file.Close()
					return errors.New("injected close failure")
				}
			},
		},
		{
			name: "chmod",
			want: "set result permissions",
			inject: func(ops *outputOps) {
				ops.chmod = func(string, os.FileMode) error { return errors.New("injected chmod failure") }
			},
		},
		{
			name: "rename",
			want: "replace result",
			inject: func(ops *outputOps) {
				ops.rename = func(string, string) error { return errors.New("injected rename failure") }
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			directory := t.TempDir()
			destination := filepath.Join(directory, "results.json")
			if err := os.WriteFile(destination, []byte("old"), 0o600); err != nil {
				t.Fatal(err)
			}
			ops := newOutputOps()
			tt.inject(&ops)
			err := writeResults(destination, pack, ops)
			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), "injected "+tt.name+" failure") {
				t.Fatalf("writeResults() error = %v, want %q and injected failure", err, tt.want)
			}
			data, readErr := os.ReadFile(destination)
			if readErr != nil || string(data) != "old" {
				t.Fatalf("destination after failure = %q, error %v", data, readErr)
			}
			entries, readDirErr := os.ReadDir(directory)
			if readDirErr != nil {
				t.Fatal(readDirErr)
			}
			if len(entries) != 1 || entries[0].Name() != "results.json" {
				t.Fatalf("directory after failure = %v, want only destination", entries)
			}
		})
	}
}

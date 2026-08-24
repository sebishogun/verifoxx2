package jsonio

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sebishogun/verifoxx2/internal/ast"
)

func adapterValidPolicy() ast.Policy {
	approve := ast.Resolution{
		Satisfied:    ast.OutcomeApprove,
		False:        ast.OutcomeApprove,
		Missing:      ast.OutcomeApprove,
		Invalid:      ast.OutcomeApprove,
		Stale:        ast.OutcomeApprove,
		Unclear:      ast.OutcomeApprove,
		Unverifiable: ast.OutcomeApprove,
		Conflict:     ast.OutcomeApprove,
	}
	return ast.Policy{
		Name:    "test-policy",
		Version: "1.0.0",
		Requirements: []ast.Requirement{{
			ID:            "R1",
			Applicability: ast.Expression{Op: ast.OperatorEqual, Field: "dataset", Value: "protected_dataset"},
			Clauses: []ast.Clause{{
				ID:         "R1_C1",
				Assertion:  ast.Expression{Op: ast.OperatorEqual, Field: "action", Value: "aggregate_analysis"},
				Resolution: approve,
			}},
		}},
	}
}

func encodeTestPolicy(t *testing.T, policy ast.Policy) []byte {
	t.Helper()
	data, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestDecodePolicyAcceptsOneStrictDocument(t *testing.T) {
	want := adapterValidPolicy()
	got, err := DecodePolicy(bytes.NewReader(encodeTestPolicy(t, want)), "memory-policy.json")
	if err != nil {
		t.Fatalf("DecodePolicy() error = %v", err)
	}
	if got.Name != want.Name || got.Version != want.Version || len(got.Requirements) != 1 {
		t.Fatalf("DecodePolicy() = %+v, want metadata and one requirement from %+v", got, want)
	}
}

func TestDecodePolicyRejectsMalformedDocuments(t *testing.T) {
	valid := encodeTestPolicy(t, adapterValidPolicy())
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"empty", nil, "decode memory-policy.json"},
		{"unknown field", []byte(`{"name":"test","version":"1","requirements":[],"unexpected":true}`), "unknown field"},
		{"second value", append(append([]byte{}, valid...), []byte("\n{}")...), "trailing JSON value"},
		{"trailing garbage", append(append([]byte{}, valid...), []byte("\nnot-json")...), "trailing JSON"},
		{"oversized", []byte(strings.Repeat(" ", MaxPolicyBytes+1)), "exceeds"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodePolicy(bytes.NewReader(tt.data), "memory-policy.json")
			if err == nil {
				t.Fatalf("DecodePolicy() succeeded, want error containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("DecodePolicy() error = %q, want substring %q", err, tt.want)
			}
		})
	}
}

func TestDecodePolicyReportsValidationPath(t *testing.T) {
	policy := adapterValidPolicy()
	policy.Requirements[0].Clauses[0].Resolution.Conflict = ""

	_, err := DecodePolicy(bytes.NewReader(encodeTestPolicy(t, policy)), "broken-policy.json")
	if err == nil {
		t.Fatal("DecodePolicy() succeeded")
	}
	for _, want := range []string{"validate broken-policy.json", "requirements[0].clauses[0].resolution.conflict"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("DecodePolicy() error = %q, want substring %q", err, want)
		}
	}
}

func TestLoadPolicyAddsOpenContext(t *testing.T) {
	_, err := LoadPolicy(t.TempDir() + "/missing.json")
	if err == nil || !strings.Contains(err.Error(), "open policy") {
		t.Fatalf("LoadPolicy() error = %v, want open policy context", err)
	}
}

func TestLoadPolicyAcceptsShippedVerifoxxPolicy(t *testing.T) {
	policy, err := LoadPolicy("../../../policies/policy.json")
	if err != nil {
		t.Fatalf("LoadPolicy(compiled fixture) error = %v", err)
	}
	if policy.Name != "verifoxx-policy" || len(policy.Requirements) != 3 {
		t.Fatalf("compiled fixture metadata = (%q, %d requirements), want (verifoxx-policy, 3)", policy.Name, len(policy.Requirements))
	}
}

package jsonio

import (
	"bytes"
	"strings"
	"testing"
)

func TestDecodeRequestsAndEvidenceStrictly(t *testing.T) {
	requests, err := DecodeRequests(bytes.NewBufferString(`[{"id":"R1","requester":"a","trust_level":"external","action":"aggregate_analysis","output_kind":"aggregate_counts","dataset":"protected_dataset","environment":"local_approved_env","usage_limit":"standard","evidence_ids":["E1"]}]`), "requests.json")
	if err != nil || len(requests) != 1 || requests[0].ID != "R1" {
		t.Fatalf("DecodeRequests() = (%+v, %v)", requests, err)
	}
	evidence, err := DecodeEvidence(bytes.NewBufferString(`[{"id":"E1","type":"approval_record","status":"valid"}]`), "evidence.json")
	if err != nil || len(evidence) != 1 || evidence[0].ID != "E1" {
		t.Fatalf("DecodeEvidence() = (%+v, %v)", evidence, err)
	}
}

func TestDecodeInputsRejectMalformedDocuments(t *testing.T) {
	tests := []struct {
		name string
		call func() error
		want string
	}{
		{"request unknown field", func() error {
			_, err := DecodeRequests(bytes.NewBufferString(`[{"id":"R1","extra":1}]`), "requests.json")
			return err
		}, "unknown field"},
		{"request trailing value", func() error { _, err := DecodeRequests(bytes.NewBufferString(`[] []`), "requests.json"); return err }, "trailing JSON value"},
		{"request empty id", func() error {
			_, err := DecodeRequests(bytes.NewBufferString(`[{"id":" "}]`), "requests.json")
			return err
		}, "empty id"},
		{"request duplicate id", func() error {
			_, err := DecodeRequests(bytes.NewBufferString(`[{"id":"R1"},{"id":"R1"}]`), "requests.json")
			return err
		}, "duplicate request id"},
		{"evidence unknown field", func() error {
			_, err := DecodeEvidence(bytes.NewBufferString(`[{"id":"E1","wat":1}]`), "evidence.json")
			return err
		}, "unknown field"},
		{"evidence duplicate id", func() error {
			_, err := DecodeEvidence(bytes.NewBufferString(`[{"id":"E1"},{"id":"E1"}]`), "evidence.json")
			return err
		}, "duplicate evidence id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

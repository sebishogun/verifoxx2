package framed

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sebishogun/verifoxx2/internal/adapters/jsonio"
)

func TestJSONCodecDecodesStrictInputAndClearsReusedValues(t *testing.T) {
	codec := NewJSONCodec(MaxOutputPayload)
	first := []byte(`{
  "requests": [{"id":"R1","requester":"partner","trust_level":"external","action":"aggregate_analysis","output_kind":"aggregate_counts","dataset":"protected_dataset","environment":"local_approved_env","usage_limit":"standard","evidence_ids":["E1"]}],
  "evidence": [{"id":"E1","type":"approval_record","status":"valid","timing":"before_execution"}]
}`)
	decoded, err := codec.Decode(first)
	if err != nil {
		t.Fatal(err)
	}
	requestStorage := &decoded.Requests[0]
	evidenceStorage := &decoded.Evidence[0]

	second := []byte(`{"requests":[{"id":"R2"}],"evidence":[{"id":"E2","type":"approval_record"}]}`)
	decoded, err = codec.Decode(second)
	if err != nil {
		t.Fatal(err)
	}
	if &decoded.Requests[0] != requestStorage || &decoded.Evidence[0] != evidenceStorage {
		t.Fatal("Decode did not reuse input record storage")
	}
	if decoded.Requests[0].TrustLevel != "" || len(decoded.Requests[0].EvidenceIDs) != 0 || decoded.Evidence[0].Status != "" || decoded.Evidence[0].Timing != "" {
		t.Fatalf("Decode retained values from the previous frame: %+v %+v", decoded.Requests[0], decoded.Evidence[0])
	}
	decoded, err = codec.Decode([]byte(`{"requests":[],"evidence":[]}`))
	if err != nil || len(decoded.Requests) != 0 || len(decoded.Evidence) != 0 {
		t.Fatalf("Decode empty arrays = (%+v, %v)", decoded, err)
	}

	for _, payload := range [][]byte{
		[]byte(`null`),
		[]byte(`{"requests":null,"evidence":null}`),
		[]byte(`{"requests":[]}`),
		[]byte(`{"evidence":[]}`),
		[]byte(`{"requests":[],"evidence":[],"unknown":true}`),
		[]byte(`{"requests":[],"evidence":[]} {}`),
		[]byte(`{"requests":[{"id":"R1","unknown":true}],"evidence":[]}`),
	} {
		if _, err := codec.Decode(payload); err == nil {
			t.Fatalf("Decode accepted invalid payload %s", payload)
		}
	}
}

func TestJSONCodecEncodesCompactSuccessAndErrorResponses(t *testing.T) {
	codec := NewJSONCodec(MaxOutputPayload)
	pack := jsonio.OutputPack{SchemaVersion: 1, PolicyName: "policy", PolicyVersion: "1", Results: []jsonio.EvaluationResult{}}
	payload, err := codec.EncodeSuccess(&pack)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte("\n")) {
		t.Fatalf("success response is not compact: %q", payload)
	}
	var response Response
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.Output == nil || response.Output.PolicyName != "policy" || response.Error != nil {
		t.Fatalf("success response = %+v", response)
	}

	payload, err = codec.EncodeError("invalid_input", "bad frame")
	if err != nil {
		t.Fatal(err)
	}
	response = Response{}
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatal(err)
	}
	if response.OK || response.Output != nil || response.Error == nil || response.Error.Code != "invalid_input" || response.Error.Message != "bad frame" {
		t.Fatalf("error response = %+v", response)
	}

	small := NewJSONCodec(8)
	if _, err := small.EncodeSuccess(&pack); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("bounded EncodeSuccess error = %v", err)
	}
}

package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/sebishogun/verifoxx2/internal/adapters/framed"
	"github.com/sebishogun/verifoxx2/internal/adapters/jsonio"
	policycompile "github.com/sebishogun/verifoxx2/internal/compile"
	"github.com/sebishogun/verifoxx2/internal/engine"
	"github.com/sebishogun/verifoxx2/internal/eval"
)

func streamInputPayload() []byte {
	return []byte(`{"requests":` + cliRequestsJSON + `,"evidence":` + cliEvidenceJSON + `}`)
}

func appendStreamFrame(t *testing.T, destination *bytes.Buffer, payload []byte) {
	t.Helper()
	var writer framed.FrameWriter
	if err := writer.Write(destination, payload); err != nil {
		t.Fatal(err)
	}
}

func decodeStreamResponses(t *testing.T, data []byte) []framed.Response {
	t.Helper()
	reader := bytes.NewReader(data)
	var frameReader framed.FrameReader
	var responses []framed.Response
	for {
		payload, ok, err := frameReader.Read(reader)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			return responses
		}
		var response framed.Response
		if err := json.Unmarshal(payload, &response); err != nil {
			t.Fatalf("decode response %q: %v", payload, err)
		}
		responses = append(responses, response)
	}
}

func TestRun_StreamProcessesFramesInOrderAndContinuesAfterInvalidInput(t *testing.T) {
	policy := writeFixture(t, "policy.json", cliPolicyJSON)
	var stdin bytes.Buffer
	appendStreamFrame(t, &stdin, streamInputPayload())
	appendStreamFrame(t, &stdin, []byte(`{"requests":`))
	appendStreamFrame(t, &stdin, []byte(`{"requests":[{"id":"DUP"},{"id":"DUP"}],"evidence":[]}`))
	appendStreamFrame(t, &stdin, streamInputPayload())

	var stdout, stderr bytes.Buffer
	code := runWithInput([]string{"--stream", "--policy", policy}, &stdin, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runWithInput exit code = %d, stderr: %s", code, stderr.String())
	}
	responses := decodeStreamResponses(t, stdout.Bytes())
	if len(responses) != 4 {
		t.Fatalf("response count = %d, want 4", len(responses))
	}
	if !responses[0].OK || responses[0].Output == nil || len(responses[0].Output.Results) != 1 || responses[0].Output.Results[0].RequestID != "REQ-1" {
		t.Fatalf("first response = %+v", responses[0])
	}
	if responses[1].OK || responses[1].Error == nil || responses[1].Error.Code != "invalid_input" {
		t.Fatalf("second response = %+v", responses[1])
	}
	if responses[2].OK || responses[2].Error == nil || responses[2].Error.Code != "invalid_input" {
		t.Fatalf("third response = %+v", responses[2])
	}
	if !responses[3].OK || responses[3].Output == nil || responses[3].Output.Results[0].Decision != "Approve" {
		t.Fatalf("fourth response = %+v", responses[3])
	}
}

func TestRun_StreamRejectsIncompatibleOneShotFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runWithInput([]string{"--stream", "--requests", "requests.json"}, bytes.NewReader(nil), &stdout, &stderr)
	if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "--requests") {
		t.Fatalf("runWithInput = %d, stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRun_StreamReportsTruncatedTransport(t *testing.T) {
	policy := writeFixture(t, "policy.json", cliPolicyJSON)
	var input bytes.Buffer
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], 4)
	input.Write(header[:])
	input.WriteByte('{')
	var stdout, stderr bytes.Buffer
	code := runWithInput([]string{"--stream", "--policy", policy}, &input, &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "frame payload") {
		t.Fatalf("runWithInput = %d, stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

type failingStreamWriter struct{ err error }

func (writer failingStreamWriter) Write([]byte) (int, error) { return 0, writer.err }

func TestRun_StreamReportsOutputFailure(t *testing.T) {
	policy := writeFixture(t, "policy.json", cliPolicyJSON)
	var input bytes.Buffer
	appendStreamFrame(t, &input, streamInputPayload())
	want := errors.New("stream output failed")
	var stderr bytes.Buffer
	code := runWithInput([]string{"--stream", "--policy", policy}, &input, failingStreamWriter{err: want}, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), want.Error()) {
		t.Fatalf("runWithInput = %d, stderr=%q", code, stderr.String())
	}
}

var _ io.Writer = failingStreamWriter{}

func suppliedStreamFixture(t testing.TB) (*engine.Engine, []byte) {
	t.Helper()
	source, err := jsonio.LoadPolicy("../../policies/policy.json")
	if err != nil {
		t.Fatal(err)
	}
	compiled, diagnostics := policycompile.Compile(source)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	runtime, err := engine.New(compiled, eval.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	requests, err := os.ReadFile("../../fixtures/requests.json")
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := os.ReadFile("../../fixtures/evidence.json")
	if err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, 0, len(requests)+len(evidence)+32)
	payload = append(payload, `{"requests":`...)
	payload = append(payload, requests...)
	payload = append(payload, `,"evidence":`...)
	payload = append(payload, evidence...)
	payload = append(payload, '}')
	return runtime, payload
}

func suppliedStreamProcessor(t testing.TB) (*streamProcessor, []byte) {
	t.Helper()
	runtime, payload := suppliedStreamFixture(t)
	return newStreamProcessor(runtime), payload
}

func TestStreamProcessorSteadyAllocation(t *testing.T) {
	processor, payload := suppliedStreamProcessor(t)
	if err := processor.Process(payload, io.Discard); err != nil {
		t.Fatal(err)
	}
	allocations := testing.AllocsPerRun(100, func() {
		if err := processor.Process(payload, io.Discard); err != nil {
			panic(err)
		}
	})
	if allocations >= 100 {
		t.Fatalf("steady framed processing allocations = %v, want fewer than 100", allocations)
	}
}

func BenchmarkSteadyFrameSuppliedPack(b *testing.B) {
	processor, payload := suppliedStreamProcessor(b)
	if err := processor.Process(payload, io.Discard); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := processor.Process(payload, io.Discard); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFirstFrameSuppliedPack(b *testing.B) {
	runtime, payload := suppliedStreamFixture(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		processor := newStreamProcessor(runtime)
		if err := processor.Process(payload, io.Discard); err != nil {
			b.Fatal(err)
		}
	}
}

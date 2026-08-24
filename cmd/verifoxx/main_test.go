package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebishogun/verifoxx2/internal/adapters/jsonio"
)

func writeFixture(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

const cliPolicyJSON = `{
  "name": "cli-policy",
  "version": "7.0.0",
  "requirements": [
    {
      "id": "R1",
      "description": "cli requirement",
      "applicability": {"op": "equal", "field": "dataset", "value": "protected_dataset"},
      "clauses": [
        {
          "id": "R1_C1",
          "assertion": {
            "op": "evidence_matches",
            "evidence": {
              "kind": "approval_record",
              "status": "valid",
              "timing": "before_execution",
              "timestamp_state": "current",
              "reviewer": "designated_reviewer"
            }
          },
          "resolution": {
            "satisfied": "Approve",
            "false": "Escalate",
            "missing": "Escalate",
            "invalid": "Escalate",
            "stale": "Escalate",
            "unclear": "Escalate",
            "unverifiable": "Escalate",
            "conflict": "Escalate"
          },
          "explanations": {
            "false": "approval is false",
            "missing": "approval is missing",
            "invalid": "approval is invalid",
            "stale": "approval is stale",
            "unclear": "approval is unclear",
            "unverifiable": "approval is unverifiable",
            "conflict": "approval conflicts"
          }
        }
      ]
    }
  ]
}`

const cliRequestsJSON = `[
  {"id": "REQ-1", "requester": "partner", "trust_level": "external", "action": "aggregate_analysis", "output_kind": "aggregate_counts", "dataset": "protected_dataset", "environment": "local_approved_env", "usage_limit": "standard", "evidence_ids": ["EV-1"]}
]`

const cliEvidenceJSON = `[
  {"id": "EV-1", "type": "approval_record", "status": "valid", "timing": "before_execution", "reviewer": "designated_reviewer", "timestamp_state": "current"}
]`

func TestRun_OutputDashEmitsOnlyJSONOnStdout(t *testing.T) {
	pol := writeFixture(t, "policy.json", cliPolicyJSON)
	req := writeFixture(t, "requests.json", cliRequestsJSON)
	ev := writeFixture(t, "evidence.json", cliEvidenceJSON)

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--policy", pol,
		"--requests", req,
		"--evidence", ev,
		"--output", "-",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() == 0 {
		t.Fatalf("expected human progress on stderr, got none")
	}
	var pack jsonio.OutputPack
	if err := json.Unmarshal(stdout.Bytes(), &pack); err != nil {
		t.Fatalf("stdout is not pure OutputPack JSON: %v\nstdout: %q", err, stdout.String())
	}
	if pack.PolicyName != "cli-policy" || pack.PolicyVersion != "7.0.0" {
		t.Fatalf("pack metadata = %q/%q, want cli-policy/7.0.0", pack.PolicyName, pack.PolicyVersion)
	}
	if len(pack.Results) != 1 || pack.Results[0].RequestID != "REQ-1" {
		t.Fatalf("results = %+v, want one REQ-1 result", pack.Results)
	}
	if strings.Contains(stdout.String(), "Loaded Policy") || strings.Contains(stdout.String(), "Request ") {
		t.Fatalf("stdout contains human text mixed into JSON: %q", stdout.String())
	}
}

func TestRun_FileOutputWritesNestedPathAndReportsOnStderr(t *testing.T) {
	pol := writeFixture(t, "policy.json", cliPolicyJSON)
	req := writeFixture(t, "requests.json", cliRequestsJSON)
	ev := writeFixture(t, "evidence.json", cliEvidenceJSON)
	base := t.TempDir()
	outPath := filepath.Join(base, "a", "b", "requests.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--policy", pol,
		"--requests", req,
		"--evidence", ev,
		"--output", outPath,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty for file output", stdout.String())
	}
	if !strings.Contains(stderr.String(), outPath) {
		t.Fatalf("stderr does not report output path %q: %s", outPath, stderr.String())
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("output file not written at nested path: %v", err)
	}
	var pack jsonio.OutputPack
	if err := json.Unmarshal(data, &pack); err != nil {
		t.Fatalf("output file is not valid JSON: %v", err)
	}
	if len(pack.Results) != 1 || pack.Results[0].RequestID != "REQ-1" {
		t.Fatalf("output file results = %+v, want one REQ-1 result", pack.Results)
	}
}

func TestRun_LoadFailureReturnsNonzeroExit(t *testing.T) {
	pol := writeFixture(t, "policy.json", cliPolicyJSON)
	ev := writeFixture(t, "evidence.json", cliEvidenceJSON)
	badReq := writeFixture(t, "requests.json", `[{"id": "REQ-1", "bogus_field": 1}]`)

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--policy", pol,
		"--requests", badReq,
		"--evidence", ev,
		"--output", "-",
	}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("run exit code = 0, want nonzero on load failure")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on load failure", stdout.String())
	}
	if !strings.Contains(stderr.String(), "requests") {
		t.Fatalf("stderr does not mention requests load error: %s", stderr.String())
	}
}

func writeFixtureRel(t *testing.T, base, rel, content string) string {
	t.Helper()
	path := filepath.Join(base, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	return path
}

func TestRun_DefaultsUseShippedPaths(t *testing.T) {
	base := t.TempDir()
	writeFixtureRel(t, base, "policies/policy.json", cliPolicyJSON)
	writeFixtureRel(t, base, "fixtures/requests.json", cliRequestsJSON)
	writeFixtureRel(t, base, "fixtures/evidence.json", cliEvidenceJSON)
	t.Chdir(base)

	var stdout, stderr bytes.Buffer
	code := run([]string{}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty for file output", stdout.String())
	}
	data, err := os.ReadFile(filepath.Join(base, "results", "requests.json"))
	if err != nil {
		t.Fatalf("default results file not written: %v", err)
	}
	var pack jsonio.OutputPack
	if err := json.Unmarshal(data, &pack); err != nil {
		t.Fatalf("default results file is not valid OutputPack JSON: %v", err)
	}
	if pack.PolicyName != "cli-policy" || pack.PolicyVersion != "7.0.0" {
		t.Fatalf("pack metadata = %q/%q, want cli-policy/7.0.0", pack.PolicyName, pack.PolicyVersion)
	}
	if len(pack.Results) != 1 || pack.Results[0].RequestID != "REQ-1" {
		t.Fatalf("default results = %+v, want one REQ-1 result", pack.Results)
	}
	if !strings.Contains(stderr.String(), "results/requests.json") {
		t.Fatalf("stderr does not report default output path: %s", stderr.String())
	}
}

func TestRun_RejectsPositionalArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"unexpected.json"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run exit code = %d, want 2; stderr: %s", code, stderr.String())
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "positional") {
		t.Fatalf("stdout=%q stderr=%q, want positional-argument error on stderr", stdout.String(), stderr.String())
	}
}

func TestRun_FailuresReportBoundaryOnStderr(t *testing.T) {
	pol := writeFixture(t, "policy.json", cliPolicyJSON)
	req := writeFixture(t, "requests.json", cliRequestsJSON)
	ev := writeFixture(t, "evidence.json", cliEvidenceJSON)
	missing := filepath.Join(t.TempDir(), "missing.json")

	tests := []struct {
		name string
		args []string
		want int
		sub  string
	}{
		{
			name: "unknown flag",
			args: []string{"--bogus-flag", "x"},
			want: 2,
			sub:  "bogus-flag",
		},
		{
			name: "missing policy",
			args: []string{"--policy", missing, "--requests", req, "--evidence", ev, "--output", "-"},
			want: 1,
			sub:  "Error loading policy AST",
		},
		{
			name: "missing evidence",
			args: []string{"--policy", pol, "--requests", req, "--evidence", missing, "--output", "-"},
			want: 1,
			sub:  "Error loading evidence",
		},
		{
			name: "output parent is a file",
			args: []string{"--policy", pol, "--requests", req, "--evidence", ev, "--output", filepath.Join(pol, "out.json")},
			want: 1,
			sub:  "Error writing results",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tt.args, &stdout, &stderr)
			if code != tt.want {
				t.Fatalf("exit code = %d, want %d; stderr: %s", code, tt.want, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.sub) {
				t.Fatalf("stderr %q does not identify the failing boundary %q", stderr.String(), tt.sub)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty on failure", stdout.String())
			}
		})
	}
}

func BenchmarkEvaluateCLISuppliedPack(b *testing.B) {
	args := []string{
		"--policy", "../../policies/policy.json",
		"--requests", "../../fixtures/requests.json",
		"--evidence", "../../fixtures/evidence.json",
		"--output", "-",
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if code := run(args, io.Discard, io.Discard); code != 0 {
			b.Fatalf("run() exit code = %d", code)
		}
	}
}

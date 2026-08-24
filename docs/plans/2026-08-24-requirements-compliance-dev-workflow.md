# Requirements Compliance And Reviewer Workflow Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the exercise fully conform to `Requirements.md`, add deterministic evidence-aware decisions, and provide local, repository-local Go, Docker, Make, fzf, and Bash reviewer workflows.

**Architecture:** Extend the existing JSON policy AST with applicability, explicit state resolution, complete evidence qualifiers, and structured remediation. Evaluate all applicable clauses, collect findings, and resolve them with `Reject > Escalate > Revise > Approve`; keep JSON, CLI, Make, Bash, and Docker as adapters around that evaluator.

**Tech Stack:** Go 1.27 standard library, JSON policy/fixtures, GNU Make, Bash, optional fzf, and an optional multi-stage Docker build.

---

Do not create Git commits unless the user explicitly requests them. Every test,
build, vet, Docker build, and script verification command below must retain its
explicit timeout.

### Task 1: Add Semantic Regression Tests

**Files:**
- Modify: `pkg/policy/evaluator_test.go`

**Step 1: Replace decision-only assertions with full result assertions**

Add helpers that load `policies/policy.json`, evaluate a request, and compare:

```go
func requireDecision(t *testing.T, got EvaluationResult, want Decision) {
	t.Helper()
	if got.Decision != want {
		t.Fatalf("decision = %s, want %s; result: %+v", got.Decision, want, got)
	}
}

func requireStrings(t *testing.T, field string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("%s = %v, want %v", field, got, want)
	}
}
```

Assert exact `requirements_applied`, `evidence_used`, missing/conflicting
evidence, uncertainty, and remediation for R1-R5.

**Step 2: Add failing boundary cases**

Add table cases for:

```text
external + above_standard_limit + E1/E2/E3 -> Revise (lower usage)
aggregate_analysis + individual_records -> Reject
trusted internal + above limit + E2 only -> Escalate (missing E1 wins)
trusted internal + wrong adjustment type -> Revise
valid E1/E2 + missing referenced E_GHOST -> Escalate
unknown action/output/trust/usage -> Escalate
irrelevant conflicting usage evidence on standard external request -> Approve
```

**Step 3: Run the focused test and verify failure**

Run:

```bash
timeout 60s go test -count=1 -timeout 60s ./pkg/policy -run TestEvaluator -v
```

Expected: FAIL on the new semantic cases and provenance assertions.

### Task 2: Extend And Validate The Policy AST

**Files:**
- Modify: `pkg/policy/ast.go`
- Modify: `pkg/policy/types.go`
- Modify: `policies/policy.json`
- Create: `pkg/policy/ast_test.go`

**Step 1: Write failing policy-validation tests**

Cover duplicate requirement/clause IDs, unknown clause kinds, invalid decisions,
missing applicability, missing evidence type, and unsupported remediation actions.

**Step 2: Run the AST tests and verify failure**

```bash
timeout 60s go test -count=1 -timeout 60s ./pkg/policy -run 'Test(Load|Validate)Policy' -v
```

Expected: FAIL because policy validation is not implemented.

**Step 3: Add bounded semantic AST types**

Use explicit types rather than generic maps:

```go
type ApplicabilityAST struct {
	Datasets    []string       `json:"datasets,omitempty"`
	TrustLevels []RequesterTrust `json:"trust_levels,omitempty"`
	UsageLimits []UsageLimit   `json:"usage_limits,omitempty"`
}

type ResolutionAST struct {
	Satisfied    Decision `json:"satisfied"`
	False        Decision `json:"false"`
	Missing      Decision `json:"missing"`
	Invalid      Decision `json:"invalid"`
	Stale        Decision `json:"stale"`
	Unclear      Decision `json:"unclear"`
	Unverifiable Decision `json:"unverifiable"`
	Conflict     Decision `json:"conflict"`
}

type RationaleAST struct {
	False        string `json:"false,omitempty"`
	Missing      string `json:"missing,omitempty"`
	Invalid      string `json:"invalid,omitempty"`
	Stale        string `json:"stale,omitempty"`
	Unclear      string `json:"unclear,omitempty"`
	Unverifiable string `json:"unverifiable,omitempty"`
	Conflict     string `json:"conflict,omitempty"`
}
```

Add requirement applicability, allowed actions/outputs/trust levels, complete
evidence qualifiers (`required_reviewer`, `required_adjustment_type`), explicit
resolution, rationales, and a remediation slice to `ClauseAST`.

Extend `Remediation` with optional `field` and `value` fields so lowering usage
is represented as data.

**Step 4: Implement strict decoding and validation**

Use `json.Decoder.DisallowUnknownFields`, require one JSON document, call
`Validate`, and reject duplicate requirement/clause IDs or unsupported values.

Supported clause kinds are:

```text
disclosure_constraint
trusted_usage
required_environment
required_evidence
```

Supported remediation actions are `add_evidence` and `set_field`.

**Step 5: Rewrite `policies/policy.json`**

Model:

```text
R1 applies to external + protected_dataset:
  aggregate_analysis + aggregate_counts, false -> Reject
  valid current designated-reviewer pre-execution approval, unresolved -> Escalate

R2 applies to protected_dataset:
  local_approved_env + verified valid attestation, any failure -> Escalate

R3 applies to protected_dataset + above_standard_limit:
  trust must be trusted_internal, false -> Revise(set usage_limit=standard)
  disclosure constraint, false -> Reject
  valid current designated-reviewer pre-execution approval, unresolved -> Escalate
  approved current designated-reviewer usage adjustment for
  above_standard_limit/trusted_internal_only, missing or clearly invalid -> Revise,
  stale/unclear/unverifiable/conflicting -> Escalate
```

**Step 6: Run AST tests**

```bash
timeout 60s go test -count=1 -timeout 60s ./pkg/policy -run 'Test(Load|Validate)Policy' -v
```

Expected: PASS.

### Task 3: Refactor Evaluation Into Findings And Resolution

**Files:**
- Modify: `pkg/policy/evaluator.go`
- Modify: `pkg/policy/evaluator_test.go`

**Step 1: Add failing precedence and deterministic-order assertions**

Assert that simultaneous E1/E3 absence selects Escalate, R5 attributes R2/R3,
and evidence output follows `Request.EvidenceIDs` order on repeated evaluation.

**Step 2: Run focused tests and verify failure**

```bash
timeout 60s go test -count=1 -timeout 60s ./pkg/policy -run TestEvaluator -v
```

Expected: FAIL under the early-return evaluator.

**Step 3: Add internal evaluation states**

Use a compact internal reason enum and one finding record:

```go
type evaluationReason uint8

const (
	reasonSatisfied evaluationReason = iota
	reasonFalse
	reasonMissing
	reasonInvalid
	reasonStale
	reasonUnclear
	reasonUnverifiable
	reasonConflict
)

type clauseFinding struct {
	requirementID string
	clauseID      string
	reason        evaluationReason
	decision      Decision
	rationale     string
	remediation   []Remediation
}
```

**Step 4: Resolve evidence without map-order output**

Build lookup maps once, but inspect and emit evidence in request order. For each
applicable evidence clause:

```text
conflict markers -> conflict
stale timestamp -> stale
unclear/incomplete state -> unclear
revoked/expired or clear qualifier mismatch -> invalid
all required qualifiers match -> satisfied
no evidence of the required type -> missing
```

Only evidence kinds required by applicable clauses can influence a decision.
A referenced ID absent from the pack creates a global Escalate finding.

**Step 5: Evaluate every applicable clause**

Do not return from clause passes. Append each applicable requirement once, mark
relevant evidence IDs, and collect candidate findings.

**Step 6: Resolve precedence and project the result**

Use a fixed rank function:

```go
func decisionRank(d Decision) uint8 {
	switch d {
	case DecisionReject:
		return 4
	case DecisionEscalate:
		return 3
	case DecisionRevise:
		return 2
	case DecisionApprove:
		return 1
	default:
		return 0
	}
}
```

Keep the first policy-ordered finding at equal rank. Generate the final rationale
from the winning clause, or the generic all-requirements-satisfied rationale for
Approve. Generate missing/conflicting descriptions from actual evidence type and
reason rather than a hardcoded evidence ID.

**Step 7: Run all policy tests**

```bash
timeout 60s go test -count=1 -timeout 60s ./pkg/policy -v
```

Expected: PASS.

### Task 4: Make JSON Loaders Strict And Output Policy-Derived

**Files:**
- Modify: `pkg/format/json.go`
- Create: `pkg/format/json_test.go`
- Modify: `cmd/verifoxx/main.go`
- Create: `cmd/verifoxx/main_test.go`

**Step 1: Write failing loader and CLI tests**

Cover unknown JSON fields, trailing JSON, duplicate request/evidence IDs, output
metadata matching a custom policy, `--output -`, missing output directories, and
no human text mixed into stdout JSON.

**Step 2: Run focused tests and verify failure**

```bash
timeout 60s go test -count=1 -timeout 60s ./pkg/format ./cmd/verifoxx -v
```

Expected: FAIL.

**Step 3: Add strict decode helpers and duplicate checks**

Use one internal generic decoder with `DisallowUnknownFields` and EOF checking.
Reject empty or duplicate IDs with indexed error messages.

**Step 4: Split result encoding from file output**

Change result construction to accept policy metadata:

```go
func NewOutputPack(ast *policy.PolicyAST, results []policy.EvaluationResult) OutputPack
func EncodeResults(w io.Writer, pack OutputPack) error
func WriteResults(filePath string, pack OutputPack) error
```

Write files through a temporary file in the destination directory, close it, and
rename it over the destination. Create the parent directory when absent.

**Step 5: Refactor CLI into a testable runner**

Implement:

```go
func run(args []string, stdout, stderr io.Writer) int
```

`main` calls `os.Exit(run(...))`. Progress and the human table go to stderr.
When `--output -` is selected, encode only JSON to stdout. Otherwise write the
file and report its path on stderr.

**Step 6: Run loader and CLI tests**

```bash
timeout 60s go test -count=1 -timeout 60s ./pkg/format ./cmd/verifoxx -v
```

Expected: PASS.

### Task 5: Add Golden Conformance And Demo Fixtures

**Files:**
- Create: `fixtures/demo/requests.json`
- Create: `fixtures/demo/evidence.json`
- Create: `fixtures/demo/expected.json`
- Create: `pkg/policy/conformance_test.go`
- Modify: `results/requests.json`

**Step 1: Add the edge-case fixture pack**

Include named requests for external above-limit, individual output, both
approvals missing, wrong adjustment qualifier, missing referenced ID, unknown
semantic value, and irrelevant conflicting evidence.

**Step 2: Write a failing conformance test**

Load the supplied and demo fixtures through production loaders. Assert the exact
decision sequence and exact deterministic JSON against:

```text
results/requests.json
fixtures/demo/expected.json
```

**Step 3: Run and verify the golden mismatch**

```bash
timeout 60s go test -count=1 -timeout 60s ./pkg/policy -run TestConformance -v
```

Expected: FAIL until golden files are regenerated from corrected behavior.

**Step 4: Regenerate both golden outputs using the CLI**

```bash
timeout 60s go run ./cmd/verifoxx --policy policies/policy.json --requests fixtures/requests.json --evidence fixtures/evidence.json --output results/requests.json
timeout 60s go run ./cmd/verifoxx --policy policies/policy.json --requests fixtures/demo/requests.json --evidence fixtures/demo/evidence.json --output fixtures/demo/expected.json
```

Expected: both commands exit 0.

**Step 5: Run conformance and full Go tests**

```bash
timeout 60s go test -count=1 -timeout 60s ./...
```

Expected: PASS.

### Task 6: Add Local Toolchain And Doctor Scripts

**Files:**
- Create: `scripts/go.sh`
- Create: `scripts/install-go.sh`
- Create: `scripts/doctor.sh`
- Create: `scripts/gofmt-check.sh`
- Modify: `.gitignore`

**Step 1: Add non-interactive dry-run interfaces**

`install-go.sh --dry-run` must report an existing-Go no-op, or print the detected
platform, official archive URL, checksum URL, and repository-local destination
without downloading. `doctor.sh` must report each dependency and use a nonzero
exit only when the local workflow cannot run.

**Step 2: Implement the Go wrapper**

Prefer `.tools/go/bin/go`, otherwise execute `go` from PATH, otherwise print:

```text
Go 1.27+ was not found. Run: make install-go
```

**Step 3: Implement repository-local installation**

Pin Go 1.27.0 by default, detect linux/darwin and amd64/arm64, download the
official archive plus `.sha256`, verify with `sha256sum` or `shasum -a 256`,
extract into a temporary directory, and atomically replace `.tools/go`. Never
invoke `sudo`.

**Step 4: Implement doctor and format check**

Doctor reports Bash, Make, Go version, Docker, curl, tar, checksum utility, and
optional fzf. The format script asks the selected Go toolchain for `GOROOT` and
runs its sibling `gofmt -l` over `cmd` and `pkg`.

**Step 5: Ignore generated local tools**

Add `.tools/` and temporary demo output paths to `.gitignore`.

**Step 6: Verify scripts without network installation**

```bash
timeout 20s bash -n scripts/go.sh scripts/install-go.sh scripts/doctor.sh scripts/gofmt-check.sh
timeout 20s scripts/install-go.sh --dry-run
timeout 20s scripts/doctor.sh
```

Expected: syntax passes; dry-run identifies the host; doctor reports statuses.

### Task 7: Expand Make And Interactive Bash Workflows

**Files:**
- Modify: `Makefile`
- Create: `scripts/menu.sh`
- Create: `scripts/demo.sh`

**Step 1: Preserve existing targets and add the new surface**

Keep `build`, `test`, `eval`, `vet`, `clean`, and `help`. Add:

```text
targets setup doctor install-go fmt-check check menu demo demo-edge
docker-build docker-eval docker-demo
compose-build compose-eval compose-demo
```

Use `GO := ./scripts/go.sh` so all local targets use the repository-local
toolchain when installed. `clean` must not delete tracked `results/requests.json`.

**Step 2: Implement the menu**

Every public Make target carries an inline `##` description. `make help`,
`make targets`, and `scripts/menu.sh --list` derive from those descriptions, so
the target list is not duplicated. The interactive path uses fzf with dependency
readiness and recipe previews when available, or the same generated rows in a
numbered prompt otherwise. The selected entry executes
`make -C "$root" <target>`.

**Step 3: Implement the demo**

The normal mode prints section headers and evaluates both supplied and edge packs
into temporary files removed by `trap`, comparing each byte-for-byte with its
tracked golden. `--edge-only` skips the supplied pack. The Make `demo` target
depends on `check`; `demo-edge` depends on `build`.

**Step 4: Verify script and Make entry points**

```bash
timeout 20s bash -n scripts/menu.sh scripts/demo.sh
timeout 20s scripts/menu.sh --list
timeout 60s make help
timeout 120s make check
timeout 120s make demo
timeout 60s make demo-edge
```

Expected: all commands exit 0 and both demos display their decision tables.

### Task 8: Add Optional Docker Execution

**Files:**
- Create: `Dockerfile`
- Create: `.dockerignore`
- Create: `scripts/docker-demo.sh`
- Modify: `Makefile`

**Step 1: Add a multi-stage Dockerfile**

Use `golang:1.27` as the builder, run the bounded Go tests, build a static binary,
then copy the binary, policy, supplied fixtures, and demo fixtures into a scratch
runtime image. Configure the default command to evaluate the supplied pack with
`--output -`.

**Step 2: Add Docker demo orchestration**

The script builds once, runs the default supplied evaluation, then overrides the
request/evidence flags for the edge pack. JSON goes to stdout and progress goes
to stderr.

**Step 3: Add Docker Make targets**

Use an overridable image name such as `verifoxx:local`. `docker-demo` delegates to
the script; `docker-eval` runs only the supplied pack.

**Step 4: Verify syntax and build when Docker is available**

```bash
timeout 20s bash -n scripts/docker-demo.sh
timeout 20s docker version
timeout 180s make docker-build
timeout 60s make docker-demo
```

Expected: if the daemon is available, image build and both evaluations pass. If
Docker is unavailable, record that optional verification as skipped with the
daemon error; local verification remains mandatory.

### Task 9: Rewrite Submission Documentation

**Files:**
- Modify: `README.md`
- Modify: `docs/DESIGN_NOTE.md`
- Delete: `docs/plans/2026-08-23-production-evolution-design.md`

**Step 1: Rewrite README around the actual submission**

Document:

```text
Go 1.27+, Bash, Make; fzf and Docker optional
make demo as the first reviewer command
make menu, setup, doctor, install-go, check, eval, demo-edge, Docker targets
direct CLI invocation and --output -
policy/request/evidence/output JSON field tables
R1-R5 expected decisions
repository structure
AI-tool use statement
```

Add this follow-on wording with a replaceable link:

```markdown
## Follow-On Project

The ideas in this exercise were later taken further in **Verifoxx**, a separate
production-oriented policy-engine repository. GitHub:
[Verifoxx repository](https://github.com/sebishogun/Verifoxx).
```

Remove the nonexistent LICENSE link and all claims of implemented SIMD,
zero-allocation execution, or measured throughput.

**Step 2: Correct the one-page design note**

Keep the required five topics: representation, why it exceeds flat extraction,
decision logic, escalation boundaries, and next improvements. Describe only code
present in this repository and keep the note concise enough for one rendered
page.

**Step 3: Remove the unsupported production whitepaper**

The README follow-on section replaces it. Do not leave benchmark numbers or
package paths that belong only to the separate repository.

**Step 4: Verify every documented local command**

```bash
timeout 60s make help
timeout 20s make doctor
timeout 120s make check
timeout 120s make demo
```

Expected: all mandatory local commands work as documented.

### Task 10: Final Compliance And Repository Verification

**Files:**
- Review: `Requirements.md`
- Review: all changed files

**Step 1: Run formatting**

```bash
timeout 60s gofmt -w cmd pkg
```

Expected: exit 0.

**Step 2: Run fresh tests**

```bash
timeout 60s go test -count=1 -timeout 60s ./...
```

Expected: PASS for every package.

**Step 3: Run vet and build**

```bash
timeout 60s go vet ./...
timeout 60s go build -o /tmp/opencode/verifoxx-final ./cmd/verifoxx
```

Expected: both exit 0.

**Step 4: Run command-surface verification**

```bash
timeout 120s make check
timeout 120s make demo
timeout 60s make demo-edge
timeout 20s scripts/menu.sh --list
timeout 20s scripts/install-go.sh --dry-run
```

Expected: all exit 0.

**Step 5: Verify generated result reproducibility**

Run evaluation to a temporary file and compare it with the committed result:

```bash
timeout 60s /tmp/opencode/verifoxx-final --policy policies/policy.json --requests fixtures/requests.json --evidence fixtures/evidence.json --output /tmp/opencode/verifoxx-results.json
cmp results/requests.json /tmp/opencode/verifoxx-results.json
```

Expected: `cmp` exits 0.

**Step 6: Check repository state and requirement matrix**

```bash
git status --short
git diff --check
```

Expected: only intended implementation/documentation changes; no whitespace
errors, generated binaries, local toolchains, or temporary demo output.

Confirm each mandatory item in `Requirements.md` against a concrete file or test
before reporting completion.

### Task 11: Install A Global Cross-Shell `mm` Command

**Files:**
- Create: `cmd/mm/main.go`
- Create: `cmd/mm/main_test.go`
- Modify: `Makefile`
- Modify: `scripts/menu-selftest.sh`
- Modify: `README.md`
- Modify: `docs/plans/2026-08-24-requirements-compliance-dev-workflow-design.md`

**Step 1: Write failing command-discovery tests**

Test that `mm` starts at the current directory, searches parent directories,
skips nearer Makefiles without a direct `menu` target, selects the nearest
matching project, passes terminal streams to `make`, propagates its exit code,
and reports a useful error when no project menu exists.

**Step 2: Run the focused tests and verify failure**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./cmd/mm -v
```

Expected: FAIL because `cmd/mm` does not exist.

**Step 3: Implement project-independent menu discovery**

Add a host-native `mm` command that recognizes the Makefile GNU Make would pick
(`GNUmakefile`, `makefile`, then `Makefile`), ignores comments and recipe lines
while detecting a direct `menu` target, and invokes:

```text
make --no-print-directory -C <nearest-project> menu
```

The process must preserve stdin, stdout, and stderr and return the Make exit
status. Normal invocation accepts no positional arguments; `--install` performs
the idempotent self-install used by `make shell`.

**Step 4: Write failing installer tests**

Use temporary home/config directories and a fake command runner. Assert:

```text
Unix destination: ~/.local/bin/mm
Windows destination: %LOCALAPPDATA%/mm/bin/mm.exe
Bash and Zsh marker blocks are appended without replacing existing content
Fish, PowerShell, and Nushell profile snippets add the same user bin
repeated installation does not duplicate or rewrite unchanged configuration
Windows user PATH setup is requested once and preserves its existing value
```

**Step 5: Implement idempotent cross-shell installation**

Copy the running host-native executable through a temporary file, compare it
with an existing installation before replacement, and preserve user-owned
profile content. Unix profile setup covers Bash, Zsh, Fish, PowerShell, and
Nushell. Windows updates the per-user PATH, making `mm.exe` visible to
PowerShell, cmd.exe, Nushell, and Unix-style shells after a new terminal starts.
Print the installed path and the exact reload requirement; never claim that a
child Make process changed the current parent shell.

**Step 6: Add the Make and menu surfaces**

Add this described phony target so it automatically appears in help and both
menu renderers:

```make
shell: ## Install the global cross-shell mm shortcut
	$(GO) run ./cmd/mm --install
```

Extend the menu readiness mapping so `shell` reports the Go prerequisite. Add a
menu self-test assertion that `shell` appears in generated discovery output.

**Step 7: Run focused and cross-platform verification**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./cmd/mm -v
timeout 60s env GOOS=windows GOARCH=amd64 ./scripts/go.sh build -o /tmp/opencode/mm.exe ./cmd/mm
timeout 20s scripts/menu-selftest.sh
timeout 20s make help
```

Expected: tests pass, the Windows executable compiles, and `shell` appears in
the generated Make/menu target list.

**Step 8: Document installation and the live follow-on repository**

Document `make shell`, the new-terminal requirement when PATH changes, nearest
project lookup, all supported shells, idempotence, and continued availability of
`make menu`. Use the canonical follow-on URL exactly:

```text
https://github.com/sebishogun/Verifoxx
```

**Step 9: Run the complete regression suite**

```bash
timeout 120s make check
timeout 120s make demo
git diff --check
```

Expected: PASS with no generated helper binary or temporary profile fixture in
the repository.

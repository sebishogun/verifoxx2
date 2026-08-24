# Compiled Scalar Policy Engine Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the interpreted, map-backed evaluator with one compact compile-once, columnar scalar engine while preserving every existing decision and machine-readable result.

**Architecture:** Strict JSON adapters decode a small generic source AST. A compiler validates and interns that AST into an immutable numeric SoA Program. A reusable builder creates columnar request/evidence batches, and a caller-owned scalar evaluator writes numeric results with zero warm allocations before a cold adapter materializes the existing JSON contract.

**Tech Stack:** Go 1.27 standard library, generic policy JSON, typed integer IDs, SoA columns, CSR edges, uint64 truth/reason bitplanes, GNU Make, Bash, Docker, and Docker Compose.

---

Do not create Git commits unless the user explicitly requests them. Do not use
subagents. Every test, benchmark, build, vet, Docker, and Compose invocation
must carry an explicit timeout. Preserve the current dirty worktree and do not
revert unrelated changes.

The approved design is
`docs/plans/2026-08-24-compiled-scalar-engine-design.md`. The final engine must
link to `https://github.com/sebishogun/Verifoxx` and must not claim SIMD,
parallel scheduling, persistence, or service features.

### Task 1: Freeze Interpreter Semantics And Baseline Costs

**Files:**
- Create: `pkg/policy/evaluator_bench_test.go`
- Modify: `pkg/policy/conformance_test.go`
- Review: `results/requests.json`
- Review: `fixtures/demo/expected.json`

**Step 1: Add a failing completeness assertion to conformance**

Extend the conformance helper to assert all output fields, not only IDs,
decisions, and applied requirements. Decode the tracked golden and compare each
production `EvaluationResult` to its complete expected result.

**Step 2: Run the focused conformance tests**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./pkg/policy -run '^TestConformance' -v
```

Expected: FAIL if any field was not previously compared; otherwise confirm the
test protects the complete existing contract before migration.

**Step 3: Add temporary interpreter benchmarks**

Benchmark the supplied five-request pack with setup outside the timed loop:

```go
func BenchmarkInterpreterSuppliedPack(b *testing.B) {
	// Load AST, requests, and evidence once.
	// Evaluate all five requests per benchmark operation.
	b.ReportAllocs()
}
```

Keep one result in a package sink so evaluation cannot be removed.

**Step 4: Measure the baseline**

```bash
timeout 90s env GOMAXPROCS=1 ./scripts/go.sh test -run '^$' -bench '^BenchmarkInterpreterSuppliedPack$' -benchmem -benchtime=500ms -count=6 -timeout 60s ./pkg/policy
```

Expected: six valid samples. Record their range in the migration notes; do not
make a production claim from the five-row fixture.

### Task 2: Add Typed IDs And The Generic Source AST

**Files:**
- Create: `internal/schema/ids.go`
- Create: `internal/schema/fields.go`
- Create: `internal/schema/ids_test.go`
- Create: `internal/ast/policy.go`
- Create: `internal/ast/expression.go`
- Create: `internal/ast/validate.go`
- Create: `internal/ast/validate_test.go`

**Step 1: Write failing typed-ID and AST tests**

Define the desired API in tests. ID zero must be invalid and every domain must
be a distinct Go type:

```go
type FieldID uint16
type NodeID uint32
type InstructionID uint32
type SymbolID uint32
type ValueID uint32
type EvidenceKindID uint16
type OutcomeID uint16
type ReasonID uint8
type RequirementID uint16
type ClauseID uint16
type RemediationID uint16
type ExplanationID uint16
```

Test source expressions for `all`, `equal`, `in`, and `evidence_matches`.
Validation tests must reject unsupported operators, wrong arity, empty literal
sets, evidence expressions in applicability, duplicate IDs, incomplete
resolution tables, non-approval states without explanations, and invalid
remediation payloads.

**Step 2: Verify RED**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/schema ./internal/ast -v
```

Expected: FAIL because the packages and APIs do not exist.

**Step 3: Implement the minimum source types**

Use explicit fields instead of untyped maps:

```go
type Expression struct {
	Op       Operator          `json:"op"`
	Field    string            `json:"field,omitempty"`
	Value    string            `json:"value,omitempty"`
	Values   []string          `json:"values,omitempty"`
	Children []Expression      `json:"children,omitempty"`
	Evidence EvidencePredicate `json:"evidence,omitempty"`
}

type EvidencePredicate struct {
	Kind             string `json:"kind"`
	Status           string `json:"status,omitempty"`
	Timing           string `json:"timing,omitempty"`
	Reviewer         string `json:"reviewer,omitempty"`
	TimestampState   string `json:"timestamp_state,omitempty"`
	Subject          string `json:"subject,omitempty"`
	AttestationState string `json:"attestation_state,omitempty"`
	Scope            string `json:"scope,omitempty"`
	AdjustmentType   string `json:"adjustment_type,omitempty"`
}
```

Keep applicability and assertion roots separate. Keep all eight source
resolution fields so the source representation remains explicit.

**Step 4: Implement bounded validation**

Validate each operator's allowed and required fields. Use iterative node
counting or bounded recursion with a maximum depth and maximum node count.
Reject whitespace-only IDs. Require a rationale for every state resolving to a
non-Approve outcome. Validate `add_evidence` and `set_field` payloads against
the known source vocabulary.

**Step 5: Verify GREEN**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/schema ./internal/ast -v
```

Expected: PASS.

### Task 3: Add Strict Policy JSON And The Generic Verifoxx Pack

**Files:**
- Create: `internal/adapters/jsonio/decode.go`
- Create: `internal/adapters/jsonio/policy_test.go`
- Create: `testdata/compiled/policy.json`
- Create: `testdata/reference/policy.json`

**Step 1: Preserve the interpreted policy fixture**

Copy the current `policies/policy.json` semantics into
`testdata/reference/policy.json` for migration-only differential tests. Do not
switch the shipped policy yet.

**Step 2: Write failing strict-decoder tests**

Tests must reject unknown fields, trailing JSON, empty input, a second value,
oversized input, and every AST validation failure with document-path context.

**Step 3: Verify RED**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/adapters/jsonio -run 'Policy' -v
```

Expected: FAIL because the adapter is absent.

**Step 4: Implement one bounded strict decoder**

Use `io.LimitReader` with an explicit policy-byte limit,
`json.Decoder.DisallowUnknownFields`, one required document, and an EOF check.
Return the validated source policy only on complete success.

**Step 5: Author the generic policy pack**

Represent the existing requirements using generic expressions:

```text
R1 applies: All(dataset == protected_dataset, trust_level == external)
  disclosure: All(action == aggregate_analysis, output_kind == aggregate_counts)
  approval: EvidenceMatches(approval_record, valid/current/before/designated)

R2 applies: dataset == protected_dataset
  environment: All(environment == local_approved_env,
                   EvidenceMatches(execution_environment_attestation, ...))

R3 applies: All(dataset == protected_dataset,
                usage_limit == above_standard_limit)
  trust: trust_level In [trusted_internal]
  disclosure: All(action == aggregate_analysis, output_kind == aggregate_counts)
  approval: EvidenceMatches(approval_record, ...)
  adjustment: EvidenceMatches(usage_limit_adjustment, ...)
```

Copy all current state-specific outcomes, rationales, and remediations exactly.

**Step 6: Verify GREEN**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/adapters/jsonio ./internal/ast -v
```

Expected: PASS and the generic fixture validates.

### Task 4: Compile Expressions Into An Immutable Program

**Files:**
- Create: `internal/program/opcode.go`
- Create: `internal/program/program.go`
- Create: `internal/program/lookup.go`
- Create: `internal/compile/interner.go`
- Create: `internal/compile/compiler.go`
- Create: `internal/compile/compiler_test.go`

**Step 1: Write failing lowering tests**

Compile tiny source policies and assert exact instruction columns, postorder,
operand CSR ranges, requirement roots, clause roots, resolution outcome IDs,
symbol identity, and deterministic output across repeated compilation.

Desired initial program columns:

```go
type Program struct {
	Opcodes      []Opcode
	Fields       []schema.FieldID
	Values       []schema.SymbolID
	OperandStarts []uint32
	OperandCounts []uint16
	Operands      []schema.InstructionID

	EvidenceKinds []schema.EvidenceKindID
	EvidenceSpecs []EvidenceSpec

	RequirementApplicabilityRoots []schema.InstructionID
	RequirementClauseStarts       []uint32
	RequirementClauseCounts       []uint16
	RequirementClauseIDs          []schema.ClauseID
	ClauseRoots                   []schema.InstructionID
	ClauseOutcomes                []schema.OutcomeID
	ClauseExplanationIDs          []schema.ExplanationID

	SymbolBytes   []byte
	SymbolStarts  []uint32
	SymbolLengths []uint32
	OutcomePrecedence []uint8
}
```

Use separate parallel columns where an embedded struct would be scanned
column-wise. Small immutable cold catalog headers may use compact structs when
they are never part of a bulk kernel.

**Step 2: Verify RED**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/compile -v
```

Expected: FAIL because the compiler is absent.

**Step 3: Implement deterministic symbol interning**

The compiler may use a temporary `map[string]SymbolID`. Freeze symbols into one
byte slab plus starts and lengths. Add allocation-free exact lookup over the
frozen table for the batch builder; either freeze an open-addressed table or use
a measured bounded linear scan for this small catalogue. The evaluator must not
call symbol lookup.

**Step 4: Implement postorder lowering**

Emit every child before `All`. Emit one instruction for `Equal`, `In`, and
`EvidenceMatches`. Store variable operands and `In` values in CSR columns.
Translate source fields, evidence kinds, reasons, outcomes, explanations, and
remediations to typed IDs.

**Step 5: Verify GREEN and inspect escapes**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/compile -v
```

Expected: tests pass. Escape output is recorded for cold compiler ownership but
does not establish the evaluator contract.

### Task 5: Validate Frozen Program Shape

**Files:**
- Create: `internal/program/validate.go`
- Create: `internal/program/validate_test.go`
- Modify: `internal/compile/compiler.go`

**Step 1: Write failing corrupt-program tests**

Copy a valid tiny Program, corrupt one invariant at a time, and assert a bounded
error for mismatched parallel lengths, zero/out-of-range IDs, forward operands,
bad CSR ranges, invalid roots, incomplete resolution tables, malformed symbol
ranges, and invalid precedence entries.

**Step 2: Verify RED**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/program -v
```

Expected: FAIL because `Program.Validate` is absent.

**Step 3: Implement complete shape validation**

Use overflow-safe `uint64` range arithmetic. Return sentinel-compatible errors
with the failing column and index. Do not panic on malformed Program data.

**Step 4: Validate before compiler publication**

`compile.Compile` must discard the candidate and return a diagnostic if final
Program validation fails.

**Step 5: Verify GREEN**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/program ./internal/compile -v
```

Expected: PASS.

### Task 6: Build Columnar Request And Evidence Batches

**Files:**
- Create: `internal/input/types.go`
- Create: `internal/eval/batch.go`
- Create: `internal/eval/builder.go`
- Create: `internal/eval/builder_test.go`
- Create: `internal/adapters/jsonio/input.go`
- Create: `internal/adapters/jsonio/input_test.go`

**Step 1: Write failing layout and rollback tests**

Build two different batches into one reusable destination. Assert exact
column-major request values, presence masks, evidence columns, CSR offsets,
request-order evidence refs, semantic issue codes, and missing-record sentinel
edges. Poison every destination slice before reuse.

Failure cases must leave all destination lengths and prior values unchanged:
row/evidence/edge limits, duplicate input IDs, malformed evidence IDs, and
capacity/sizing overflow.

**Step 2: Verify RED**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/eval ./internal/adapters/jsonio -run '(Builder|Input)' -v
```

Expected: FAIL because the batch types do not exist.

**Step 3: Implement input DTOs and strict loading**

Keep the current request and evidence JSON shapes. Strictly reject unknown
fields, trailing values, empty/duplicate top-level IDs, and configured byte or
record limits. Unknown semantic values remain evaluable uncertainty rather than
a load error.

**Step 4: Implement transactional sizing and fill**

Perform one sizing pass before growing any destination. Size request columns as
`fieldCount * rows`, presence as `fieldCount * ceil(rows/64)`, evidence columns
as evidence count, and CSR edges as total referenced IDs. Fill every active
element and clear final-word tails.

The cold builder may create one evidence-ID map and use the Program symbol
lookup. It records, rather than rejects, a well-formed referenced evidence ID
whose record is absent.

**Step 5: Verify GREEN**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/eval ./internal/adapters/jsonio -run '(Builder|Input)' -v
```

Expected: PASS.

### Task 7: Add Reusable Truth And Reason Planes

**Files:**
- Create: `internal/eval/context.go`
- Create: `internal/eval/truth.go`
- Create: `internal/eval/scalar.go`
- Create: `internal/eval/scalar_test.go`

**Step 1: Write failing truth-kernel tests**

Test rows at `0, 1, 5, 63, 64, 65`. Verify exact positive, negative, unknown,
and conflict bits; dirty tail bits must be cleared. Test `Equal`, `In`, and
`All` over complete row slices. Unknown or absent request symbols must carry an
unclear/semantic reason without string formatting.

**Step 2: Verify RED**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/eval -run 'Truth|Scalar' -v
```

Expected: FAIL because context and kernels are absent.

**Step 3: Implement reusable Context sizing**

Flatten planes by instruction slot and mask word:

```go
type Context struct {
	Positive []uint64
	Negative []uint64
	Reasons  []uint64 // reason * instruction * words
	Rows     uint32
	Words    uint32
}
```

Provide a cold `Ensure(program, rows)` method and a non-allocating reset that
overwrites every active word. Do not use `sync.Pool`.

**Step 4: Implement scalar whole-plane kernels**

Kernels accept complete source/destination slices. `Equal` and `In` scan one
contiguous field column. `All` performs mask-word `AND`/`OR`. No function call
is made once per row through an interface or callback.

**Step 5: Verify GREEN**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/eval -run 'Truth|Scalar' -v
```

Expected: PASS at every bit boundary.

### Task 8: Evaluate Evidence Through CSR

**Files:**
- Create: `internal/eval/evidence.go`
- Create: `internal/eval/evidence_test.go`
- Modify: `internal/eval/scalar.go`

**Step 1: Write failing evidence-state tests**

Cover no record, missing reference, satisfied, wrong qualifier, revoked,
expired, stale, unclear/incomplete, explicit conflict, one valid plus one stale,
one valid plus one revoked, wrong subject, wrong scope, wrong reviewer, wrong
timing, and wrong adjustment type. Assert exact truth and reason planes.

**Step 2: Verify RED**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/eval -run 'Evidence' -v
```

Expected: FAIL because EvidenceMatches is not executed.

**Step 3: Implement CSR evidence reduction**

For each row, inspect only `EvidenceRefs[Offsets[row]:Offsets[row+1]]`. Compare
numeric kind and qualifier IDs. Preserve the current conservative state
priority and distinguish an absent referenced record from absence of the
required evidence kind.

**Step 4: Verify GREEN**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/eval -run 'Evidence' -v
```

Expected: PASS.

### Task 9: Resolve Requirements Into Caller-Owned Numeric Results

**Files:**
- Create: `internal/result/batch.go`
- Create: `internal/result/batch_test.go`
- Create: `internal/eval/executor.go`
- Create: `internal/eval/executor_test.go`

**Step 1: Write failing result-capacity tests**

Define the caller-owned result shape:

```go
type Batch struct {
	OutcomeIDs       []schema.OutcomeID
	DriverClauseIDs  []schema.ClauseID
	DriverReasonIDs  []schema.ReasonID
	RequirementOffsets []uint32
	RequirementIDs     []schema.RequirementID
	EvidenceOffsets    []uint32
	EvidenceRefs       []uint32
	IssueIDs           []schema.ReasonID
	RemediationOffsets []uint32
	RemediationIDs     []schema.RemediationID
}
```

Test insufficient capacity for every column. `EvaluateInto` must return a typed
capacity error before changing lengths or values.

**Step 2: Write failing semantic-resolution tests**

Using a compiled tiny policy and built batch, cover applicability false,
applicability unknown, no applicable requirement, all clause reasons, equal
precedence, and `Reject > Escalate > Revise > Approve`. Assert exact numeric
drivers and CSR provenance.

**Step 3: Verify RED**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/result ./internal/eval -run '(Result|Executor|Resolve|Capacity)' -v
```

Expected: FAIL because numeric resolution is absent.

**Step 4: Implement result sizing and reset**

Provide a cold `Ensure(program, batch)` upper-bound allocator and a warm reset.
Use maximum possible requirement rows, evidence edges, issues, and remediation
rows to guarantee success-path append cannot grow.

**Step 5: Implement deterministic row resolution**

Evaluate instructions once for the complete batch. Resolve rows in input order,
requirements in source order, and clauses in source order. Keep the first
candidate at equal precedence. Append only existing relevant evidence refs in
request order, deduplicated without a map by using a reusable numeric seen
generation array or bounded scan.

**Step 6: Verify GREEN**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/result ./internal/eval -v
```

Expected: PASS.

### Task 10: Materialize Exact Explanations And Output JSON

**Files:**
- Create: `internal/adapters/jsonio/output.go`
- Create: `internal/adapters/jsonio/output_test.go`
- Modify: `internal/program/program.go`
- Modify: `internal/compile/compiler.go`

**Step 1: Write failing output tests**

Materialize one result for every reason and remediation kind. Assert exact
bounded rationale, evidence issue, assumption, uncertainty, and structured
remediation text. Assert two-space deterministic JSON and one trailing newline.

**Step 2: Verify RED**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/adapters/jsonio -run 'Output|Materialize|Encode' -v
```

Expected: FAIL because numeric results cannot yet be encoded.

**Step 3: Freeze explanation catalogues**

Compile static rationale, uncertainty, and remediation text into Program-owned
byte slabs with typed IDs. Keep dynamic missing-reference and evidence-kind
details as bounded adapter templates keyed by numeric reason.

**Step 4: Implement cold materialization**

Build the existing output DTO after evaluation. It may allocate, but must size
all known result slices and encoded buffers. Keep human progress on stderr and
machine JSON on stdout for `--output -`.

**Step 5: Verify GREEN**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/adapters/jsonio -v
```

Expected: PASS.

### Task 11: Differentially Match The Interpreter

**Files:**
- Create: `internal/conformance/differential_test.go`
- Create: `internal/conformance/golden_test.go`
- Modify: `testdata/compiled/policy.json`
- Modify: compiler, evaluator, or materializer files only when a failing case identifies a semantic mismatch

**Step 1: Write the differential harness**

Load `testdata/reference/policy.json` through the interpreter and
`testdata/compiled/policy.json` through the compiler. Run every request from the
supplied and edge packs. Compare complete materialized results field for field.

**Step 2: Add generated combinations**

Generate bounded combinations of trust, action, output, environment, usage,
and evidence-state cases. Do not use random map iteration. Give each case a
stable name and compare interpreter and compiled results.

**Step 3: Verify RED or exact parity**

```bash
timeout 90s ./scripts/go.sh test -count=1 -timeout 60s ./internal/conformance -v
```

Expected: any mismatch fails with request input and complete old/new results.

**Step 4: Correct one semantic mismatch at a time**

For every mismatch, first retain the failing named test, then make the smallest
compiler/evaluator/materializer correction. Never weaken the current golden
contract merely to simplify the runtime.

**Step 5: Verify exact golden bytes**

```bash
timeout 90s ./scripts/go.sh test -count=1 -timeout 60s ./internal/conformance -v
```

Expected: complete differential parity and exact bytes for both tracked golden
files.

### Task 12: Switch The CLI And Remove The Interpreter

**Files:**
- Modify: `cmd/verifoxx/main.go`
- Modify: `cmd/verifoxx/main_test.go`
- Replace: `policies/policy.json` with the validated generic source policy
- Delete: `pkg/policy/ast.go`
- Delete: `pkg/policy/evaluator.go`
- Delete: `pkg/policy/types.go`
- Delete: obsolete `pkg/policy/*_test.go`
- Delete: `pkg/format/json.go`
- Delete: obsolete `pkg/format/json_test.go`
- Move or recreate: final conformance coverage under `internal/conformance`
- Modify: `Dockerfile`
- Modify: `scripts/gofmt-check.sh`

**Step 1: Write failing CLI compiled-path tests**

Retain tests for defaults, strict load failures, pure JSON stdout, nested atomic
output, boundary-specific stderr, and unknown flags. Add positional-argument
rejection and assert that the generic shipped policy is compiled exactly once
per invocation before all rows are evaluated as one batch.

**Step 2: Verify RED**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./cmd/verifoxx -v
```

Expected: new compiled-path or positional-argument assertions fail.

**Step 3: Replace the CLI pipeline**

Implement:

```text
decode policy -> compile Program
decode requests/evidence -> BuildInto Batch
Ensure Context and ResultBatch
EvaluateInto once for all rows
materialize OutputPack
encode stdout or atomically replace output file
```

Reject `flags.NArg() != 0` with exit code 2.

**Step 4: Remove the production interpreter**

After CLI and conformance parity, delete the old AST/evaluator/format packages,
temporary reference policy, differential test imports, and interpreter
benchmark. Keep final generic conformance tests and goldens only.

**Step 5: Update build inputs**

Docker copies `internal` instead of `pkg`. The format checker scans `cmd` and
`internal`. No obsolete package path remains in source or documentation.

**Step 6: Verify GREEN and absence**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./cmd/verifoxx ./internal/... -v
```

Expected: PASS. Searches for `type Evaluator struct`, `evaluateClause`, and the
old fixed clause kinds find no production interpreter implementation.

### Task 13: Prove Zero-Allocation Warm Evaluation

**Files:**
- Create: `internal/eval/executor_bench_test.go`
- Create: `internal/eval/allocation_test.go`
- Create: `internal/adapters/jsonio/benchmark_test.go`
- Modify: evaluator storage only in response to measured allocation or scaling evidence

**Step 1: Add the failing allocation contract**

Prime Program, Batch, Context, and ResultBatch outside `testing.AllocsPerRun`.
Evaluate repeatedly and require exactly zero allocations:

```go
if got := testing.AllocsPerRun(100, func() {
	if err := evaluator.EvaluateInto(&ctx, batch, &results); err != nil {
		panic(err)
	}
}); got != 0 {
	t.Fatalf("warm EvaluateInto allocations = %v, want 0", got)
}
```

**Step 2: Verify RED if allocations remain**

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/eval -run 'Allocation' -v
```

Expected: FAIL until all success-path growth, formatting, interfaces, and
escaping allocations are outside evaluation.

**Step 3: Remove measured allocations without pooling**

Use exact capacity sizing, caller-owned slabs, direct loops, and in-place reset.
Do not introduce `sync.Pool`. Inspect escapes when a source is unclear:

```bash
timeout 60s ./scripts/go.sh test -run '^$' -gcflags=-m ./internal/eval
```

**Step 4: Add scaling benchmarks**

Benchmark `1`, `5`, `64`, and `1,024` rows for warm evaluation. Separately
benchmark cold decode/compile, batch build, result materialization, and complete
CLI work. Every benchmark calls `ReportAllocs`.

**Step 5: Run six bounded samples**

```bash
timeout 120s env GOMAXPROCS=1 ./scripts/go.sh test -run '^$' -bench 'Benchmark(Evaluate|Compile|BuildBatch|Materialize)' -benchmem -benchtime=500ms -count=6 -timeout 90s ./internal/...
```

Expected: warm evaluation reports `0 B/op` and `0 allocs/op`; cold stages report
their measured costs without unsupported comparisons.

### Task 14: Harden Remaining Error Boundaries

**Files:**
- Modify: `scripts/install-go.sh`
- Create or modify: installer rollback regression under `scripts/`
- Modify: `internal/adapters/jsonio/output.go`
- Modify: relevant tests

**Step 1: Write a failing Go-installer rollback test**

Inject command shims and a temporary repository root so replacement fails after
the prior local toolchain is moved. Assert the original backup remains if
restoration fails and is restored when restoration succeeds.

**Step 2: Verify RED**

```bash
timeout 30s scripts/install-go-rollback-test.sh
```

Expected: FAIL because cleanup currently suppresses restore failure and then
deletes the backup.

**Step 3: Correct rollback ownership**

Delete the backup only after successful restoration or successful installation.
If restoration fails, preserve its exact path and report it on stderr.

**Step 4: Add output durability/error tests**

Retain destination-preservation and temporary-file cleanup tests. Verify encoder,
close, chmod, and rename failures return path context. Do not claim crash
durability unless directory sync is implemented and tested.

**Step 5: Verify GREEN**

```bash
timeout 30s scripts/install-go-rollback-test.sh
```

Expected: PASS.

### Task 15: Rewrite Architecture And Performance Documentation

**Files:**
- Modify: `README.md`
- Rewrite: `docs/DESIGN_NOTE.md`
- Create: `docs/architecture.md`
- Create: `docs/performance.md`
- Modify: `docs/plans/2026-08-24-compiled-scalar-engine-design.md` only if implementation differs from the approved design

**Step 1: Update the README around implemented behavior**

Document compile-once/batch-evaluate data flow, package layout, ownership,
generic expressions, exact commands, formats, error behavior, limits, and
measured performance. Keep the expected R1-R5 table and AI-use statement.

Add a prominent evolution section linking exactly to:

```text
https://github.com/sebishogun/Verifoxx
```

Explain that this repository supplies the compact compiler/runtime foundation,
while the larger project adds SIMD, scheduling, persistence, services, and
debugging.

**Step 2: Keep the submission note within one rendered page**

Cover representation, why it exceeds flat extraction, decision logic,
escalation boundaries, and future improvements. Render it during verification;
do not rely only on word count.

**Step 3: Add architecture and performance references**

`docs/architecture.md` records package dependencies, SoA/CSR layouts, ownership,
and extension seams. `docs/performance.md` records host/runtime, benchmark
commands, six-run ranges, allocations, memory formulas, and exclusions.

**Step 4: Check every documentation claim**

Search for deleted package paths, interpreter wording, unimplemented SIMD or
service claims, old benchmark numbers, and stale policy schema examples.

### Task 16: Final Size, Compliance, And Runtime Verification

**Files:**
- Review: `Requirements.md`
- Review: all changed production, test, fixture, workflow, and documentation files

**Step 1: Run format and syntax checks**

```bash
timeout 20s bash -n scripts/*.sh
git diff --check
```

Expected: all exit 0.

**Step 2: Run the full fresh and race suites**

```bash
timeout 120s ./scripts/go.sh test -count=1 -timeout 60s ./...
```

Expected: PASS.

**Step 3: Run local conformance and workflow checks**

```bash
timeout 120s make check
```

Expected: exact supplied and edge golden matches.

**Step 4: Run container verification**

```bash
timeout 240s make docker-demo
```

Expected: exact goldens and no leftover containers or project network.

**Step 5: Re-run final performance evidence**

```bash
timeout 120s env GOMAXPROCS=1 ./scripts/go.sh test -run '^$' -bench 'Benchmark(Evaluate|Compile|BuildBatch|Materialize)' -benchmem -benchtime=500ms -count=6 -timeout 90s ./internal/...
```

Expected: measurements match `docs/performance.md` within the stated run ranges;
warm evaluation remains zero-allocation.

**Step 6: Audit size and architecture constraints**

Count production and test Go separately. Expected ranges:

```text
production Go: 4,050-5,550 lines including cmd/mm
```

Search `internal/eval` for maps, reflection, string comparison, `fmt`, and
per-row function callbacks. Confirm the final production tree contains no old
interpreter or fixed clause dispatch.

**Step 7: Verify assignment deliverables**

Map every line of `Requirements.md` to source, test, output, README, or the
one-page design note. Confirm `results/requests.json` is reproducible, the
follow-on URL is present, and every performance statement cites final measured
output.

**Step 8: Inspect final repository state**

```bash
git status --short
git diff --check
```

Expected: only intended uncommitted changes, no temporary migration fixture,
benchmark binary, local toolchain, profile fixture, or generated output outside
tracked deliverables.

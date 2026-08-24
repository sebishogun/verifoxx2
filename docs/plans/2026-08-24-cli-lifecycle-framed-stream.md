# CLI Lifecycle And Framed Stream Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Preserve the existing one-shot CLI while adding a bounded framed mode that compiles once, reuses session storage, and processes the supplied pack in fewer than 100 steady-state allocations per frame.

**Architecture:** An immutable `engine.Engine` owns the compiled program and evaluator; a private `engine.Session` owns mutable builder, batch, context, numeric result, and materialized output storage. A framed adapter reads and writes big-endian length-prefixed JSON, while `cmd/verifoxx` selects the unchanged one-shot path or a sequential stream loop.

**Tech Stack:** Go 1.27 standard library, structure-of-arrays evaluator, `encoding/json`, Go benchmarks and allocation tests.

---

All build and test commands in this plan have explicit timeouts. Keep changes
uncommitted unless the user explicitly asks for a commit.

### Task 1: Reuse Builder Scratch Maps

**Files:**
- Modify: `internal/eval/builder.go`
- Modify: `internal/eval/builder_test.go`
- Modify: `internal/eval/executor_bench_test.go`
- Modify: `internal/adapters/jsonio/benchmark_test.go`
- Modify: `cmd/verifoxx/main.go`

**Step 1: Write the failing allocation and stale-map tests**

Add a `TestBuilderWarmPathAllocation` that warms one `Builder` and `Batch`, then
uses `testing.AllocsPerRun(100, ...)` to require zero allocations for the same
request/evidence shape. Extend the reuse test to build a large pack, then a
smaller pack with different IDs, proving removed IDs are not retained.

**Step 2: Verify the new test fails**

Run:

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/eval -run 'TestBuilder(WarmPathAllocation|FullyOverwritesReusedBatch)' -v
```

Expected: the allocation test reports the current map allocations.

**Step 3: Move scratch ownership into `Builder`**

Give `Builder` reusable `requestIDs map[string]struct{}` and
`evidenceByID map[string]uint32` fields. Change `BuildInto` to a pointer receiver,
clear both maps before validation, lazily size them for the incoming pack, and
reuse their buckets. Keep all destination mutation after validation so current
rollback behavior remains unchanged.

Update temporary `NewBuilder(...).BuildInto(...)` call sites to hold a builder
variable. Document that a builder belongs to one session and is not concurrent.

**Step 4: Verify builder tests and benchmark**

Run:

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/eval ./internal/adapters/jsonio ./cmd/verifoxx
timeout 60s env GOMAXPROCS=1 ./scripts/go.sh test -run '^$' -bench '^BenchmarkBuildBatchRows1024$' -benchmem -benchtime=500ms -count=3 -timeout 45s ./internal/eval
```

Expected: tests pass and the warmed 1,024-row builder reports 0 allocs/op.

### Task 2: Reuse Materialized Output

**Files:**
- Modify: `internal/adapters/jsonio/output.go`
- Modify: `internal/adapters/jsonio/output_test.go`
- Modify: `internal/adapters/jsonio/benchmark_test.go`

**Step 1: Write failing reuse tests**

Materialize the supplied results into one `OutputPack`, poison scalar and nested
slice values, then materialize a smaller result into the same pack. Assert exact
content, no stale remediation/evidence/uncertainty, and retained useful
capacities. Add separate cold and warmed materialization benchmarks.

**Step 2: Verify the reuse test fails**

Run:

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/adapters/jsonio -run 'TestMaterializeInto.*Reuse' -v
```

Expected: current `MaterializeInto` replaces the output and nested storage.

**Step 3: Materialize into existing rows and slices**

Resize `dst.Results` with reuse, retain hidden tail capacity for later growth,
and replace `materializeRow` with a destination-taking helper. Reset every row
before republishing it, then append into each
row's `RequirementsApplied`, `EvidenceUsed`, detail, assumption, uncertainty,
and remediation slices. Overwrite every scalar field on every call.

Do not use `sync.Pool`, unsafe conversions, or a new intermediate object graph.
String conversion from the immutable symbol slab may remain allocation-bearing
until the post-lifecycle profile identifies it as dominant.

**Step 4: Verify output tests and measure**

Run:

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/adapters/jsonio ./internal/conformance
timeout 60s env GOMAXPROCS=1 ./scripts/go.sh test -run '^$' -bench '^BenchmarkMaterialize' -benchmem -benchtime=500ms -count=3 -timeout 45s ./internal/adapters/jsonio
```

Expected: conformance remains exact and warmed materialization allocates less
than the existing 47 allocs/op.

### Task 3: Add Engine And Session Lifetimes

**Files:**
- Create: `internal/engine/engine.go`
- Create: `internal/engine/engine_test.go`
- Create: `internal/engine/benchmark_test.go`
- Modify: `cmd/verifoxx/main.go`

**Step 1: Write failing engine tests**

Cover construction rejection for an invalid program, supplied-pack evaluation,
reuse across growing then shrinking packs, and two independent sessions sharing
one engine concurrently. Compare output to `results/requests.json` through the
existing strict decoder or canonical JSON.

**Step 2: Verify the package is missing**

Run:

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/engine
```

Expected: package or symbols do not exist.

**Step 3: Implement immutable engine and private session storage**

Use this API shape:

```go
type Engine struct {
    compiled  program.Program
    evaluator eval.Evaluator
    limits    eval.Limits
}

type Session struct {
    engine  *Engine
    builder eval.Builder
    batch   eval.Batch
    context eval.Context
    numeric result.Batch
    output  jsonio.OutputPack
}

func New(compiled program.Program, limits eval.Limits) (*Engine, error)
func (engine *Engine) NewSession() *Session
func (session *Session) Evaluate(requests []input.Request, evidence []input.Evidence) (*jsonio.OutputPack, error)
```

`New` validates before publication. `Evaluate` builds, sizes reusable storage,
evaluates, and materializes in one ordered path. An error returns no output.
Engine state is immutable; session state is single-owner.

**Step 4: Route one-shot orchestration through the session**

Keep policy loading/compilation and all current user-visible output in
`cmd/verifoxx`. Replace its direct builder/context/result orchestration with one
session call.

**Step 5: Verify engine, CLI, race behavior, and goldens**

Run:

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/engine ./cmd/verifoxx ./internal/conformance
timeout 90s ./scripts/go.sh test -race -count=1 -timeout 75s ./internal/engine
timeout 150s make demo
timeout 30s make clean
```

Expected: all pass and tracked output remains byte-identical.

### Task 4: Implement The Bounded Frame Codec

**Files:**
- Create: `internal/adapters/framed/codec.go`
- Create: `internal/adapters/framed/codec_test.go`
- Create: `internal/adapters/framed/json.go`
- Create: `internal/adapters/framed/json_test.go`

**Step 1: Write failing transport tests**

Cover two frames in one reader, one-byte fragmented reads, short writes, clean
EOF, a partial four-byte header, truncated payload, zero-length payload, and a
length above 16 MiB. Assert no payload allocation occurs for an oversized
length.

**Step 2: Implement frame reading and writing**

Use a reusable `[4]byte` header and payload slice. Decode length with
`binary.BigEndian.Uint32`, reject before growing above `MaxInputPayload`, and
use `io.ReadFull`. Treat zero-byte EOF at a header boundary as clean EOF.
Implement a `writeFull` loop and reject encoded responses above 64 MiB.

**Step 3: Write failing strict JSON envelope tests**

Cover the request shape `{"requests":[...],"evidence":[...]}`, unknown fields,
trailing JSON, success response, bounded error response, compact output, and
repeated decode into reused input storage without stale fields.

**Step 4: Implement reusable JSON envelope state**

Keep reusable request/evidence slices and response bytes in codec-owned state.
Use `json.Decoder.DisallowUnknownFields`, reject a second JSON value, and reset
all old input values before decoding. Encode one compact success or error
envelope into the reusable bounded response buffer.

**Step 5: Verify codec tests**

Run:

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./internal/adapters/framed -v
```

Expected: all framing, strictness, bound, and stale-storage tests pass.

### Task 5: Add `--stream` To The CLI

**Files:**
- Modify: `cmd/verifoxx/main.go`
- Modify: `cmd/verifoxx/main_test.go`
- Create: `cmd/verifoxx/stream.go`
- Create: `cmd/verifoxx/stream_test.go`

**Step 1: Write failing stream integration tests**

Change `run` to receive stdin. Test two valid frames, an invalid complete frame
between two valid frames, response ordering, policy startup failure, truncated
transport, output write failure, and rejection of explicitly supplied
`--requests`, `--evidence`, or `--output` with `--stream`.

Decode returned frames and compare successful `OutputPack` values with the
one-shot result. Assert stream stdout contains frames only.

**Step 2: Verify tests fail on the missing flag**

Run:

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./cmd/verifoxx -run 'TestRun_Stream' -v
```

Expected: `--stream` is unknown.

**Step 3: Implement the sequential stream loop**

Add `--stream`, validate incompatible explicitly visited flags, load and compile
the policy once, create one engine/session, and process frames until clean EOF.
A complete invalid input writes `invalid_input` and continues. Framing,
internal-invariant, and output errors write stderr and return exit code 1.
Stream mode emits no per-request progress table.

Keep existing one-shot exit codes and exact output behavior unchanged.

**Step 4: Verify CLI and conformance**

Run:

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s ./cmd/verifoxx ./internal/conformance
timeout 150s make demo
timeout 30s make clean
```

Expected: stream tests and both existing goldens pass.

### Task 6: Enforce The Steady-Frame Performance Budget

**Files:**
- Modify: `cmd/verifoxx/main_test.go`
- Modify: `cmd/verifoxx/stream_test.go`
- Modify: `internal/engine/benchmark_test.go`
- Modify as identified by profile: `internal/adapters/framed/json.go`
- Modify as identified by profile: `internal/adapters/jsonio/output.go`

**Step 1: Add lifecycle-separated benchmarks**

Keep `BenchmarkEvaluateCLISuppliedPack` for a fresh one-shot call. Add engine
construction, first-frame, and steady-frame benchmarks. Pre-grow all reusable
state before timing the steady benchmark.

**Step 2: Add the allocation regression**

Use `testing.AllocsPerRun(100, ...)` around steady supplied-pack frame
processing. Require fewer than 100 allocs/frame; retain the exact evaluator
zero-allocation test.

**Step 3: Run benchmarks and profile only if the budget fails**

Run:

```bash
timeout 90s env GOMAXPROCS=1 ./scripts/go.sh test -p 1 -run 'Test.*Allocation' -bench 'Benchmark(Engine|FirstFrame|SteadyFrame|EvaluateCLI)' -benchmem -benchtime=500ms -count=3 -timeout 75s ./cmd/verifoxx ./internal/engine ./internal/eval
```

If steady framing is at or above 100 allocations, create a one-thousand-frame
allocation profile under `/tmp/opencode`, inspect `alloc_objects`, and remove
only the top repeated allocator. Prefer destination reuse and direct compact
encoding before considering a specialized bounded decoder. Repeat until the
budget passes or the profile proves standard JSON string creation alone exceeds
it; in that case document the measured boundary rather than hiding it.

**Step 4: Verify no one-shot regression**

Compare six interleaved baseline/current samples when a retained baseline is
available. At minimum, report the new six-sample range against the recorded
`88.647-94.895 us/op`, 516 allocs/op, and about 126 KiB/op baseline. Do not claim
service throughput from this microbenchmark.

### Task 7: Document And Verify The Complete Change

**Files:**
- Modify: `README.md`
- Modify: `docs/architecture.md`
- Modify: `docs/performance.md`
- Modify: `Makefile` only if a documented stream demo target adds clear value

**Step 1: Document protocol, ownership, limits, and measurements**

Add the exact binary frame layout, request/response envelope examples, stream
error behavior, engine/session lifetime rules, reproduction commands, first
versus steady-frame results, and explicit benchmark exclusions. Preserve the
candidate design note's one-page limit; do not expand it for this optional
runtime feature.

**Step 2: Run final formatting and verification**

Run:

```bash
timeout 120s ./scripts/go.sh test -count=1 -timeout 90s ./...
timeout 150s ./scripts/go.sh test -race -count=1 -timeout 120s ./...
timeout 150s make demo
timeout 30s make clean
timeout 30s git diff --check
```

Expected: all commands pass, both goldens remain exact, and no build artifact is
left in the worktree.

**Step 3: Review scope and worktree**

Run:

```bash
timeout 30s git status --short
timeout 30s git diff --stat
```

Expected: only intended source, test, benchmark, and documentation changes are
present. Do not commit unless explicitly requested.

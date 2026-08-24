# CLI Lifecycle And Framed Stream Design

## Goal

Keep the existing one-shot CLI and byte-identical result format while adding a
long-running framed mode that compiles a policy once and reuses all practical
working storage across request packs. Optimize from measured allocation
profiles, preserve the zero-allocation warm evaluator, and stop before a custom
JSON implementation unless the staged allocation budget requires one.

The current supplied-pack baseline is 516 allocations and about 126 KiB for a
complete in-process CLI invocation. Roughly 300 of those allocations load,
validate, and compile the policy. They are legitimate cold work for a fresh
process, but wasteful when many packs use the same policy.

## Scope

- Preserve the current flags, defaults, diagnostics, exit codes, and pretty
  one-shot JSON output.
- Add a `--stream` mode using stdin and stdout.
- Compile and validate the policy once per process.
- Reuse transport buffers, input slices, builder scratch, the columnar batch,
  evaluation context, numeric results, and materialized output between frames.
- Keep frame processing sequential and responses ordered.
- Retain the standard library JSON codec during the first stage.

This change does not add a network daemon, asynchronous responses, policy hot
reload, a persisted compiled-policy format, or a custom JSON parser.

## Architecture

Add an internal engine boundary with two lifetimes:

- `Engine` owns the validated immutable `program.Program`, evaluator, and
  limits. It is safe to share after construction.
- `Session` owns all mutable storage for one sequential worker: input slices,
  builder scratch maps, `eval.Batch`, `eval.Context`, `result.Batch`,
  `jsonio.OutputPack`, and frame buffers. A session is not shared concurrently.

The one-shot command constructs one engine and one session, processes the
existing request and evidence files, emits the existing output, and exits. The
stream command constructs them once and processes frames until clean EOF.
Future parallelism creates one session per worker while sharing the engine.

`BuildInto` will accept caller-owned scratch maps rather than allocate temporary
maps on every build. `MaterializeInto` will resize and clear existing slices
instead of replacing the output pack and every nested result slice. Context and
numeric-result reuse remain unchanged.

## Framed Protocol

The stream starts after `verifoxx --stream --policy <path>`. The one-shot
`--requests`, `--evidence`, and `--output` flags are invalid when explicitly
supplied with `--stream`.

Each message is:

```text
4-byte unsigned big-endian payload length
exactly N bytes of UTF-8 JSON payload
```

An input payload is a strict object:

```json
{"requests":[...],"evidence":[...]}
```

A successful response is:

```json
{"ok":true,"output":{"schema_version":1,"policy_name":"...","policy_version":"...","results":[...]}}
```

A complete but invalid input frame receives an ordered error response and the
stream continues:

```json
{"ok":false,"error":{"code":"invalid_input","message":"..."}}
```

Clean EOF at a frame boundary exits successfully. A partial header, partial
payload, or oversized length is fatal because the stream cannot be resynchronized
safely. An internal invariant failure or output write failure is also fatal.
Stdout contains frames only; startup information and fatal errors use stderr.

Input payloads are limited to 16 MiB. Encoded responses are limited to 64 MiB.
The frame reader checks lengths before growing storage, and all writes use a
short-write-safe loop.

## Data Flow

```text
startup: policy file -> strict decode -> validate -> compile -> Engine

one-shot:
  request/evidence files -> Session -> existing pretty OutputPack JSON

stream frame:
  reusable payload buffer
    -> strict frame decode into reusable input slices
    -> reusable builder scratch + Batch
    -> reusable Context + numeric result Batch
    -> reusable OutputPack
    -> compact JSON in reusable response buffer
    -> length-prefixed response
```

No policy AST, compiler maps, or flag parser enters the repeated framed path.
No text conversion enters the evaluator.

## Performance Plan

Keep separate benchmarks so unlike lifetimes are not conflated:

- Fresh one-shot invocation: all current cold stages.
- Engine construction: policy decode, validation, and compilation.
- First stream frame: includes initial storage growth.
- Steady stream frame: all storage already sized.
- Warm evaluator: unchanged zero-allocation kernel.

Stage-one acceptance on the supplied five-row pack under `GOMAXPROCS=1`:

- Warm evaluation remains exactly 0 B/op and 0 allocs/op.
- One-shot output and edge goldens remain byte-identical, with no material
  latency or allocation regression from the 516-allocation baseline.
- Steady framed processing stays below 100 allocs/frame and 32 KiB/frame.
- Repeated frames do not retain stale rows, evidence, provenance, or output.

After lifecycle reuse lands, profile steady framed processing again. If the
budget is missed, remove the largest measured allocator first. A specialized
bounded JSON decoder or direct result encoder is justified only when standard
JSON string materialization or reflection remains the dominant cost.

## Correctness And Failure Handling

- Existing one-shot conformance tests remain authoritative.
- The same input pack must produce semantically identical one-shot and framed
  outputs.
- Engine construction publishes only a fully validated immutable program.
- Session operations validate capacity and input before mutating published
  output.
- A rejected frame resets reusable lengths before the next frame.
- Error responses expose stable codes and bounded messages, never partial
  successful results.
- Independent sessions sharing one engine are race-tested.

## Tests

- Golden one-shot tests for the supplied and edge packs.
- Frame codec tests for multiple frames, fragmented reads, short writes, clean
  EOF, truncated headers, truncated payloads, zero length, and oversized input.
- Stream integration tests for two valid frames, an invalid frame between valid
  frames, fatal transport errors, and exact response ordering.
- Reuse tests with shrinking and growing packs to detect stale data.
- Allocation regression tests for evaluator and steady framed processing.
- Benchmarks for engine construction, first frame, steady frame, and complete
  one-shot execution.
- Race tests for independent sessions sharing one engine.

## Evolution

The binary framing and engine/session lifetimes map directly to a future
service transport: the immutable engine can be cached by policy version and
sessions can be sharded per worker. SIMD kernels and row sharding remain behind
the same numeric program, batch, context, and result layouts. The framed codec
is an adapter, not part of policy semantics or evaluation.

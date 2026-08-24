# Architecture

## Scope

Verifoxx has one evaluation path: strict source documents are compiled and
converted to numeric, contiguous data before evaluation. One-shot file mode and
the framed stdin/stdout adapter both use the same immutable engine and private
session. The runtime is scalar and single-process. SIMD, parallel scheduling,
persistence, network services, and interactive debugging are extension points
rather than implemented features.

## Performance Tenets

Verifoxx is performance-focused at the level that matters first: data layout,
lifetime, and repeated work. The design is causally ordered:

```text
parallel arrays + grouped lifetimes + pre-sized caller storage
  -> contiguous, uniformly typed runtime data
  -> bulk scalar kernels with no per-row allocation
  -> stable future boundaries for SIMD and row sharding
```

| Tenet | How this repository applies it |
| --- | --- |
| Design the data before the control flow | `program.Program`, `eval.Batch`, `eval.Context`, and `result.Batch` use parallel arrays, bitplanes, and CSR ranges instead of per-row object graphs |
| Compile and validate once | Policy strings and expression shapes become typed IDs, postorder instructions, and immutable tables before evaluation |
| Allocate outside the per-row path | Builders, contexts, numeric results, materialized outputs, and frame buffers are reusable; correctly pre-sized warm evaluation performs zero allocations |
| Work in bulk | Field-major columns and instruction-major bitplanes let scalar loops process contiguous values or 64-row words |
| Do not repeat boundary work | Input strings are resolved once during batch construction; text reconstruction happens only after numeric evaluation |
| Group values by lifetime | One immutable `Engine` owns policy data while each `Session` owns all mutable storage for one sequential worker |
| Parallelize only at a natural boundary | Independent sessions may share an engine; the current CLI remains sequential and does not claim parallel speedup |
| Keep ownership visible | No `sync.Pool` is used; the caller or session that reuses a slice also owns its overwrite rules |
| Measure unlike lifetimes separately | Compilation, cold destination growth, first frame, steady frame, warm evaluation, and materialization have separate benchmarks |

The current kernels are scalar. The layouts permit later SIMD dispatch and row
sharding, but neither is implemented here. JSON decoding and human-readable
materialization are boundary work and may allocate; the zero-allocation
contract applies to correctly sized warm `EvaluateInto` calls.

## Core Runtime Types

These types carry data through the production path. Source objects and output
objects stay outside the numeric evaluator.

| Type | Representation | Owner and lifetime | Responsibility |
| --- | --- | --- | --- |
| [`ast.Policy`](../internal/ast/policy.go) | Ordered requirements, expression nodes, resolutions, explanations, remediations | Loader/caller; cold and discardable after compilation | Human-readable source intermediate representation |
| [`program.Program`](../internal/program/program.go) | Immutable parallel arrays, typed IDs, CSR ranges, resolution tables, one interned text slab | `engine.Engine`; long-lived and shareable | Complete validated numeric policy used by evaluation and materialization |
| [`input.Request` and `input.Evidence`](../internal/input/types.go) | Strict JSON-facing structs with source strings and IDs | Adapter/session; retained through materialization | Preserve boundary data and exact source identifiers |
| [`eval.Builder`](../internal/eval/builder.go) | Program pointer, limits, reusable validation and lookup maps | One session; reused between packs | Validate source IDs and resolve strings into a numeric batch |
| [`eval.Batch`](../internal/eval/batch.go) | Field-major request columns, evidence qualifier columns, bitplanes, CSR evidence edges | One session; rebuilt in place for each pack | Numeric request and evidence input to the evaluator |
| [`eval.Context`](../internal/eval/context.go) | Positive, negative, and reason bitplanes indexed by instruction and row word | One session/worker; resized before evaluation and never shared | Reusable instruction-evaluation scratch |
| [`eval.Evaluator`](../internal/eval/executor.go) | Read-only pointer to one immutable program | `engine.Engine`; shareable | Execute all instructions and resolve the highest-ranked deterministic driver |
| [`result.Batch`](../internal/result/batch.go) | Parallel numeric result columns plus CSR provenance and remediation IDs | One session/worker; caller-sized and reused | Receive decisions without allocating or formatting in the evaluator |
| [`jsonio.OutputPack`](../internal/adapters/jsonio/output.go) | JSON-facing result structs and reusable nested slices | One session or one-shot output boundary | Reconstruct source IDs, explanations, uncertainty, and remediation text |
| [`engine.Engine`](../internal/engine/engine.go) | Validated `Program`, `Evaluator`, and limits | Application; long-lived and immutable | Publish one compiled policy safely to one or more sessions |
| [`engine.Session`](../internal/engine/engine.go) | Builder, batch, context, numeric results, and output pack | One sequential worker; not shared concurrently | Group mutable same-lifetime storage and evaluate repeated packs |
| [`framed.FrameReader`, `FrameWriter`, and `JSONCodec`](../internal/adapters/framed/) | Reusable length header, payload/input/output buffers, and response envelopes | Framed CLI loop; sequential | Enforce transport bounds and keep repeated protocol storage private |

The core ownership split is `Engine` for immutable shared data and `Session`
for mutable private data. A returned output pack belongs to its session and is
overwritten by that session's next evaluation.

## Data Flow

```text
policies/policy.json
  -> jsonio.DecodePolicy
  -> ast.Policy.Validate
  -> compile.Compile
  -> program.Program.Validate
  -> immutable Program

requests.json + evidence.json
  -> strict input DTOs
  -> eval.Builder.BuildInto
  -> Batch (SoA fields/evidence + CSR references)

Program + Batch + Context + result.Batch
  -> eval.Evaluator.EvaluateInto
  -> numeric outcomes, drivers, provenance, remediation IDs
  -> jsonio.MaterializeInto
  -> OutputPack
  -> EncodeResults or atomic WriteResults

framed mode startup
  -> compile once -> immutable Engine
  -> one private Session

each frame
  -> bounded length prefix + strict JSON Input
  -> Session reuses Builder + Batch + Context + result.Batch + OutputPack
  -> compact success/error JSON + bounded length prefix
```

Decode, validation, compilation, batch construction, and materialization are
cold stages. The warm stage begins after `Context.Ensure` and
`result.Batch.Ensure`; repeated `EvaluateInto` calls reuse that storage.

## Package Boundaries

| Package | Responsibility | Depends on |
| --- | --- | --- |
| `internal/schema` | Typed nonzero IDs, fields, outcomes, reasons | none |
| `internal/ast` | Generic source policy and semantic validation | `schema` |
| `internal/program` | Immutable numeric layout and publication validator | `schema` |
| `internal/compile` | Deterministic postorder lowering and string interning | `ast`, `program`, `schema` |
| `internal/input` | Request and evidence transport records | none |
| `internal/eval` | Batch builder, bitplanes, scalar kernels, row resolution | `input`, `program`, `result`, `schema` |
| `internal/engine` | Immutable program lifetime and private reusable sessions | `eval`, `jsonio`, `program`, `result` |
| `internal/result` | Caller-owned numeric output columns and capacity contract | `schema` |
| `internal/adapters/jsonio` | Strict JSON boundaries and cold text materialization | all data packages |
| `internal/adapters/framed` | Bounded length-prefixed JSON transport | `input`, `jsonio` |
| `cmd/verifoxx` | One-shot and framed CLI orchestration | adapters, engine |

The source AST never enters the evaluator. Output DTOs never enter the
evaluator. This keeps parsing strings and presentation allocations outside the
numeric kernel.

## Source Model

A policy contains ordered requirements. Each requirement has one applicability
expression and ordered clauses; each clause has one assertion. Four operators
are supported:

- `equal(field, value)`
- `in(field, values)`
- `all(children)`
- `evidence_matches(predicate)`

`evidence_matches` is prohibited in applicability because evidence determines
whether an applicable clause is satisfied, not whether the requirement governs
the request. A clause has an explicit outcome and explanation entry for each
of eight reason states. `Revise` clauses own typed remediation records.

Validation rejects malformed operator shapes, unknown fields, duplicate IDs or
set values, missing resolutions, invalid outcomes/remediations, excessive
depth, excessive node count, and oversized explanations before compilation.
Request literals, evidence kinds, and remediation payloads are checked against
the same closed exercise vocabulary used by the batch builder.

## Immutable Program

`compile.Compile` traverses child expressions first, then appends the parent.
Every operand ID therefore precedes its consumer and evaluation can scan
instructions once in array order.

The `Program` uses parallel arrays rather than an instruction struct:

```text
Opcodes[i]             operation
Fields[i] / Values[i]  equal payload
SetStarts/Counts[i]    range in SetValues for in
OperandStarts/Counts[i] range in Operands for all
EvidenceSpecIndexes[i] evidence predicate for evidence_matches
```

Requirements and clauses use the same shape: dense typed IDs, root instruction
IDs, and CSR-style starts/counts into flat ownership arrays. Resolution and
explanation tables are indexed as:

```text
clauseIndex * ReasonCount + reasonIndex
```

All strings needed by the runtime are interned into one immutable `SymbolText`
slab with parallel start and length arrays. Substrings are zero-copy views;
compile-time maps disappear when the interner freezes.
`Program.Validate` is the publication gate and checks aligned columns, typed ID
bounds, symbol coverage, CSR ranges, topological order, clause ownership,
resolution tables, remediation payloads, and precedence.

After successful publication the program is treated as immutable. Multiple
evaluators may share it, but no goroutine may mutate its slices.

## Columnar Batch

For `N` request rows, seven request fields are stored field-major:

```text
Values[field*N + row]       uint32 SymbolID
Present[field*W + word]     uint64 bitplane
Valid[field*W + word]       uint64 bitplane
SemanticIssueMasks[row]     uint16 field mask
W = ceil(N / 64)
```

This keeps every scan over one request field contiguous. Strings are resolved
against the frozen symbol table once in the builder. Present-but-unknown values
have a present bit, no valid bit, and a semantic issue bit; missing values have
neither present nor valid.

Evidence uses one slice per qualifier (`Status`, `Timing`, `Reviewer`, and so
on), plus presence and conflict/stale/revoked flags. Request-to-evidence links
use CSR:

```text
EvidenceRefOffsets: N + 1 entries
EvidenceRefs:       one uint32 per source evidence_ids edge
```

A reference value of zero is an explicit missing-record sentinel. Nonzero
values are one-based indexes into the evidence columns. Source IDs remain in
the cold input DTOs so materialization can report the exact missing ID.

`BuildInto` validates all IDs, limits, and total edge count before resizing or
mutating the destination. Each builder owns reusable validation and lookup maps.

## Truth And Evidence Evaluation

`Context` owns three reusable instruction-major stores:

- Positive bitplane per instruction.
- Negative bitplane per instruction.
- Reason bitplane per reason and instruction.

`equal` and `in` scan one field column. `all` intersects child positive words
and unions child negative words; if neither settles the result, it selects a
reason in fixed order. Evidence kernels scan only each row's CSR edge range.
For a matching evidence kind, precedence is conflict, stale, revoked,
incomplete qualifier, satisfied qualifier, wrong qualifier, then missing.
Conflict sets both positive and negative truth while retaining
`ReasonConflict`.

All instructions are evaluated. Row resolution then:

1. Records semantic and missing-reference escalation candidates.
2. Selects applicable requirements from their positive roots.
3. Resolves every owned clause through its reason-indexed outcome table.
4. Keeps the first finding at the highest rank
   (`Reject > Escalate > Revise > Approve`).
5. Appends applicable requirement IDs, required evidence references, and the
   winning clause's remediation IDs to caller-owned result CSR columns.

The warm path uses typed numeric comparisons and direct function calls. It has
no maps, reflection, formatting, string comparisons, per-row callbacks, or
pooling.

## Ownership And Reuse

| Value | Owner | Lifetime and rule |
| --- | --- | --- |
| `ast.Policy` | loader/caller | Cold; discard after successful compilation |
| `program.Program` | application | Long-lived, immutable after validation |
| `engine.Engine` | application | Long-lived and immutable; shared by sessions |
| `engine.Session` | one worker | Reuses builder, batch, context, numeric results, and output |
| `input.Request` / `input.Evidence` | loader/caller | Retained through materialization for source IDs/text |
| `eval.Batch` | caller | Rebuild or reuse between input packs; do not mutate during evaluation |
| `eval.Context` | one worker | Reuse after `Ensure`; never share concurrently |
| `result.Batch` | one worker | Caller sizes it, evaluator resets/fills it; never share concurrently |
| `jsonio.OutputPack` | session/output boundary | Reused by a session; overwritten by its next evaluation |

Independent sessions can evaluate through the same immutable engine
concurrently. A `Builder`, `Context`, result batch, output pack, and framed codec
remain private to one sequential session. No `sync.Pool` is used; ownership
stays visible at the call site.

## Framed Transport

Stream messages use a four-byte unsigned big-endian length followed by an exact
JSON payload. Input is capped at 16 MiB before the payload buffer grows; output
is capped at 64 MiB while encoding. A clean EOF between headers is normal. A
partial or oversized frame is fatal because safe resynchronization is
impossible. Complete invalid input receives an ordered bounded error response,
then the session continues with cleared reusable state.

## Capacity Contract

`EvaluateInto` does not grow storage. Before entering it, callers use:

```go
context.Ensure(compiled, batch.Rows)
numeric.Ensure(
    batch.Rows,
    rows*len(compiled.RequirementSymbols),
    len(batch.EvidenceRefs),
    rows*len(compiled.Remediations),
)
```

The evaluator checks every fixed and variable result column before clearing or
writing any result. Insufficient capacity returns a typed `CapacityError`
without partial result mutation. Context shape and all input CSR ranges are
also checked before instruction execution.

## Extension Seams

- A new request field starts in `schema`, then adds one builder column and
  validation rule; generic `equal` and `in` need no clause-specific dispatch.
- A new expression operator requires source validation, one opcode payload,
  compiler lowering, program validation, and a kernel case.
- SIMD kernels can replace word/row loops behind the same immutable program,
  batch, context, and result layouts.
- Parallel evaluation can shard rows and give each shard private context and
  result storage before one ordered merge.
- Persistence, services, authoring, and debugging belong outside the evaluator
  and should consume or produce the same validated boundaries.

The larger project at `https://github.com/sebishogun/Verifoxx` explores those
broader features. They are not part of this repository's runtime.

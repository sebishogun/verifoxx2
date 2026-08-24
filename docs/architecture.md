# Architecture

## Scope

Verifoxx has one production execution path: strict source documents are
compiled and converted to numeric, contiguous data before evaluation. The
runtime is scalar and single-process. SIMD, parallel scheduling, persistence,
network services, and interactive debugging are extension points rather than
implemented features.

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
| `internal/result` | Caller-owned numeric output columns and capacity contract | `schema` |
| `internal/adapters/jsonio` | Strict JSON boundaries and cold text materialization | all data packages |
| `cmd/verifoxx` | Compile-once CLI orchestration | adapters, compiler, evaluator, result |

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

All strings needed by the runtime are interned into one `SymbolBytes` slab with
parallel start and length arrays. Compile-time maps disappear when the interner
freezes.
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
mutating the destination. Its temporary maps exist only during batch
construction.

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
| `input.Request` / `input.Evidence` | loader/caller | Retained through materialization for source IDs/text |
| `eval.Batch` | caller | Rebuild or reuse between input packs; do not mutate during evaluation |
| `eval.Context` | one worker | Reuse after `Ensure`; never share concurrently |
| `result.Batch` | one worker | Caller sizes it, evaluator resets/fills it; never share concurrently |
| `jsonio.OutputPack` | output boundary | Cold, allocation-bearing, disposable |

Independent contexts and result batches can evaluate the same immutable
program and batch concurrently. No `sync.Pool` is used; ownership stays visible
at the call site.

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

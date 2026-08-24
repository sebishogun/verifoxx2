# Compiled Scalar Policy Engine Design

**Date:** 2026-08-24

**Status:** Approved

## Objective

Replace the row-oriented policy interpreter with a compact compiler and
columnar scalar runtime that still satisfies the candidate exercise exactly.
The final repository should be a readable, reduced implementation of the
foundations developed further in
[Verifoxx](https://github.com/sebishogun/Verifoxx), not an implementation that
assumes five requests are the permanent workload.

The final production path will compile policy source once, build typed request
and evidence batches, evaluate complete columns into caller-owned numeric
results, and materialize explanations only at the adapter boundary.

## Scope

The implementation includes:

- A small generic source expression language.
- Typed schema, node, instruction, symbol, outcome, reason, and remediation IDs.
- Strict source validation and bounded admission limits.
- Symbol interning during compilation.
- An immutable struct-of-arrays execution program.
- Columnar request data and CSR request-to-evidence edges.
- Positive, negative, and reason bitplanes in reusable evaluation storage.
- Scalar whole-plane kernels with no per-row callback.
- Caller-owned numeric result columns and CSR provenance.
- Deferred rationale, uncertainty, evidence-issue, and JSON materialization.
- Exact compatibility with the supplied and edge-case golden results.
- Benchmarks proving warm evaluation allocation behavior and scaling.

The implementation excludes:

- SIMD kernels or an external SIMD dependency.
- Parallel worker scheduling.
- PostgreSQL and decision journaling.
- HTTP, gRPC, TUI, and debugger adapters.
- Policy publication and a long-running service.
- A compatibility mode that retains the interpreter after migration.

These exclusions are explicit extension seams, not implied features.

## Design Alternatives

### Compiler-first replacement

Compile a readable source AST into numeric columns, then evaluate columnar
batches. This is the selected design because it removes maps and strings from
the evaluator and preserves a direct route to SIMD and row sharding.

### Optimize the current interpreter

Pre-size more slices, cache lookups, and pool temporary findings. This would
reduce some allocations but retain fixed clause dispatch, repeated evidence
scans, string comparisons, and an array-of-struct request path. The shape would
still block the larger design.

### Reduce the larger repository by deletion

Copy its engine and remove services, persistence, and tooling. This would import
abstractions and implementation volume that the exercise does not need. The
smaller repository should share architectural principles and semantics, not
become a partial fork.

## System Flow

```text
policy JSON
    -> source AST
    -> validate / intern / compile
    -> immutable Program

request and evidence JSON
    -> reusable BatchBuilder
    -> columnar Batch plus CSR evidence edges

Program + Batch + EvalContext
    -> scalar whole-plane execution
    -> numeric ResultBatch
    -> explanation and JSON materialization
```

Compilation and batch construction are cold boundaries. Repeated evaluation of
one immutable program over correctly sized caller-owned storage is the hot
boundary.

## Package Boundaries

```text
internal/schema    typed IDs, field catalogue, value kinds
internal/ast       generic source expressions and policy semantics
internal/compile   validation, interning, and postorder lowering
internal/program   immutable instruction and semantic columns
internal/eval      batch builder, context, and scalar executor
internal/result    numeric results and explanation materialization
internal/adapters  strict JSON policy, batch, and result adapters
cmd/verifoxx       thin command-line boundary
```

The existing `cmd/mm` utility remains independent of policy execution.

## Source Policy Model

The source AST remains optimized for clarity rather than hot-path layout. A
policy contains metadata, requirements, and clauses. Applicability is a
separate expression from clause satisfaction. Every clause has an assertion,
a complete resolution table, explanation data, and optional bounded
remediation.

The initial generic expression set is deliberately small:

- `All`: every child must hold.
- `Equal`: one request field equals one typed literal.
- `In`: one request field belongs to a literal set.
- `EvidenceMatches`: attached evidence of one kind satisfies state and
  qualifier requirements.

Operators are source nodes rather than clause kinds. Adding an operator later
requires one source node, compiler validation/lowering, and one execution
kernel; it does not require a new requirement or clause representation.

Evidence matching preserves these reasons:

- satisfied
- false
- missing
- invalid
- stale
- unclear
- unverifiable
- conflict

Every reason maps to an explicit outcome. A non-approval mapping must have a
bounded explanation. A `Revise` mapping must reference a valid structured
remediation.

## Typed IDs And Symbols

The compiler replaces policy strings with typed integer IDs. ID zero is invalid
for every domain.

```text
FieldID         NodeID          InstructionID
SymbolID        ValueID         EvidenceKindID
OutcomeID       ReasonID        RequirementID
ClauseID        RemediationID   ExplanationID
```

Policy symbols are stored once in one byte slab with parallel start and length
columns. Compilation may use a temporary map or open-addressed table. Evaluation
uses only IDs and numeric columns.

## Immutable Program

The compiled program owns all data needed for evaluation and result decoding.
Representative columns are:

```text
Opcodes[]
Fields[]
Values[]
OperandStarts[] / OperandCounts[] / Operands[]

EvidenceKinds[]
EvidenceStateRequirements[]
EvidenceQualifierStarts[] / Counts[] / Qualifiers[]

RequirementApplicabilityRoots[]
RequirementClauseStarts[] / Counts[] / ClauseIDs[]
ClauseAssertionRoots[]
ClauseResolutionOutcomeIDs[]
ClauseExplanationIDs[]
ClauseRemediationStarts[] / Counts[] / RemediationIDs[]

OutcomePrecedence[]
SymbolBytes[] / SymbolStarts[] / SymbolLengths[]
```

Instructions are emitted in postorder, so every operand precedes its consumer.
The first implementation does not need common-subexpression elimination or
liveness reuse, but its instruction and operand layout permits both.

`Program.Validate` checks all parallel lengths, IDs, CSR ranges, instruction
ordering, semantic references, and resolution tables before publication. A
program is immutable after successful compilation.

## Columnar Batch

The request batch stores policy-visible values by field column rather than by
request object:

```text
Rows
SymbolValues[field * rows + row]
PresenceMasks[field * words + word]

EvidenceOffsets[row : row+2]
EvidenceRefs[]
EvidenceKinds[]
EvidenceStates[]
EvidenceQualifierValues[]
EvidenceQualifierPresence[]
```

Request IDs remain adapter metadata indexed by row; they do not enter the
evaluation kernel. The builder resolves input strings to program symbols once.
An absent field and a present but unknown symbol remain distinct.

Request-to-evidence relationships use CSR. The cold builder may use a temporary
evidence-ID lookup map. The evaluator never performs a map lookup.

`BatchBuilder.BuildInto` either produces one complete valid batch or restores
all destination lengths. It enforces row, evidence, and edge limits before
growing storage.

## Truth And Reason Planes

Each evaluated instruction uses positive and negative row bitplanes:

| Positive | Negative | Meaning |
|---:|---:|---|
| 1 | 0 | true |
| 0 | 1 | false |
| 0 | 0 | unknown |
| 1 | 1 | conflict |

`All` composes complete words:

```text
positive = positive AND child.positive
negative = negative OR child.negative
```

Sideband reason planes retain missing, invalid, stale, unclear, unverifiable,
and conflict. Evidence evaluation reduces CSR evidence ranges to request-row
planes. Scalar code processes whole slices or mask words, which provides a
stable future boundary for SIMD wrappers.

`EvalContext` owns all mutable truth and reason storage. It is reusable for one
program and row capacity. Independent contexts permit concurrent callers
without evaluator locks.

## Result Resolution

`ResultBatch` contains numeric columns:

```text
OutcomeIDs[]
DriverClauseIDs[]
DriverReasonIDs[]
RequirementOffsets[] / RequirementIDs[]
EvidenceOffsets[] / EvidenceIDs[]
IssueOffsets[] / IssueIDs[]
RemediationOffsets[] / RemediationIDs[]
```

The resolver applies the policy's precedence table in deterministic requirement
and clause order. The Verifoxx policy defines
`Reject > Escalate > Revise > Approve`.

The evaluator does not format strings. The result adapter uses program-owned
explanation and remediation catalogues plus row metadata to reproduce the
existing machine-readable output exactly.

## API And Ownership Contract

The intended API shape is:

```go
Compile(source ast.Policy) (program.Program, []compile.Diagnostic)
Builder.BuildInto(dst *eval.Batch, requests []Request, evidence []Evidence) error
Evaluator.EvaluateInto(ctx *eval.Context, batch eval.Batch, dst *result.Batch) error
MaterializeInto(dst []byte, program program.Program, batch eval.Batch, results result.Batch) ([]byte, error)
```

The program owns immutable policy data. The builder owns reusable batch slabs.
The context owns reusable evaluator scratch. The result batch owns reusable
numeric output. Returned views never outlive their owner.

Cold setup functions may allocate within configured limits. Once batch,
context, and result capacities are established, repeated `EvaluateInto` calls
must perform zero allocations.

## Error And Admission Contract

User-controlled data returns errors or compiler diagnostics; it must not panic.
The implementation will reject:

- Unknown JSON fields and trailing JSON values.
- Empty or duplicate IDs.
- Unsupported operators and incorrect arity.
- Unknown fields or incompatible literal types.
- Cycles or forward references in a compiled program.
- Missing or incomplete resolution mappings.
- Non-approval states without bounded explanations.
- Invalid or unreferenced remediation payloads.
- Malformed CSR ranges or mismatched parallel columns.
- Malformed evidence references. A well-formed reference whose record is absent
  is retained as semantic uncertainty and resolves through policy evaluation.
- Insufficient caller-owned context or result capacity.
- Positional CLI arguments.

Evaluation validates every shape and capacity before mutating output. Capacity
errors leave prior result lengths and values unchanged.

Fixed defaults will bound policy nodes, requirements, clauses, batch rows,
evidence records, evidence edges, and encoded output. Limits remain adapter or
builder concerns and do not add branches per evaluated element.

The existing repository-local Go installer rollback path will also be corrected
so a failed restore never deletes the only backup.

## Migration

The interpreter remains only as a temporary differential oracle:

1. Add committed baseline benchmarks for the current interpreter.
2. Add the generic source AST and equivalent policy fixture.
3. Add compiler validation, symbol interning, and immutable Program lowering.
4. Add columnar batch construction and CSR evidence.
5. Add reusable truth/reason storage and scalar execution.
6. Add numeric results and exact explanation materialization.
7. Compare interpreted and compiled output across every existing scenario.
8. Switch the CLI, demos, and conformance tests to compiled execution.
9. Delete the interpreter, fixed clause dispatch, and evidence-map hot path.
10. Measure the final runtime and update documentation.

The final repository has one production execution path: the compiled scalar
engine.

## Testing

Tests will be written before each production stage. Coverage includes:

- Source-expression and resolution validation.
- Compiler IDs, instruction order, symbols, and CSR ranges.
- Corrupt-program rejection.
- Batch column layout, unknown symbols, and evidence edges.
- Every truth and evidence reason.
- Applicability distinct from satisfaction.
- Outcome precedence and equal-rank deterministic ordering.
- Exact requirement, evidence, issue, uncertainty, and remediation provenance.
- Capacity errors without partial mutation.
- Context reuse after poisoned scratch and result storage.
- Differential interpreter/compiled agreement during migration.
- Exact supplied and edge golden JSON.
- Race safety with independent contexts.
- Docker and Compose behavior.

After migration, tests no longer import or execute the interpreter.

## Performance Verification

Committed benchmarks cover 1, 5, 64, and 1,024 rows. Every result reports
`ns/op`, `B/op`, and `allocs/op`. Benchmarks separate:

- cold policy decode and compile;
- cold request/evidence batch construction;
- warm compiled evaluation;
- explanation and JSON materialization; and
- complete CLI execution.

Warm evaluation acceptance is `0 B/op` and `0 allocs/op`. Scaling and memory are
reported as measured; no SIMD, parallel, or production-throughput claim is
allowed. The benchmark records CPU, Go version, and exact commands.

## Documentation And Evolution Story

The README will identify this repository as the compact semantic/compiler
foundation. It will explain that the larger
[Verifoxx repository](https://github.com/sebishogun/Verifoxx) develops the same
ideas into a broader engine with SIMD, scheduling, persistence, services, and
debugging.

The documentation set will include:

- `README.md`: use, formats, architecture summary, results, and evolution link.
- `docs/DESIGN_NOTE.md`: the required maximum-one-page submission note.
- `docs/architecture.md`: data flow, package boundaries, and ownership.
- `docs/performance.md`: reproducible measurements and allocation contracts.

The story is prototype to compiler, not a claim that the original interpreter
was itself a production runtime.

## Size Budget

| Area | Estimated production Go lines |
|---|---:|
| Source AST and typed IDs | 450-650 |
| Validation, interning, compiler | 800-1,100 |
| Immutable Program | 300-450 |
| SoA batch builder | 500-700 |
| Scalar evaluator and context | 800-1,100 |
| Results and materialization | 350-550 |
| CLI and JSON adapters | 300-450 |
| Existing `mm` utility | about 550 |
| **Production Go total** | **4,050-5,550** |

Tests and benchmarks are expected to bring total Go code to roughly
8,000-11,000 lines. The complete submission, including scripts, fixtures,
policy data, and user-facing documentation, should remain around 11,000-14,000
lines. The limit is a scope guard, not a reason to compress readable code.

Implementation note (2026-08-24): the verified tree is 3,517 production Go
lines and 6,472 total Go lines. The estimate above overstated the code needed
for the implemented layouts and standard-library adapters; no compatibility
layer or line-count padding was added. The behavioral, ownership, and
allocation acceptance criteria remain unchanged.

## Acceptance Criteria

- The final CLI uses only the compiled scalar engine.
- The interpreter and fixed clause dispatch are absent from production code.
- R1-R5 and every edge fixture match the tracked JSON byte for byte.
- Policy semantics are represented through generic expression instructions.
- Program, batch, context, and result hot data use SoA, CSR, or bitplanes.
- Warm `EvaluateInto` performs no map lookup, reflection, or string comparison;
  cold `Builder.BuildInto` may use maps and source strings.
- Warm `EvaluateInto` reports zero bytes and zero allocations per operation.
- Scalar execution processes complete columns or mask words.
- Invalid source, program, batch, or capacity returns a bounded error without
  partial result mutation.
- The README, one-page note, architecture document, and performance document
  describe only implemented behavior.
- The README links to `https://github.com/sebishogun/Verifoxx` and explains the
  architectural progression.
- Production and total Go code stay within the approved size range unless a
  measured or correctness requirement justifies the difference.

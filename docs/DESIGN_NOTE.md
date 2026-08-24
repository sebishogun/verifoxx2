# Verifoxx Design Note

## From Requirements To A Semantic Model

The requirements become a bounded JSON expression AST, not request-specific
code. Ordered requirements define applicability and clauses using `equal`,
`in`, `all`, and qualified `evidence_matches`. Each clause maps eight
truth/evidence states to a decision, explanation, and optional remediation.
The shipped model is [`policies/policy.json`](../policies/policy.json).

This is more than flat extraction because a boolean cannot distinguish absent
approval from invalid, stale, incomplete, unverifiable, or conflicting
evidence. The evaluator preserves positive, negative, and reason planes.
Numeric results retain applicable requirements, evidence, the winning
clause/reason, uncertainty, and remediation before those IDs become bounded
human-readable output.

## Decision Process

All applicable clauses are evaluated. Findings use
`Reject > Escalate > Revise > Approve`; equal ranks retain request,
requirement, and clause order. This makes hard disclosure violations win over
uncertainty, and uncertainty win over a bounded usage correction. The supplied
requests resolve to `Approve`, `Reject`, `Revise`, `Escalate`, and `Escalate`
for R1 through R5 respectively.

`Escalate` covers unknown or missing semantics, absent references, stale,
unclear or conflicting mandatory evidence, and unverifiable environments. The
engine does not guess. `Revise` is reserved for an explicit bounded correction,
such as lowering usage or supplying an allowed evidence item. `Reject` remains
a configured non-negotiable violation.

## Performance-Aware Runtime

Validation and postorder compilation turn policy text into an immutable numeric
`Program` of parallel opcode arrays, typed IDs, interned text, and CSR ranges.
Requests and evidence become a field-major `Batch`; a private `Context` holds
truth/reason bitplanes; caller-owned `result.Batch` columns receive decisions;
`OutputPack` reconstructs text only after evaluation.

```text
Policy -> Program -> Batch -> Context/result.Batch -> OutputPack
```

The data layout is the performance decision: compile once, resolve strings
before the kernel, keep columns contiguous, size storage before evaluation, and
reuse one private `Session` per sequential worker. Correctly sized warm
evaluation performs zero allocations. The implementation is scalar; SIMD and
parallel row scheduling are extension seams rather than current claims.

## Scope And Next Steps

This repository remains one process with no service, persistence,
natural-language parser, or policy authoring system. Future work includes richer
vocabularies, reviewed authoring, fuzzing, broader provenance, SIMD kernels, and
row scheduling. See [`architecture.md`](architecture.md) for types, layouts,
ownership, and edge boundaries, and [`performance.md`](performance.md) for
reproducible measurements and exclusions. The larger
[Verifoxx](https://github.com/sebishogun/Verifoxx) project explores the broader
runtime.

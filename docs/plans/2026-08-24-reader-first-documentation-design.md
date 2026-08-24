# Reader-First Documentation Design

## Goal

Make the repository understandable in layers. A first-time reviewer should be
able to learn what Verifoxx decides, why the model exists, how to run and verify
it, and where to inspect each claim before encountering compiler and storage
details.

## Audience And Reading Order

The documentation serves three readers:

1. An exercise reviewer checking the five decisions and reasoning.
2. A Go engineer tracing data from JSON through the evaluator.
3. A performance engineer checking layouts, ownership, allocations, and
   measurements.

The root README is the entry point for all three. It should present this order:

1. Plain-English purpose and one-minute system walkthrough.
2. Quick verification and the five supplied decisions.
3. A reading guide identifying the source of truth for each topic.
4. Key semantic and engineering choices with their reasons.
5. Edge cases that demonstrate uncertainty, precedence, and relevance.
6. Input, policy, output, and CLI reference material.
7. Repository layout, operational tooling, limits, and performance summary.

## Documentation Responsibilities

| Document | Responsibility |
| --- | --- |
| `Requirements.md` | Original exercise and decision meanings |
| `README.md` | Human-first overview, verification path, decisions, and navigation |
| `docs/DESIGN_NOTE.md` | One-page submission explanation and key tradeoffs |
| `docs/architecture.md` | Execution flow, data layouts, core structs, ownership, and extension seams |
| `docs/performance.md` | Reproducible measurements, allocation contracts, and exclusions |
| `policies/policy.json` | Executable semantic policy and reason-to-outcome mappings |
| `fixtures/` and `results/` | Supplied inputs, edge inputs, and exact expected outputs |
| `docs/plans/` | Historical design and implementation records, not primary reference docs |

The README summarizes rather than duplicates deep architecture or benchmark
tables. It links readers to the canonical detail.

## Performance-Aware Model

The project is performance-focused even though it is intentionally scalar and
single-process. Its design follows these tenets:

- Model data before control flow. Runtime policy and request data use parallel
  arrays, bitplanes, and CSR ranges rather than per-row object graphs.
- Compile and validate once. Evaluation consumes typed numeric IDs and immutable
  arrays rather than repeatedly interpreting strings and maps.
- Allocate outside the per-row kernel. Callers size reusable context and result
  storage before `EvaluateInto`; the warmed evaluator performs zero allocations.
- Process in bulk. Instructions operate over row bitplanes and contiguous field
  columns, leaving a direct boundary for future SIMD and row sharding.
- Make lifetimes explicit. An immutable `Engine` is shared; each sequential
  worker owns one private reusable `Session`.
- Avoid hidden reuse. The implementation uses caller-owned storage rather than
  `sync.Pool`, so ownership and overwrite rules remain visible.
- Measure each lifecycle separately. Cold compilation, first use, steady framed
  processing, materialization, and warm evaluation have separate benchmarks.

These are implemented constraints, not claims that SIMD, parallel scheduling,
or a production service already exists.

## Core Type Story

The architecture document will identify the types that carry data through the
system:

| Type | Role |
| --- | --- |
| `ast.Policy` | Validated source representation of requirements and clauses |
| `program.Program` | Immutable numeric policy with parallel arrays and interned text |
| `eval.Builder` | Resolves input strings and builds reusable numeric batches |
| `eval.Batch` | Field-major request columns, evidence columns, and CSR references |
| `eval.Context` | Reusable positive, negative, and reason bitplanes |
| `eval.Evaluator` | Applies compiled instructions and deterministic resolution |
| `result.Batch` | Caller-owned numeric decisions, drivers, provenance, and remediation IDs |
| `engine.Engine` | Shareable owner of one validated immutable program |
| `engine.Session` | Private reusable state for one sequential request-pack worker |
| `jsonio.OutputPack` | Human-readable result objects reconstructed at the boundary |

## Edge-Case Story

The README will expose the eight golden edge scenarios rather than merely link
to raw JSON. It will also summarize the deeper invariant tests by category:

- Evidence state and qualifier reduction.
- Semantic uncertainty and missing references.
- Outcome precedence and deterministic ties.
- Strict JSON, IDs, bounds, and transport failures.
- Reuse, no-partial-mutation, output replacement, and independent sessions.

This keeps the README concise while making the intended failure model visible.

## Scope

This change edits documentation only. It does not change policy semantics,
runtime behavior, result bytes, public flags, measured numbers, or package
boundaries.

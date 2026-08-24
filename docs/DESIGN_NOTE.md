# Verifoxx Design Note

## Intermediate Representation

The requirements are a bounded JSON expression AST, not request-specific code.
Ordered requirements have applicability and clauses built from `equal`, `in`,
`all`, and qualified `evidence_matches`. Each clause maps eight truth/evidence
states to a decision, explanation, and optional remediation. Validation and
postorder compilation produce an immutable numeric `Program` of parallel
opcode arrays, typed IDs, interned runtime strings, and CSR ranges. Evaluation
needs no policy-string lookup or clause-specific dispatch.

## Why This Is More Than Flat Extraction

A boolean cannot distinguish absent approval from stale, revoked, incomplete,
unverifiable, or conflicting approval. The evaluator preserves positive,
negative, and reason planes. Numeric results retain requirements, evidence,
the winning clause/reason, uncertainty, and remediation; materialization turns
that provenance into bounded text.

## Decision Process

The CLI compiles once, builds structure-of-arrays columns and CSR evidence
links, evaluates reusable bitplanes, and resolves every applicable clause.
Findings use `Reject > Escalate > Revise > Approve`; equal ranks retain request,
requirement, and clause order. R1 is `Approve`; R2 is `Reject` for hard
disclosure; R3 is `Revise` because its missing approval is bounded; R4 and R5
are `Escalate`.

## Escalation Boundaries

`Escalate` covers malformed semantics, absent references, unsafe evidence
state, and unverifiable environments. R4 lacks a verifiable environment; R5
has conflicting valid/revoked approval. The engine does not guess. `Revise`
and `Reject` remain configured bounded corrections and hard violations. Future
work includes richer vocabularies, provenance, reviewed authoring, and fuzzing;
broader runtime work belongs to
[Verifoxx](https://github.com/sebishogun/Verifoxx).

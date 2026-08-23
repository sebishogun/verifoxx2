# Verifoxx Design Note: Semantic Decision Representation

## 1. Intermediate Representation Overview

The policy engine models natural-language requirements (`R1`–`R3`) using an explicit intermediate semantic representation (IR) rather than flat field extractions or hardcoded request branching.

The IR separates requirements into three distinct operational abstractions:
* **Non-Negotiable Constraints**: Absolute boundary conditions (such as R1's restriction prohibiting individual-level record exports on protected datasets). Violations immediately produce a `Reject` decision.
* **Revisable Conditions**: Requirements that can be satisfied through bounded corrective changes (such as R3's rule requiring a specific `usage_limit_adjustment` approval for trusted internal teams requesting above-standard capacity). Failure to provide this approval yields a `Revise` decision along with explicit remediation actions.
* **Verifiable Environment & Provenance Attestations**: Verification rules requiring evidence to be valid, current, and unconflicting (such as R2's requirement for verified local execution environments and R1/R3's requirement for unconflicting pre-execution approvals). Unverified, missing, or conflicting attestations produce an `Escalate` decision.

---

## 2. Why Semantic Modeling Superiority Over Flat Extraction

Flat field extraction techniques (such as evaluating dynamic JSON paths or generic boolean logic) suffer from critical failure modes in safety-critical governance contexts:

1. **Explicit Uncertainty & Conflict Resolution**: Flat boolean evaluators collapse missing data or conflicting approvals into binary `false` values, causing false rejections or silent failures. The semantic IR explicitly distinguishes between a clear policy violation (`Reject`), a fixable missing document (`Revise`), and a security ambiguity (`Escalate`).
2. **Deterministic Rationale & Audit Provenance**: Every decision is tied to specific applied requirements (`requirements_applied`), used evidence IDs (`evidence_used`), and explicit statements of unresolved uncertainty (`unresolved_uncertainty`).

---

## 3. Decision Logic and Escalation Boundaries

Decisions are computed using strict ordering:

1. **Non-Negotiable Violation Check (`Reject`)**: If a request attempts forbidden operations (such as row-level data exports), it is rejected immediately.
2. **Environment & Attestation Check (`Escalate`)**: If the execution environment is unverified (`unverified_remote_env` or missing `E2` attestation), or if attached approvals contain conflicting states (`E4`), the engine escalates.
3. **Usage Adjustment Check (`Revise`)**: If a trusted internal team requests above-standard capacity without a matching `usage_limit_adjustment` approval (`E3`), the engine issues a `Revise` outcome with a structured `add_evidence` remediation.
4. **Full Approval (`Approve`)**: If all constraints, attestation checks, and pre-execution approval conditions are met, the engine returns `Approve`.

---

## 4. Next Improvements and Scaling Strategy

While this baseline implementation provides full semantic coverage for the assignment brief, future extensions can scale the engine:

* **Zero-Allocation Bytecode Kernel**: Lowering the IR into a Structure-of-Arrays (SoA) columnar program for zero-allocation (`0 B/op`) batch evaluation.
* **Bitplane Vector Execution**: Evaluating rules using 64-bit uint64 bitplanes and AVX2/AVX-512 SIMD vector operations (`120+ GB/s` throughput).
* **Multi-Language Frontends**: Providing parsers for Google CEL and OPA Rego to compile existing enterprise policies down to the zero-allocation runtime.

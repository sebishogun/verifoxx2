# Verifoxx Design Note

## Semantic Model

I modeled each requirement in two parts: when it applies and what must be true
when it does. Applicability uses request fields such as trust level, dataset,
and usage limit. Clauses describe the required request values or supporting
evidence. For every clause, the policy records the decision and explanation for
eight states: satisfied, false, missing, invalid, stale, unclear, unverifiable,
and conflicting. The complete model is in
[`policies/policy.json`](../policies/policy.json).

This keeps more meaning than flat field extraction. A missing approval, a stale
approval, and two conflicting approvals are different situations and can lead
to different decisions. Results also retain the requirements that applied, the
evidence considered, any remaining uncertainty, and an allowed remediation.
Nothing branches on the five supplied request IDs.

## Decision Process

For each request, the evaluator validates the request fields, resolves its
evidence references, and checks every applicable clause. The final precedence
is `Reject > Escalate > Revise > Approve`. Policy order breaks ties, so the same
input always produces the same rationale.

`Reject` is used for a non-negotiable violation such as individual-level
disclosure. `Revise` is used when a specific change can make the request
acceptable, such as lowering the usage limit or supplying an allowed approval.
`Escalate` is used when required facts cannot be established safely, including
missing references, stale or conflicting evidence, and an unverifiable
environment. The supplied requests produce `Approve`, `Reject`, `Revise`,
`Escalate`, and `Escalate` for R1 through R5.

## Performance-Aware Runtime

The policy is validated and compiled once. Request and evidence strings are
then converted to numeric columns before evaluation, while explanations are
rebuilt afterward. This adds some implementation complexity, but repeated
evaluation avoids string lookups and allocation in the warm path. The runtime
is currently scalar and sequential. The detailed layouts are documented in
[`architecture.md`](architecture.md), and the benchmark setup and results are
in [`performance.md`](performance.md).

## Scope And Next Steps

The evaluator assumes the structured request and evidence fields faithfully
represent their source records. My next priorities would be safer policy
authoring, fuzzing malformed policies and evidence packs, and more detailed
provenance for reviewers. A service or persistence layer would come later, once
there was a concrete deployment need.

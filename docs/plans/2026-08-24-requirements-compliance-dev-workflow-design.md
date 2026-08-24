# Requirements Compliance And Reviewer Workflow Design

**Date:** 2026-08-24

**Status:** Approved

## Objective

Bring this exercise-sized repository into full compliance with `Requirements.md`,
including semantic behavior beyond the five supplied examples, while preserving
minimal setup. Add a discoverable Make/Bash reviewer workflow, an optional
repository-local Go installer, and an optional single-image Docker path.

The supplied requests remain the conformance baseline:

| Request | Required decision |
|---|---|
| R1 | Approve |
| R2 | Reject |
| R3 | Revise |
| R4 | Escalate |
| R5 | Escalate |

## Scope

The implementation will:

- Preserve the natural-language applicability of R1-R3.
- Distinguish non-negotiable violations, bounded revisions, and uncertainty.
- Evaluate all applicable clauses before resolving one final decision.
- Produce deterministic, policy-derived explanations and provenance.
- Validate malformed policy and input documents safely.
- Document setup, dependencies, formats, AI-tool use, and the follow-on project.
- Keep existing Make targets and add script-backed reviewer workflows.
- Support local Go, repository-local Go, Docker, and Docker Compose execution paths.

The implementation will not add a long-running service, database, web UI, or
production compiler/runtime features. Those belong in the separate follow-on
repository.

## Semantic Representation

The policy remains data in `policies/policy.json`; outcomes will not branch on
request IDs. Each requirement will describe:

- Applicability to request fields such as trust, dataset, and usage.
- One or more semantic clauses.
- The assertion or evidence condition for each clause.
- The outcome for false, missing, invalid, stale, unclear, unverifiable, and
  conflicting states.
- An optional bounded remediation.

The bounded clause vocabulary will cover:

- Disclosure constraints over action and output.
- Required execution environment and matching attestation.
- Required pre-execution approval.
- Above-standard usage entitlement and matching adjustment approval.

Evidence qualifiers will include type, status, timing, subject, scope, reviewer,
timestamp state, attestation state, and adjustment type where applicable.

## Evaluation

The evaluator will perform these stages:

1. Validate request fields needed to establish applicability. Unknown or missing
   semantic values become unresolved uncertainty and produce Escalate.
2. Resolve referenced evidence in request order. A missing referenced record is
   explicit uncertainty, not a note attached to an approval.
3. Evaluate every applicable requirement and clause.
4. Record each clause result, evidence considered, uncertainty, and remediation.
5. Resolve the final candidate outcomes with deterministic precedence:
   `Reject > Escalate > Revise > Approve`.
6. Build one deterministic result from the evaluated clauses.

This ordering ensures that a non-negotiable disclosure violation wins over all
other states, and that missing or conflicting mandatory evidence wins over a
bounded usage revision.

For above-standard usage, only trusted internal teams can use a qualifying
adjustment approval. An external requester must lower usage to standard, so the
bounded result is Revise rather than Approve. A trusted internal requester with
no qualifying adjustment can add the allowed evidence and receive Revise. Stale,
unclear, or conflicting approval evidence produces Escalate.

## Result Model

The required result fields remain:

- `request_id`
- `decision`
- `rationale`
- `requirements_applied`
- `evidence_used`
- `missing_or_conflicting_evidence`
- `assumptions`
- `unresolved_uncertainty`
- optional structured `remediation`

A structured driver may identify the requirement, clause, and reason that won
outcome resolution. Requirement IDs and evidence IDs will come from evaluated
policy and input data. Policy name and version will come from the loaded policy.
Slices will preserve policy or request order; map iteration will not affect JSON.

File output will be atomic. `--output -` will write machine-readable JSON to
stdout while human progress goes to stderr.

## Validation And Errors

Malformed JSON, duplicate top-level IDs, unknown clause kinds, missing policy
fields, and invalid policy outcome names will fail before evaluation with a clear
nonzero exit. Valid documents containing semantic uncertainty will still produce
a result, normally Escalate, so uncertainty remains auditable.

The CLI will create an output parent directory when needed and will never emit
human narration into JSON written to stdout.

## Tests

Tests will be written before each semantic correction. Coverage will include:

- Exact R1-R5 decisions and result provenance.
- External above-standard usage with and without adjustment evidence.
- Individual-level output hidden behind an aggregate action label.
- Simultaneously missing pre-execution and usage-adjustment approvals.
- Wrong adjustment type, scope, reviewer, status, or timestamp state.
- Missing, stale, revoked, unclear, and conflicting evidence.
- Missing referenced evidence IDs with otherwise valid evidence.
- Unknown or missing semantic request values.
- Irrelevant attached evidence that must not change a decision.
- Dynamic policy IDs and deterministic output ordering.
- Strict loader and policy-validation errors.
- CLI stdout/file behavior and committed result conformance.
- Bash syntax and non-interactive script paths.

## Make And Bash Workflow

Existing Make targets remain. The expanded command surface will include:

- `make setup`: check prerequisites and download Go modules.
- `make doctor`: report Bash, Make, Go, Docker, and optional fzf status.
- `make install-go`: reuse any runnable Go, otherwise install the pinned toolchain under `.tools/go`.
- `make shell`: install the global, cross-shell `mm` shortcut.
- `make check`: run formatting checks, tests, vet, build, and conformance.
- `make menu`: open fzf when available or a numbered Bash fallback.
- `make demo`: run checks, evaluate R1-R5, and present the required outcomes.
- `make demo-edge`: evaluate a separate edge-case request/evidence pack.
- `make docker-build`: build the single application image.
- `make docker-demo`: run the same demonstration in Docker.

Scripts will live under `scripts/` and resolve the repository root from their own
location. Inline `##` target descriptions in the Makefile are the single source
for `make help`, `make targets`, the fzf picker, and the numbered fallback. The
menu shows dependency readiness and recipe previews, then delegates back to Make,
keeping Make as the canonical command surface.

### Global `mm` Shortcut

`make shell` will build and install a host-native `mm` executable in the user's
bin directory. The executable is independent of this repository: it searches
from the current directory toward the filesystem root for the nearest Makefile
that defines a `menu` target, then runs `make -C <project> menu` with the current
terminal attached. If no matching project exists, it exits nonzero with a short
diagnostic.

The installer will be idempotent and preserve existing shell configuration. On
Unix it will make the user bin available to Bash, Zsh, Fish, PowerShell, and
Nushell. On Windows it will install `mm.exe` and update the per-user PATH so the
command is available to PowerShell, cmd.exe, Nushell, and Unix-style shells in a
new terminal. Re-running `make shell` will replace the helper only when needed
and will not duplicate PATH/profile entries.

Tests will cover nearest-project selection, invocation from nested directories,
missing menu targets, exit-code propagation, profile generation, repeated
installation, and Windows compilation. `make menu` remains the canonical direct
entry point and continues to work without installing `mm`.

## Go Bootstrap

`scripts/install-go.sh` will be explicit and opt-in. It will:

- Exit without network or file changes when a runnable Go already exists.
- Detect supported OS and architecture combinations.
- Download the pinned official Go archive and checksum over HTTPS.
- Verify the archive before extraction.
- Install only under `.tools/go`, without `sudo` or global filesystem changes.
- Leave system Go installations untouched.

A small wrapper will prefer `.tools/go/bin/go` and otherwise use `go` from PATH.
`.tools/` will be ignored by Git.

## Docker

A multi-stage Dockerfile will compile a static binary with Go 1.27 and copy only
the binary plus assignment policy and fixtures into the runtime image. Docker is
optional. `compose.yaml` will expose the same one-shot CLI image without adding
ports or external service dependencies.

The default container command will evaluate the supplied pack and emit JSON to
stdout. The Docker demo target will run the conformance and edge demonstrations
through the same CLI behavior used locally.

## Documentation

The README will describe:

- The assignment-sized purpose and the four outcomes.
- Go 1.27, Bash, Make, optional fzf, and optional Docker prerequisites.
- Local, repository-local Go, and Docker quickstarts.
- Make targets and direct CLI flags.
- Policy, request, evidence, and output JSON formats.
- Expected R1-R5 decisions.
- AI-tool use in requirement review, edge-case analysis, implementation support,
  testing, and documentation editing.
- A follow-on note explaining that the ideas were taken further in the separate
  Verifoxx repository at `https://github.com/sebishogun/Verifoxx`.

The one-page design note will retain its required topics and remove unsupported
coverage claims. The existing production-evolution document will be removed or
reframed so this repository does not claim unimplemented SIMD, zero-allocation,
or benchmark results. The broken license reference will be removed unless an
actual license file is added later.

## Acceptance Criteria

The work is complete when:

- All mandatory items in `Requirements.md` are present and documented.
- R1-R5 produce the required decisions and exact deterministic result fields.
- Every identified semantic counterexample has a passing regression test.
- `make check`, `make demo`, and `make demo-edge` pass from a clean checkout with
  a supported Go toolchain.
- The fzf and numbered menu paths are both valid.
- The local Go bootstrap is checksum-verified and never requires root access.
- The Docker image builds and its default evaluation succeeds when Docker is
  available.
- Generated `results/requests.json` matches the tested conformance output.
- No documentation claims features or benchmark numbers absent from this repo.

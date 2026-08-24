# Verifoxx

Verifoxx turns structured policy requirements, requests, and evidence into one
of four deterministic decisions:

- `Approve`: the applicable requirements and evidence are satisfied.
- `Reject`: a non-negotiable condition is violated.
- `Revise`: a bounded change can make the request acceptable.
- `Escalate`: uncertainty or unsafe evidence prevents an automatic decision.

This repository implements the semantic decision exercise in
[`Requirements.md`](Requirements.md). The policy is data in
[`policies/policy.json`](policies/policy.json); outcomes do not branch on the
five supplied request IDs. Runtime decisions are made by deterministic Go code
and do not call an AI model.

## What This Repository Does

The exercise describes three policy requirements in prose:

1. External partners may use protected data only for aggregate output, with a
   valid approval recorded before execution.
2. Protected-data processing must run in the verified local environment.
3. Trusted internal teams may exceed the standard usage limit only with a
   specific adjustment approval; disclosure and pre-execution approval rules
   still apply.

Verifoxx represents those rules as a small JSON expression language, validates
and compiles that source once, converts request and evidence fields into numeric
columns, evaluates every applicable clause, and reconstructs bounded
human-readable results at the output boundary.

```text
policy JSON -> validated source AST -> immutable numeric Program
request/evidence JSON -> columnar Batch + CSR evidence references
Program + Batch -> truth/reason bitplanes -> numeric result Batch
numeric results + source IDs -> human-readable OutputPack -> JSON
```

This implementation is a single-process Go CLI built with the standard
library. It does not include persistence, a network service, natural-language
parsing, SIMD, or parallel scheduling.

## Quickstart

Run the complete local check and golden-file demonstration:

```bash
make demo
```

`make demo` runs formatting checks, fresh tests, installer and menu
regressions, `go vet`, and a build. It then evaluates the supplied and edge
packs in a temporary directory and compares both outputs byte-for-byte with
[`results/requests.json`](results/requests.json) and
[`fixtures/demo/expected.json`](fixtures/demo/expected.json).

To build and regenerate the tracked supplied-pack output:

```bash
make all
```

If Go is unavailable, use `make docker-demo` or `make compose-demo`. Neither
requires a host Go installation.

## Supplied Decisions

The evaluator checks every applicable clause, then chooses the highest-ranked
finding:

```text
Reject > Escalate > Revise > Approve
```

This ordering keeps a hard violation above uncertainty and uncertainty above a
bounded revision. Findings with the same rank retain deterministic first
occurrence: request semantic checks, missing evidence references, then policy
requirement and clause order.

| Supplied request | Decision | Driver |
| --- | --- | --- |
| R1 | Approve | Aggregate output, valid pre-execution approval, and verified local environment |
| R2 | Reject | Individual-record export violates the non-negotiable disclosure rule |
| R3 | Revise | Above-standard usage lacks the required adjustment approval |
| R4 | Escalate | The requested execution environment cannot be verified |
| R5 | Escalate | Approval evidence contains conflicting valid and revoked state |

The exact machine-readable output, including provenance and uncertainty, is in
[`results/requests.json`](results/requests.json).

## Reading Guide

| Question | Source of truth |
| --- | --- |
| What was the original exercise? | [`Requirements.md`](Requirements.md) |
| What is the short design argument? | [`docs/DESIGN_NOTE.md`](docs/DESIGN_NOTE.md) |
| How are the rules represented? | [`policies/policy.json`](policies/policy.json) |
| How does data move through the engine? | [`docs/architecture.md`](docs/architecture.md) |
| How was performance measured and reproduced? | [`docs/performance.md`](docs/performance.md) |
| What were the supplied inputs? | [`fixtures/requests.json`](fixtures/requests.json) and [`fixtures/evidence.json`](fixtures/evidence.json) |
| What output must remain exact? | [`results/requests.json`](results/requests.json) |
| Which semantic edge cases are demonstrated? | [`fixtures/demo/requests.json`](fixtures/demo/requests.json) and [`fixtures/demo/expected.json`](fixtures/demo/expected.json) |
| Where are golden checks enforced? | [`internal/conformance/golden_test.go`](internal/conformance/golden_test.go) |
| Why were the major architecture choices made? | [`docs/plans/`](docs/plans/) |

The design records explain why the larger changes were made. The README, design
note, architecture, policy, fixtures, and performance report are the current
reference documents.

## Key Design Choices

| Choice | Why | Consequence |
| --- | --- | --- |
| Policy as a generic expression AST | Rules must be data, not hardcoded branches for R1-R5 | New policies use the same compiler and evaluator vocabulary |
| Eight explicit reason states | A boolean cannot distinguish missing, invalid or revoked, stale, unclear, unverifiable, or conflicting evidence | Each clause configures a decision and explanation for every state |
| Compile once to an immutable numeric program | Repeated string lookup and clause-specific dispatch do not belong in evaluation | The warm evaluator uses typed IDs and array indexes |
| Structure-of-arrays request and evidence storage | Field scans should touch contiguous, uniformly typed memory | The scalar kernel has a direct future path to SIMD and row sharding |
| CSR request-to-evidence links | Evidence relationships are variable-length but naturally grouped by request | Each row scans one bounded contiguous edge range |
| Caller-owned reusable context and results | Per-row allocation would make the hot path scale with allocator pressure | Correctly sized warm evaluation reports `0 B/op` and `0 allocs/op` |
| Evaluate all clauses, then apply fixed precedence | Early exit can hide a stronger violation or uncertainty | `Reject` outranks `Escalate`, which outranks `Revise` and `Approve` |
| Materialize text only after numeric evaluation | Explanations and JSON are boundary concerns | Strings, formatting, and output allocation stay outside the kernel |
| Immutable `Engine`, private reusable `Session` | Shared policy data and mutable scratch have different lifetimes | Sessions can run independently without locks or `sync.Pool` |
| Strict bounded input and framed transport | User-controlled data must fail predictably before unsafe growth or partial mutation | Limits, diagnostics, and fatal transport boundaries are explicit |

### Performance Tenets

Performance work starts with data layout and object lifetime rather than
low-level instructions:

1. Put columnwise data in contiguous parallel arrays.
2. Compile and validate policy text once; do not interpret it per request.
3. Allocate and size storage before entering the per-row evaluator.
4. Process rows in bulk through bitplanes and contiguous evidence ranges.
5. Keep immutable program data shared and mutable session data private.
6. Avoid hidden pooled ownership; reuse caller-visible storage instead.
7. Measure cold compilation, first use, steady framing, materialization, and
   warm evaluation separately.

The evaluator currently runs scalar and sequentially. Its columnar layout can
support SIMD or row sharding later. See
[`docs/architecture.md`](docs/architecture.md) for layouts and ownership. The
benchmark commands, measured results, and exclusions are recorded in
[`docs/performance.md`](docs/performance.md).

## Edge Cases We Designed For

The committed edge pack makes the intended boundaries visible:

| Case | Decision | Principle demonstrated |
| --- | --- | --- |
| `EDGE_EXTERNAL_ABOVE` | Revise | An external requester must lower above-standard usage even when adjustment evidence is attached |
| `EDGE_INDIVIDUAL_OUTPUT` | Reject | Individual records remain forbidden even when the action is labelled aggregate analysis |
| `EDGE_MISSING_APPROVALS` | Escalate | Simultaneously missing pre-execution and usage-adjustment approvals remain uncertainty, with deterministic driver selection |
| `EDGE_WRONG_ADJUSTMENT` | Revise | Wrong adjustment qualifiers produce a bounded request for valid evidence |
| `EDGE_MISSING_REFERENCE` | Escalate | A referenced but absent evidence record preserves its ID and uncertainty |
| `EDGE_UNKNOWN_ACTION` | Escalate | Unknown request semantics are not silently accepted |
| `EDGE_IRRELEVANT_CONFLICT` | Approve | Conflicting evidence of an irrelevant kind must not contaminate a satisfied rule |
| `EDGE_MIXED_DISCLOSURE` | Reject | A hard disclosure violation outranks simultaneous semantic uncertainty |

The test suite goes further than the golden pack:

| Boundary | Examples | Tests |
| --- | --- | --- |
| Evidence reduction | missing, wrong, revoked, stale, unclear, conflicting, expired, wrong timing/reviewer/subject/scope/adjustment | [`internal/eval/evidence_test.go`](internal/eval/evidence_test.go) |
| Resolution | no applicable requirement, equal-rank ties, `Reject` over semantic `Escalate` | [`internal/eval/executor_test.go`](internal/eval/executor_test.go) |
| Source admission | malformed expressions, duplicate IDs, incomplete resolutions, invalid remediation, depth and size limits | [`internal/ast/validate_test.go`](internal/ast/validate_test.go) |
| JSON boundaries | unknown fields, trailing values, oversized documents, malformed IDs | [`internal/adapters/jsonio/`](internal/adapters/jsonio/) |
| Framed transport | fragmented reads, partial frames, oversized frames, short writes, recoverable invalid input | [`internal/adapters/framed/`](internal/adapters/framed/) and [`cmd/verifoxx/stream_test.go`](cmd/verifoxx/stream_test.go) |
| Reuse and ownership | shrinking/growing packs, stale-state poisoning, no partial mutation, independent sessions | [`internal/engine/engine_test.go`](internal/engine/engine_test.go) and [`internal/eval/`](internal/eval/) |
| File output | writer failures, destination preservation, temporary-file cleanup | [`internal/adapters/jsonio/output_test.go`](internal/adapters/jsonio/output_test.go) |

## Semantic Model

### Policy Format

The top-level policy contains `name`, `version`, and one or more ordered
`requirements`. A requirement contains:

| Field | Meaning |
| --- | --- |
| `id` | Unique requirement ID |
| `description` | Requirement text retained as source metadata |
| `non_negotiable` | Policy metadata for a hard constraint |
| `applicability` | Expression deciding which requests the requirement governs |
| `clauses` | Assertions and their explicit reason-to-outcome mappings |

Each clause has a globally unique `id`, an `assertion`, a `resolution` for all
eight reason states, optional per-state `explanations`, and optional bounded
`remediations`.

#### Expressions

| Operator | Shape | Meaning |
| --- | --- | --- |
| `all` | `children` | All child expressions must be satisfied |
| `equal` | `field`, `value` | Request field equals one symbol |
| `in` | `field`, `values` | Request field belongs to a non-empty unique set |
| `evidence_matches` | `evidence` | Referenced evidence of one kind satisfies every supplied qualifier |

Request fields are `requester`, `trust_level`, `action`, `output_kind`,
`dataset`, `environment`, and `usage_limit`. `evidence_matches` is allowed only
in clause assertions. Its predicate requires `kind` and can constrain `status`,
`timing`, `reviewer`, `timestamp_state`, `subject`, `attestation_state`, `scope`,
and `adjustment_type`.

Policy literals and `set_field` remediations use the closed exercise
vocabulary: `external`/`trusted_internal`,
`aggregate_analysis`/`row_level_export`,
`aggregate_counts`/`individual_records`, `protected_dataset`,
`local_approved_env`/`unverified_remote_env`, and
`standard`/`above_standard_limit`; requester values may be any non-empty
trimmed string. Supported evidence kinds are `approval_record`,
`execution_environment_attestation`, and `usage_limit_adjustment`. Unknown
policy literals or remediation values are load-time validation errors.

Every clause resolution supplies an outcome for `satisfied`, `false`,
`missing`, `invalid`, `stale`, `unclear`, `unverifiable`, and `conflict`.
Non-Approve states require bounded explanation text. Any clause that can
resolve to `Revise` requires at least one remediation:

- `add_evidence`: `evidence_kind` plus an optional description.
- `set_field`: a known request `field` and a non-empty `value`.

[`policies/policy.json`](policies/policy.json) is the complete shipped example.

### Compile And Evaluate

The CLI has one production execution path:

```text
strict JSON -> source AST -> compiler -> immutable Program
strict JSON -> requests/evidence -> columnar Batch + CSR references
Program + private reusable Session -> Batch + Context + numeric result Batch
numeric results + source IDs -> OutputPack -> JSON
```

1. Policy JSON is size-limited, strictly decoded, and validated.
2. The compiler interns strings into one immutable text slab and lowers
   expressions in deterministic postorder to typed opcodes, numeric IDs, and
   CSR ranges.
3. Request and evidence strings are resolved once while building
   structure-of-arrays columns. Missing evidence references become explicit
   sentinel edges.
4. The evaluator fills reusable positive, negative, and reason bitplanes, then
   resolves all applicable clauses into caller-owned numeric result columns.
5. Human-readable strings and JSON objects are reconstructed only after the
   warm evaluation stage.

The warmed evaluator and batch builder perform no per-row allocation and
contain no runtime string allocation or hash lookup.

### Core Runtime Types

| Type | Source | Role and lifetime |
| --- | --- | --- |
| `ast.Policy` | [`internal/ast/policy.go`](internal/ast/policy.go) | Human-readable source representation; discarded after compilation |
| `program.Program` | [`internal/program/program.go`](internal/program/program.go) | Validated immutable opcodes, typed IDs, CSR ranges, resolution tables, and interned text |
| `eval.Builder` | [`internal/eval/builder.go`](internal/eval/builder.go) | Reuses validation and lookup maps while converting input DTOs to numeric columns |
| `eval.Batch` | [`internal/eval/batch.go`](internal/eval/batch.go) | Field-major request values, evidence qualifier columns, and CSR reference edges |
| `eval.Context` | [`internal/eval/context.go`](internal/eval/context.go) | Private reusable positive, negative, and reason bitplanes |
| `eval.Evaluator` | [`internal/eval/executor.go`](internal/eval/executor.go) | Applies immutable instructions and deterministic result precedence |
| `result.Batch` | [`internal/result/batch.go`](internal/result/batch.go) | Caller-owned numeric decisions, drivers, provenance, and remediation IDs |
| `engine.Engine` | [`internal/engine/engine.go`](internal/engine/engine.go) | Long-lived shareable owner of one validated immutable program |
| `engine.Session` | [`internal/engine/engine.go`](internal/engine/engine.go) | Private reusable builder, batch, context, result, and materialized output for one sequential worker |
| `jsonio.OutputPack` | [`internal/adapters/jsonio/output.go`](internal/adapters/jsonio/output.go) | Human-readable JSON-facing results reconstructed at the adapter boundary |

The deep layouts, capacity contract, and ownership rules are documented in
[`docs/architecture.md`](docs/architecture.md).

### Input And Output

Request and evidence documents are JSON arrays. Unknown object fields,
duplicate or empty IDs, malformed IDs, oversized documents, and trailing JSON
are rejected.

Request fields:

```text
id, requester, trust_level, action, output_kind, dataset,
environment, usage_limit, evidence_ids
```

Evidence fields:

```text
id, type, status, timing, reviewer, reviewer_state, timestamp_state,
subject, attestation_state, scope, adjustment_type
```

Unknown or missing request semantic values are not silently accepted; they
produce `Escalate` with the affected field. A referenced evidence ID absent
from the evidence pack also escalates with the missing ID preserved.

The output top level contains `schema_version`, `policy_name`,
`policy_version`, and `results`. Each result contains:

| Field | Meaning |
| --- | --- |
| `request_id` | Source request ID |
| `decision` | `Approve`, `Reject`, `Revise`, or `Escalate` |
| `rationale` | Explanation from the winning driver |
| `requirements_applied` | Applicable requirements in policy order |
| `evidence_used` | Required existing evidence in request order, deduplicated |
| `missing_or_conflicting_evidence` | Bounded evidence detail, when applicable |
| `assumptions` | Explicit source-data assumption |
| `unresolved_uncertainty` | Remaining uncertainty for the winning driver |
| `remediation` | Bounded changes, present only for `Revise` |

## Running The CLI

### One-Shot Files

```bash
make build
./bin/verifoxx \
  --policy policies/policy.json \
  --requests fixtures/requests.json \
  --evidence fixtures/evidence.json \
  --output -
```

The four paths default to the supplied files, with output defaulting to
`results/requests.json`. `--output -` reserves stdout for indented result JSON;
the decision table and errors go to stderr. A file output is written to a
temporary file in the destination directory, set to mode `0644`, and renamed
over the destination. The rename makes replacement atomic. Because the
directory is not synced, it does not guarantee durability after a system crash.

### Repeated Framed Packs

For repeated request/evidence packs under one policy, compile once and reuse a
session:

```bash
./bin/verifoxx --stream --policy policies/policy.json
```

Stdin and stdout then contain binary-framed JSON only. Every message is a
four-byte unsigned big-endian payload length followed by exactly that many JSON
bytes. An input payload is:

```json
{"requests":[...],"evidence":[...]}
```

Successful and rejected frames return ordered response envelopes:

```json
{"ok":true,"output":{"schema_version":1,"policy_name":"...","policy_version":"...","results":[...]}}
{"ok":false,"error":{"code":"invalid_input","message":"..."}}
```

Complete invalid JSON or input receives an error frame and processing
continues. A partial header, partial payload, oversized frame, internal failure,
or output failure terminates the stream because safe resynchronization is not
possible. Input payloads are limited to 16 MiB, responses to 64 MiB, and error
messages to 1,024 bytes. Explicit `--requests`, `--evidence`, and `--output`
flags are incompatible with `--stream`.

Unknown flags and positional arguments return exit code 2. Decode, compile,
batch-build, evaluation, materialization, and output failures return exit code
1 with the failing boundary and path on stderr.

## Dependencies And Workflows

### Dependencies

- Go 1.27+; the module uses only the standard library.
- Bash 3.2+ and GNU Make for local workflows.
- Optional: `fzf`, Docker, and the Docker Compose plugin.

`make doctor` reports readiness. `make install-go` reuses any runnable Go or,
when none exists, installs the pinned release under `.tools/go` without
`sudo`. The installer supports Linux and macOS on amd64 and arm64 and verifies
the official go.dev checksum before replacing a local toolchain.

### Make Targets

| Target | Purpose |
| --- | --- |
| `all` | Build, then regenerate `results/requests.json` |
| `build` | Compile `bin/verifoxx` |
| `eval` | Evaluate the supplied pack into the tracked result |
| `check` | Run formatting, fresh tests, workflow regressions, vet, and build |
| `demo` | Check and compare supplied plus edge outputs with their goldens |
| `demo-edge` | Compare only the edge pack with its golden |
| `setup` | Check prerequisites and download modules |
| `doctor` | Report required and optional dependencies |
| `install-go` | Reuse Go or install the pinned toolchain under `.tools/go` |
| `shell` | Install the cross-shell global `mm` shortcut |
| `menu` | Open the generated `fzf` target picker or numbered fallback |
| `targets` | List the menu's targets non-interactively |
| `docker-demo` | Build once and verify both packs in Docker |
| `compose-demo` | Build once and verify both packs through Compose |
| `docker-eval` | Emit supplied-pack JSON from Docker on stdout |
| `compose-eval` | Emit supplied-pack JSON from Compose on stdout |
| `clean` | Remove `bin/` while preserving tracked results |

Every public Make target has an inline `##` description. `make help`,
`make targets`, and `make menu` derive from that source. The menu uses `fzf`
when available and a numbered prompt otherwise.

### Global `mm` Shortcut

Run `make shell` once. It installs a host-native `mm` under the user bin
directory and adds that directory to Bash, Zsh, Fish, PowerShell, Nushell, and
Windows user-PATH configuration as applicable. From any project subdirectory,
`mm` searches upward for the nearest Makefile that directly defines `menu` and
runs it.

The install is idempotent. Unix profile edits are prepared before the binary
is replaced and completed edits are rolled back if a later profile write
fails. On Windows, a failed user-PATH command restores the prior binary. Open a
new terminal when the user bin was not already on `PATH`.

## Limits

| Boundary | Limit |
| --- | ---: |
| Policy JSON | 1 MiB |
| Request JSON | 8 MiB |
| Evidence JSON | 8 MiB |
| Requests per batch | 1,048,576 |
| Evidence records per batch | 1,048,576 |
| Request-to-evidence edges | 4,194,304 |
| Expression depth | 32 |
| Expression nodes per policy | 4,096 |
| Explanation text | 1,024 bytes per state |
| Remediation description | 4,096 bytes |

The source and compiled forms are validated before publication. Compiled
validation checks aligned columns, IDs, CSR ranges and bounds, symbol ranges,
instruction topology, clause ownership, resolutions, remediation ranges, and
precedence.

## Performance Summary

On an AMD Ryzen AI MAX+ 395, Linux amd64, Go 1.27.0, six single-threaded
500 ms samples measured the complete steady framed five-request path at
`18.708-19.727 us/op`, `9,336 B/op`, and `35 allocs/op`. A fresh one-shot call
measured `94.506-104.288 us/op` and 504 allocations, down from the recorded 516.
Warm evaluation measured `552.6-594.8 ns/op` for one row and
`146.511-149.817 us/op` for 1,024 rows; every warm evaluator size reported
`0 B/op` and `0 allocs/op`.

These figures describe local microbenchmarks. Commands, lifecycle stages,
data-size formulas, and excluded work are documented in
[`docs/performance.md`](docs/performance.md).

## Repository Layout

```text
cmd/verifoxx/                 CLI and end-to-end tests
cmd/mm/                       global nearest-project Make menu shortcut
internal/schema/              typed fields, IDs, outcomes, and reasons
internal/ast/                 generic source policy representation
internal/compile/             deterministic lowering and string interning
internal/program/             immutable numeric program and validator
internal/input/               request and evidence transport records
internal/eval/                columnar builder, bitplanes, scalar evaluator
internal/engine/              immutable engine and private reusable sessions
internal/result/              caller-owned numeric result columns
internal/adapters/jsonio/     strict decoders and cold JSON materialization
internal/adapters/framed/     bounded length-prefixed JSON transport
internal/conformance/         supplied and edge golden checks
policies/                     shipped source policy
fixtures/                     supplied and edge request/evidence packs
results/                      tracked supplied-pack output
scripts/                      local, menu, installer, demo, Docker workflows
docs/                         design, architecture, performance, and plans
```

## Docker And Compose

`Dockerfile` tests and builds in `golang:1.27.0-bookworm`, then copies the
static binary, policy, and fixtures into a non-root scratch image. The Compose
service is one-shot, network-disabled, read-only, capability-free, and uses
`no-new-privileges`.

For machine-readable output:

```bash
make docker-eval > supplied.json
make compose-eval > supplied.json
```

For golden verification:

```bash
make docker-demo
make compose-demo
```

## AI Tool Use

OpenCode was used to explore designs, draft tests and documentation, review
code, and run verification commands. The author reviewed the policy model,
implementation, and final decisions. Runtime decisions are made by the Go
engine and do not call an AI model.

## Evolution

This repository contains the compiled scalar engine built for the exercise.
The larger [Verifoxx](https://github.com/sebishogun/Verifoxx) project adds SIMD,
scheduling, persistence, services, and debugging.

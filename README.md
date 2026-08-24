# Verifoxx

Verifoxx is a compact Go implementation of the semantic decision exercise in
`Requirements.md`. The requirement prose is represented as a generic JSON
expression AST, compiled once into an immutable numeric program, and evaluated
over a columnar batch of requests and evidence. Each request resolves to
`Approve`, `Reject`, `Revise`, or `Escalate` with bounded provenance and
remediation data.

This repository is deliberately small: one process, standard library only,
and no persistence, network service, natural-language parser, SIMD kernel, or
scheduler. It is the compiler/runtime foundation. The larger project at
[github.com/sebishogun/Verifoxx](https://github.com/sebishogun/Verifoxx) develops
the same ideas further with SIMD, scheduling, persistence, services, and
debugging.

## Quickstart

Run the complete local check and golden-file demonstration:

```bash
make demo
```

`make demo` runs formatting checks, fresh tests, installer and menu
regressions, `go vet`, and a build. It then evaluates the supplied and edge
packs in a temporary directory and compares both outputs byte-for-byte with
`results/requests.json` and `fixtures/demo/expected.json`.

To build and regenerate the tracked supplied-pack output:

```bash
make all
```

If Go is unavailable, use `make docker-demo` or `make compose-demo`. Neither
requires a host Go installation.

## Dependencies

- Go 1.27+; the module uses only the standard library.
- Bash 3.2+ and GNU Make for local workflows.
- Optional: `fzf`, Docker, and the Docker Compose plugin.

`make doctor` reports readiness. `make install-go` reuses any runnable Go or,
when none exists, installs the pinned release under `.tools/go` without
`sudo`. The installer supports Linux and macOS on amd64 and arm64 and verifies
the official go.dev checksum before replacing a local toolchain.

## Commands

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
`make targets`, and `make menu` derive from that one source. The menu uses
`fzf` when available and a numbered prompt otherwise.

### Global `mm` shortcut

Run `make shell` once. It installs a host-native `mm` under the user bin
directory and adds that directory to Bash, Zsh, Fish, PowerShell, Nushell, and
Windows user-PATH configuration as applicable. From any project subdirectory,
`mm` searches upward for the nearest Makefile that directly defines `menu` and
runs it.

The install is idempotent. Unix profile edits are prepared before the binary
is replaced and completed edits are rolled back if a later profile write
fails. On Windows, a failed user-PATH command restores the prior binary. Open a
new terminal when the user bin was not already on `PATH`.

## CLI

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
over the destination. The directory is not synced, so this is atomic
replacement rather than a claim of crash durability.

Unknown flags and positional arguments return exit code 2. Decode, compile,
batch-build, evaluation, materialization, and output failures return exit code
1 with the failing boundary and path on stderr.

## Compile And Evaluate

The CLI has one production execution path:

```text
strict JSON -> source AST -> compiler -> immutable Program
strict JSON -> requests/evidence -> columnar Batch + CSR references
Program + Batch + reusable Context -> numeric result Batch
numeric results + source IDs -> OutputPack -> JSON
```

1. Policy JSON is size-limited, strictly decoded, and validated.
2. The compiler interns strings once and lowers expressions in deterministic
   postorder to typed opcodes, numeric IDs, and CSR ranges.
3. Request and evidence strings are resolved once while building
   structure-of-arrays columns. Missing evidence references become explicit
   sentinel edges.
4. The evaluator fills reusable positive, negative, and reason bitplanes, then
   resolves all applicable clauses into caller-owned numeric result columns.
5. Human-readable strings and JSON objects are reconstructed only after the
   warm evaluation stage.

The warm evaluator performs no per-row allocation and contains no runtime
string comparison or hash lookup. See `docs/architecture.md` for layouts and
ownership and `docs/performance.md` for reproducible measurements.

## Policy Format

The top-level policy contains `name`, `version`, and one or more
`requirements`. A requirement contains:

| Field | Meaning |
| --- | --- |
| `id` | Unique requirement ID |
| `description` | Requirement text retained as source metadata |
| `non_negotiable` | Policy metadata for a hard constraint |
| `applicability` | Expression deciding which requests the requirement governs |
| `clauses` | Assertions and their explicit resolutions |

Each clause has a globally unique `id`, an `assertion`, a `resolution` for all
eight reason states, optional per-state `explanations`, and optional bounded
`remediations`.

### Expressions

| Operator | Shape | Meaning |
| --- | --- | --- |
| `all` | `children` | All child expressions must be satisfied |
| `equal` | `field`, `value` | Request field equals one symbol |
| `in` | `field`, `values` | Request field belongs to a non-empty unique set |
| `evidence_matches` | `evidence` | Referenced evidence of one kind satisfies every supplied qualifier |

Request fields are `requester`, `trust_level`, `action`, `output_kind`,
`dataset`, `environment`, and `usage_limit`. `evidence_matches` is allowed only
in clause assertions. Its predicate requires `kind` and can constrain
`status`, `timing`, `reviewer`, `timestamp_state`, `subject`,
`attestation_state`, `scope`, and `adjustment_type`.

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

`policies/policy.json` is the complete shipped example.

## Input And Output

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

## Decisions

All applicable clauses are evaluated; there is no first-match exit. The fixed
precedence is:

```text
Reject > Escalate > Revise > Approve
```

Equal-rank findings retain deterministic first occurrence: request semantic
checks, missing evidence references, then requirement and clause order. This
keeps hard violations above uncertainty and uncertainty above revisable
conditions while preserving stable explanations.

| Request | Decision | Driver |
| --- | --- | --- |
| R1 | Approve | Aggregate output, valid pre-execution approval, verified local environment |
| R2 | Reject | Individual-record export violates the non-negotiable disclosure constraint |
| R3 | Revise | Above-standard usage lacks the required usage-adjustment approval |
| R4 | Escalate | The requested execution environment cannot be verified |
| R5 | Escalate | Approval evidence contains conflicting valid and revoked state |

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
validation checks aligned columns, IDs, CSR ranges and bounds, symbol
ranges, instruction topology, clause ownership, resolutions, remediation
ranges, and precedence.

## Performance

On an AMD Ryzen AI MAX+ 395, Linux amd64, Go 1.27.0, six single-threaded
500 ms samples measured warm evaluation at `542.8-563.9 ns/op` for one row and
`144.670-151.201 us/op` for 1,024 rows. Every measured warm batch size (1, 5,
64, and 1,024 rows) reported `0 B/op` and `0 allocs/op`. These are local
microbenchmark ranges, not service-throughput claims. Commands, all lifecycle
stages, allocations, and memory formulas are in `docs/performance.md`.

## Repository Layout

```text
cmd/verifoxx/                 CLI and end-to-end tests
cmd/mm/                       global nearest-project Make menu shortcut
internal/ast/                 generic source policy representation
internal/compile/             deterministic lowering and string interning
internal/program/             immutable numeric program and validator
internal/input/               request and evidence DTOs
internal/eval/                columnar builder, bitplanes, scalar evaluator
internal/result/              caller-owned numeric result columns
internal/adapters/jsonio/     strict decoders and cold JSON materialization
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

AI tooling was used through OpenCode for design exploration, implementation,
test construction, code review, documentation, and running the verification
matrix. The human supplied requirements, approved architecture and tradeoffs,
and remained responsible for the final submission. Runtime policy compilation
and decisions are deterministic Go code and do not call an AI model.

## Evolution

This repository stops at the compact compiled scalar engine required by the
exercise. [Verifoxx](https://github.com/sebishogun/Verifoxx) is the larger
evolution at exactly `https://github.com/sebishogun/Verifoxx`; it adds SIMD,
scheduling, persistence, services, and debugging outside this submission's
scope.

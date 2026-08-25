# Performance

## Measurement Environment

Measurements below were taken on 2026-08-24 with:

| Item | Value |
| --- | --- |
| OS | Linux 7.1.8-arch1-3, amd64 |
| CPU | AMD Ryzen AI MAX+ 395 with Radeon 8060S, 16 cores / 32 threads |
| Go | go1.27.0 linux/amd64 |
| Module | Standard library only |
| Benchmark scheduling | `GOMAXPROCS=1` |
| Sample duration | 500 ms |
| Samples | 6 per benchmark |

The machine was not frequency-pinned. The tables report the minimum and
maximum observed sample rather than a throughput projection.

## Reproduce

For a bounded reviewer run covering the complete one-shot CLI, steady framed
processing, reusable session, and warm evaluation at five and 1,024 rows:

```bash
make bench
# Without Make:
./scripts/bench.sh
```

This command runs three single-threaded 500 ms samples and reports `ns/op`,
`B/op`, and `allocs/op`. The warm evaluator rows should retain their measured
zero-allocation contract. Timing varies with the host CPU and power state.

The final six-sample measurement command covered every stage sequentially by
package:

```bash
env GOMAXPROCS=1 ./scripts/go.sh test \
  -p 1 \
  -run '^$' \
  -bench 'Benchmark(Evaluate|Compile|BuildBatch|Materialize|Session|FirstFrame|SteadyFrame)' \
  -benchmem -benchtime=500ms -count=6 -timeout 90s \
  ./cmd/verifoxx ./internal/...
```

The allocation regression is independent of timing:

```bash
./scripts/go.sh test -count=1 -timeout 60s \
  ./cmd/verifoxx ./internal/eval ./internal/program \
  -run 'Test.*Allocation' -v
```

## Results

### Warm Evaluation

Setup, compilation, batch construction, context sizing, and result sizing are
outside the timer. Each iteration calls `Evaluator.EvaluateInto` with reusable
storage.

| Rows | Time range | B/op | allocs/op |
| ---: | ---: | ---: | ---: |
| 1 | 552.6-594.8 ns | 0 | 0 |
| 5 | 1.113-1.200 us | 0 | 0 |
| 64 | 9.417-9.721 us | 0 | 0 |
| 1,024 | 146.511-149.817 us | 0 | 0 |

Zero allocation is the contract for a correctly pre-sized warm call, not an
incidental sample result. `EvaluateInto` checks all caller-owned capacities and
returns before mutation when they are insufficient.

### Reusable And Framed Stages

| Benchmark | Scope | Time range | B/op | allocs/op |
| --- | --- | ---: | ---: | ---: |
| `SessionSuppliedPack` | Warm build, evaluate, and materialize five rows | 5.110-5.434 us | 352 | 5 |
| `SteadyFrameSuppliedPack` | Strict decode through compact framed response | 18.708-19.727 us | 9,336 | 35 |
| `FirstFrameSuppliedPack` | New session and all first-use storage growth | 22.755-23.925 us | 21,698 | 117 |

The framed benchmarks exclude policy startup and input length-prefix reading;
they include request/evidence JSON decoding, batch build, evaluation,
materialization, response JSON encoding, and writing the response length prefix
plus payload to `io.Discard`. The steady benchmark warms every reusable store
before timing. Its allocation budget is fewer than 100 allocations per frame;
the regression test currently measures 35.

### Cold And Boundary Stages

| Benchmark | Scope | Time range | B/op | allocs/op |
| --- | --- | ---: | ---: | ---: |
| `CompilePolicy` | Validated source AST to immutable program | 15.252-16.809 us | 21,241 | 180 |
| `CompilePolicyJSON` | In-memory policy JSON decode, validate, compile | 52.534-55.120 us | 97,129 | 322 |
| `BuildBatchRows1024` | Warm builder and destination for 1,024 rows | 239.775-243.228 us | 0 | 0 |
| `BuildBatchColdDestinationRows1024` | Warm builder into a zero destination | 247.321-252.824 us | 45,764-45,766 | 17 |
| `MaterializeSuppliedPack` | Warm five-row output materialization | 1.514-1.731 us | 352 | 5 |
| `MaterializeColdSuppliedPack` | Five rows into a zero output pack | 2.256-2.339 us | 1,952 | 28 |
| `EvaluateCLISuppliedPack` | Fresh complete five-row one-shot call | 94.506-104.288 us | 127,092 | 504 |

`CompilePolicyJSON` reads the policy file into a byte slice before the timer;
the measured loop starts at strict JSON decoding. The warmed batch benchmark
reuses destination columns and builder-owned ID maps. The cold-destination
benchmark includes destination-column allocation but retains warmed builder
maps across iterations. The complete CLI benchmark calls `run` and reads the
three source files on every iteration, writes JSON to `io.Discard`, and excludes
operating-system process startup and file-output rename. Its allocation count
fell from the pre-change baseline of 516 to 504; a fresh process still decodes
and compiles its policy.

Materialization reconstructs source IDs, explanations, evidence details,
remediations, and JSON-facing slices after numeric evaluation. Reused slices and
zero-copy views into the immutable symbol text slab leave five allocations for
the supplied pack's dynamically composed evidence details. Encoding and file
replacement are output-boundary work, not part of the warm evaluator.

## Data Size Formulas

These formulas count slice payloads only. They exclude slice headers, allocator
size classes, spare capacity, source DTO strings, maps used during cold build,
the immutable program, and output materialization.

Let:

```text
N = request rows
W = ceil(N / 64)
E = evidence records
X = request-to-evidence edges
I = compiled instructions
R = reason count = 8
Q = emitted applicable-requirement IDs
U = emitted used-evidence references
M = emitted remediation IDs
```

### Input Batch

The current schema has seven request fields and eight evidence qualifier symbol
columns.

```text
request values             7 * N * 4       = 28N
present + valid planes     2 * 7 * W * 8   = 112W
semantic issue masks       N * 2           = 2N
evidence kind              E * 2
evidence qualifier symbols 8 * E * 4       = 32E
evidence presence + flags  E * (2 + 1)     = 3E
evidence CSR offsets       (N + 1) * 4
evidence CSR refs          X * 4
------------------------------------------------
batch payload              34N + 112W + 37E + 4X + 4 bytes
```

For the repeated 1,024-row supplied-pack benchmark (`E=4`, `X=2,047`), this
formula gives 44,948 bytes of live batch payload before headers, capacities,
and cold-builder maps.

### Evaluation Context

Positive and negative truth each use one `uint64` per instruction word. Reasons
use one plane for each of eight reasons:

```text
context payload = (2 + R) * I * W * 8
                = 80 * I * W bytes
```

The context is allocated or grown by `Context.Ensure`, then cleared and reused
by every warm evaluation.

### Numeric Results

Fixed row columns occupy 15 bytes per row. Three CSR offset arrays add
`12*(N+1)` bytes. Variable output IDs add their own payload:

```text
result payload = 27N + 2Q + 4U + 2M + 12 bytes
```

Callers size the variable columns to worst-case bounds before evaluation. The
evaluator resets lengths and content but never grows them.

## Runtime Shape

- `Engine` owns an immutable shared `Program` whose symbols occupy one text slab.
- Each session owns reusable builder maps, batch, context, results, and output.
- `Batch` is field-major and evidence-column-major; request-to-evidence links
  are CSR ranges.
- `Context` is instruction-major bitplane storage private to one worker.
- `result.Batch` is caller-owned numeric storage private to one worker.
- Builders use private reusable maps for ID validation and lookup. Warm
  evaluation uses no map, reflection, formatting, string comparison, callback
  dispatch, or pool.

The layouts leave room for a future SIMD kernel and row sharding, but the
measurements in this document are for the scalar, single-threaded
implementation only. Framed mode is sequential and ordered.

## Exclusions

No result here is a service capacity, latency SLO, production-throughput,
cross-machine, interpreter comparison, SIMD, or parallel-scaling claim. Disk
durability, scheduler overhead, network transport, persistence, process
startup, and container startup are outside the warm benchmark. The output
writer uses temporary-file rename but does not sync the destination directory.

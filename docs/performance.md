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

The final command measured every stage sequentially by package:

```bash
timeout 120s env GOMAXPROCS=1 ./scripts/go.sh test \
  -p 1 \
  -run '^$' \
  -bench 'Benchmark(Evaluate|Compile|BuildBatch|Materialize)' \
  -benchmem -benchtime=500ms -count=6 -timeout 90s \
  ./cmd/verifoxx ./internal/...
```

The allocation regression is independent of timing:

```bash
timeout 60s ./scripts/go.sh test -count=1 -timeout 60s \
  ./internal/eval -run '^TestEvaluateIntoWarmPathAllocation$' -v
```

## Results

### Warm Evaluation

Setup, compilation, batch construction, context sizing, and result sizing are
outside the timer. Each iteration calls `Evaluator.EvaluateInto` with reusable
storage.

| Rows | Time range | B/op | allocs/op |
| ---: | ---: | ---: | ---: |
| 1 | 542.8-563.9 ns | 0 | 0 |
| 5 | 1.113-1.133 us | 0 | 0 |
| 64 | 9.459-9.696 us | 0 | 0 |
| 1,024 | 144.670-151.201 us | 0 | 0 |

Zero allocation is the contract for a correctly pre-sized warm call, not an
incidental sample result. `EvaluateInto` checks all caller-owned capacities and
returns before mutation when they are insufficient.

### Lifecycle Stages

| Benchmark | Scope | Time range | B/op | allocs/op |
| --- | --- | ---: | ---: | ---: |
| `CompilePolicy` | Validated source AST to immutable program | 13.996-14.760 us | 21,241 | 180 |
| `CompilePolicyJSON` | In-memory policy JSON decode, validate, compile | 49.681-51.143 us | 97,129 | 322 |
| `BuildBatchRows1024` | Build 1,024 rows into a warmed destination | 259.237-268.559 us | 54,608 | 5 |
| `BuildBatchColdRows1024` | Build 1,024 rows into a zero destination | 262.098-271.553 us | 100,328 | 22 |
| `MaterializeSuppliedPack` | Five numeric rows to output DTOs, no JSON encoding | 2.422-2.522 us | 2,500 | 47 |
| `EvaluateCLISuppliedPack` | Read, decode, compile, build, evaluate, materialize, encode five rows | 87.657-93.044 us | 126,178-126,180 | 516 |

`CompilePolicyJSON` reads the policy file into a byte slice before the timer;
the measured loop starts at strict JSON decoding. The warmed batch benchmark
reuses all destination columns but still allocates its temporary ID maps. The
cold batch benchmark includes destination-column allocation. The complete CLI
benchmark calls `run` and reads the three source files on every iteration,
writes JSON to `io.Discard`, and excludes operating-system process startup and
file-output rename.

Materialization is intentionally allocation-bearing: it reconstructs source
IDs, explanations, evidence details, remediations, and JSON-facing slices after
numeric evaluation. Encoding and file replacement are output-boundary work,
not part of the warm evaluator.

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

- `Program` is immutable and shared.
- `Batch` is field-major and evidence-column-major; request-to-evidence links
  are CSR ranges.
- `Context` is instruction-major bitplane storage private to one worker.
- `result.Batch` is caller-owned numeric storage private to one worker.
- Cold builders use maps for ID validation and lookup. Warm evaluation uses no
  map, reflection, formatting, string comparison, callback dispatch, or pool.

The layouts leave room for a future SIMD kernel and row sharding, but the
measurements in this document are for the scalar, single-threaded
implementation only.

## Exclusions

No result here is a service capacity, latency SLO, production-throughput,
cross-machine, interpreter comparison, SIMD, or parallel-scaling claim. Disk
durability, scheduler overhead, network transport, persistence, process
startup, and container startup are outside the warm benchmark. The output
writer uses temporary-file rename but does not sync the destination directory.

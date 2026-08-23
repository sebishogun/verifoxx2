# Verifoxx

> A High-Throughput, Zero-Allocation Vectorized Policy and Compliance Decision Engine in Go.

---

## 1. Project Background and Evolution

This repository originated from a candidate engineering exercise designed to evaluate semantic modeling and decision reasoning. The initial brief specified turning three natural-language policy requirements (`R1`–`R3`) into an intermediate semantic representation and evaluating five request scenarios (`R1`–`R5`) with explicit evidence tracking (`E1`–`E4`) into one of four bounded decision outcomes: **`Approve`**, **`Reject`**, **`Revise`**, or **`Escalate`**.

Following the completion of the baseline candidate exercise, the implementation was extended into a production-grade policy compiler and vectorized execution engine. The objective of this extended project is to demonstrate how low-level systems engineering, zero-allocation memory management, register liveness analysis, and SIMD hardware acceleration can be applied to enterprise compliance and policy decision systems in Go.

---

## 2. Performance and Benchmarks

Verifoxx is engineered for low-latency, high-throughput batch evaluation. Benchmarks demonstrate zero heap allocations across hot evaluation paths and processing speeds operating near CPU memory bandwidth limits.

| Benchmark Target | Scale / Workload | Latency / Throughput | Heap Allocations | Memory Overhead |
| :--- | :---: | :---: | :---: | :---: |
| **Bitwise NOT Bitplanes** | 8,192 row batch | **33.8 ns** (121,115 MB/s) | **0 B/op** | **0 allocs/op** |
| **Bitwise AND Bitplanes** | 8,192 row batch | **46.4 ns** (132,246 MB/s) | **0 B/op** | **0 allocs/op** |
| **Bitwise OR Bitplanes** | 8,192 row batch | **49.9 ns** (123,049 MB/s) | **0 B/op** | **0 allocs/op** |
| **Scalar Batch Execution** | 1,024 requests | **250 µs** (~244 ns / req) | **0 B/op** | **0 allocs/op** |
| **Evidence Evaluation** | Batch evidence pack | **10.5 µs** | **0 B/op** | **0 allocs/op** |
| **Symbol Intern Lookup** | Single lookup | **7.2 ns** | **0 B/op** | **0 allocs/op** |

*Environment: AMD Ryzen AI Max+ 395 under `go test -bench . -benchmem`.*

---

## 3. Architecture Overview

The system employs a four-tier compiler and runtime architecture:

```
+-----------------------------------------------------------------+
| Tier 4: Enterprise Infrastructure & Adapters                    |
| (PostgreSQL PGQ Audit Log, gRPC Service, TUI, CLI)              |
+-----------------------------------------------------------------+
| Tier 3: Zero-Alloc Bytecode & Vectorized Kernel                 |
| (Structure-of-Arrays, Bitplane Masks, AVX2/512 SIMD)           |
+-----------------------------------------------------------------+
| Tier 2: Compiler & Static Validation Pipeline                   |
| (DAG Reachability, Cycle Detection, Symbol Interning)           |
+-----------------------------------------------------------------+
| Tier 1: Semantic IR & Bounded Decision Logic                    |
| (Approve, Reject, Revise, Escalate + Rationale Engine)          |
+-----------------------------------------------------------------+
```

### Tier Descriptions

1. **Tier 1: Semantic Intermediate Representation (IR)**
   - Models non-negotiable restrictions vs. revisable conditions.
   - Computes deterministic 4-state decisions with full evidence provenance and missing or conflicting attestation tracking.

2. **Tier 2: Compiler and Static Validation Pipeline**
   - Validates directed acyclic graph (DAG) reachability and eliminates dependency cycles.
   - Performs symbol interning for fields and values to eliminate string allocations during rule checks.

3. **Tier 3: Zero-Allocation Vector Kernel**
   - **Structure-of-Arrays (SoA)**: Stores data columns contiguously for optimal L1/L2 cache locality.
   - **Bitplane Vector Execution**: 64-bit word operations evaluate 64 requests per CPU cycle.
   - **Register Liveness Analysis**: DAG slot allocation recycles intermediate memory registers (`0 B/op`).
   - **Hardware SIMD Facade (`internal/simdops`)**: Supports 256/512-bit vector dispatch via `github.com/sebishogun/simd`.

4. **Tier 4: Enterprise Adapters**
   - Features reflection-free streaming JSON decoders, a CLI/TUI interface, a gRPC server, and a PostgreSQL 19 SQL/PGQ audit persistence layer.

---

## 4. Decision Outcomes

The engine produces four bounded decision outcomes:

* **`Approve`**: The request satisfies all relevant policy requirements and verified supporting evidence.
* **`Reject`**: The request violates a non-negotiable policy restriction (e.g., individual-level record export on protected datasets).
* **`Revise`**: The request is unacceptable as submitted but can become acceptable through a bounded modification (e.g., providing a missing usage adjustment approval).
* **`Escalate`**: The request cannot be decided automatically due to missing, incomplete, stale, or conflicting required evidence (e.g., conflicting reviewer state).

---

## 5. Usage and Execution

### Build and Verification Commands

```bash
# Run unit and integration tests
go test -timeout 60s ./...

# Run performance benchmarks with allocation verification
go test -timeout 120s -run='^$' -bench='.' -benchmem ./...

# Compile executable binary
go build -o bin/verifoxx ./cmd/verifoxx
```

### CLI Evaluation

```bash
./bin/verifoxx eval \
  --policy policies/verifoxx/policy.json \
  --requests internal/fixtures/verifoxx-requests.json \
  --evidence internal/fixtures/verifoxx-evidence.json \
  --output results/requests.json
```

---

## 6. Directory Structure

```
.
├── cmd/verifoxx/             # CLI application entry point
├── docs/                     # Design whitepapers and implementation plans
│   └── plans/                # Architecture specifications and benchmark plans
├── internal/
│   ├── adapters/             # Streaming JSON decoders, CLI/TUI, gRPC handlers
│   ├── arena/                # Memory slab allocators
│   ├── ast/                  # Intermediate semantic representation
│   ├── compile/              # Lowering passes, reachability analysis, liveness planning
│   ├── eval/                 # Core batch evaluator, SoA execution, SIMD dispatch
│   ├── program/              # Compiled immutable program layout
│   ├── schema/               # Symbol interning and schema definitions
│   ├── simdops/              # Hardware SIMD vector operations
│   └── truth/                # Bitplane logic and SWAR vector operations
└── results/                  # Machine-readable output files
```

---

## 7. License

Licensed under the [Apache License, Version 2.0](LICENSE).

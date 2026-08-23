# Verifoxx

> A High-Throughput, Zero-Allocation Vectorized Policy and Compliance Decision Engine in Go.

---

## 1. Project Background and Evolution

This repository originated from a candidate engineering exercise designed to evaluate semantic modeling and decision reasoning. The initial brief specified turning three natural-language policy requirements (`R1`–`R3`) into an intermediate semantic representation and evaluating five request scenarios (`R1`–`R5`) with explicit evidence tracking (`E1`–`E4`) into one of four bounded decision outcomes: **`Approve`**, **`Reject`**, **`Revise`**, or **`Escalate`**.

Following the completion of the baseline candidate exercise, the implementation was extended into a production-grade policy compiler and vectorized execution engine. The objective of this extended project is to demonstrate how low-level systems engineering, zero-allocation memory management, register liveness analysis, and SIMD hardware acceleration can be applied to enterprise compliance and policy decision systems in Go.

---

## 2. Decision Outcomes

The engine produces four bounded decision outcomes:

* **`Approve`**: The request satisfies all relevant policy requirements and verified supporting evidence.
* **`Reject`**: The request violates a non-negotiable policy restriction (e.g., individual-level record export on protected datasets).
* **`Revise`**: The request is unacceptable as submitted but can become acceptable through a bounded modification (e.g., providing a missing usage adjustment approval).
* **`Escalate`**: The request cannot be decided automatically due to missing, incomplete, stale, or conflicting required evidence (e.g., conflicting reviewer state).

---

## 3. Quickstart and Usage

### Build and Verification Commands

A `Makefile` is provided with standard targets for compilation, testing, and evaluation:

```bash
# Run unit tests
make test

# Build the verifoxx binary to bin/verifoxx
make build

# Build and execute policy evaluation against requests R1-R5
make eval

# Run static analysis
make vet

# Clean build artifacts and results
make clean
```

---

## 4. Directory Structure

```
.
├── Makefile                                  # Build and evaluation automation
├── README.md                                 # Executive overview and repository guide
├── Requirements.md                           # Original assignment specification
├── go.mod                                    # Go module definition
├── cmd/
│   └── verifoxx/
│       └── main.go                           # CLI executable entry point
├── pkg/
│   ├── policy/
│   │   ├── ast.go                            # Policy AST parser and IR representation
│   │   ├── types.go                          # Request, Evidence, and Decision models
│   │   ├── evaluator.go                      # Dynamic semantic policy evaluator
│   │   └── evaluator_test.go                 # Unit test suite for requests R1-R5
│   └── format/
│       └── json.go                           # JSON loader and results exporter
├── fixtures/
│   ├── requests.json                         # Candidate request pack (R1-R5)
│   └── evidence.json                         # Candidate evidence pack (E1-E4)
├── policies/
│   └── policy.json                           # Policy AST JSON definition (R1-R3)
├── results/
│   └── requests.json                         # Generated machine-readable evaluation results
└── docs/
    ├── DESIGN_NOTE.md                        # 1-page design note (candidate brief)
    └── plans/
        └── 2026-08-23-production-evolution-design.md  # Extended architecture whitepaper
```

---

## 5. License

Licensed under the [Apache License, Version 2.0](LICENSE).

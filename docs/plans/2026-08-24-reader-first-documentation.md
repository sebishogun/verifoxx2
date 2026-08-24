# Reader-First Documentation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Reorganize Verifoxx documentation so readers can understand the decisions, performance-aware data model, core structs, verification path, and edge-case strategy in a deliberate order.

**Architecture:** Keep `README.md` as the human-first entry point, `docs/DESIGN_NOTE.md` as the one-page submission rationale, and `docs/architecture.md` as the deep technical reference. Summarize canonical information once and link to deeper detail instead of creating competing reference documents.

**Tech Stack:** Markdown, Go source references, GNU Make verification workflows.

---

### Task 1: Reorder The Root README

**Files:**
- Modify: `README.md`

**Step 1: Record the current navigation gaps**

Confirm that the README does not link `docs/DESIGN_NOTE.md`, does not list
`internal/schema/`, and does not name the eight golden edge scenarios.

**Step 2: Add the reader-first opening**

Replace the jargon-first introduction with a plain-English problem statement,
one-minute walkthrough, supplied decision table, quick verification command,
and documentation map.

**Step 3: Add explicit choices and edge cases**

Add compact tables explaining the selected semantic and performance choices and
the expected result of each golden edge case. Link deeper architecture and test
coverage rather than embedding implementation detail in every row.

**Step 4: Reorder the reference sections**

Present semantic model, data flow, input/output, and CLI before installer,
global shortcut, Docker, limits, and performance details. Preserve all commands,
protocol limits, benchmark caveats, AI-use disclosure, and scope exclusions.

**Step 5: Check the resulting document**

Run: `git diff --check -- README.md`

Expected: no whitespace errors.

### Task 2: Explain Performance Tenets And Core Types

**Files:**
- Modify: `docs/architecture.md`

**Step 1: Add performance-aware design tenets**

Explain data-first layout, compile-once execution, zero-allocation warm
evaluation, bulk bitplanes, explicit ownership, no pooling, and lifecycle-based
measurement without claiming implemented SIMD or parallel scheduling.

**Step 2: Add a core-type reference**

Map each relevant struct to its source file, owner, lifetime, representation,
and role in the data flow. Keep layout formulas and ownership rules in their
existing detailed sections.

**Step 3: Check terminology against source**

Confirm names against `internal/ast`, `internal/program`, `internal/eval`,
`internal/result`, `internal/engine`, and `internal/adapters/jsonio`.

### Task 3: Tighten The One-Page Design Note

**Files:**
- Modify: `docs/DESIGN_NOTE.md`

**Step 1: Preserve assignment coverage**

Keep the intermediate representation, why it exceeds flat extraction,
decision process, escalation boundary, and future-work explanation.

**Step 2: Add the performance-aware shape**

Summarize the `Policy -> Program -> Batch -> Context -> result.Batch ->
OutputPack` path and why storage ownership matters, while keeping the document
to approximately one rendered page.

**Step 3: Add canonical deep links**

Point to architecture and performance documents for details and measurements.

### Task 4: Verify The Documentation-Only Change

**Files:**
- Review: `README.md`
- Review: `docs/DESIGN_NOTE.md`
- Review: `docs/architecture.md`
- Review: `docs/performance.md`
- Review: `fixtures/demo/expected.json`

**Step 1: Inspect the complete diff**

Run: `git diff --check && git diff --stat && git status --short`

Expected: only planned Markdown files are modified or added, with no whitespace
errors.

**Step 2: Run the repository demonstration**

Run: `timeout 150s make demo`

Expected: formatting, tests, installer/menu regressions, vet, build, supplied
golden, and edge golden all pass.

**Step 3: Confirm documentation claims**

Check the README decision and edge tables against `results/requests.json` and
`fixtures/demo/expected.json`. Check all referenced paths exist.

**Step 4: Present the result for review**

Report the new reading order, affected files, verification output, and any
remaining documentation caveats. Do not commit or push unless the user asks.

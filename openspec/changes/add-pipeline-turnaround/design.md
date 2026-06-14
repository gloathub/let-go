# Design — Two-Stage Self-Hosted Native Lowering

## Key finding: the primitives already exist

Reconnaissance of `cmd/lgbgen/main.go`, `scripts/generate.lg`, and `Makefile`
shows the building blocks for two-stage generation are already present. This
change is **orchestration + measurement + gates**, not new pass machinery.

Existing primitives reused:

| Primitive | Location | Role |
|---|---|---|
| `--only-ns <prefix>` | main.go (segment-boundary prefix filter) | scope lowering to `ir.*` |
| `--skip-optimizers` | main.go:560 → `ir.passes.pipeline/*optimize?*`=false | Stage 0 optimizers-off |
| `-tags bootstrap,gogen_ir` build | `cmd/lgbgen/main_gogen_ir.go` (imports all lowered pkgs) | native pipeline binary |
| `runTieredGoStage2` | main.go:821-840 | full-tree native lowering (Stage 1) |
| per-ns timing + `typeinfer-budget-bails` | `printGoTimingSummary` main.go:804-819 | measurement (task 1) |
| `pipelineLowerStats` | main.go:1190-1212 | reads bail counts from `*last-lower-stats*` |
| typeinfer bail diagnostic | `…/typeinfer/solver.lg:178-188` | per-fn bail line on stderr |

## Stage mapping decision

The spec's stage 0/1 is a **variant of the existing `--bootstrap-tiered-go`
seed→full path**. The difference: the tiered *seed* stage lowers a curated
seed namespace set with optimizers **on** (`runGoTarget(…goPhaseSeed…false)`),
whereas the spec wants stage 0 to lower **all `ir.*`** with optimizers **off**.
Task 2.1 confirms this intent verbatim: "compose existing `--only-ns` +
`--skip-optimizers`".

Concrete mapping:

- **Stage 0** (`ir.*`, optimizers off, interpreted bootstrap):
  `go run -tags bootstrap ./cmd/lgbgen --target=go --only-ns ir --skip-optimizers`
  Regenerates the `core_go_lowered/ir_*` packages from current `.lg` source.
  Non-`ir` packages (edn, hash, io, test, walk) keep their committed lowered
  `.go` — that is the self-hosting seed.

- **Stage 1 build**: `go build -tags bootstrap,gogen_ir ./cmd/lgbgen` →
  native-pipeline lgbgen (passes execute as compiled Go, not interpreted
  bytecode). All `core_go_lowered/*` packages must compile (gogen_ir wireup
  imports every one), which is exactly the "stage 0 output compiles + dispatches
  natively" gate (task 2.2).

- **Stage 1 run** (full tree, optimizers on, native): mirror
  `runTieredGoStage2` —
  `go run -tags bootstrap,gogen_ir ./cmd/lgbgen --target=go --go-phase=full`.

- **Bootstrap fallback** (spec scenario 3) stays invokable unchanged:
  `go run -tags bootstrap ./cmd/lgbgen --target=go` (current generate.lg:119).

## Where the orchestration lives

Per the project's "prefer the Lisp layer" convention, the stage 0 → stage 1
driving logic goes in `scripts/generate.lg` (it already shells `go run` for the
bundle and lowered tree). A `--two-stage` flag selects stage0→stage1; absent it,
the single-stage path (line 119) remains the fallback. No new Go flags required.

## Measurement (task 1)

`printGoTimingSummary` already emits, on stderr:
```
phase2.lower.<ns>=<ms> typeinfer-budget-bails=<count>
totals.typeinfer-budget-bails=<total>
```
Task 1 captures this into `docs/perf/` as a ratchet baseline (per-ns time + bail
count). Task 1.2 adds per-function naming to the machine-readable bail summary
(today the per-fn name only appears in the stderr bail diagnostic, not the
machine-readable totals).

## Gates (task 4)

Extend `make check-generated` with a bail-count ratchet (stage-1 run must report
bails ≤ recorded baseline; zero is the target) and a turnaround budget check
(<30s scoped `--only-ns`; <2min full). CI alerts at >10% over baseline.

## Fingerprint-gated stage skipping (task group 5)

Now in scope (was deferred). Reuses the landed `pkg/genmanifest` content-digest
primitive rather than a new hashing scheme. Single hashing source of truth.

### The two-sided key

A lowered package is up-to-date iff **both** of its inputs are unchanged:

- `ir-stack-fp` — SHA256 over the `ir.*` sources (`pkg/rt/core/ir/**/*.lg`) plus
  lgbgen's own Go sources (`cmd/lgbgen/*.go`). This is the *pipeline*. If it
  changes, the lowering logic itself changed, so **every** output is suspect.
- `module-src-fp` — SHA256 of that namespace's own `.lg` source.

The staleness key is the **pair** `(ir-stack-fp, module-src-fp)`. A module is
relowered unless its recorded pair matches the current pair exactly. Because
`ir-stack-fp` is one half of *every* module's key, an ir-stack edit forces a
full relower (spec scenario "Either fingerprint changing forces work") with no
special-casing — it falls out of the key design. This is the two-sided,
content-based guard the proposal demands (mtime- and single-sided staleness have
burned this codebase before).

### Mechanism: `--only-ns` is the skip lever

No change to the lowering loop. The Lisp orchestrator computes the stale-module
set and passes it as `--only-ns ns1,ns2,...` (the existing comma-list segment
filter, `nsMatchesOnly`). Namespaces outside the set are not lowered and keep
their committed `.go` — exactly "not relowered". Stage-0/build skip is a
provenance comparison: if current `ir-stack-fp` equals the recorded one, the
native pipeline binary is already valid (Go build cache holds it), so stage 0
and the stage-1 rebuild are skipped.

### Pieces

| Piece | Location | Role |
|---|---|---|
| `IRStackDigest` / `IRStackFiles` / `FileDigest` | `pkg/genmanifest` | the one hashing impl (Go) |
| `--emit-fingerprints` | `cmd/lgbgen/main.go` | read-only EDN dump of current `{:ir-stack-fp, :modules {ns→src-fp}}` |
| `plan-incremental` + `self-test` | `scripts/incremental_gen.lg` | pure decision fn + adversarial test (mirrors `turnaround-ratchet.lg`) |
| `--incremental` wiring + provenance `:ir-stack-fp` | `scripts/generate.lg` | drive stage skip + `--only-ns`, record fingerprints |
| `pkg/rt/generated.fingerprints` | committed EDN manifest | recorded `(ir-stack-fp, module-src-fp)` pairs |

### Correctness boundary

The fingerprint gate is an **optimization**, not the correctness oracle.
`make check-generated` (content digest of all sources vs `generated.sums`) stays
the lockstep gate; skipping a module never changes that digest, because the
digest is over **sources**, and a skipped module's source is unchanged by
definition. `--incremental` is opt-in; the default `--two-stage` and
single-stage paths are untouched fallbacks.

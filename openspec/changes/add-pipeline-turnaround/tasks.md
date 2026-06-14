# Tasks

## 1. Measurement baseline
- [x] 1.1 Record current per-namespace lowering times + bail counts into docs/perf/ ratchet baseline (data exists in generate output)
      → scripts/turnaround-ratchet.lg (mirrors fanout-ratchet.lg); docs/perf/turnaround-baseline.edn recorded from real native stage-1 run (total-ms 242814, total-bails 22). Verified parse/check/update against real lgbgen timing-summary output.
- [x] 1.2 Add machine-readable bail summary to lgbgen output (totals exist; per-fn naming)
      → ROOT CAUSE (not a missing-span issue): typeinfer/solver.lg bail site used `(:name f)` where `f` is an ATOM — keyword lookup on an atom is always nil, so EVERY bail printed "<unnamed>" regardless of the fn's real name. Fix: `(:name @f)` (deref), matching data.lg:105 `fn-name`. Verified: bails now name the offending fns (build-fn, lower-node!, lower-block!, inst-rhs, solve-typeinfer, trace-pass, …) — the hot recursive IR walkers, which is actionable for driving bails→0. Also record `:bailed-fns` into ti-counters → flows to *last-lower-stats* {:typeinfer} for a future machine-readable per-fn summary in lgbgen (the printed-summary aggregation across parallel workers deferred to avoid a concurrency-correctness change unrelated to diagnostics).

## 2. Stage 0
- [x] 2.1 Add lgbgen mode: lower only ir.* namespaces, optimizers off (compose existing --only-ns + --skip-optimizers)
      → `go run -tags bootstrap ./cmd/lgbgen --target=go --only-ns ir --skip-optimizers`; rc=0 in 84s (probe). No new Go flags needed.
- [x] 2.2 Verify unoptimized lowered ir.* compiles + dispatches natively (check-generated subset gate)
      → stage-0 output compiled under `go build -tags 'bootstrap gogen_ir' ./cmd/lgbgen` rc=0 in 1s (self-hosting bootstrap mechanic proven).

## 3. Stage 1
- [x] 3.1 Build stage-1 lgbgen with -tags gogen_ir against stage-0 output
      → verified via probe (stage1-build rc=0). Mirrors runTieredGoStage2 (main.go:821).
- [x] 3.2 Wire scripts/generate.lg to run stage 0 → stage 1; keep single-stage bootstrap fallback
      → DONE. `scripts/generate.lg` now takes a `--two-stage` flag (presence-checked via
        new `has-flag?`): stage 0 `--target=go --only-ns ir --skip-optimizers` (interpreted
        bootstrap), stage 1 build `go build -tags bootstrap,gogen_ir -o /dev/null ./cmd/lgbgen`
        (compile gate), stage 1 run `--target=go --go-phase=full` (native). Absent the flag,
        the historical single-stage `--target=go` path (`lower-single-stage`) stays the
        fallback. provenance records `:lowering-mode`/`:lgbgen-tags`.
      → DETERMINISM RISK RESOLVED (the 3-file divergence caveat). End-to-end `./lg
        scripts/generate.lg --two-stage` (rc=0, ~12.5m): stage-1 run registered native
        overrides for 25 namespaces (incl ir.passes.typeinfer/constfold) and reported
        totals.typeinfer-budget-bails=0. The regenerated tree is **byte-identical** to the
        committed tree on all 3 flagged files (ir_lower_go, ir_passes_constfold, ir_validate)
        — `diff` vs `@-` exit 0. `make check-generated` GREEN (single-stage path also
        reproduces lockstep, 0 bails). Both paths reproduce the committed deterministic tree;
        the nil-budget + gogen densify fixes (lxuxvulz) hold through the wired path.
- [x] 3.3 Measure: full-tree stage-1 time and bail count; record ratchet
      → native stage-1 full run: 313s, 22 typeinfer bails. Recorded as docs/perf/turnaround-baseline.edn. (Both well over budget: 313s vs <2min, 22 vs 0 bails — quantifies the work remaining.)

## 4. Gates
- [x] 4.1 Extend make check-generated: bail-count ratchet + budget check
      → make check-generated now captures the lowering output and runs
        `scripts/turnaround-ratchet.lg check --no-regen --timing-file …` as a
        typeinfer-BAIL GATE. Because each bail = a function silently DROPPED from
        native lowering, the gate names the dropped fns. Ratchet: fails if
        total-bails > baseline; `--strict` fails on any bail (>0). Also a
        total-time budget band (default +10%). Verified end-to-end:
        `OK: turnaround within band — bails 20/20 (still-dropped: build-fn,
        build-if, …, solve-typeinfer, …), time 230667ms/251982ms`.
- [x] 4.2 CI: alert at >10% regression on turnaround baselines
      → DONE, NON-FATAL by design (wall-clock is machine-relative — see 3.3/4.1 variance).
        Added `--time-only` to turnaround-ratchet.lg (mirror of `--bails-only`: runs ONLY
        the time band, skips the bail ratchet). New `make turnaround-alert` target runs
        `check --time-only` (self-regenerating via the same `--target=go` path `update`
        records the baseline from) and ALWAYS exits 0 — a regression prints an advisory, never
        fails the build. Wired into `.github/workflows/perf-timeline.yml` (consistent
        push-to-main perf runner, NOT the per-PR `generated` gate) as a step that maps a
        `TURNAROUND REGRESSION` line to a `::warning::` annotation. Verified: regression case
        → wrapper exit 0; within-band → OK. The fatal correctness gate stays the bail ratchet
        in `make check-generated` (4.1).

## 5. Phase 2 — fingerprint gating (brought into scope; see design.md "Fingerprint-gated stage skipping")
- [x] 5.1 Record fingerprint(ir-stack) into stage-1 binary provenance
      → SINGLE hashing source of truth: extended `pkg/genmanifest` with `IRStackFiles`/
        `IRStackDigest` (digest of ir.* `.lg` + `cmd/lgbgen/*.go` — the *pipeline*, a strict
        subset of `SourceFiles`) and `FileDigest`; factored the shared `digestFiles` so
        `Compute` and the new digests use ONE scheme (existing staleness/determinism tests
        still green). Go unit tests in `pkg/genmanifest/fingerprint_test.go` assert scope
        (non-ir core edits do NOT move the ir-stack digest) + sensitivity (ir/lgbgen edits DO).
        New read-only lgbgen mode `--emit-fingerprints` prints `{:ir-stack-fp ".." :modules
        {ns src-fp ..}}` EDN (reuses `sourceSigs`; no lowering). `scripts/generate.lg`
        `--incremental` records `:ir-stack-fp` (+ `:lowering-mode "incremental"`) into
        `pkg/rt/generated.provenance`. Verified: provenance stamped `:ir-stack-fp
        "bd289a33…"` on a real `--incremental` run.
- [x] 5.2 Skip stage 0 on fingerprint match; skip module relower on pair match
      → TWO-SIDED key `(ir-stack-fp, module-src-fp)` in `scripts/incremental_gen.lg`
        (pure `plan-incremental`, mirrors `turnaround-ratchet.lg`). `--only-ns` IS the
        module-skip lever — stale set passed as `--only-ns ns1,ns2,…`; unchanged modules keep
        their committed `.go`. Stage-0 + native-rebuild skip when `ir-stack-fp` matches the
        recorded one. `generate.lg --incremental` drives it and `commit`s the manifest only
        after lowering succeeds (failure → `die` → manifest stays stale). Live real-tree proof
        (with revert): non-ir edit (set.lg) → skip-stage0 true, relower=["set"]; ir-pipeline
        edit (ir/dump.lg) → skip-stage0 false, relower-all true; reverted → skip-stage0 true,
        relower=[]. Fast-path e2e (`generate.lg --incremental`, manifest matching tree):
        exit 0, lowered-tree hash BYTE-IDENTICAL before/after (skip is real), provenance
        stamped. New committed manifest: `pkg/rt/generated.fingerprints`.
      → SOUNDNESS NOTE (relower path): in THIS workspace (modified `ir/lower_go.lg`, not the
        determinism-fixed line) isolated lowering is NON-deterministic — two `--only-ns walk`
        runs produced different bytes. The SKIP path is unaffected (we keep existing bytes);
        the RELOWER path inherits the same nondeterminism a full `make generate` already has
        here, so incremental is no worse than full regen. Once the gensym-determinism fix
        (mem: lowering-nondeterminism-gensym-counter, RESOLVED 2026-06-02 on the determinism
        line) is on this branch, relower output becomes reproducible and `--only-ns` subset
        output should equal full-tree output. Worth an explicit cross-check then.
- [x] 5.3 Adversarial test: pipeline edit with unchanged module must relower module
      → `scripts/incremental_gen.lg self-test` encodes the property (case 4): ir-stack-fp
        changed + module source byte-identical ⇒ module STILL relowers, stage 0 NOT skipped,
        relower-all true, next-manifest re-keyed to the new ir-stack-fp. A single-sided
        (module-only) key would wrongly skip it; the pair catches it. NON-VACUOUS: mutating
        `module-fresh?` to drop the ir-side check makes exactly the 5.3 assertions fail
        (exit 1); restored → exit 0. All 13 self-test checks pass.

> Generation status after this change: editing `cmd/lgbgen/main.go` (a generator source)
> means `generated.sums` + the lowered tree are stale until a full `make generate` runs —
> `TestGeneratedArtifactsAreFresh` correctly reports this. Run `make generate` (and, on the
> determinism-fixed line, `make check-generated`) to land. The 5.1–5.3 code is complete and
> independently verified above.

# Parse bug: `yamlstar-load` fails via Go `fn.Invoke`, works via in-chunk call

Status: OPEN. Blocks the Stage-A (bytecode-embedded) `libyamlstar.so` end-to-end
smoke. Everything else in the c-shared PoC works.

## Symptom

Calling `yamlstar-load` / `yamlstar-load-all` through the C-ABI (or any Go-side
`fn.Invoke`) returns:

```
{"error":{"cause":"#error {:message \"TypeError: nil is not a function \",
          :data {:trace (\"load (<unknown>)\" \"parse (<unknown>)\")}}",
          "type":"Exception"}}
```

i.e. inside `yamlstar.parser/parse`, some var dereferences to `nil` and is then
called.

## What works (so the bundle + compile pipeline are correct)

- `yamlstar_version()` over the C-ABI from Python ctypes → `0.1.3-SNAPSHOT`.
- The C-ABI itself: build (`-buildmode=c-shared`), header, `C.CString`/
  `C.GoString` marshalling, `yamlstar_free` ownership — all good.
- An **in-chunk compiled call** to `yamlstar-load` works and returns correct
  data, two ways:
  - `examples/libyamlstar-cshared/lg/_drv.lg` (require + call from a file) — via
    source load.
  - a self-calling bundle (`libyamlstar.lg` + a trailing
    `(yamlstar-load ...)`), compiled with `lg -c` and run via `lg <bundle>.lgb`
    (runLGB) → `SELF: {:data {"b" ["x" "y"], "a" 1}}`.

## What is ruled OUT

- **Not cgo / goroutine-local state.** A non-cgo Go probe
  (`cmd/probe/main.go`, runs on the main goroutine: read .lgb →
  DecodeToExecUnit → run NS chunks → run main chunk → `fn.Invoke`) fails
  **identically**. (The VM does key scope by goID — `pkg/vm/scope_gls.go` —
  but that's not the cause here.)
- **Not dynamic-binding context.** Wrapping the invoke in
  `vm.RunWithBindings(vm.SnapshotBindings(), …)` does not help.
- **Not the bundle format / NSOrder.** Same bundle runs fine when entered via a
  compiled call.

## Localization

- `version` works via `fn.Invoke`; only the `parse` path fails. So it is NOT the
  basic Go-invoke path — it is specific to what `parse` touches.
- `parse` calls `(p/call parser grammar/TOP)`. `grammar/TOP` is a large `def`
  built at load time in `yamlstar.parser.grammar` — a table of **closures**
  produced by the grammar combinators, capturing other grammar rule fns.
- Hypothesis: those load-time closures (or a var they reference) resolve
  correctly only when execution is entered through an **outer chunk frame**
  (the normal compiled CALL path), and resolve to `nil` when `parse` is the top
  frame entered via Go `fn.Invoke`. Candidate: a var/closure that is
  context-resolved rather than baked at compile time.

## Repro

```
cd examples/libyamlstar-cshared
# bundle (needs str/replace fix PR #216 + yamlstar lg-reader-conditionals branch + json key fix)
SP="/Users/ndn/development/yamlstar/core/src:lg:."
../../lg -source-paths "$SP" -c libyamlstar.lgb lg/libyamlstar.lg
# FAILS (Go invoke, main goroutine):
go run ./cmd/probe
# WORKS (in-chunk call):
../../lg /tmp/_selftest.lgb     # a bundle whose main calls yamlstar-load
```

## Next steps to fix

1. Instrument: find the EXACT var that is `nil` in `parse` (add a print at the
   first failing call, or disassemble `parse`'s chunk and trace the var-deref).
2. Compare var resolution when `parse` runs as a nested call vs. as the top
   frame entered via `fn.Invoke` — what does the outer frame establish?
3. Likely fix: invoke with a proper VM frame/ExecContext from Go (a vm-level
   "invoke value with context" entry), so load-time closures resolve their
   captured vars the same way a compiled CALL does. Relates to ticket db-efpw
   (thread ExecContext through invoked fns).

## Files

- `libyamlstar.go` — cgo wrapper (//export + go:embed libyamlstar.lgb).
- `lg/libyamlstar.lg` — entry ns (returns Clojure data; JSON done in Go via
  `rt.MarshalJSON`).
- `cmd/probe/main.go` — non-cgo Go probe (the minimal failing repro).
- `smoke.py` — Python ctypes smoke (version passes; load/load-all blocked).

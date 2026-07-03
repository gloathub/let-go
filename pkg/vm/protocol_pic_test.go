/*
 * Copyright (c) 2026 let-go contributors
 * SPDX-License-Identifier: MIT
 */

package vm

import "testing"

func withProtoStats(t *testing.T, fn func()) (hits, misses uint64) {
	t.Helper()
	protoCacheHits.Store(0)
	protoCacheMisses.Store(0)
	protoCacheStats.Store(true)
	defer protoCacheStats.Store(false)
	fn()
	return protoCacheHits.Load(), protoCacheMisses.Load()
}

// ProtocolFn must stay value-copyable: with-meta does `cp := *f`. The cache
// lives behind a pointer, so the copy is copylocks-safe (go vet enforces this)
// and shares the cache with the original — correct, since a meta'd copy
// dispatches on the same protocol and method. Warming via the original must be
// visible to the copy: it hits without a cold miss of its own.
func TestProtocolFnValueCopySharesCache(t *testing.T) {
	pf := buildSizedProto()
	args := []Value{Int(3)}
	if _, err := pf.invokeIn(RootExecContext, args); err != nil { // warm the original
		t.Fatal(err)
	}
	cp := *pf // exactly what ProtocolFn.WithMeta will do
	hits, misses := withProtoStats(t, func() {
		for i := 0; i < 100; i++ {
			if _, err := cp.invokeIn(RootExecContext, args); err != nil {
				t.Fatal(err)
			}
		}
	})
	if hits != 100 || misses != 0 {
		t.Fatalf("copy did not share the warmed cache: %d hits / %d misses, want 100 / 0", hits, misses)
	}
}

// A monomorphic loop should miss exactly once (cold fill) then hit forever.
func TestProtocolPICMonomorphicHitRate(t *testing.T) {
	pf := buildSizedProto()
	args := []Value{Int(3)}
	const n = 1000
	hits, misses := withProtoStats(t, func() {
		for i := 0; i < n; i++ {
			if _, err := pf.invokeIn(RootExecContext, args); err != nil {
				t.Fatal(err)
			}
		}
	})
	if misses != 1 || hits != n-1 {
		t.Fatalf("monomorphic: got %d hits / %d misses, want %d / 1", hits, misses, n-1)
	}
}

// An alternating bimorphic loop thrashes a single-entry cache: every call misses.
// This documents the known ceiling of the monomorphic design (a 2-way PIC is the
// fix) and guards against a future change silently claiming a hit it can't have.
func TestProtocolPICBimorphicThrashes(t *testing.T) {
	pf := buildSizedProto()
	intArgs := []Value{Int(3)}
	strArgs := []Value{String("xy")}
	const n = 1000
	hits, misses := withProtoStats(t, func() {
		for i := 0; i < n; i++ {
			args := intArgs
			if i&1 == 1 {
				args = strArgs
			}
			if _, err := pf.invokeIn(RootExecContext, args); err != nil {
				t.Fatal(err)
			}
		}
	})
	if hits != 0 || misses != n {
		t.Fatalf("bimorphic: got %d hits / %d misses, want 0 / %d", hits, misses, n)
	}
}

// The watchpoint: re-extending a type the cache is already serving must strand
// the stale entry, so the next call resolves the NEW impl. Without gen tagging
// this is exactly where an inline cache goes wrong (serves a redefined method's
// old body).
func TestProtocolPICInvalidatesOnRedefine(t *testing.T) {
	method := Symbol("sz")
	p := NewProtocol("Sized", []Symbol{method})
	v1, _ := NativeFnType.Wrap(func(a []Value) (Value, error) { return Int(1), nil })
	p.Extend(IntType, NewPersistentMap([]Value{Keyword(method), v1}))
	pf := NewProtocolFn(p, method)

	got, _ := pf.invokeIn(RootExecContext, []Value{Int(0)})
	if got != Value(Int(1)) {
		t.Fatalf("before redefine: got %v, want 1", got)
	}

	// Redefine the Int impl to return 2. The gen bump must invalidate the entry
	// cached from the first call.
	v2, _ := NativeFnType.Wrap(func(a []Value) (Value, error) { return Int(2), nil })
	p.Extend(IntType, NewPersistentMap([]Value{Keyword(method), v2}))

	got, _ = pf.invokeIn(RootExecContext, []Value{Int(0)})
	if got != Value(Int(2)) {
		t.Fatalf("after redefine: got %v, want 2 (stale cache served old impl)", got)
	}
}

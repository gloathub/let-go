/*
 * Copyright (c) 2026 Norman Nunley, Jr <nnunley@gmail.com>
 * Part of the let-go project; see CONTRIBUTORS for full list of authors.
 * SPDX-License-Identifier: MIT
 */

package vm

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// GoroutineRegistry tracks every goroutine the VM spawns (futures,
// agents, core.async go-blocks, channel pipelines). Go has no goroutine
// registry of its own and cannot force-kill a goroutine, so without this
// the runtime has no way to observe or drain the work it started — a
// long-lived process (or a benchmark that re-runs a whole suite per
// iteration) leaks every still-running future and the heap it pins.
//
// The registry provides:
//   - a live count (Live) for observability,
//   - Await to block until in-flight goroutines finish,
//   - CancelAll, which cancels the context shared by all in-flight
//     goroutines so that context-aware blocking ops (sleep, channel
//     ops) return promptly, then installs a fresh context for future
//     spawns,
//   - Drain = CancelAll + Await.
//
// Cancellation is bulk, not per-goroutine: CancelAll signals every
// goroutine spawned under the current context at once. That matches the
// intended uses — process shutdown and between-iteration bench drains —
// where the goal is "stop everything the VM started," not "cancel this
// one future."
type ctxState struct {
	ctx    context.Context
	cancel context.CancelFunc
}

type GoroutineRegistry struct {
	wg    sync.WaitGroup
	live  atomic.Int64
	state atomic.Pointer[ctxState] // current context generation
}

// Goroutines is the process-wide VM goroutine registry. Every VM spawn
// goes through it; blocking ops consult its Context for cancellation.
var Goroutines = newGoroutineRegistry()

func newGoroutineRegistry() *GoroutineRegistry {
	r := &GoroutineRegistry{}
	ctx, cancel := context.WithCancel(context.Background())
	r.state.Store(&ctxState{ctx: ctx, cancel: cancel})
	return r
}

// Go runs fn in a tracked goroutine. fn receives the registry's current
// cancellation context; long-running work should select on ctx.Done()
// so CancelAll/Drain can stop it.
func (r *GoroutineRegistry) Go(fn func(ctx context.Context)) {
	ctx := r.state.Load().ctx
	r.wg.Add(1)
	r.live.Add(1)
	go func() {
		defer r.wg.Done()
		defer r.live.Add(-1)
		fn(ctx)
	}()
}

// Live reports how many tracked goroutines are currently running.
func (r *GoroutineRegistry) Live() int {
	return int(r.live.Load())
}

// Context returns the current cancellation context. Blocking native ops
// (sleep, channel send/recv, alts!) select on its Done() so they unblock
// when CancelAll fires. Read it at the point of blocking, not cached, so
// a freshly-installed context after CancelAll is observed. Lock-free (a
// single atomic load) so it is cheap to call on every channel op.
//
// Drain semantics are bulk: a CancelAll releases ops that are PARKED on
// the context at that moment. An op that reads Context() AFTER a
// CancelAll sees the fresh context and is not affected — which is what
// we want for shutdown / between-iteration drains, where spawning and
// draining are temporally separated. Per-goroutine cancellation would
// require threading each goroutine's spawn context through every native
// op (goroutine-local state); the registry intentionally only offers the
// bulk form.
func (r *GoroutineRegistry) Context() context.Context {
	return r.state.Load().ctx
}

// CancelAll cancels the context shared by all in-flight tracked
// goroutines, then installs a fresh context so subsequent spawns are not
// born cancelled. Does not wait — pair with Await (or use Drain).
//
// Lock-free: the fresh generation is swapped in atomically, and we
// cancel whatever generation we displaced. Concurrent CancelAlls are
// safe — each cancels exactly the generation it replaced, so every
// displaced generation is cancelled and the surviving state is always a
// live (uncancelled) one.
func (r *GoroutineRegistry) CancelAll() {
	ctx, cancel := context.WithCancel(context.Background())
	old := r.state.Swap(&ctxState{ctx: ctx, cancel: cancel})
	old.cancel()
}

// Await blocks until every tracked goroutine has exited, or timeout
// elapses. Returns true if fully drained, false on timeout. A
// non-positive timeout waits indefinitely.
func (r *GoroutineRegistry) Await(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	if timeout <= 0 {
		<-done
		return true
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case <-done:
		return true
	case <-t.C:
		return false
	}
}

// Drain cancels all in-flight goroutines and waits up to timeout for
// them to exit. Returns true if fully drained. Context-aware blocking
// ops return promptly on cancel; anything ignoring the context still
// races the timeout.
func (r *GoroutineRegistry) Drain(timeout time.Duration) bool {
	r.CancelAll()
	return r.Await(timeout)
}

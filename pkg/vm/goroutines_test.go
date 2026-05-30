/*
 * Copyright (c) 2026 Norman Nunley, Jr <nnunley@gmail.com>
 * Part of the let-go project; see CONTRIBUTORS for full list of authors.
 * SPDX-License-Identifier: MIT
 */

package vm

import (
	"context"
	"testing"
	"time"
)

func TestGoroutineRegistryTracksLiveAndAwait(t *testing.T) {
	r := newGoroutineRegistry()
	release := make(chan struct{})

	for i := 0; i < 3; i++ {
		r.Go(func(ctx context.Context) { <-release })
	}
	// Live count should reflect the 3 spawned (allow brief scheduling).
	deadline := time.Now().Add(time.Second)
	for r.Live() != 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := r.Live(); got != 3 {
		t.Fatalf("expected 3 live goroutines, got %d", got)
	}

	// Await times out while they're blocked.
	if r.Await(50 * time.Millisecond) {
		t.Fatal("Await should have timed out while goroutines blocked")
	}

	close(release)
	if !r.Await(2 * time.Second) {
		t.Fatal("Await should have drained after release")
	}
	if got := r.Live(); got != 0 {
		t.Fatalf("expected 0 live after drain, got %d", got)
	}
}

func TestGoroutineRegistryCancelUnblocksContextAwareWork(t *testing.T) {
	r := newGoroutineRegistry()
	// A goroutine that blocks on its context (mimics a ctx-aware sleep).
	for i := 0; i < 4; i++ {
		r.Go(func(ctx context.Context) {
			select {
			case <-ctx.Done():
			case <-time.After(30 * time.Second): // would hang without cancel
			}
		})
	}

	if !r.Drain(2 * time.Second) {
		t.Fatal("Drain should have cancelled+drained ctx-aware goroutines fast")
	}
	if got := r.Live(); got != 0 {
		t.Fatalf("expected 0 live after drain, got %d", got)
	}

	// After CancelAll a fresh context is installed: new spawns are NOT
	// born cancelled.
	ctx := r.Context()
	select {
	case <-ctx.Done():
		t.Fatal("fresh context after drain should not be cancelled")
	default:
	}
}

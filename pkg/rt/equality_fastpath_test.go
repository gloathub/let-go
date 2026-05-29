package rt

import (
	"testing"
	"time"

	"github.com/nooga/let-go/pkg/vm"
)

// TestArrayVector_SameBacking_ShortCircuit verifies that two ArrayVector
// values built over the same backing slice short-circuit in O(1) without
// walking elements. Regression guard for the shared-substructure case
// (compiler IR graphs where the same node is reachable through many parents).
func TestArrayVector_SameBacking_ShortCircuit(t *testing.T) {
	backing := make([]vm.Value, 1024)
	for i := range backing {
		backing[i] = vm.Int(int64(i))
	}
	av := vm.ArrayVector(backing)
	bv := vm.ArrayVector(backing)
	if !valueEquals(av, bv) {
		t.Fatal("expected equal for shared-backing ArrayVector")
	}
}

// TestArrayVector_HashRejects_Unequal exercises the hash fast-reject path:
// two equal-length but distinct ArrayVectors must return false.
func TestArrayVector_HashRejects_Unequal(t *testing.T) {
	a := vm.ArrayVector{vm.Int(1), vm.Int(2), vm.Int(3)}
	b := vm.ArrayVector{vm.Int(1), vm.Int(2), vm.Int(4)}
	if valueEquals(a, b) {
		t.Fatal("expected not equal")
	}
}

// TestArrayVector_EqualDistinctBacking verifies correctness when neither
// identity nor hash decides — must still return true via element walk.
func TestArrayVector_EqualDistinctBacking(t *testing.T) {
	a := vm.ArrayVector{vm.Int(1), vm.Int(2), vm.Int(3)}
	b := vm.ArrayVector{vm.Int(1), vm.Int(2), vm.Int(3)}
	if !valueEquals(a, b) {
		t.Fatal("expected equal for structurally-equal ArrayVectors")
	}
}

// TestSharedSubstructure_DoesNotExplode constructs a binary-tree-shaped
// vector graph where every level reuses the same child node. Pre-fix the
// recursive walk visits the child 2^depth times. Post-fix the visited-pair
// memo (and pointer-identity on the shared subtree) bounds it to O(depth).
//
// We assert a wall-time bound rather than an instruction count because the
// pre-fix blowup is dramatic enough that any reasonable upper bound is
// decisive — pre-fix this test does not finish.
func TestSharedSubstructure_DoesNotExplode(t *testing.T) {
	leaf := vm.ArrayVector{vm.Int(1), vm.Int(2), vm.Int(3)}
	build := func() vm.ArrayVector {
		node := leaf
		for i := 0; i < 24; i++ {
			node = vm.ArrayVector{node, node}
		}
		return node
	}
	a := build()
	b := build()
	done := make(chan bool, 1)
	go func() {
		done <- valueEquals(a, b)
	}()
	select {
	case got := <-done:
		if !got {
			t.Fatal("expected shared-substructure trees to compare equal")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("valueEquals did not finish in 2s on 2^24 shared-substructure tree (pre-fix this hangs)")
	}
}

// TestValueEquals_NilHandling guards the contract that nil-pair compares
// as equal and nil-vs-non-nil as unequal.
func TestValueEquals_NilHandling(t *testing.T) {
	if !valueEquals(nil, nil) {
		t.Fatal("nil == nil should be true")
	}
	if valueEquals(nil, vm.Int(0)) {
		t.Fatal("nil == Int(0) should be false")
	}
	if valueEquals(vm.Int(0), nil) {
		t.Fatal("Int(0) == nil should be false")
	}
}

// TestValueEquals_ScalarShortCircuit spot-checks the comparable-scalar fast
// path: same scalar value ⇒ true, different scalar ⇒ false, with no descent.
func TestValueEquals_ScalarShortCircuit(t *testing.T) {
	if !valueEquals(vm.Keyword("foo"), vm.Keyword("foo")) {
		t.Fatal("keyword equality")
	}
	if valueEquals(vm.Keyword("foo"), vm.Keyword("bar")) {
		t.Fatal("keyword inequality")
	}
	if !valueEquals(vm.Int(42), vm.Int(42)) {
		t.Fatal("int equality")
	}
	if !valueEquals(vm.String("x"), vm.String("x")) {
		t.Fatal("string equality")
	}
}

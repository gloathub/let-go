package rt

import (
	"sync"
	"testing"

	"github.com/nooga/let-go/pkg/vm"
)

// TestSetRandSeedReproducible pins the contract: the same seed yields the same
// sequence, and a different seed diverges. It asserts sequence *equality*, not
// specific generator outputs — those are an implementation detail of the RNG
// and may change across Go / math/rand/v2 versions.
func TestSetRandSeedReproducible(t *testing.T) {
	draw := func() []int {
		out := make([]int, 64)
		for i := range out {
			out[i] = rngIntn(1_000_000)
		}
		return out
	}

	setRandSeed(42)
	a := draw()
	setRandSeed(42)
	b := draw()
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("same seed diverged at %d: %d != %d", i, a[i], b[i])
		}
	}

	setRandSeed(43)
	c := draw()
	identical := true
	for i := range a {
		if a[i] != c[i] {
			identical = false
			break
		}
	}
	if identical {
		t.Fatal("different seeds produced an identical sequence")
	}
}

// TestRandConcurrentNoRace hammers the shared RNG and the reseed from many
// goroutines. Its purpose is to fail under `go test -race` if rngMu ever stops
// guarding access — output is nondeterministic by design, so it asserts only
// that draws stay in range and nothing panics.
func TestRandConcurrentNoRace(t *testing.T) {
	const goroutines = 16
	const iters = 2000

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := range goroutines {
		go func(g int) {
			defer wg.Done()
			for i := range iters {
				switch i % 4 {
				case 0:
					if v := rngIntn(100); v < 0 || v >= 100 {
						t.Errorf("rngIntn out of range: %d", v)
					}
				case 1:
					if v := rngFloat64(); v < 0 || v >= 1 {
						t.Errorf("rngFloat64 out of range: %v", v)
					}
				case 2:
					rngShuffle(8, func(i, j int) {})
				case 3:
					setRandSeed(int64(g*iters + i))
				}
			}
		}(g)
	}
	wg.Wait()
}

// TestSetRandSeedBangThroughRuntime drives set-rand-seed! and rand-int as
// resolved language primitives (not the Go helpers), so it covers the
// registration and wiring end to end: reseeding to the same value through the
// runtime reproduces the rand-int sequence.
func TestSetRandSeedBangThroughRuntime(t *testing.T) {
	core := NS("core")
	getFn := func(name string) vm.Fn {
		t.Helper()
		v := core.Lookup(vm.Symbol(name))
		if v == nil {
			t.Fatalf("core/%s not found", name)
		}
		fn, ok := v.(*vm.Var).Deref().(vm.Fn)
		if !ok {
			t.Fatalf("core/%s is not an Fn", name)
		}
		return fn
	}
	setSeed := getFn("set-rand-seed!")
	randInt := getFn("rand-int")

	seq := func() []vm.Value {
		out := make([]vm.Value, 16)
		for i := range out {
			v, err := randInt.Invoke([]vm.Value{vm.MakeInt(1_000_000)})
			if err != nil {
				t.Fatalf("rand-int: %v", err)
			}
			out[i] = v
		}
		return out
	}

	if _, err := setSeed.Invoke([]vm.Value{vm.MakeInt(7)}); err != nil {
		t.Fatalf("set-rand-seed!: %v", err)
	}
	a := seq()
	if _, err := setSeed.Invoke([]vm.Value{vm.MakeInt(7)}); err != nil {
		t.Fatalf("set-rand-seed!: %v", err)
	}
	b := seq()
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("set-rand-seed! through runtime diverged at %d: %v != %v", i, a[i], b[i])
		}
	}
}

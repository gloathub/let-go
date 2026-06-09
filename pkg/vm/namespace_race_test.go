package vm

import (
	"strconv"
	"sync"
	"testing"
)

// TestNamespaceConcurrentAccess exercises the per-namespace RWMutex (Part A /
// F-1) under -race: many goroutines concurrently Def/DefStub/LookupOrAdd/Refer/
// ReferList/Alias/ImportVar/Exclude/Lookup against one shared namespace and
// across refer/alias targets. The governing invariant is that no goroutine ever
// holds one namespace's lock while acquiring another's, so cross-namespace reads
// (refer/alias resolution) snapshot under the foreign lock rather than holding
// the local one.
//
// Run with: go test -race -run TestNamespaceConcurrentAccess ./pkg/vm
func TestNamespaceConcurrentAccess(t *testing.T) {
	const workers = 16
	const iters = 200

	target := NewNamespace("race.target")
	for i := 0; i < 32; i++ {
		target.Def("shared-"+strconv.Itoa(i), Int(int64(i)))
	}

	host := NewNamespace("race.host")
	host.Refer(target, "", true)

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				name := "v-" + strconv.Itoa(id) + "-" + strconv.Itoa(i)
				host.Def(name, Int(int64(i)))
				host.DefStub("stub-" + strconv.Itoa(id) + "-" + strconv.Itoa(i))
				host.LookupOrAdd(Symbol(name))
				host.Lookup(Symbol(name))
				host.Lookup(Symbol("shared-" + strconv.Itoa(i%32)))
				host.ImportVar(target, Symbol("shared-"+strconv.Itoa(i%32)), Symbol("imp-"+strconv.Itoa(id)+"-"+strconv.Itoa(i)))
				host.Exclude("excl-" + strconv.Itoa(id) + "-" + strconv.Itoa(i))
				if i%16 == 0 {
					other := NewNamespace("race.other-" + strconv.Itoa(id) + "-" + strconv.Itoa(i))
					other.Def("x", Int(1))
					host.Alias(Symbol("a"+strconv.Itoa(id)+"-"+strconv.Itoa(i)), other)
					host.ReferList(other, []Symbol{Symbol("x")})
				}
			}
		}(w)
	}
	wg.Wait()
}

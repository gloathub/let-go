// Non-cgo probe: loads the libyamlstar bundle and invokes yamlstar-load from
// Go on the main goroutine. Localizes whether the c-shared failure is
// goroutine/thread-specific (works here, fails via cgo) or a general Go
// fn.Invoke issue (fails here too).
package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/nooga/let-go/pkg/bytecode"
	"github.com/nooga/let-go/pkg/rt"
	_ "github.com/nooga/let-go/pkg/rt/corefns"
	"github.com/nooga/let-go/pkg/vm"
)

func main() {
	data, err := os.ReadFile("libyamlstar.lgb")
	if err != nil {
		fmt.Println("read err:", err)
		os.Exit(1)
	}
	resolve := func(nsName, name string) *vm.Var {
		n := rt.DefNSBare(nsName)
		if v := n.LookupLocal(vm.Symbol(name)); v != nil {
			return v
		}
		return n.DefStub(name)
	}
	unit, err := bytecode.DecodeToExecUnit(bytes.NewReader(data), resolve)
	if err != nil {
		fmt.Println("decode err:", err)
		os.Exit(1)
	}
	for _, nm := range unit.NSOrder {
		ch := unit.NSChunks[nm]
		if ch == nil || ch == unit.MainChunk {
			continue
		}
		fr := vm.NewFrame(ch, nil)
		if _, e := fr.RunProtected(); e != nil {
			fmt.Println("ns load err", nm, e)
			os.Exit(1)
		}
		vm.ReleaseFrame(fr)
	}
	fr := vm.NewFrame(unit.MainChunk, nil)
	if _, e := fr.RunProtected(); e != nil {
		fmt.Println("main chunk err:", e)
		os.Exit(1)
	}
	vm.ReleaseFrame(fr)

	fnVar := rt.NS("libyamlstar").Lookup(vm.Symbol("yamlstar-load")).(*vm.Var)
	fn := fnVar.Deref().(vm.Fn)
	res, err := fn.Invoke([]vm.Value{vm.String("a: 1\nb: [x, y]"), vm.String("")})
	fmt.Println("invoke result:", res, "err:", err)
}

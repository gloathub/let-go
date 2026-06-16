// Package main builds a C-shared library (.so/.dylib) exposing yamlstar's
// load / load-all / version over a C ABI.
//
// Stage A (bytecode-embedded): the yamlstar bytecode bundle is embedded with
// go:embed and run on the let-go VM (importing pkg/rt bootstraps clojure.core
// via its installers). JSON marshalling happens here in Go at the boundary
// (rt.MarshalJSON -> encoding/json), keeping JSON out of the lg/lowered layer.
//
// Build:
//
//	./lg -source-paths "<yamlstar>/core/src:lg:." -c libyamlstar.lgb lg/libyamlstar.lg
//	go build -buildmode=c-shared -o libyamlstar.dylib .   # .so on Linux
//
// C ABI (see generated libyamlstar.h):
//
//	char* yamlstar_load(char* yaml, char* opts);      // -> JSON {"data":...}|{"error":...}
//	char* yamlstar_load_all(char* yaml, char* opts);  // -> JSON
//	char* yamlstar_version(void);                      // -> version string
//	void  yamlstar_free(char* p);                      // free a returned string
package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
	"unsafe"

	_ "embed"

	"github.com/nooga/let-go/pkg/bytecode"
	"github.com/nooga/let-go/pkg/rt"
	_ "github.com/nooga/let-go/pkg/rt/corefns"
	"github.com/nooga/let-go/pkg/vm"
)

//go:embed libyamlstar.lgb
var bundleLGB []byte

var (
	loadOnce     sync.Once
	loadErr      error
	rootBindings vm.BindingSnapshot
)

// ensureLoaded decodes and runs the embedded bundle exactly once, installing
// the yamlstar.* and libyamlstar namespaces. clojure.core is already present
// (pkg/rt installers run from its package init).
func ensureLoaded() error {
	loadOnce.Do(func() {
		resolve := func(nsName, name string) *vm.Var {
			n := rt.DefNSBare(nsName)
			if v := n.LookupLocal(vm.Symbol(name)); v != nil {
				return v
			}
			return n.DefStub(name)
		}
		// runBundle eagerly decodes and runs every namespace chunk (then main)
		// of an .lgb, defining its vars' roots.
		runBundle := func(lgb []byte, what string) error {
			u, derr := bytecode.DecodeToExecUnit(bytes.NewReader(lgb), resolve)
			if derr != nil {
				return fmt.Errorf("decoding %s: %w", what, derr)
			}
			for _, nm := range u.NSOrder {
				ch := u.NSChunks[nm]
				if ch == nil || ch == u.MainChunk {
					continue
				}
				fr := vm.NewFrame(ch, nil)
				_, e := fr.RunProtected()
				vm.ReleaseFrame(fr)
				if e != nil {
					return fmt.Errorf("loading %s namespace %s: %w", what, nm, e)
				}
			}
			if u.MainChunk != nil {
				fr := vm.NewFrame(u.MainChunk, nil)
				_, e := fr.RunProtected()
				vm.ReleaseFrame(fr)
				if e != nil {
					return fmt.Errorf("running %s main: %w", what, e)
				}
			}
			return nil
		}
		// Stage-A stopgap: the bytecode-embedded app bundle references .lg-defined
		// core/stdlib fns (empty?, string/ends-with?, …) whose vars are nil-rooted
		// until their defining chunks run. A normal `lg` run bootstraps the .lg
		// core via the compiler; this bare loader must do it explicitly. Eager-load
		// the embedded core (core_compiled.lgb) before the app bundle.
		// TODO(gloat-model): once all of core is lowered to native Go, the lowered
		// library is self-contained and must NOT load any bytecode core at runtime —
		// drop this once core is fully lowered.
		if err := runBundle(rt.CoreCompiledLGB, "core"); err != nil {
			loadErr = err
			return
		}
		unit, err := bytecode.DecodeToExecUnit(bytes.NewReader(bundleLGB), resolve)
		if err != nil {
			loadErr = fmt.Errorf("decoding bundle: %w", err)
			return
		}
		for _, nm := range unit.NSOrder {
			ch := unit.NSChunks[nm]
			if ch == nil || ch == unit.MainChunk {
				continue
			}
			fr := vm.NewFrame(ch, nil)
			_, e := fr.RunProtected()
			vm.ReleaseFrame(fr)
			if e != nil {
				loadErr = fmt.Errorf("loading namespace %s: %w", nm, e)
				return
			}
		}
		fr := vm.NewFrame(unit.MainChunk, nil)
		_, e := fr.RunProtected()
		vm.ReleaseFrame(fr)
		loadErr = e
		// Capture the post-load dynamic-binding context so exports invoked
		// from Go run with the same root bindings the VM established at load
		// (otherwise dynamic vars the parser relies on deref to nil).
		rootBindings = vm.SnapshotBindings()
	})
	return loadErr
}

func errJSON(cause string) string {
	b, _ := json.Marshal(map[string]any{"error": map[string]any{"cause": cause}})
	return string(b)
}

func invokeExport(fnName string, args ...vm.Value) (vm.Value, error) {
	if err := ensureLoaded(); err != nil {
		return vm.NIL, err
	}
	v := rt.NS("libyamlstar").Lookup(vm.Symbol(fnName))
	fnVar, ok := v.(*vm.Var)
	if !ok {
		return vm.NIL, fmt.Errorf("export not found: %s", fnName)
	}
	fn, ok := fnVar.Deref().(vm.Fn)
	if !ok {
		return vm.NIL, fmt.Errorf("export not callable: %s", fnName)
	}
	return vm.RunWithBindings(rootBindings, func() (vm.Value, error) {
		return fn.Invoke(args)
	})
}

// jsonResult marshals a successful result to JSON, or returns an error payload.
func jsonResult(res vm.Value, err error) string {
	if err != nil {
		return errJSON(err.Error())
	}
	out, e := rt.MarshalJSON(res)
	if e != nil {
		return errJSON("json marshal: " + e.Error())
	}
	return out
}

//export yamlstar_load
func yamlstar_load(yaml *C.char, opts *C.char) *C.char {
	res, err := invokeExport("yamlstar-load",
		vm.String(C.GoString(yaml)), vm.String(C.GoString(opts)))
	return C.CString(jsonResult(res, err))
}

//export yamlstar_load_all
func yamlstar_load_all(yaml *C.char, opts *C.char) *C.char {
	res, err := invokeExport("yamlstar-load-all",
		vm.String(C.GoString(yaml)), vm.String(C.GoString(opts)))
	return C.CString(jsonResult(res, err))
}

//export yamlstar_version
func yamlstar_version() *C.char {
	res, err := invokeExport("yamlstar-version")
	if err != nil {
		return C.CString(errJSON(err.Error()))
	}
	if s, ok := res.(vm.String); ok {
		return C.CString(string(s))
	}
	return C.CString(errJSON("version: not a string"))
}

// yamlstar_free releases a string returned by the exports above. Callers that
// retain returned strings must free them to avoid leaks (the strings are
// C.CString-allocated on the C heap).
//
//export yamlstar_free
func yamlstar_free(p *C.char) {
	C.free(unsafe.Pointer(p))
}

func main() {}

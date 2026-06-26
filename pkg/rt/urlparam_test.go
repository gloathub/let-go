/*
 * Copyright (c) 2026 let-go contributors
 * SPDX-License-Identifier: MIT
 */

package rt

import (
	"testing"

	"github.com/nooga/let-go/pkg/vm"
)

// Off-browser there is no page URL, so hostURLParam yields nothing for any
// name. The wasm side that parses _lgUrlSearch runs in a browser, not here.
func TestHostURLParamDefaultStub(t *testing.T) {
	for _, name := range []string{"seed", "bench", "missing", ""} {
		if v, ok := hostURLParam(name); ok || v != "" {
			t.Fatalf("hostURLParam(%q) = %q, %v; want \"\", false", name, v, ok)
		}
	}
}

// (js/url-param ...) is registered on the js namespace and returns nil rather
// than erroring on bad arity or a non-string name — boot code reading an
// optional param shouldn't crash. Off-browser, a present name also yields nil.
func TestURLParamFn(t *testing.T) {
	v := NS("js").Lookup(vm.Symbol("url-param"))
	if v == nil {
		t.Fatal("js/url-param not registered")
	}
	fn, ok := v.(*vm.Var).Deref().(vm.Fn)
	if !ok {
		t.Fatalf("js/url-param is not an Fn: %T", v)
	}
	cases := [][]vm.Value{
		{},                               // arity 0
		{vm.String("a"), vm.String("b")}, // arity 2
		{vm.NIL},                         // non-string name
		{vm.String("seed")},              // valid name, absent off-browser
	}
	for _, args := range cases {
		got, err := fn.Invoke(args)
		if err != nil {
			t.Fatalf("Invoke(%v): unexpected error %v", args, err)
		}
		if got != vm.NIL {
			t.Fatalf("Invoke(%v) = %v; want nil", args, got)
		}
	}
}

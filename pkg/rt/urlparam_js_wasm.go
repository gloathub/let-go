//go:build js && wasm

/*
 * Copyright (c) 2026 let-go contributors
 * SPDX-License-Identifier: MIT
 *
 * hostURLParam is the WASM side of (js/url-param ...). WASM runs in a Web
 * Worker whose own `location` is the worker script URL, not the page that
 * loaded the bundle — so the host shell forwards window.location.search into
 * the worker as the _lgUrlSearch global, which we parse here.
 */

package rt

import "syscall/js"

func hostURLParam(name string) (string, bool) {
	search := js.Global().Get("_lgUrlSearch")
	if search.IsUndefined() || search.IsNull() {
		return "", false
	}
	v := js.Global().Get("URLSearchParams").New(search).Call("get", name)
	if v.IsNull() || v.IsUndefined() {
		return "", false
	}
	return v.String(), true
}

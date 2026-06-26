//go:build !(js && wasm)

/*
 * Copyright (c) 2026 let-go contributors
 * SPDX-License-Identifier: MIT
 *
 * hostURLParam off-browser: there is no page URL, so (js/url-param ...)
 * always returns nil. Keeps the primitive callable in native dev/tests.
 */

package rt

func hostURLParam(string) (string, bool) { return "", false }

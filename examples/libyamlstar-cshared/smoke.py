#!/usr/bin/env python3
"""ctypes smoke test for the let-go-built libyamlstar c-shared library.

Demonstrates the PoC: a Clojure (yamlstar) core compiled by let-go into a
.dylib/.so, called from Python over the C ABI. String ownership: every char*
returned by an export is heap-allocated in Go (C.CString); the caller must
free it with yamlstar_free.
"""
import ctypes
import json
import sys
import platform

ext = "dylib" if platform.system() == "Darwin" else "so"
lib = ctypes.CDLL(f"./libyamlstar.{ext}")

for name, argc in [("yamlstar_version", 0), ("yamlstar_load", 2), ("yamlstar_load_all", 2)]:
    f = getattr(lib, name)
    f.restype = ctypes.c_void_p           # raw pointer so we can free it
    f.argtypes = [ctypes.c_char_p] * argc
lib.yamlstar_free.argtypes = [ctypes.c_void_p]


def call(f, *args):
    ptr = f(*[a.encode("utf-8") for a in args])
    s = ctypes.cast(ptr, ctypes.c_char_p).value.decode("utf-8")
    lib.yamlstar_free(ptr)               # honor the ownership contract
    return s


def main():
    print("version :", call(lib.yamlstar_version))
    print("load    :", call(lib.yamlstar_load, "a: 1\nb: [x, y]\nc: true\nd: null", ""))
    print("load-all:", call(lib.yamlstar_load_all, "---\nx: 1\n---\ny: 2\n", ""))

    # round-trip correctness
    out = json.loads(call(lib.yamlstar_load, "name: yamlstar\nnums: [1, 2, 3]\nok: true", ""))
    assert out == {"data": {"name": "yamlstar", "nums": [1, 2, 3], "ok": True}}, out

    # error path must not panic across the boundary
    err = json.loads(call(lib.yamlstar_load, "a: [1, 2\n", ""))   # malformed
    assert "error" in err or "data" in err, err

    # repeated calls in one process (VM reused, no re-init)
    for i in range(1000):
        call(lib.yamlstar_load, f"i: {i}", "")

    print("ROUNDTRIP + ERROR-PATH + 1000x OK")


if __name__ == "__main__":
    sys.exit(main())

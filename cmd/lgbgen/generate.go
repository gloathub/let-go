/*
 * Copyright (c) 2026 Norman Nunley, Jr <nnunley@gmail.com>
 * Part of the let-go project; see CONTRIBUTORS for full list of authors.
 * SPDX-License-Identifier: MIT
 */

// This file carries the go:generate directives that regenerate the
// core bundle and the lowered Go tree from the .lg sources. It has no
// build tag (unlike main.go, which is //go:build bootstrap) so that
// `go generate ./...` discovers the directives without the bootstrap
// tag set on the discovering build.
//
// NOTE: `go build` does NOT run these — Go has no build-time codegen
// hook. Run them explicitly with `go generate ./cmd/lgbgen` (or, more
// conveniently, `make generate`, which also handles ordering and the
// other generated files). The genmanifest staleness test fails in
// `go test ./...` if you forget.
//
//go:generate go run -tags bootstrap .
//go:generate go run -tags bootstrap . --target=go
package main

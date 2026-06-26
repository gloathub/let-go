/*
 * Copyright (c) 2026 Norman Nunley, Jr <nnunley@gmail.com>
 * Part of the let-go project; see CONTRIBUTORS for full list of authors.
 * SPDX-License-Identifier: MIT
 */

package ir_test

import (
	"strings"
	"testing"

	"github.com/nooga/let-go/pkg/vm"
)

// Regression for the DCE/quot trap. quot is value-pure (CSE/LICM eligible)
// but traps on a zero divisor, so DCE must NOT delete an unused (quot _ 0):
// removing it would silently drop the divide-by-zero, diverging from the
// bytecode compiler. purity/effect-free-inst? excludes trapping ops; CSE/LICM
// still see quot as pure via pure-inst?.
func TestDCEKeepsUnusedTrappingQuot(t *testing.T) {
	ensureLoader()

	// Zero-use (quot 1 0): value-pure but trapping. DCE must keep it.
	f := buildLispIR(t, `(defn dead-quot [] (let [unused (quot 1 0)] :ok))`)
	runLispPass(t, "ir.passes.dce", "dce", f)
	if dump := lispDump(t, f); !strings.Contains(dump, "Quot") {
		t.Fatalf("DCE removed an unused trapping (quot 1 0); its divide-by-zero "+
			"trap would be lost.\ndump:\n%s", dump)
	}

	// Contrast: an unused non-trapping pure op IS still removable, proving DCE
	// wasn't blanket-disabled.
	g := buildLispIR(t, `(defn dead-add [] (let [unused (+ 1 2)] :ok))`)
	runLispPass(t, "ir.passes.dce", "dce", g)
	if dump := lispDump(t, g); strings.Contains(dump, "Add") {
		t.Fatalf("DCE should still remove an unused non-trapping (+ 1 2).\ndump:\n%s", dump)
	}

	// The op classification that drives the split: quot is both pure (so
	// CSE/LICM keep applying) and trapping (so DCE keeps it).
	if got := prStr(t, `(ir.passes.purity/trapping-op? :quot)`); got != "true" {
		t.Fatalf("trapping-op? :quot = %s, want true", got)
	}
	if got := prStr(t, `(ir.passes.purity/pure-op? :quot)`); got != "true" {
		t.Fatalf("pure-op? :quot = %s, want true (still CSE/LICM-eligible)", got)
	}
}

func prStr(t *testing.T, expr string) string {
	t.Helper()
	return string(runLispExpr(t, "(pr-str "+expr+")").(vm.String))
}

/*
 * Copyright (c) 2026 Norman Nunley, Jr <nnunley@gmail.com>
 * Part of the let-go project; see CONTRIBUTORS for full list of authors.
 * SPDX-License-Identifier: MIT
 */

// check-generated exits non-zero when the committed generated artifacts
// (pkg/rt/core_compiled.lgb + the lowered Go tree) are stale relative to
// their .lg / lgbgen sources. Content-based, so it is reliable across
// git/jj checkouts. Used by the Makefile `check-generated` target, by
// CI, and by the git pre-commit hook (scripts/pre-commit).
package main

import (
	"fmt"
	"os"

	"github.com/nooga/let-go/pkg/genmanifest"
)

func main() {
	root, err := genmanifest.FindRepoRoot(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-generated: %v\n", err)
		os.Exit(2)
	}

	res, err := genmanifest.Check(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-generated: %v\n", err)
		os.Exit(2)
	}

	if res.Recorded == "" {
		fmt.Fprintf(os.Stderr, "check-generated: manifest %s missing.\n%s\n",
			genmanifest.ManifestRelPath, genmanifest.Remediation)
		os.Exit(1)
	}
	if !res.Fresh {
		fmt.Fprintf(os.Stderr,
			"check-generated: STALE — .lg/lgbgen sources changed without regeneration.\n"+
				"  recorded: %s\n  computed: %s\n%s\n",
			res.Recorded, res.Computed, genmanifest.Remediation)
		os.Exit(1)
	}

	fmt.Println("check-generated: OK — bundle + lowered tree are in sync with sources.")
}

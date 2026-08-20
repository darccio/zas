/*
 * Copyright (c) 2013 Dario Castañé.
 * This file is part of Zas.
 *
 * Zas is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * Zas is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with Zas.  If not, see <http://www.gnu.org/licenses/>.
 */

package zas

import (
	"os"
	"path/filepath"
	"testing"
)

// walk used to unconditionally MkdirAll the deploy counterpart of every
// source directory it visited, before knowing whether anything inside it
// would actually be deployed. A directory holding only skipped content
// (e.g. just a .zas.yml, which the dotfile check excludes) got a
// permanently empty counterpart in deploy: nothing ever wrote into it, and
// nothing ever cleaned it up afterward either.

func TestGenerateSkipsEmptyDeployDirForAllDotfileSourceDir(t *testing.T) {
	newTestSite(t, "empty-dir-site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	assertDeployHas(t, "index.html")
	if _, err := os.Stat(filepath.Join(".zas", "deploy", "emptydir")); err == nil {
		t.Fatal("deploy/emptydir exists, want no deploy counterpart for a source directory whose entire content was skipped")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

// A subdirectory with a mix of skipped (sub/.zas.yml) and real content
// (sub/page.html) must still get its deploy counterpart, created on demand
// once the real file is written - covered by the existing
// TestGenerateProducesDeployTree in generate_e2e_test.go against
// testdata/site's sub/ directory, not duplicated here.

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

// copy used to open its destination through atomicWriteFile, which always
// applies the fixed ZAS_DEFAULT_FILE_PERM (0644) regardless of the
// source's own mode - so an executable asset (a script, a binary) lost its
// +x bit on every deploy.

func TestCopyPreservesExecutableBit(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "script.sh")
	if err := os.WriteFile(src, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(tmp, "deploy", "script.sh")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}

	gen := &Generator{}
	if err := gen.copy(dst, src); err != nil {
		t.Fatalf("copy() error = %v, want nil", err)
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o755); got != want {
		t.Fatalf("deployed file mode = %o, want %o (source's own mode preserved)", got, want)
	}
}

func TestCopyPreservesNonExecutableMode(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "data.json")
	if err := os.WriteFile(src, []byte(`{}`), 0o640); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(tmp, "deploy", "data.json")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}

	gen := &Generator{}
	if err := gen.copy(dst, src); err != nil {
		t.Fatalf("copy() error = %v, want nil", err)
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	// A regression check for the fix itself, not just the executable case:
	// the deployed mode should track the source's own mode (0640) rather
	// than always landing on the fixed ZAS_DEFAULT_FILE_PERM (0644) that
	// atomicWriteFile applies before copy's chmod runs.
	if got, want := info.Mode().Perm(), os.FileMode(0o640); got != want {
		t.Fatalf("deployed file mode = %o, want %o (source's own mode preserved, not the fixed default)", got, want)
	}
	if got := os.FileMode(0o644); got == info.Mode().Perm() {
		t.Fatalf("deployed file mode = %o, want it to differ from the fixed ZAS_DEFAULT_FILE_PERM default, proving the source's own mode was actually applied", got)
	}
}

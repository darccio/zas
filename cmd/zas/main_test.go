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
package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeStubPlugin(t *testing.T, dir, name, script string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestRunUnknownCommandDoesNotPanic covers the everyday typo case: no
// zs<name> binary exists on PATH at all. run must report a clean message
// and a defined exit code instead of the panic/stack trace this is a
// regression test for.
func TestRunUnknownCommandDoesNotPanic(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w

	code := run([]string{"thisdoesnotexist12345"})

	os.Stderr = origStderr
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}

	if code != 127 {
		t.Errorf("run() = %d, want 127", code)
	}
	msg := string(out)
	if !strings.Contains(msg, "unknown command") || !strings.Contains(msg, "thisdoesnotexist12345") {
		t.Errorf("stderr = %q, want it to report the unknown command", msg)
	}
	if strings.Contains(msg, "panic") || strings.Contains(msg, "goroutine") {
		t.Errorf("stderr = %q, want no panic/stack trace", msg)
	}
}

// TestRunPluginExitCodePropagates covers a plugin that starts but exits
// non-zero: its exact exit code must reach the caller, not a panic.
func TestRunPluginExitCodePropagates(t *testing.T) {
	dir := t.TempDir()
	writeStubPlugin(t, dir, "zsstubfail", "exit 42")
	t.Setenv("PATH", dir)

	if code := run([]string{"stubfail"}); code != 42 {
		t.Errorf("run() = %d, want 42", code)
	}
}

// TestRunPluginSuccess is the regression check that a well-behaved plugin
// still works normally after the error-handling changes.
func TestRunPluginSuccess(t *testing.T) {
	dir := t.TempDir()
	writeStubPlugin(t, dir, "zsstubok", "exit 0")
	t.Setenv("PATH", dir)

	if code := run([]string{"stubok"}); code != 0 {
		t.Errorf("run() = %d, want 0", code)
	}
}

// TestRunUnknownFlagOnInternalSubcommand is a narrow regression check that
// the unrelated cmd.Flag.Parse usage-error path (exit 2), left untouched by
// this fix, still works after extracting run out of main.
func TestRunUnknownFlagOnInternalSubcommand(t *testing.T) {
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = devNull.Close() }()

	origStderr := os.Stderr
	os.Stderr = devNull
	defer func() { os.Stderr = origStderr }()

	if code := run([]string{"generate", "--no-such-flag"}); code != 2 {
		t.Errorf("run() = %d, want 2", code)
	}
}

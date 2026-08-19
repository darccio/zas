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

// captureOutput redirects *target (os.Stdout or os.Stderr) to a pipe for
// the duration of fn, and returns everything written to it.
func captureOutput(t *testing.T, target **os.File, fn func()) string {
	t.Helper()

	orig := *target
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	*target = w

	fn()

	*target = orig
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
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

// TestRunHelpListsSubcommands covers "zas help": it must succeed and list
// every internal subcommand's one-line usage, instead of falling through to
// runPlugin looking for a nonexistent "zshelp" binary.
func TestRunHelpListsSubcommands(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	var code int
	out := captureOutput(t, &os.Stdout, func() {
		code = run([]string{"help"})
	})

	if code != 0 {
		t.Errorf("run() = %d, want 0", code)
	}
	for _, name := range []string{"init", "generate", "help", "version"} {
		if !strings.Contains(out, name) {
			t.Errorf("stdout = %q, want it to mention subcommand %q", out, name)
		}
	}
}

// TestRunTopLevelHelpFlag covers "zas -h" and "zas --help" used bare (no
// other subcommand): both must now show the same general help as "zas
// help", not the old behavior of being rewritten to "generate -h".
func TestRunTopLevelHelpFlag(t *testing.T) {
	for _, arg := range []string{"-h", "-help", "--help"} {
		t.Run(arg, func(t *testing.T) {
			t.Setenv("PATH", t.TempDir())

			var code int
			out := captureOutput(t, &os.Stdout, func() {
				code = run([]string{arg})
			})

			if code != 0 {
				t.Errorf("run() = %d, want 0", code)
			}
			// The general help lists every subcommand's usage line,
			// including "version" - generate's own -h usage would not.
			if !strings.Contains(out, "version") {
				t.Errorf("stdout = %q, want general help mentioning the version subcommand", out)
			}
		})
	}
}

// TestRunGenerateHelpStillShowsGenerateUsage is the regression guard for the
// routing change above: "zas generate -h" (subcommand named explicitly)
// must keep showing generate's own flag usage, not the general help.
func TestRunGenerateHelpStillShowsGenerateUsage(t *testing.T) {
	var code int
	// flag.FlagSet's default usage output goes to its Output(), which
	// defaults to os.Stderr.
	out := captureOutput(t, &os.Stderr, func() {
		code = run([]string{"generate", "-h"})
	})

	if code != 0 {
		t.Errorf("run() = %d, want 0", code)
	}
	if !strings.Contains(out, "Usage of generate:") {
		t.Errorf("stderr = %q, want it to report \"Usage of generate:\" (not \"Usage of :\")", out)
	}
	if !strings.Contains(out, "-verbose") || !strings.Contains(out, "-full") {
		t.Errorf("stderr = %q, want it to list generate's -verbose and -full flags", out)
	}
}

// TestRunVersion covers "zas version" and the top-level "-version"/
// "--version" flags: all three must print version information and exit 0.
func TestRunVersion(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"-version"}, {"--version"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var code int
			out := captureOutput(t, &os.Stdout, func() {
				code = run(args)
			})

			if code != 0 {
				t.Errorf("run() = %d, want 0", code)
			}
			if !strings.Contains(out, "zas version") {
				t.Errorf("stdout = %q, want it to report a zas version", out)
			}
		})
	}
}

// TestRunPluginDispatchIsCaseInsensitive covers the casing-consistency fix:
// internal dispatch already lower-cases args[0] before matching against the
// subcommands table, so plugin dispatch must fold the command name the same
// way before looking for a zs<name> binary. Without the fix, only the
// all-lowercase spelling would find the (lowercase-named) stub binary; the
// mixed- and upper-case spellings would fail with "unknown command" even
// though the same plugin is unambiguously being asked for.
func TestRunPluginDispatchIsCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	writeStubPlugin(t, dir, "zsstubcase", "exit 0")
	t.Setenv("PATH", dir)

	for _, arg := range []string{"stubcase", "StubCase", "STUBCASE"} {
		t.Run(arg, func(t *testing.T) {
			if code := run([]string{arg}); code != 0 {
				t.Errorf("run([]string{%q}) = %d, want 0", arg, code)
			}
		})
	}
}

// TestRunUnexpectedPositionalArgumentRejected covers the fix for silently
// dropped positional arguments: leftover non-flag arguments after
// cmd.Flag.Parse are now a usage error (stderr message, exit code 2 - the
// same code flag.Parse itself uses for a bad flag) instead of being
// discarded. init and generate both take no positional arguments in their
// own logic, so both must reject one identically rather than special-casing
// either.
func TestRunUnexpectedPositionalArgumentRejected(t *testing.T) {
	for _, args := range [][]string{
		{"generate", "-verbose", "some-unexpected-argument"},
		{"init", "some-unexpected-argument"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var code int
			out := captureOutput(t, &os.Stderr, func() {
				code = run(args)
			})

			if code != 2 {
				t.Errorf("run(%v) = %d, want 2", args, code)
			}
			if !strings.Contains(out, "some-unexpected-argument") {
				t.Errorf("stderr = %q, want it to mention the unexpected argument", out)
			}
		})
	}
}

// TestRunHelpRejectsUnexpectedArgument is the symmetry check for the fix
// above: help and version route through the exact same dispatch loop as
// init and generate, so an unexpected positional argument must be rejected
// there too, rather than being silently accepted by some subcommands and
// rejected by others.
func TestRunHelpRejectsUnexpectedArgument(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	var code int
	out := captureOutput(t, &os.Stderr, func() {
		code = run([]string{"help", "some-unexpected-argument"})
	})

	if code != 2 {
		t.Errorf("run() = %d, want 2", code)
	}
	if !strings.Contains(out, "some-unexpected-argument") {
		t.Errorf("stderr = %q, want it to mention the unexpected argument", out)
	}
}

// TestRunPluginReceivesStdin covers the stdin fix: runPlugin previously
// wired the plugin subprocess's stdout and stderr but not its stdin, so a
// filter-style plugin reading piped input would see immediate EOF. The stub
// here is a "cat", which echoes back whatever it reads from stdin; if the
// fix works, that content shows up on zas's own stdout.
func TestRunPluginReceivesStdin(t *testing.T) {
	dir := t.TempDir()
	writeStubPlugin(t, dir, "zsstubecho", "cat")
	// Unlike the other stub-plugin tests, PATH keeps the real system
	// directories (after dir, so the stub still wins on name collisions):
	// the stub script itself execs the real "cat" binary to copy stdin to
	// stdout, so it needs "cat" to be findable too.
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	origStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	const piped = "some piped input"
	go func() {
		_, _ = w.WriteString(piped)
		_ = w.Close()
	}()

	var code int
	out := captureOutput(t, &os.Stdout, func() {
		code = run([]string{"stubecho"})
	})

	if code != 0 {
		t.Errorf("run() = %d, want 0", code)
	}
	if !strings.Contains(out, piped) {
		t.Errorf("stdout = %q, want it to contain the piped stdin content %q", out, piped)
	}
}

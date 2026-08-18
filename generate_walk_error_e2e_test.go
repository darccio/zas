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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// End-to-end reproduction: when filepath.Walk itself errors partway
// through the source tree (e.g. a permission-denied subdirectory), Run
// must still wait for every already-spawned renderAsync goroutine before
// returning. Proving that needs a real process exit - main.go calls
// os.Exit(1) the instant Run returns a non-nil error, which kills any
// goroutine still mid-write - so this launches the actual generate path in
// a subprocess via TestMain instead of merely calling Run() in-process,
// where nothing would ever interrupt a lagging goroutine.

const walkErrorHelperEnv = "ZAS_WALK_ERROR_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(walkErrorHelperEnv) == "1" {
		runWalkErrorHelperProcess()
		return
	}
	os.Exit(m.Run())
}

// runWalkErrorHelperProcess mirrors cmd/zas/main.go's generate subcommand:
// run a full generation over the current directory and exit 1 immediately
// on error, exactly like the real CLI.
func runWalkErrorHelperProcess() {
	gen := &Generator{Full: true}
	if err := gen.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

// TestHelperHarness only exists as a -test.run target for the subprocess
// launched below; TestMain always intercepts before this would run.
func TestHelperHarness(_ *testing.T) {}

// buildWalkErrorFixture writes a site with many source files spread across
// several directories, chmods one directory partway through the tree (not
// first, not last) to 0o000, and returns its path. filepath.Walk visits a
// directory's siblings in lexical order and fully descends into each
// before moving to the next, so sect-00..sect-08 (populated) are walked
// and rendered before the burst of files in sect-09 - some of which are
// still being rendered when the walk reaches sect-10-bad and aborts - and
// sect-11..sect-15 (populated but unreachable) prove the failure isn't
// simply the last entry in the tree.
func buildWalkErrorFixture(t *testing.T, dir string) (badDir string) {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	copyFixture(t, "walk-error-base", dir)

	writePages := func(sub string, n int) {
		must(os.MkdirAll(sub, 0o755))
		for f := range n {
			name := fmt.Sprintf("page-%03d.md", f)
			body := fmt.Sprintf("# Page %s/%s\n\nBody text for %s/%s.\n", sub, name, sub, name)
			must(os.WriteFile(filepath.Join(sub, name), []byte(body), 0o644))
		}
	}

	for d := range 9 {
		writePages(filepath.Join(dir, fmt.Sprintf("sect-%02d", d)), 10)
	}
	// A large burst positioned immediately before the permission-denied
	// directory, so plenty of renderAsync goroutines are still starting up
	// (or mid-render) at the exact moment the walk hits it and aborts.
	writePages(filepath.Join(dir, "sect-09"), 250)

	badDir = filepath.Join(dir, "sect-10-bad")
	must(os.MkdirAll(badDir, 0o755))
	must(os.WriteFile(filepath.Join(badDir, "unreachable.md"), []byte("# Unreachable\n"), 0o644))

	for d := 11; d <= 15; d++ {
		writePages(filepath.Join(dir, fmt.Sprintf("sect-%02d", d)), 10)
	}

	must(os.Chmod(badDir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(badDir, 0o755) })
	return badDir
}

// TestGenerateWalkErrorDrainsInFlightGoroutines is the end-to-end
// regression test: it runs the fixture above through a real subprocess so
// main.go's os.Exit(1) can genuinely interrupt any goroutine Run failed to
// wait for, then verifies every file that made it into deploy is complete.
func TestGenerateWalkErrorDrainsInFlightGoroutines(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: directory permission bits don't restrict access, so the injected error can't be triggered")
	}

	dir := t.TempDir()
	badDir := buildWalkErrorFixture(t, dir)

	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperHarness$")
	cmd.Env = append(os.Environ(), walkErrorHelperEnv+"=1")
	cmd.Dir = dir
	out, runErr := cmd.CombinedOutput()

	// (a) the run fails as expected.
	var exitErr *exec.ExitError
	if !errors.As(runErr, &exitErr) {
		t.Fatalf("subprocess error = %v, want *exec.ExitError (output: %s)", runErr, out)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("subprocess exit code = %d, want 1 (output: %s)", exitErr.ExitCode(), out)
	}

	// (c) gen.errs / the returned error still reflects the walk failure.
	if !strings.Contains(string(out), "permission denied") {
		t.Fatalf("subprocess output = %q, want it to mention the permission error", out)
	}
	if !strings.Contains(string(out), filepath.Base(badDir)) {
		t.Fatalf("subprocess output = %q, want it to mention %q", out, filepath.Base(badDir))
	}

	// (b) no truncated/corrupt file survives in deploy: every file that
	// exists must be a complete render, never a partial write.
	deployRoot := filepath.Join(dir, ZAS_DIR, "deploy")
	rendered := 0
	walkErr := filepath.Walk(deployRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rendered++
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Errorf("ReadFile(%q): %v", path, readErr)
			return nil
		}
		if !strings.Contains(string(content), "</html>") {
			t.Errorf("%s: content = %q (%d bytes), want a complete render ending in </html>", path, content, len(content))
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walking deploy dir: %v", walkErr)
	}
	if rendered == 0 {
		t.Fatal("no files were rendered to deploy at all; fixture didn't exercise the in-flight scenario")
	}
	t.Logf("verified %d rendered files in deploy, none truncated", rendered)

	// Extra coverage: fixing the permission problem and rerunning
	// incrementally must pick up every file the aborted walk never reached
	// - nothing about the failed run should leave them permanently stuck.
	if err := os.Chmod(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	gen := &Generator{}
	if err := gen.Run(); err != nil {
		t.Fatalf("incremental rerun after fixing the permission error: error = %v, want nil", err)
	}
	assertDeployHas(t, filepath.Join("sect-10-bad", "unreachable.html"))
	assertDeployHas(t, filepath.Join("sect-15", "page-009.html"))
}

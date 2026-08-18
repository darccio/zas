package zas

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestGenerateSurvivesStrayLeftoverTempFile simulates what a prior crash
// would leave behind: a temporary file dropped mid-atomic-write, in the same
// directory atomicWriteFile itself would use, before the rename that would
// have replaced it ever ran. A subsequent run must not be confused by it -
// reaper has no matching source for it, so it must simply be removed like
// any other orphaned deploy file.
func TestGenerateSurvivesStrayLeftoverTempFile(t *testing.T) {
	newTestSite(t, "site")
	if err := generate(t); err != nil {
		t.Fatalf("first generate() error = %v, want nil", err)
	}

	strayPath := filepath.Join(".zas", "deploy", ".about.html.999999999.tmp")
	if err := os.WriteFile(strayPath, []byte("leftover from a simulated crash, before its rename"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := generate(t); err != nil {
		t.Fatalf("second generate() error = %v, want nil", err)
	}

	if _, err := os.Stat(strayPath); !os.IsNotExist(err) {
		t.Fatalf("stray temp file still present after a subsequent run (stat err = %v), want reaped", err)
	}
	if out := readDeploy(t, "about.html"); !strings.Contains(out, "This is the about page.") {
		t.Fatalf("about.html = %q, want it to still contain the body text", out)
	}
}

const (
	killFixtureNPages    = 3000
	killFixtureNAssets   = 25
	killFixtureAssetSize = 25 * 1024 * 1024
)

// buildAtomicWriteKillFixture writes a site with many markdown pages and a
// handful of sizeable binary assets, so a real, externally-delivered kill
// has a wide window to land while at least one renderAsync goroutine is
// between opening its temporary file (via atomicWriteFile) and renaming it
// onto the real destination. Dynamically generated per CONTRIBUTING.md
// (a loop writing many files doesn't belong in a checked-in fixture); only
// the static config/layout pair comes from testdata/walk-error-base.
func buildAtomicWriteKillFixture(t *testing.T, dir string) {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	copyFixture(t, "walk-error-base", dir)

	for i := range killFixtureNPages {
		name := fmt.Sprintf("page-%04d.md", i)
		body := fmt.Sprintf("# Page %d\n\n%s\n", i, strings.Repeat("Lorem ipsum dolor sit amet. ", 20))
		must(os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}
	data := bytes.Repeat([]byte{0xAB}, killFixtureAssetSize)
	for i := range killFixtureNAssets {
		name := fmt.Sprintf("asset-%03d.bin", i)
		must(os.WriteFile(filepath.Join(dir, name), data, 0o644))
	}
}

// countDeployedFiles counts regular (non-directory) entries anywhere under
// root, ignoring read errors from entries that may vanish mid-walk.
func countDeployedFiles(root string) int {
	n := 0
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			n++
		}
		return nil
	})
	return n
}

// TestGenerateKillMidRunNeverLeavesPartialFinalFile is the end-to-end proof
// that atomicWriteFile makes writes crash-safe: a real SIGKILL delivered to
// a subprocess genuinely mid-run - a strictly more general interruption than
// the walk error TestGenerateWalkErrorDrainsInFlightGoroutines exercises,
// since it kills the process outright regardless of any in-process goroutine
// draining - must never leave a partially-written file observable at any
// final (non-temporary) deploy path. It reuses that same test's subprocess
// re-exec harness (TestMain / runWalkErrorHelperProcess / TestHelperHarness).
func TestGenerateKillMidRunNeverLeavesPartialFinalFile(t *testing.T) {
	dir := t.TempDir()
	buildAtomicWriteKillFixture(t, dir)
	const total = killFixtureNPages + killFixtureNAssets

	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperHarness$")
	cmd.Env = append(os.Environ(), walkErrorHelperEnv+"=1")
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	deployRoot := filepath.Join(dir, ZAS_DIR, "deploy")
	deadline := time.Now().Add(30 * time.Second)
	killed := false
	for time.Now().Before(deadline) {
		n := countDeployedFiles(deployRoot)
		if n > total/10 && n < total*8/10 {
			killed = cmd.Process.Kill() == nil
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !killed {
		_ = cmd.Process.Kill()
	}
	_ = cmd.Wait()
	if !killed {
		t.Skip("could not land the kill mid-run on this machine (finished too fast); rerun to try again")
	}

	// The core property under test: every FINAL-named file in deploy (i.e.
	// every path that isn't one of atomicWriteFile's own leftover ".tmp"
	// files) is either a complete render/copy or doesn't exist at all -
	// never truncated or partial. Leftover ".tmp" files are expected crash
	// residue, not a failure - counting them is a positive control, proving
	// the kill actually landed while a write was in flight rather than
	// passing vacuously because it landed in a quiet moment.
	checked, strayTmp := 0, 0
	walkErr := filepath.Walk(deployRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			t.Errorf("walking deploy: %v", err)
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".tmp") {
			strayTmp++
			return nil
		}
		checked++
		switch {
		case strings.HasSuffix(path, ".html"):
			content, rerr := os.ReadFile(path)
			if rerr != nil {
				t.Errorf("ReadFile(%q): %v", path, rerr)
				return nil
			}
			if !strings.Contains(string(content), "</html>") {
				t.Errorf("%s: incomplete render, %d bytes, missing </html>", path, len(content))
			}
		case strings.HasSuffix(path, ".bin"):
			if info.Size() != killFixtureAssetSize {
				t.Errorf("%s: incomplete copy, %d of %d bytes", path, info.Size(), killFixtureAssetSize)
			}
		default:
			t.Errorf("%s: unexpected entry in deploy", path)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walking deploy dir: %v", walkErr)
	}
	if checked == 0 {
		t.Fatal("no final-path files were observed in deploy at all; fixture didn't exercise the in-flight scenario")
	}
	if strayTmp == 0 {
		t.Skip("no leftover .tmp file found after the kill; the interruption didn't land mid-write, so this run didn't actually exercise the race - rerun")
	}
	t.Logf("verified %d final-path files in deploy after a mid-run kill, none truncated (and %d leftover temp files confirm the kill genuinely landed mid-write)", checked, strayTmp)

	// A follow-up incremental run must succeed, must clean up any leftover
	// temp files the kill left behind, and must leave every source with a
	// complete output - unlike the pre-fix bug, nothing here can be
	// permanently stuck, because no truncated file was ever left at a final
	// path for sourceIsNewer to mistake as up to date.
	t.Chdir(dir)
	gen := &Generator{}
	if err := gen.Run(); err != nil {
		t.Fatalf("incremental rerun after kill: error = %v, want nil", err)
	}
	_ = filepath.Walk(deployRoot, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".tmp") {
			t.Errorf("leftover temp file survived a follow-up run: %s", path)
		}
		return nil
	})
	for i := range killFixtureNPages {
		rel := fmt.Sprintf("page-%04d.html", i)
		content, rerr := os.ReadFile(filepath.Join(deployRoot, rel))
		if rerr != nil {
			t.Fatalf("%s missing after follow-up run: %v", rel, rerr)
		}
		if !strings.Contains(string(content), "</html>") {
			t.Errorf("%s still incomplete after follow-up run", rel)
		}
	}
	for i := range killFixtureNAssets {
		rel := fmt.Sprintf("asset-%03d.bin", i)
		info, serr := os.Stat(filepath.Join(deployRoot, rel))
		if serr != nil {
			t.Fatalf("%s missing after follow-up run: %v", rel, serr)
		}
		if info.Size() != killFixtureAssetSize {
			t.Errorf("%s still incomplete after follow-up run: %d of %d bytes", rel, info.Size(), killFixtureAssetSize)
		}
	}
}

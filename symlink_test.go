package zas

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// filepath.Walk calls os.Lstat, so a symlinked directory reaches walk with
// info.IsDir() == false (Lstat reports the link itself) and used to fall
// through to the ordinary file branch, which handed it to copy(). copy()'s
// os.Open transparently follows the link, and io.Copy then failed writing a
// regular file where the destination should have been a directory - a
// confusing "is a directory" error. A symlinked file has the same Lstat
// shape and used to be copied by silently dereferencing it, with nothing in
// the deploy output indicating the source was ever a link. These tests
// confirm both kinds are now skipped explicitly, with a message, instead of
// either mishandling.

// captureStderr redirects os.Stderr for the duration of fn and returns
// everything written to it. printLine (generate.go) writes there.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stderr = orig
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestWalkSkipsSymlinkedDirectoryWithMessage(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.Mkdir("realdir", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("realdir", "linkdir"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat("linkdir")
	if err != nil {
		t.Fatal(err)
	}
	gen := &Generator{}
	var walkErr error
	out := captureStderr(t, func() {
		walkErr = gen.walk("linkdir", info, nil)
	})
	if walkErr != nil {
		t.Fatalf("walk(%q) error = %v, want nil", "linkdir", walkErr)
	}
	if !strings.Contains(out, "linkdir") || !strings.Contains(out, "symlink") {
		t.Fatalf("walk(%q) printed %q, want a message mentioning the path and \"symlink\"", "linkdir", out)
	}
	gen.wg.Wait()
	if len(gen.errs) != 0 {
		t.Fatalf("gen.errs = %v, want none", gen.errs)
	}
}

func TestWalkSkipsSymlinkedFileWithMessage(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("real.txt", []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.txt", "link.txt"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat("link.txt")
	if err != nil {
		t.Fatal(err)
	}
	gen := &Generator{}
	var walkErr error
	out := captureStderr(t, func() {
		walkErr = gen.walk("link.txt", info, nil)
	})
	if walkErr != nil {
		t.Fatalf("walk(%q) error = %v, want nil", "link.txt", walkErr)
	}
	if !strings.Contains(out, "link.txt") || !strings.Contains(out, "symlink") {
		t.Fatalf("walk(%q) printed %q, want a message mentioning the path and \"symlink\"", "link.txt", out)
	}
	gen.wg.Wait()
	if len(gen.errs) != 0 {
		t.Fatalf("gen.errs = %v, want none", gen.errs)
	}
}

// TestGenerateSkipsSymlinksCleanly drives the full Generator.Run pipeline
// against a site containing both a directory symlink (realdir/linkdir) and
// a file symlink (real.txt/link.txt) alongside normal content. Before this
// fix, the directory symlink made the run fail with a copy error ("is a
// directory") and the file symlink was silently dereferenced into the
// deploy output. Now both must be absent from the deploy tree and the rest
// of the site must generate normally, with no fatal error.
func TestGenerateSkipsSymlinksCleanly(t *testing.T) {
	newTestSite(t, "site")

	if err := os.Mkdir("realdir", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("realdir", "page.md"), []byte("# Real\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("realdir", "linkdir"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("real.txt", []byte("hello from real"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.txt", "link.txt"); err != nil {
		t.Fatal(err)
	}

	if err := generate(t, fullGen); err != nil {
		t.Fatalf("generate() with symlinks present: error = %v, want nil (no more \"is a directory\" copy failure)", err)
	}

	// The symlinks themselves must not appear in deploy output at all -
	// neither as a mis-copied file (the old directory-symlink failure mode)
	// nor as a silently dereferenced copy (the old file-symlink failure
	// mode).
	assertDeployMissing(t, "linkdir")
	assertDeployMissing(t, "link.txt")

	// The symlink target reached directly (not through the link) is
	// unaffected: it's an ordinary source path.
	assertDeployHas(t, filepath.Join("realdir", "page.html"))

	// Normal content elsewhere in the site still generates fine alongside
	// the symlinks.
	assertDeployHas(t, "index.html")
	assertDeployHas(t, "about.html")
	assertDeployHas(t, filepath.Join("sub", "page.html"))
}

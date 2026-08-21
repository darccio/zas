package zas

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// This file provides shared helpers for the end-to-end generation tests in
// generate_e2e_test.go. They all resolve embed src and walk the source tree
// relative to the process's current directory, so every test here calls
// t.Chdir and none of them can run under t.Parallel.

// newTestSite copies testdata/<fixture> into a fresh temp directory and
// chdirs into it.
func newTestSite(t *testing.T, fixture string) string {
	t.Helper()
	dir := t.TempDir()
	copyFixture(t, fixture, dir)
	t.Chdir(dir)
	return dir
}

// copyFixture copies testdata/<fixture> into dir, which must already exist.
func copyFixture(t *testing.T, fixture, dir string) {
	t.Helper()
	src, err := filepath.Abs(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(dir, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
}

// generate runs a full Generator.Run against the current directory.
func generate(t *testing.T, opts ...func(*Generator)) error {
	t.Helper()
	gen := &Generator{}
	for _, opt := range opts {
		opt(gen)
	}
	return gen.Run()
}

func fullGen(g *Generator) {
	g.Full = true
}

func readDeploy(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(".zas", "deploy", rel))
	if err != nil {
		t.Fatalf("readDeploy(%q): %v", rel, err)
	}
	return string(data)
}

func assertDeployHas(t *testing.T, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(".zas", "deploy", rel)); err != nil {
		t.Fatalf("assertDeployHas(%q): %v", rel, err)
	}
}

func assertDeployMissing(t *testing.T, rel string) {
	t.Helper()
	_, err := os.Stat(filepath.Join(".zas", "deploy", rel))
	if err == nil {
		t.Fatalf("assertDeployMissing(%q): file exists, want missing", rel)
	}
	if !os.IsNotExist(err) {
		t.Fatalf("assertDeployMissing(%q): %v", rel, err)
	}
}

// ageSources sets the mtime of every file in the current directory tree
// (expected to be source files only - call this before the first generate)
// to time.Now().Add(d), so incremental-mode comparisons against freshly
// written deploy output are deterministic regardless of filesystem mtime
// resolution.
func ageSources(t *testing.T, d time.Duration) {
	t.Helper()
	when := time.Now().Add(d)
	err := filepath.WalkDir(".", func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if de.IsDir() {
			return nil
		}
		return os.Chtimes(p, when, when)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// touchFuture sets path's mtime an hour ahead of time.Now(), so a
// dependency file rewritten after the first generate is unambiguously
// newer than the deploy output that first generate just wrote, regardless
// of filesystem mtime resolution.
func touchFuture(t *testing.T, path string) {
	t.Helper()
	when := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

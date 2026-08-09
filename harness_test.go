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

// newTestSite copies testdata/<fixture> into a fresh temp directory, chdirs
// into it, and snapshots ZAS_DEFAULT_CONF so a test that triggers the
// mergo aliasing in NewConfig can't leak mutations into later tests.
func newTestSite(t *testing.T, fixture string) string {
	t.Helper()
	src, err := filepath.Abs(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	saveGlobals(t)
	return dir
}

// saveGlobals deep-copies ZAS_DEFAULT_CONF and restores it after the test,
// undoing any mutation reachable through mergo's aliasing of nested config
// sections in NewConfig.
func saveGlobals(t *testing.T) {
	t.Helper()
	snapshot := deepCopyConfigSection(ZAS_DEFAULT_CONF)
	t.Cleanup(func() {
		ZAS_DEFAULT_CONF = snapshot
	})
}

func deepCopyConfigSection(cs ConfigSection) ConfigSection {
	if cs == nil {
		return nil
	}
	out := make(ConfigSection, len(cs))
	for k, v := range cs {
		if nested, ok := v.(ConfigSection); ok {
			out[k] = deepCopyConfigSection(nested)
		} else {
			out[k] = v
		}
	}
	return out
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

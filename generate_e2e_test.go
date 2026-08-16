package zas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// End-to-end tests driving Generator.Run against testdata/site, the
// pipeline every other behavioral fix in this codebase needs a place to
// land a regression test against.

func TestGenerateProducesDeployTree(t *testing.T) {
	newTestSite(t, "site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	assertDeployHas(t, "index.html")
	assertDeployHas(t, "about.html")
	assertDeployHas(t, filepath.Join("sub", "page.html"))

	want, err := os.ReadFile(filepath.Join("assets", "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := readDeploy(t, filepath.Join("assets", "data.json")); got != string(want) {
		t.Fatalf("assets/data.json = %q, want %q", got, want)
	}
}

func TestGeneratePageConfigOverridesTitle(t *testing.T) {
	newTestSite(t, "site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	if out := readDeploy(t, "index.html"); !strings.Contains(out, "<title>Home</title>") {
		t.Fatalf("index.html = %q, want it to contain %q", out, "<title>Home</title>")
	}
}

func TestGenerateConvertsMarkdown(t *testing.T) {
	newTestSite(t, "site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	out := readDeploy(t, "about.html")
	if !strings.Contains(out, "<h1>About</h1>") {
		t.Fatalf("about.html = %q, want it to contain %q", out, "<h1>About</h1>")
	}
	if !strings.Contains(out, "This is the about page.") {
		t.Fatalf("about.html = %q, want it to contain the body text", out)
	}
}

func TestGenerateResolvesHTMLEmbed(t *testing.T) {
	newTestSite(t, "site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	out := readDeploy(t, "index.html")
	if !strings.Contains(out, "<nav>") || !strings.Contains(out, "Home</a>") {
		t.Fatalf("index.html = %q, want it to contain the embedded nav", out)
	}
}

func TestGenerateAppliesDirectoryConfig(t *testing.T) {
	newTestSite(t, "site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	if out := readDeploy(t, "index.html"); !strings.Contains(out, "Hello") {
		t.Fatalf("index.html = %q, want it to contain %q", out, "Hello")
	}
	if out := readDeploy(t, filepath.Join("sub", "page.html")); !strings.Contains(out, "Hola") {
		t.Fatalf("sub/page.html = %q, want it to contain %q", out, "Hola")
	}
}

func TestGenerateIncrementalSkipsUnchanged(t *testing.T) {
	newTestSite(t, "site")
	ageSources(t, -time.Hour)
	if err := generate(t); err != nil {
		t.Fatalf("first generate() error = %v, want nil", err)
	}
	target := filepath.Join(".zas", "deploy", "about.html")
	before, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := generate(t); err != nil {
		t.Fatalf("second generate() error = %v, want nil", err)
	}
	after, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("about.html mtime changed on an unchanged incremental run: before=%v after=%v", before.ModTime(), after.ModTime())
	}
}

func TestGenerateFullRebuilds(t *testing.T) {
	newTestSite(t, "site")
	if err := generate(t, fullGen); err != nil {
		t.Fatalf("first generate() error = %v, want nil", err)
	}
	stale := filepath.Join(".zas", "deploy", "stale.html")
	if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generate(t, fullGen); err != nil {
		t.Fatalf("second generate() error = %v, want nil", err)
	}
	assertDeployMissing(t, "stale.html")
	assertDeployHas(t, "index.html")
}

func TestGenerateIncrementalReapsRemovedDirectory(t *testing.T) {
	newTestSite(t, "site")
	if err := os.MkdirAll("sect", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("sect", "index.md"), []byte("# Sect\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("sect", "more.md"), []byte("# More\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generate(t, fullGen); err != nil {
		t.Fatalf("first generate() error = %v, want nil", err)
	}
	assertDeployHas(t, filepath.Join("sect", "index.html"))
	assertDeployHas(t, filepath.Join("sect", "more.html"))

	if err := os.RemoveAll("sect"); err != nil {
		t.Fatal(err)
	}
	if err := generate(t); err != nil {
		t.Fatalf("incremental generate() after removing a non-empty source directory: error = %v, want nil", err)
	}
	assertDeployMissing(t, "sect")
	assertDeployMissing(t, filepath.Join("sect", "index.html"))
	assertDeployMissing(t, filepath.Join("sect", "more.html"))
}

func TestGenerateFailsOutsideRepository(t *testing.T) {
	t.Chdir(t.TempDir())
	err := generate(t)
	if err == nil {
		t.Fatal("generate() outside a Zas repository: want error, got nil")
	}
	if !strings.Contains(err.Error(), "not a valid Zas repository") {
		t.Fatalf("generate() error = %v, want it to mention %q", err, "not a valid Zas repository")
	}
}

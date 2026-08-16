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

func TestGenerateHTMLDotMDSurvivesIncrementalRun(t *testing.T) {
	newTestSite(t, "site")
	if err := os.WriteFile("x.html.md", []byte("# X\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generate(t, fullGen); err != nil {
		t.Fatalf("first generate() error = %v, want nil", err)
	}
	assertDeployHas(t, "x.html.html")

	if err := generate(t); err != nil {
		t.Fatalf("incremental generate() error = %v, want nil", err)
	}
	assertDeployHas(t, "x.html.html")
}

func TestGenerateExtensionLikeDirectoryNotCorrupted(t *testing.T) {
	newTestSite(t, "site")
	if err := os.MkdirAll("v1.mdx", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("v1.mdx", "page.md"), []byte("# Page\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	assertDeployHas(t, filepath.Join("v1.mdx", "page.html"))
}

func TestGenerateSkipsNestedHiddenDirectory(t *testing.T) {
	newTestSite(t, "site")
	if err := os.MkdirAll(filepath.Join("sect", ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("sect", "index.md"), []byte("# Sect\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("sect", ".hidden", "page.md"), []byte("# Hidden\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	assertDeployHas(t, filepath.Join("sect", "index.html"))
	assertDeployMissing(t, filepath.Join("sect", ".hidden"))
	assertDeployMissing(t, filepath.Join("sect", ".hidden", "page.html"))
	assertDeployMissing(t, ".zas")
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

// TestGenerateDeployPathOutsideZasDirNotWalkedAsSource is a regression test:
// a deploy path outside .zas/ used to be walked as a source directory, so
// each run re-rendered the previous run's own output through the layout
// again and nested it one level deeper (public/public/..., compounding
// every run).
func TestGenerateDeployPathOutsideZasDirNotWalkedAsSource(t *testing.T) {
	t.Chdir(t.TempDir())
	saveGlobals(t)

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(ZAS_DIR, 0o755))
	must(os.WriteFile(ZAS_CONF_FILE, []byte(`zas:
  layout: `+ZAS_DIR+`/layout.html
  deploy: public
site:
  baseurl: http://example.com
  language: en
mimetypes:
  text/markdown: markdown
  text/plain: plain
  text/html: html
`), 0o644))
	must(os.WriteFile(filepath.Join(ZAS_DIR, "layout.html"), []byte(
		`<html><head><title>{{.Title}}</title></head><body>{{.Body}}</body></html>`), 0o644))
	must(os.WriteFile("index.md", []byte("# Hello\n"), 0o644))

	for range 3 {
		if err := generate(t); err != nil {
			t.Fatalf("generate() error = %v, want nil", err)
		}
	}

	if _, err := os.Stat(filepath.Join("public", "public")); !os.IsNotExist(err) {
		t.Fatalf("public/public exists after 3 runs (stat err = %v), want it absent", err)
	}
	if _, err := os.Stat(filepath.Join("public", "index.html")); err != nil {
		t.Fatalf("public/index.html missing: %v", err)
	}
}

// TestGenerateLayoutPathOutsideZasDirNotPublishedAsPage is a regression
// test: a layout file outside .zas/ used to be walked like any other page
// and published under its own name in the deploy tree.
func TestGenerateLayoutPathOutsideZasDirNotPublishedAsPage(t *testing.T) {
	t.Chdir(t.TempDir())
	saveGlobals(t)

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(ZAS_DIR, 0o755))
	must(os.WriteFile(ZAS_CONF_FILE, []byte(`zas:
  layout: mylayout.html
  deploy: `+ZAS_DIR+`/deploy
site:
  baseurl: http://example.com
  language: en
mimetypes:
  text/markdown: markdown
  text/plain: plain
  text/html: html
`), 0o644))
	must(os.WriteFile("mylayout.html", []byte(
		`<html><head><title>{{.Title}}</title></head><body>{{.Body}}</body></html>`), 0o644))
	must(os.WriteFile("index.md", []byte("# Hello\n"), 0o644))
	must(os.WriteFile("about.md", []byte("# About\n"), 0o644))

	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}

	assertDeployMissing(t, "mylayout.html")
	assertDeployHas(t, "index.html")
	assertDeployHas(t, "about.html")
}

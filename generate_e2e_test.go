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

// renderAsync's dispatch switch used to match ".md"/".html" case-
// sensitively, so an uppercase-extension source like PAGE.MD fell through
// to the copy branch and shipped into deploy verbatim (unrendered),
// keeping its original PAGE.MD name instead of being converted and
// deployed as PAGE.html.
//
// generate(t) here exercises the full pipeline in one call, including the
// reap phase - which matters, because fixing only the dispatch switch
// uncovered a second bug: reaper reconstructs a candidate source path from
// a deploy path via swapExtension, which always normalizes the guessed
// extension's casing, so its guess for PAGE.html's source was "page.md",
// not "PAGE.MD". A plain os.Open on that lowercase guess never finds the
// real, differently-cased source, so reaper wrongly treated PAGE.html as
// stale and deleted it immediately after render() created it. PAGE.html
// still being present after generate(t) returns is proof reaper's own
// existence check (existsFold) is now case-insensitive too, not just the
// render dispatch.
func TestGenerateRendersUppercaseMarkdownExtension(t *testing.T) {
	newTestSite(t, "site")
	if err := os.WriteFile("PAGE.MD", []byte("# Upper\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	assertDeployHas(t, "PAGE.html")
	assertDeployMissing(t, "PAGE.MD")
	if out := readDeploy(t, "PAGE.html"); !strings.Contains(out, "<h1>Upper</h1>") {
		t.Fatalf("PAGE.html = %q, want it converted from Markdown, not copied verbatim", out)
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

// TestGenerateDirNameEndingInZasDirNotExcluded is a regression test: a
// source directory whose name merely ends in Dir (".zas"), such as
// "docs.zas", used to be silently excluded from generation because the
// walk's exclusion check matched the substring ".zas/" anywhere in the
// path, not just the real ".zas" directory.
func TestGenerateDirNameEndingInZasDirNotExcluded(t *testing.T) {
	newTestSite(t, "site")
	if err := os.MkdirAll("docs.zas", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("docs.zas", "page.md"), []byte("# Page\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	assertDeployHas(t, filepath.Join("docs.zas", "page.html"))
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

// zasConfigWithAllowedDotDirs is testdata/site's own .zas/config.yml (see
// that fixture) with an allowed_dotdirs entry added under the zas section,
// for tests exercising the dot-directory allowlist escape hatch.
const zasConfigWithAllowedDotDirs = `zas:
  layout: .zas/layout.html
  deploy: .zas/deploy
  allowed_dotdirs: [".well-known"]
site:
  baseurl: http://example.com
  language: en
mimetypes:
  text/markdown: markdown
  text/plain: plain
  text/html: html
`

// TestGenerateAllowedDotDirIsDeployed is a regression test for the "no
// escape hatch" gap left after the original dot-directory-pruning fix: a
// top-level dot-directory named in the zas section's allowed_dotdirs config
// (here ".well-known", the standard location a site's ACME challenge files
// and similar well-known resources live in) must actually be walked and
// deployed, not pruned like every other dot-directory.
func TestGenerateAllowedDotDirIsDeployed(t *testing.T) {
	newTestSite(t, "site")
	if err := os.WriteFile(filepath.Join(".zas", "config.yml"), []byte(zasConfigWithAllowedDotDirs), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(".well-known", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".well-known", "security.txt"), []byte("Contact: mailto:security@example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	assertDeployHas(t, filepath.Join(".well-known", "security.txt"))
	if got := readDeploy(t, filepath.Join(".well-known", "security.txt")); got != "Contact: mailto:security@example.com\n" {
		t.Fatalf("security.txt = %q, want it copied verbatim", got)
	}
}

// TestGenerateUnlistedDotDirStillPrunedWithAllowlistConfigured confirms the
// allowlist is exact, not a blanket re-enable of dot-directory walking: with
// ".well-known" allowed, an unrelated top-level dot-directory like
// ".secrets" must still never reach deploy.
func TestGenerateUnlistedDotDirStillPrunedWithAllowlistConfigured(t *testing.T) {
	newTestSite(t, "site")
	if err := os.WriteFile(filepath.Join(".zas", "config.yml"), []byte(zasConfigWithAllowedDotDirs), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(".secrets", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".secrets", "token.txt"), []byte("do-not-deploy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	assertDeployMissing(t, ".secrets")
	assertDeployMissing(t, filepath.Join(".secrets", "token.txt"))
}

// TestGenerateCollidingMDAndHTMLFailsDeterministically is a regression
// test: a source .md and .html file in the same directory used to both
// render to the identical deploy output path in concurrent goroutines,
// picking a nondeterministic winner (or worse, an interleaved/corrupted
// write) with no error at all. It must now fail the build and always
// keep the same winner - the first file filepath.Walk visits, since only
// that one ever gets a renderAsync goroutine spawned for it. This runs
// several fresh builds to confirm the outcome is deterministic, not just
// usually right; go test -race confirms there's no write race on the
// shared output file either.
func TestGenerateCollidingMDAndHTMLFailsDeterministically(t *testing.T) {
	for range 5 {
		t.Run("run", func(t *testing.T) {
			newTestSite(t, "site")
			if err := os.WriteFile("collide.md", []byte("# Marker A\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile("collide.html", []byte("<h1>Marker B</h1>\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			err := generate(t, fullGen)
			if err == nil {
				t.Fatal("generate() error = nil, want a collision error")
			}
			msg := err.Error()
			for _, want := range []string{"collide.md", "collide.html"} {
				if !strings.Contains(msg, want) {
					t.Fatalf("generate() error = %q, want it to mention %q", msg, want)
				}
			}

			out := readDeploy(t, "collide.html")
			if !strings.Contains(out, "Marker B") || strings.Contains(out, "Marker A") {
				t.Fatalf("collide.html = %q, want only the .html source's content", out)
			}
		})
	}
}

// TestGenerateSurvivesCollisionElsewhereInSite confirms a collision
// between two files doesn't block the rest of the site: every other page
// must still render even though the build's overall error return still
// signals the collision.
func TestGenerateSurvivesCollisionElsewhereInSite(t *testing.T) {
	newTestSite(t, "site")
	if err := os.WriteFile("collide.md", []byte("# Marker A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("collide.html", []byte("<h1>Marker B</h1>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generate(t, fullGen); err == nil {
		t.Fatal("generate() error = nil, want a collision error")
	}
	assertDeployHas(t, "index.html")
	assertDeployHas(t, "about.html")
	assertDeployHas(t, filepath.Join("sub", "page.html"))
	assertDeployHas(t, "collide.html")
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
	newTestSite(t, "deploy-outside-zas")

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
	newTestSite(t, "layout-outside-zas")

	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}

	assertDeployMissing(t, "mylayout.html")
	assertDeployHas(t, "index.html")
	assertDeployHas(t, "about.html")
}

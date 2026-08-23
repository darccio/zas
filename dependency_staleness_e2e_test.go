package zas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// End-to-end tests proving sourceIsNewer invalidates pages when a shared
// dependency changes, not just when the page's own source file does.
//
// The embed-related tests below cover embedTargetModTime, the same
// mechanism's extension to a page's (or layout.html's) own <embed src>
// targets.

func TestGenerateLayoutChangeInvalidatesEveryPage(t *testing.T) {
	newTestSite(t, "site")
	ageSources(t, -time.Hour)
	if err := generate(t); err != nil {
		t.Fatalf("first generate() error = %v, want nil", err)
	}

	layout := filepath.Join(".zas", "layout.html")
	data, err := os.ReadFile(layout)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(data), "<body>", `<body><p class="marker">layout-v2</p>`, 1)
	if updated == string(data) {
		t.Fatal("test fixture layout.html has no <body> tag to mark")
	}
	if err := os.WriteFile(layout, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	touchFuture(t, layout)

	if err := generate(t); err != nil {
		t.Fatalf("second generate() error = %v, want nil", err)
	}

	for _, page := range []string{"about.html", "index.html", filepath.Join("sub", "page.html")} {
		if out := readDeploy(t, page); !strings.Contains(out, "layout-v2") {
			t.Fatalf("%s = %q, want it to reflect the edited layout", page, out)
		}
	}
}

func TestGenerateConfigChangeInvalidatesEveryPage(t *testing.T) {
	newTestSite(t, "site")
	ageSources(t, -time.Hour)
	if err := generate(t); err != nil {
		t.Fatalf("first generate() error = %v, want nil", err)
	}
	if out := readDeploy(t, "about.html"); !strings.Contains(out, "http://example.com") {
		t.Fatalf("about.html = %q, want it to contain the original baseurl", out)
	}

	config := filepath.Join(".zas", "config.yml")
	data, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(data), "http://example.com", "http://updated.example.com", 1)
	if updated == string(data) {
		t.Fatal("test fixture config.yml has no baseurl to update")
	}
	if err := os.WriteFile(config, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	touchFuture(t, config)

	if err := generate(t); err != nil {
		t.Fatalf("second generate() error = %v, want nil", err)
	}

	for _, page := range []string{"about.html", "index.html", filepath.Join("sub", "page.html")} {
		out := readDeploy(t, page)
		if !strings.Contains(out, "http://updated.example.com") {
			t.Fatalf("%s = %q, want it to reflect the updated baseurl", page, out)
		}
	}
}

func TestGenerateI18nChangeInvalidatesEveryPage(t *testing.T) {
	newTestSite(t, "site")
	ageSources(t, -time.Hour)
	if err := generate(t); err != nil {
		t.Fatalf("first generate() error = %v, want nil", err)
	}
	if out := readDeploy(t, "about.html"); !strings.Contains(out, "Hello") {
		t.Fatalf("about.html = %q, want it to contain the original translation", out)
	}

	i18n := filepath.Join(".zas", "i18n.yml")
	if err := os.WriteFile(i18n, []byte("greeting:\n  en: Hi\n  es: Hola\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	touchFuture(t, i18n)

	if err := generate(t); err != nil {
		t.Fatalf("second generate() error = %v, want nil", err)
	}

	// about.html (root, language "en") and sub/page.html (its own
	// .zas.yml sets language "es") both embed the layout's {{.E
	// "greeting"}}, so both must pick up the new translation - proving
	// i18n.yml invalidation is global, not scoped to one page.
	if out := readDeploy(t, "about.html"); !strings.Contains(out, "Hi") || strings.Contains(out, "Hello") {
		t.Fatalf("about.html = %q, want the updated \"Hi\" translation", out)
	}
	if out := readDeploy(t, filepath.Join("sub", "page.html")); !strings.Contains(out, "Hola") {
		t.Fatalf("sub/page.html = %q, want it regenerated with the (unchanged) Hola translation", out)
	}
}

// TestGenerateZasYMLScopesInvalidationToItsSubtree is the trickiest case:
// editing a .zas.yml must regenerate pages under its own directory without
// touching pages elsewhere in the site, including an unrelated sibling
// subdirectory that has no .zas.yml of its own.
func TestGenerateZasYMLScopesInvalidationToItsSubtree(t *testing.T) {
	newTestSite(t, "site")
	if err := os.MkdirAll("other", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("other", "index.md"), []byte("# Other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ageSources(t, -time.Hour)
	if err := generate(t); err != nil {
		t.Fatalf("first generate() error = %v, want nil", err)
	}
	if out := readDeploy(t, filepath.Join("sub", "page.html")); !strings.Contains(out, "Hola") {
		t.Fatalf("sub/page.html = %q, want it to contain the original Hola translation", out)
	}

	subTarget := filepath.Join(".zas", "deploy", "sub", "page.html")
	otherTarget := filepath.Join(".zas", "deploy", "other", "index.html")
	aboutTarget := filepath.Join(".zas", "deploy", "about.html")

	subBefore, err := os.Stat(subTarget)
	if err != nil {
		t.Fatal(err)
	}
	otherBefore, err := os.Stat(otherTarget)
	if err != nil {
		t.Fatal(err)
	}
	aboutBefore, err := os.Stat(aboutTarget)
	if err != nil {
		t.Fatal(err)
	}

	zasYML := filepath.Join("sub", DirConfigFile)
	if err := os.WriteFile(zasYML, []byte("language: en\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	touchFuture(t, zasYML)

	if err := generate(t); err != nil {
		t.Fatalf("second generate() error = %v, want nil", err)
	}

	// Direction 1: sub/page.html is under the changed .zas.yml's
	// directory, so it must regenerate and reflect the new language.
	subAfter, err := os.Stat(subTarget)
	if err != nil {
		t.Fatal(err)
	}
	if subBefore.ModTime().Equal(subAfter.ModTime()) {
		t.Fatalf("sub/page.html mtime unchanged after editing sub/.zas.yml, want it regenerated: mtime=%v", subBefore.ModTime())
	}
	if out := readDeploy(t, filepath.Join("sub", "page.html")); !strings.Contains(out, "Hello") || strings.Contains(out, "Hola") {
		t.Fatalf("sub/page.html = %q, want it to reflect the new .zas.yml language (Hello, not Hola)", out)
	}

	// Direction 2: other/index.html and about.html are outside sub/, so
	// editing sub/.zas.yml must not touch them.
	otherAfter, err := os.Stat(otherTarget)
	if err != nil {
		t.Fatal(err)
	}
	if !otherBefore.ModTime().Equal(otherAfter.ModTime()) {
		t.Fatalf("other/index.html mtime changed after editing sub/.zas.yml, want unaffected sibling directory: before=%v after=%v", otherBefore.ModTime(), otherAfter.ModTime())
	}
	aboutAfter, err := os.Stat(aboutTarget)
	if err != nil {
		t.Fatal(err)
	}
	if !aboutBefore.ModTime().Equal(aboutAfter.ModTime()) {
		t.Fatalf("about.html mtime changed after editing sub/.zas.yml, want unaffected: before=%v after=%v", aboutBefore.ModTime(), aboutAfter.ModTime())
	}
}

// TestGenerateEmbedChangeInvalidatesEmbeddingPage proves editing only a
// page's <embed> target (index.html embeds partials/nav.html in the test
// fixture) - leaving index.html's own source, layout.html, config.yml,
// i18n.yml, and any .zas.yml all untouched - still invalidates index.html
// on the next incremental run.
func TestGenerateEmbedChangeInvalidatesEmbeddingPage(t *testing.T) {
	newTestSite(t, "site")
	ageSources(t, -time.Hour)
	if err := generate(t); err != nil {
		t.Fatalf("first generate() error = %v, want nil", err)
	}
	if out := readDeploy(t, "index.html"); !strings.Contains(out, "Home</a>") {
		t.Fatalf("index.html = %q, want it to contain the original embedded nav", out)
	}

	nav := filepath.Join("partials", "nav.html")
	data, err := os.ReadFile(nav)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(data), "Home</a>", "Home v2</a>", 1)
	if updated == string(data) {
		t.Fatal("test fixture partials/nav.html has no \"Home</a>\" to mark")
	}
	if err := os.WriteFile(nav, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	touchFuture(t, nav)

	if err := generate(t); err != nil {
		t.Fatalf("second generate() error = %v, want nil", err)
	}
	if out := readDeploy(t, "index.html"); !strings.Contains(out, "Home v2</a>") {
		t.Fatalf("index.html = %q, want it regenerated to reflect the edited embed target", out)
	}
}

// TestGenerateLayoutEmbedChangeInvalidatesEveryPage is the layout-level
// counterpart: an <embed> written directly into layout.html (outside
// {{.Body}}) resolves against the site root, per Generate's own
// data.embedBaseDir reset - editing only that embedded file must still
// invalidate every page, the same way editing layout.html itself does.
func TestGenerateLayoutEmbedChangeInvalidatesEveryPage(t *testing.T) {
	newTestSite(t, "site")
	if err := os.WriteFile("footer.html", []byte(`<footer class="marker">footer-v1</footer>`), 0o644); err != nil {
		t.Fatal(err)
	}
	layout := filepath.Join(".zas", "layout.html")
	data, err := os.ReadFile(layout)
	if err != nil {
		t.Fatal(err)
	}
	withEmbed := strings.Replace(string(data), "{{.Body}}", `{{.Body}}<embed src="footer.html" type="text/html">`, 1)
	if withEmbed == string(data) {
		t.Fatal("test fixture layout.html has no {{.Body}} to mark")
	}
	if err := os.WriteFile(layout, []byte(withEmbed), 0o644); err != nil {
		t.Fatal(err)
	}

	ageSources(t, -time.Hour)
	if err := generate(t); err != nil {
		t.Fatalf("first generate() error = %v, want nil", err)
	}
	if out := readDeploy(t, "about.html"); !strings.Contains(out, "footer-v1") {
		t.Fatalf("about.html = %q, want it to contain the layout's embedded footer", out)
	}

	if err := os.WriteFile("footer.html", []byte(`<footer class="marker">footer-v2</footer>`), 0o644); err != nil {
		t.Fatal(err)
	}
	touchFuture(t, "footer.html")

	if err := generate(t); err != nil {
		t.Fatalf("second generate() error = %v, want nil", err)
	}
	for _, page := range []string{"about.html", "index.html", filepath.Join("sub", "page.html")} {
		if out := readDeploy(t, page); !strings.Contains(out, "footer-v2") {
			t.Fatalf("%s = %q, want it to reflect the layout's changed embed target", page, out)
		}
	}
}

// TestGenerateTemplatedEmbedSrcNotTrackedForStaleness documents
// embedTargetModTime's deliberate limitation: an <embed src="{{...}}">
// whose src is a template action can't be resolved without running the
// page's own template, so editing only the target doesn't invalidate the
// embedding page - -full remains the escape hatch for this case, exactly
// as it did before embed staleness tracking existed at all.
func TestGenerateTemplatedEmbedSrcNotTrackedForStaleness(t *testing.T) {
	newTestSite(t, "site")
	src := "<embed src=\"{{if true}}partials/nav.html{{end}}\" type=\"text/html\">\n"
	if err := os.WriteFile("templated.md", []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	ageSources(t, -time.Hour)
	if err := generate(t); err != nil {
		t.Fatalf("first generate() error = %v, want nil", err)
	}
	target := filepath.Join(".zas", "deploy", "templated.html")
	before, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if out := readDeploy(t, "templated.html"); !strings.Contains(out, "Home</a>") {
		t.Fatalf("templated.html = %q, want the template-resolved embed to still render at generation time", out)
	}

	nav := filepath.Join("partials", "nav.html")
	data, err := os.ReadFile(nav)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nav, append(data, []byte("<!-- changed -->\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	touchFuture(t, nav)

	if err := generate(t); err != nil {
		t.Fatalf("second generate() error = %v, want nil", err)
	}
	after, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("templated.html mtime changed after editing a templated embed's target, want it left untracked (documented limitation): before=%v after=%v", before.ModTime(), after.ModTime())
	}
}

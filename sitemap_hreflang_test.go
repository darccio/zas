package zas

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// End-to-end tests driving Generator.Run against testdata/sitemap-i18n-site
// (Stage 2b): root pages plus es/ and ca/ language subdirectories, each
// declaring "language" in its own .zas.yml, matching the i18n convention
// documented in README.md's "你会说普通话?" section.
//
// These assert against sitemap.xml's raw text rather than round-tripping
// it through xml.Unmarshal into sitemapURLSet: Go's XML *decoder* treats
// an "xmlns:xhtml" attribute as a namespace declaration and resolves
// "<xhtml:link>" into {Space: "http://...", Local: "link"} accordingly -
// even though Marshal, deliberately, never performs that resolution on
// the way out (see the doc comment on sitemapXhtmlLink). That asymmetry
// means a struct tagged "xhtml:link" can marshal correctly and still
// silently decode back as zero links; regexp on the actual bytes verifies
// what's really on disk instead.

var (
	sitemapURLBlockRe     = regexp.MustCompile(`(?s)<url>.*?</url>`)
	sitemapHreflangLinkRe = regexp.MustCompile(`<xhtml:link rel="alternate" hreflang="([^"]*)" href="([^"]*)"></xhtml:link>`)
)

type hreflangLink struct {
	hreflang, href string
}

func urlBlockFor(t *testing.T, xmlText, loc string) string {
	t.Helper()
	for _, block := range sitemapURLBlockRe.FindAllString(xmlText, -1) {
		if strings.Contains(block, "<loc>"+loc+"</loc>") {
			return block
		}
	}
	t.Fatalf("sitemap has no <url> block for loc %q in %q", loc, xmlText)
	return ""
}

func hreflangLinksIn(block string) []hreflangLink {
	matches := sitemapHreflangLinkRe.FindAllStringSubmatch(block, -1)
	links := make([]hreflangLink, 0, len(matches))
	for _, m := range matches {
		links = append(links, hreflangLink{hreflang: m[1], href: m[2]})
	}
	return links
}

func TestGenerateSitemapHreflangReciprocalCluster(t *testing.T) {
	newTestSite(t, "sitemap-i18n-site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	out := readDeploy(t, "sitemap.xml")
	if !strings.Contains(out, `xmlns:xhtml="`+sitemapXhtmlNS+`"`) {
		t.Fatalf("sitemap.xml = %q, want xmlns:xhtml declared on <urlset>", out)
	}

	block := urlBlockFor(t, out, "https://example.com/index.html")
	links := hreflangLinksIn(block)
	wantLinks := map[string]string{
		"en": "https://example.com/index.html",
		"es": "https://example.com/es/index.html",
		"ca": "https://example.com/ca/index.html",
	}
	if len(links) != len(wantLinks) {
		t.Fatalf("root index.html has %d xhtml:link entries, want %d: %+v", len(links), len(wantLinks), links)
	}
	foundSelf := false
	for _, link := range links {
		want, ok := wantLinks[link.hreflang]
		if !ok || want != link.href {
			t.Errorf("unexpected/wrong hreflang link: %+v, want one of %v", link, wantLinks)
		}
		if link.hreflang == "en" && link.href == "https://example.com/index.html" {
			foundSelf = true
		}
	}
	if !foundSelf {
		t.Fatalf("root index.html's hreflang links = %+v, want a self-referencing en entry", links)
	}
}

func TestGenerateSitemapHreflangFAQClusterExcludesMissingTranslation(t *testing.T) {
	newTestSite(t, "sitemap-i18n-site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	out := readDeploy(t, "sitemap.xml")
	block := urlBlockFor(t, out, "https://example.com/faq.html")
	links := hreflangLinksIn(block)
	if len(links) != 2 {
		t.Fatalf("faq.html has %d hreflang links, want 2 (en+es only, ca/faq.md doesn't exist): %+v", len(links), links)
	}
	for _, link := range links {
		if link.hreflang == "ca" {
			t.Fatalf("faq.html unexpectedly has a ca hreflang link: %+v", links)
		}
	}
}

func TestGenerateSitemapHreflangNestedDirectory(t *testing.T) {
	newTestSite(t, "sitemap-i18n-site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	out := readDeploy(t, "sitemap.xml")
	block := urlBlockFor(t, out, "https://example.com/blog/post.html")
	links := hreflangLinksIn(block)
	if len(links) != 2 {
		t.Fatalf("blog/post.html has %d hreflang links, want 2 (en+es): %+v", len(links), links)
	}
	var esHref string
	for _, link := range links {
		if link.hreflang == "es" {
			esHref = link.href
		}
	}
	if want := "https://example.com/es/blog/post.html"; esHref != want {
		t.Fatalf("es hreflang href = %q, want %q (nested dir within a language must keep its own structure)", esHref, want)
	}
}

func TestGenerateSitemapHreflangSingletonIsNotAnError(t *testing.T) {
	newTestSite(t, "sitemap-i18n-site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	out := readDeploy(t, "sitemap.xml")
	block := urlBlockFor(t, out, "https://example.com/ca/about.html")
	if links := hreflangLinksIn(block); len(links) != 0 {
		t.Fatalf("ca/about.html (no translation elsewhere) has hreflang links %+v, want none", links)
	}
}

func TestGenerateSitemapNoHreflangWhenSingleLanguage(t *testing.T) {
	newTestSite(t, "sitemap-site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	out := readDeploy(t, "sitemap.xml")
	if strings.Contains(out, "xmlns:xhtml") {
		t.Fatalf("sitemap.xml = %q, want no xmlns:xhtml for a site with no translation clusters", out)
	}
	if strings.Contains(out, "<xhtml:link") {
		t.Fatalf("sitemap.xml = %q, want no xhtml:link entries for a site with no translation clusters", out)
	}
}

func TestGenerateSitemapHreflangDuplicateLanguageFails(t *testing.T) {
	newTestSite(t, "sitemap-i18n-site")
	// dup/ declares the same "es" language already used by es/index.html,
	// and its index.md's clusterKey ("index.html", once "dup/" is
	// stripped) collides with the existing root/es/ca index.html cluster -
	// two members of one cluster can't both claim "es".
	if err := os.MkdirAll("dup", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("dup/.zas.yml", []byte("language: es\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("dup/index.md", []byte("# Duplicate\n\nDuplicate ES page.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := generate(t)
	if err == nil {
		t.Fatal("generate() error = nil, want an error for two cluster members resolving to the same language")
	}
	if !strings.Contains(err.Error(), "same language") {
		t.Fatalf("generate() error = %v, want it to mention the duplicate-language conflict", err)
	}
	assertDeployMissing(t, "sitemap.xml")
}

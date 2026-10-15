package zas

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// These tests query the public template-data API after a real build, so
// they exercise the same immutable snapshot that concurrent templates see.
// All site helpers change the working directory; these tests are not parallel.
func newPageListingsTestSite(t *testing.T, siteOptions string, files map[string]string) *Generator {
	t.Helper()
	t.Chdir(t.TempDir())
	writeMarkdownTestFile(t, ConfigFile, "zas:\n  layout: .zas/layout.html\n  deploy: .zas/deploy\nsite:\n  baseurl: https://example.test/base/\n  language: en\n  page_listings: true\n"+siteOptions)
	writeMarkdownTestFile(t, filepath.Join(Dir, "layout.html"), "{{.Body}}")
	for name, source := range files {
		writeMarkdownTestFile(t, filepath.FromSlash(name), source)
	}
	return &Generator{NoPlugins: true}
}

func pageListingsDate(t *testing.T, value string) time.Time {
	t.Helper()
	date, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return date
}

func assertPageListings(t *testing.T, got, want []PageSummary) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("Pages() = %#v, want %#v", got, want)
	}
	for i := range want {
		g, w := got[i], want[i]
		if !g.Date.Equal(w.Date) || g.Date.Format(time.RFC3339Nano) != w.Date.Format(time.RFC3339Nano) {
			t.Errorf("Pages()[%d].Date = %v, want %v preserving its offset", i, g.Date, w.Date)
		}
		g.Date, w.Date = time.Time{}, time.Time{}
		if g != w {
			t.Errorf("Pages()[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestPageListingsMetadataSectionsAndOrdering(t *testing.T) {
	gen := newPageListingsTestSite(t, "", map[string]string{
		"index.md":                       "# Home\n",
		"posts/index.md":                 "# Posts section\n",
		"posts/.zas.yml":                 "language: es\n",
		"posts/a.md":                     "<!--\ndate: 2025-02-03T12:00:00Z\nlanguage: fr\nsummary: First summary.\n-->\n# Markdown *title* &amp; text\n",
		"posts/b.html":                   "<!DOCTYPE html>\n<!--\ntitle: Configured title\ndate: 2025-02-03T13:00:00+01:00\n-->\n<h1>Ignored HTML heading</h1>",
		"posts/old.md":                   "<!-- date: 2024-01-01T00:00:00Z -->\n# Older\n",
		"posts/space & café/index.md":    "<!-- date: 2026-01-01T00:00:00Z -->\n# Escaped path\n",
		"posts/nested/.zas.yml":          "language: ca\n",
		"posts/nested/index.md":          "Nested **heading**\n==================\n",
		"posts/undated.html":             "<h1>HTML &amp; <strong>heading</strong></h1>",
		"posts/private.md":               "<!--\npublish: false\ndate: invalid\nsummary: [ignored]\n-->\n# Private\n",
		"posts/draft.md":                 "<!--\ndraft: true\ndate: invalid\nsummary: [ignored]\n-->\n# Draft\n",
		"posts-archive/outside.md":       "# Outside the section\n",
		"posts/nested/.hidden/secret.md": "# Hidden\n",
		"posts/asset.json":               `{"not":"a page"}`,
	})
	if err := gen.Run(); err != nil {
		t.Fatal(err)
	}
	data := NewZasData("index.md", gen)
	got, err := data.Pages("posts")
	if err != nil {
		t.Fatal(err)
	}
	want := []PageSummary{
		{Title: "Escaped path", URL: "/base/posts/space%20&%20caf%C3%A9/", Canonical: "https://example.test/base/posts/space%20&%20caf%C3%A9/", Date: pageListingsDate(t, "2026-01-01T00:00:00Z"), Language: "es"},
		{Title: "Markdown title & text", URL: "/base/posts/a.html", Canonical: "https://example.test/base/posts/a.html", Date: pageListingsDate(t, "2025-02-03T12:00:00Z"), Language: "fr", Summary: "First summary."},
		{Title: "Configured title", URL: "/base/posts/b.html", Canonical: "https://example.test/base/posts/b.html", Date: pageListingsDate(t, "2025-02-03T13:00:00+01:00"), Language: "es"},
		{Title: "Older", URL: "/base/posts/old.html", Canonical: "https://example.test/base/posts/old.html", Date: pageListingsDate(t, "2024-01-01T00:00:00Z"), Language: "es"},
		{Title: "Nested heading", URL: "/base/posts/nested/", Canonical: "https://example.test/base/posts/nested/", Language: "ca"},
		{Title: "HTML & heading", URL: "/base/posts/undated.html", Canonical: "https://example.test/base/posts/undated.html", Language: "es"},
	}
	assertPageListings(t, got, want)
	trailingSlash, err := data.Pages("posts/")
	if err != nil {
		t.Fatal(err)
	}
	assertPageListings(t, trailingSlash, want)
	if nested, err := data.Pages("posts/nested"); err != nil || len(nested) != 0 {
		t.Fatalf("Pages(posts/nested) = (%#v, %v), want its own index excluded", nested, err)
	}
	for _, section := range []string{"", "."} {
		root, err := data.Pages(section)
		if err != nil {
			t.Fatal(err)
		}
		urls := make(map[string]bool)
		for _, page := range root {
			urls[page.URL] = true
		}
		if len(root) != 8 || !urls["/base/posts/"] || !urls["/base/posts-archive/outside.html"] || urls["/base/"] {
			t.Fatalf("Pages(%q) = %#v, want root index excluded and nested section index retained", section, root)
		}
	}
}

func TestPageListingsRejectInvalidSections(t *testing.T) {
	gen := newPageListingsTestSite(t, "", map[string]string{"posts/a.md": "# Article\n"})
	if err := gen.Run(); err != nil {
		t.Fatal(err)
	}
	data := NewZasData("posts/a.md", gen)
	for _, section := range []string{"/posts", "../posts", "posts/../private", "posts/../../private", `posts\nested`, "posts?sort=date", "posts#part", "%2e%2e", "posts/%2fsecret", "C:/posts", "https://example.test/posts"} {
		t.Run(section, func(t *testing.T) {
			if pages, err := data.Pages(section); err == nil || len(pages) != 0 {
				t.Fatalf("Pages(%q) = (%#v, %v), want an invalid-section diagnostic", section, pages, err)
			}
		})
	}
	if pages, err := data.Pages("missing"); err != nil || len(pages) != 0 {
		t.Fatalf("Pages(missing) = (%#v, %v), want an empty result", pages, err)
	}
}

func TestPageListingsConcurrentCallsReturnIndependentSnapshots(t *testing.T) {
	gen := newPageListingsTestSite(t, "", map[string]string{
		"posts/a.md": "<!--\nsummary: Original summary\ndate: 2025-01-01T00:00:00Z\n-->\n# Original\n",
		"posts/b.md": "# Second\n",
	})
	if err := gen.Run(); err != nil {
		t.Fatal(err)
	}
	data := NewZasData("index.html", gen)
	want := PageSummary{Title: "Original", URL: "/base/posts/a.html", Canonical: "https://example.test/base/posts/a.html", Date: pageListingsDate(t, "2025-01-01T00:00:00Z"), Language: "en", Summary: "Original summary"}
	// Changing the filesystem after the build must not alter what templates
	// can observe during that build's rendering pass.
	writeMarkdownTestFile(t, "posts/a.md", "# Changed on disk\n")
	var wg sync.WaitGroup
	errs := make(chan error, 24)
	for range 24 {
		wg.Go(func() {
			for range 20 {
				pages, err := data.Pages("posts")
				if err != nil {
					errs <- fmt.Errorf("Pages() failed: %w", err)
					return
				}
				if len(pages) != 2 {
					errs <- fmt.Errorf("Pages() = %#v, want two original pages", pages)
					return
				}
				if pages[0] != want || pages[1].Title != "Second" {
					errs <- fmt.Errorf("snapshot changed: %#v", pages)
					return
				}
				pages[0] = PageSummary{Title: "Mutated by caller"}
				pages[1].Summary = "Mutated by caller"
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	pages, err := data.Pages("posts")
	if err != nil || len(pages) != 2 || pages[0] != want || pages[1].Summary != "" {
		t.Fatalf("caller mutation escaped into snapshot: (%#v, %v)", pages, err)
	}
}

func TestPageListingsNavigationPreservesBasePath(t *testing.T) {
	for _, tc := range []struct {
		name, base, navigationPrefix, canonicalPrefix string
	}{
		// NewConfig fills an empty baseurl from the existing site defaults.
		{"empty uses default origin", "", "", "http://example.com"},
		{"root slash", "/", "", ""},
		{"origin root", "https://example.test/", "", "https://example.test"},
		{"mounted", "https://example.test/base/", "/base", "https://example.test/base"},
		{"escaped mount", "https://example.test/my%20site/", "/my%20site", "https://example.test/my%20site"},
		{"literal space mount", "https://example.test/my site/", "/my%20site", "https://example.test/my site"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gen := newPageListingsTestSite(t, "", map[string]string{
				"posts/index.md":            "# Section\n",
				"posts/100% complete.md":    "# Percent filename\n",
				"posts/space café/index.md": "# Article index\n",
			})
			config, err := os.ReadFile(ConfigFile)
			if err != nil {
				t.Fatal(err)
			}
			writeMarkdownTestFile(t, ConfigFile, strings.Replace(string(config), "baseurl: https://example.test/base/", "baseurl: '"+tc.base+"'", 1))
			if err := gen.Run(); err != nil {
				t.Fatal(err)
			}
			data := NewZasData("index.html", gen)
			pages, err := data.Pages("posts")
			if err != nil {
				t.Fatal(err)
			}
			assertPageListings(t, pages, []PageSummary{
				{Title: "Percent filename", URL: tc.navigationPrefix + "/posts/100%25%20complete.html", Canonical: tc.canonicalPrefix + "/posts/100%25%20complete.html", Language: "en"},
				{Title: "Article index", URL: tc.navigationPrefix + "/posts/space%20caf%C3%A9/", Canonical: tc.canonicalPrefix + "/posts/space%20caf%C3%A9/", Language: "en"},
			})
		})
	}
}

func TestPageListingsDisabledReturnsUsefulError(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(fmt.Sprintf("explicit-false-%t", configured), func(t *testing.T) {
			newTestSite(t, "site")
			if configured {
				config, err := os.ReadFile(ConfigFile)
				if err != nil {
					t.Fatal(err)
				}
				writeMarkdownTestFile(t, ConfigFile, strings.Replace(string(config), "site:\n", "site:\n  page_listings: false\n", 1))
			}
			gen := &Generator{NoPlugins: true}
			if err := gen.Run(); err != nil {
				t.Fatal(err)
			}
			data := NewZasData("index.html", gen)
			if pages, err := data.Pages("posts"); err == nil || len(pages) != 0 || !strings.Contains(err.Error(), "page_listings") {
				t.Fatalf("disabled Pages() = (%#v, %v), want an error naming page_listings", pages, err)
			}
		})
	}
}

func TestPageListingsMetadataErrorsPreserveDeploy(t *testing.T) {
	for _, tc := range []struct {
		name, config, field string
	}{
		{"title number", "title: 3", "title"},
		{"title list", "title: [wrong]", "title"},
		{"date day", "date: 2025-01-01", "date"},
		{"date timezone", "date: 2025-01-01T12:00:00", "date"},
		{"date number", "date: 3", "date"},
		{"date null", "date: null", "date"},
		{"date alias", "original: &original 2025-01-01\ndate: *original", "date"},
		{"summary list", "summary: [wrong]", "summary"},
		{"language number", "language: 3", "language"},
		{"publish string", "publish: 'false'", "publish"},
		{"draft string", "draft: 'true'", "draft"},
		{"dynamic title", `title: '{{.Extra "/site/heading"}}'`, "title"},
		{"dynamic summary", `summary: '{{.Extra "/site/heading"}}'`, "summary"},
		{"dynamic date", `date: '{{.Extra "/site/date"}}'`, "date"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gen := newPageListingsTestSite(t, "", map[string]string{
				"posts/bad.md": "<!--\n" + tc.config + "\n-->\n# Invalid metadata\n",
			})
			writeMarkdownTestFile(t, filepath.Join(Dir, "deploy", "marker.txt"), "KEEP\n")
			gen.Full = true
			err := gen.Run()
			if err == nil || !strings.Contains(err.Error(), "posts/bad.md") || !strings.Contains(strings.ToLower(err.Error()), tc.field) {
				t.Fatalf("Run() error = %v, want source path and %q diagnostic", err, tc.field)
			}
			if got := readDeploy(t, "marker.txt"); got != "KEEP\n" {
				t.Fatalf("metadata preflight changed existing output: %q", got)
			}
		})
	}
}

func TestPageListingsReusedGeneratorRefreshesAndDisablesSnapshot(t *testing.T) {
	gen := newPageListingsTestSite(t, "", map[string]string{"posts/a.md": "# First\n"})
	if err := gen.Run(); err != nil {
		t.Fatal(err)
	}
	writeMarkdownTestFile(t, "posts/a.md", "<!--\nsummary: Changed summary\ndate: 2025-01-02T00:00:00Z\nlanguage: ca\n-->\n# Changed\n")
	if err := gen.Run(); err != nil {
		t.Fatalf("second Run() reused stale build state: %v", err)
	}
	data := NewZasData("index.html", gen)
	pages, err := data.Pages("posts")
	if err != nil {
		t.Fatal(err)
	}
	assertPageListings(t, pages, []PageSummary{{Title: "Changed", URL: "/base/posts/a.html", Canonical: "https://example.test/base/posts/a.html", Date: pageListingsDate(t, "2025-01-02T00:00:00Z"), Language: "ca", Summary: "Changed summary"}})
	config, err := os.ReadFile(ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	writeMarkdownTestFile(t, ConfigFile, strings.Replace(string(config), "page_listings: true", "page_listings: false", 1))
	if err := gen.Run(); err != nil {
		t.Fatal(err)
	}
	disabled := NewZasData("index.html", gen)
	if pages, err := disabled.Pages("posts"); err == nil || len(pages) != 0 {
		t.Fatalf("reused disabled generator retained snapshot: (%#v, %v)", pages, err)
	}
}

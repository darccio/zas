package zas

import (
	"encoding/xml"
	"os"
	"strings"
	"testing"
)

// End-to-end tests driving Generator.Run against testdata/sitemap-site,
// covering Stage 1 (loc/toggle) and the sharding logic from Stage 3.

func TestGenerateSitemapOffByDefault(t *testing.T) {
	newTestSite(t, "site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	assertDeployMissing(t, "sitemap.xml")
}

func TestGenerateSitemapWritesLocPerPage(t *testing.T) {
	newTestSite(t, "sitemap-site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	assertDeployHas(t, "sitemap.xml")
	var urlset sitemapURLSet
	if err := xml.Unmarshal([]byte(readDeploy(t, "sitemap.xml")), &urlset); err != nil {
		t.Fatalf("unmarshal sitemap.xml: %v", err)
	}
	if urlset.Xmlns != sitemapNS {
		t.Fatalf("urlset xmlns = %q, want %q", urlset.Xmlns, sitemapNS)
	}
	got := make(map[string]bool, len(urlset.URLs))
	for _, u := range urlset.URLs {
		got[u.Loc] = true
	}
	want := []string{
		"https://example.com/index.html",
		"https://example.com/about.html",
	}
	for _, loc := range want {
		if !got[loc] {
			t.Errorf("sitemap.xml missing <loc>%s</loc>; got %v", loc, got)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("sitemap.xml has %d URLs, want %d (got %v) - hidden.md (publish: false) must not appear, and sitemap.xml must never list itself", len(got), len(want), got)
	}
}

func TestGenerateSitemapExcludesUnpublishedPage(t *testing.T) {
	newTestSite(t, "sitemap-site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	out := readDeploy(t, "sitemap.xml")
	if strings.Contains(out, "hidden.html") {
		t.Fatalf("sitemap.xml = %q, must not reference hidden.html (publish: false)", out)
	}
}

func TestGenerateSitemapOmittedWhenNoPages(t *testing.T) {
	newTestSite(t, "sitemap-site")
	for _, f := range []string{"index.html", "about.md", "hidden.md"} {
		if err := os.Remove(f); err != nil {
			t.Fatal(err)
		}
	}
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	assertDeployMissing(t, "sitemap.xml")
}

func TestGenerateSitemapNoChangefreqOrPriority(t *testing.T) {
	newTestSite(t, "sitemap-site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	out := readDeploy(t, "sitemap.xml")
	for _, tag := range []string{"changefreq", "priority"} {
		if strings.Contains(out, tag) {
			t.Fatalf("sitemap.xml = %q, must not contain %q", out, tag)
		}
	}
}

// Sharding (Stage 3): shardSitemapEntries is a pure function, tested
// directly with synthetic entries - no filesystem needed.

func TestShardSitemapEntriesByCount(t *testing.T) {
	restore := overrideSitemapLimits(t, 2, 1<<30)
	defer restore()
	entries := []*sitemapEntry{{loc: "a"}, {loc: "b"}, {loc: "c"}, {loc: "d"}, {loc: "e"}}
	shards := shardSitemapEntries(entries)
	if len(shards) != 3 {
		t.Fatalf("shardSitemapEntries() = %d shards, want 3", len(shards))
	}
	if len(shards[0]) != 2 || len(shards[1]) != 2 || len(shards[2]) != 1 {
		t.Fatalf("shard sizes = %d,%d,%d, want 2,2,1", len(shards[0]), len(shards[1]), len(shards[2]))
	}
}

func TestShardSitemapEntriesByBytes(t *testing.T) {
	restore := overrideSitemapLimits(t, 1000, 1)
	defer restore()
	entries := []*sitemapEntry{{loc: "a"}, {loc: "b"}, {loc: "c"}}
	shards := shardSitemapEntries(entries)
	if len(shards) != 3 {
		t.Fatalf("shardSitemapEntries() = %d shards, want 3 (byte cap forces one entry per shard)", len(shards))
	}
}

func TestShardSitemapEntriesEmpty(t *testing.T) {
	if shards := shardSitemapEntries(nil); shards != nil {
		t.Fatalf("shardSitemapEntries(nil) = %v, want nil", shards)
	}
}

func TestGenerateSitemapShardsPastThreshold(t *testing.T) {
	restore := overrideSitemapLimits(t, 1, 1<<30)
	defer restore()
	newTestSite(t, "sitemap-site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	assertDeployMissing(t, "sitemap.xml")
	assertDeployHas(t, "sitemap-index.xml")
	assertDeployHas(t, "sitemap-1.xml")
	assertDeployHas(t, "sitemap-2.xml")

	var index sitemapIndex
	if err := xml.Unmarshal([]byte(readDeploy(t, "sitemap-index.xml")), &index); err != nil {
		t.Fatalf("unmarshal sitemap-index.xml: %v", err)
	}
	if len(index.Sitemaps) != 2 {
		t.Fatalf("sitemap-index.xml has %d <sitemap> entries, want 2", len(index.Sitemaps))
	}

	robots := readDeploy(t, "robots.txt")
	if !strings.Contains(robots, "Sitemap: https://example.com/sitemap-index.xml") {
		t.Fatalf("robots.txt = %q, want it to point at the sitemap index, not a shard", robots)
	}
}

// TestGenerateSitemapCoversUnchangedPagesOnIncrementalRun proves the
// sitemap pass walks the finished deploy tree (like reaper) rather than
// reusing claimedOutputs, which - per walk's own doc comments - only ever
// holds the sources actually re-rendered during a given run. A second,
// incremental run that only touches about.md must still produce a
// <lastmod> for index.html, which this run never re-rendered at all.
func TestGenerateSitemapCoversUnchangedPagesOnIncrementalRun(t *testing.T) {
	newTestSite(t, "sitemap-site")
	if err := generate(t); err != nil {
		t.Fatalf("first generate() error = %v, want nil", err)
	}
	touchFuture(t, "about.md")
	if err := generate(t); err != nil {
		t.Fatalf("second generate() error = %v, want nil", err)
	}
	out := readDeploy(t, "sitemap.xml")
	for _, loc := range []string{"https://example.com/index.html", "https://example.com/about.html"} {
		if !strings.Contains(out, "<loc>"+loc+"</loc>") {
			t.Fatalf("sitemap.xml = %q, want it to still list %s even though this run didn't re-render it", out, loc)
		}
	}
	var urlset sitemapURLSet
	if err := xml.Unmarshal([]byte(out), &urlset); err != nil {
		t.Fatalf("unmarshal sitemap.xml: %v", err)
	}
	for _, u := range urlset.URLs {
		if u.Lastmod == "" {
			t.Errorf("%s has no <lastmod>, want one even for a page unchanged this run", u.Loc)
		}
	}
}

// overrideSitemapLimits temporarily lowers the package-level sharding
// thresholds so a test can exercise sharding without writing tens of
// thousands of real files to disk, restoring the originals via the
// returned func (defer'd) once the test finishes.
func overrideSitemapLimits(t *testing.T, maxURLs, maxBytes int) func() {
	t.Helper()
	origURLs, origBytes := sitemapMaxURLsPerFile, sitemapMaxBytesPerFile
	sitemapMaxURLsPerFile, sitemapMaxBytesPerFile = maxURLs, maxBytes
	return func() {
		sitemapMaxURLsPerFile, sitemapMaxBytesPerFile = origURLs, origBytes
	}
}

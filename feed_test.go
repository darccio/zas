package zas

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"html"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type decodedFeed struct {
	XMLName xml.Name
	Version string `xml:"version,attr"`
	Channel struct {
		Title         string `xml:"title"`
		Description   string `xml:"description"`
		Language      string `xml:"language"`
		LastBuildDate string `xml:"lastBuildDate"`
		Self          struct {
			Href string `xml:"href,attr"`
			Rel  string `xml:"rel,attr"`
			Type string `xml:"type,attr"`
		} `xml:"http://www.w3.org/2005/Atom link"`
		Link  string `xml:"link"`
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			PubDate     string `xml:"pubDate"`
			Description string `xml:"description"`
			GUID        struct {
				Permalink string `xml:"isPermaLink,attr"`
				Value     string `xml:",chardata"`
			} `xml:"guid"`
			Content    string   `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
			Creator    string   `xml:"http://purl.org/dc/elements/1.1/ creator"`
			Language   string   `xml:"http://purl.org/dc/elements/1.1/ language"`
			XMLLang    string   `xml:"http://www.w3.org/XML/1998/namespace lang,attr"`
			Categories []string `xml:"category"`
		} `xml:"item"`
	} `xml:"channel"`
}

func decodeFeed(t *testing.T, contents string) decodedFeed {
	t.Helper()
	var result decodedFeed
	if err := xml.Unmarshal([]byte(contents), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func writeFeedSource(t *testing.T, name, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func replaceFeedConfig(t *testing.T, old, replacement string) {
	t.Helper()
	contents, err := os.ReadFile(ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), old) {
		t.Fatalf("configuration does not contain %q", old)
	}
	writeFeedSource(t, ConfigFile, strings.Replace(string(contents), old, replacement, 1))
}

func TestFeedFullIncrementalContent(t *testing.T) {
	newTestSite(t, "feed-site")
	ageSources(t, -time.Hour)
	if err := generate(t, fullGen, noPluginsGen); err != nil {
		t.Fatal(err)
	}
	original := readDeploy(t, "feed.xml")
	en := readDeploy(t, "en/feed.xml")
	es := readDeploy(t, "es/feed.xml")
	feed := decodeFeed(t, original)
	if feed.XMLName.Space != "" || feed.XMLName.Local != "rss" || feed.Version != "2.0" {
		t.Fatalf("RSS root: %#v", feed)
	}
	if feed.Channel.Title != "Feed & Site" || feed.Channel.Description != "Channel <plain> & description" || feed.Channel.Link != "https://example.test/" {
		t.Fatalf("channel: %#v", feed.Channel)
	}
	if feed.Channel.Self.Href != "https://example.test/feed.xml" || feed.Channel.Self.Rel != "self" || feed.Channel.Self.Type != "application/rss+xml" {
		t.Fatalf("atom self: %#v", feed.Channel.Self)
	}
	if len(feed.Channel.Items) != 2 || feed.Channel.LastBuildDate != "Wed, 05 Feb 2025 12:30:00 +0000" {
		t.Fatalf("combined limit/date: %#v", feed.Channel)
	}
	item := feed.Channel.Items[0]
	if item.Title != "Newest & <title>" || item.Link != "https://example.test/posts/newest/" || item.GUID.Value != item.Link || item.GUID.Permalink != "true" || item.PubDate != "Fri, 03 Jan 2025 10:00:00 +0100" {
		t.Fatalf("item identity/date: %#v", item)
	}
	if item.Creator != "A & B" || item.Language != "en" || item.XMLLang != "en" || strings.Join(item.Categories, "|") != "writing|<tag>" {
		t.Fatalf("namespaced metadata: %#v", item)
	}
	if got := mustParseDoc(t, item.Description).Find("body").Text(); got != "Literal <target>, & and ©." {
		t.Fatalf("reader summary text = %q", got)
	}
	for _, absent := range []string{"Layout navigation", "Layout footer", "updated:", "summary:"} {
		if strings.Contains(item.Content, absent) {
			t.Errorf("feed body includes %q", absent)
		}
	}
	doc := mustParseDoc(t, item.Content)
	for text, want := range map[string]string{
		"Next": "https://example.test/posts/newest/next.html", "Root": "https://example.test/root?q=x#part",
		"Query": "https://example.test/posts/newest/?q=x", "Fragment": "https://example.test/posts/newest/#part",
		"CDN": "https://cdn.test/file", "Email": "mailto:a@example.test",
	} {
		got := ""
		doc.Find("a").Each(func(_ int, selection *goquery.Selection) {
			if selection.Text() == text {
				got, _ = selection.Attr("href")
			}
		})
		if got != want {
			t.Errorf("%s href = %q, want %q", text, got, want)
		}
	}
	if src, _ := doc.Find("img").Attr("src"); src != "https://example.test/posts/newest/image.png" {
		t.Errorf("image src = %q", src)
	}
	if srcset, _ := doc.Find("img").Attr("srcset"); srcset != "https://example.test/posts/newest/small.png 1x, https://example.test/large.png 2x, data:image/png;base64,AAAA 3x" {
		t.Errorf("image srcset = %q", srcset)
	}
	if !strings.Contains(doc.Text(), "complete article") || !strings.Contains(doc.Text(), "CDATA ending ]]> survives.") {
		t.Fatalf("full content not retained: %s", item.Content)
	}
	spanish := decodeFeed(t, es)
	if len(spanish.Channel.Items) != 2 || spanish.Channel.Items[0].Title != "Español" || spanish.Channel.Title != "Feed & Site (Español)" || spanish.Channel.Language != "es" {
		t.Fatalf("independent language limit: %#v", spanish.Channel)
	}
	if decodeFeed(t, en).Channel.Title != "Feed & Site (English)" {
		t.Fatal("English title missing")
	}
	decoder := xml.NewDecoder(strings.NewReader(original))
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if start, ok := token.(xml.StartElement); ok && (start.Name.Local == "item" || start.Name.Local == "title" || start.Name.Local == "channel") && start.Name.Space != "" {
			t.Fatalf("RSS core namespaced: %#v", start.Name)
		}
	}
	assertDeployMissing(t, "feed-outputs.json")
	if err := generate(t, noPluginsGen); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"feed.xml": original, "en/feed.xml": en, "es/feed.xml": es} {
		if got := readDeploy(t, name); got != want {
			t.Errorf("incremental %s changed despite unchanged sources", name)
		}
	}
}

func TestFeedEditsDeletionAndManagedCleanup(t *testing.T) {
	newTestSite(t, "feed-site")
	if err := generate(t, fullGen, noPluginsGen); err != nil {
		t.Fatal(err)
	}
	writeFeedSource(t, "posts/second.md", "<!--\ndate: 2025-01-02T11:00:00Z\nlanguage: en\n-->\n# Second edited\n\nUpdated body.\n")
	if err := generate(t, noPluginsGen); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readDeploy(t, "feed.xml"), "Updated body.") {
		t.Fatal("edited article body was not collected")
	}
	if err := os.RemoveAll("posts/newest"); err != nil {
		t.Fatal(err)
	}
	if err := generate(t, noPluginsGen); err != nil {
		t.Fatal(err)
	}
	if got := decodeFeed(t, readDeploy(t, "feed.xml")).Channel.Items[0].Title; got != "Second edited" {
		t.Fatalf("deleted article retained: %s", got)
	}
	for _, source := range []string{"posts/spanish", "posts/older.md"} {
		if err := os.RemoveAll(source); err != nil {
			t.Fatal(err)
		}
	}
	if err := generate(t, noPluginsGen); err != nil {
		t.Fatal(err)
	}
	assertDeployMissing(t, "es/feed.xml")
	replaceFeedConfig(t, "feed_per_language: true", "feed_per_language: false")
	if err := generate(t, noPluginsGen); err != nil {
		t.Fatal(err)
	}
	assertDeployMissing(t, "en/feed.xml")
	replaceFeedConfig(t, "feed: true", "feed: false")
	if err := generate(t, noPluginsGen); err != nil {
		t.Fatal(err)
	}
	assertDeployMissing(t, "feed.xml")
	if _, err := os.Stat(feedManifestPath); !os.IsNotExist(err) {
		t.Fatalf("ownership manifest remains: %v", err)
	}
}

func TestFeedStrictFailurePreservesFeeds(t *testing.T) {
	for _, metadata := range []string{"", "article: true", "article: 'false'", "date: 2025-01-01", "date: 2025-01-01T12:00:00", "date: 2025-01-01T12:00:00Z\nupdated: invalid", "date: 2025-01-01T12:00:00Z\nsummary: 3", "date: 2025-01-01T12:00:00Z\ntags: [3]", "date: 2025-01-01T12:00:00Z\nlanguage: ../escape"} {
		t.Run(metadata, func(t *testing.T) {
			newTestSite(t, "feed-site")
			if err := generate(t, fullGen, noPluginsGen); err != nil {
				t.Fatal(err)
			}
			original := readDeploy(t, "feed.xml")
			manifest, err := os.ReadFile(feedManifestPath)
			if err != nil {
				t.Fatal(err)
			}
			writeFeedSource(t, "posts/bad/index.md", "<!--\n"+metadata+"\n-->\n# Bad\n")
			replaceFeedConfig(t, "feed_per_language: true", "feed_per_language: false")
			if err := generate(t, noPluginsGen); err == nil || !strings.Contains(err.Error(), "feed:") {
				t.Fatalf("strict invalid article: %v", err)
			}
			if readDeploy(t, "feed.xml") != original {
				t.Fatal("strict failure replaced feed")
			}
			assertDeployHas(t, "es/feed.xml")
			if got, err := os.ReadFile(feedManifestPath); err != nil || !bytes.Equal(got, manifest) {
				t.Fatalf("strict failure changed ownership: %v", err)
			}
			replaceFeedConfig(t, "feed: true", "feed: true\n  feed_strict: false")
			if err := generate(t, noPluginsGen); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(readDeploy(t, "feed.xml"), "<title>Bad</title>") {
				t.Fatal("lenient feed retained invalid article")
			}
		})
	}
}

func TestFeedCollisionPrecedesFullCleanup(t *testing.T) {
	for _, collision := range []string{"source", "language-source-ancestor", "deploy", "language-deploy", "modified-owned", "symlink-deploy-parent"} {
		t.Run(collision, func(t *testing.T) {
			newTestSite(t, "feed-site")
			writeFeedSource(t, ".zas/deploy/sentinel.txt", "keep")
			switch collision {
			case "source":
				writeFeedSource(t, "feed.xml", "user feed")
			case "language-source-ancestor":
				writeFeedSource(t, "en", "user file")
			case "deploy":
				writeFeedSource(t, ".zas/deploy/feed.xml", "user feed")
			case "language-deploy":
				writeFeedSource(t, ".zas/deploy/fr/feed.xml", "user feed")
			case "modified-owned":
				if err := generate(t, noPluginsGen); err != nil {
					t.Fatal(err)
				}
				writeFeedSource(t, ".zas/deploy/sentinel.txt", "keep")
				writeFeedSource(t, ".zas/deploy/feed.xml", "edited feed")
			case "symlink-deploy-parent":
				outside := t.TempDir()
				writeFeedSource(t, filepath.Join(outside, "sentinel.txt"), "keep")
				if err := os.Symlink(outside, "linked-output"); err != nil {
					t.Skip(err)
				}
				replaceFeedConfig(t, "deploy: .zas/deploy", "deploy: linked-output/deploy")
			}
			if err := generate(t, fullGen, noPluginsGen); err == nil {
				t.Fatal("collision accepted")
			}
			if got := readDeploy(t, "sentinel.txt"); got != "keep" {
				t.Fatalf("full cleanup ran before collision: %q", got)
			}
		})
	}
}

func TestFeedDisabledLeavesExistingOutputUnchanged(t *testing.T) {
	newTestSite(t, "site")
	ageSources(t, -time.Hour)
	if err := generate(t, fullGen, noPluginsGen); err != nil {
		t.Fatal(err)
	}
	before := snapshotFeedDeploy(t)
	// Disabled options are deliberately inert, even if they are invalid.
	config := string(mustReadFeedFile(t, ConfigFile))
	config = strings.Replace(config, "site:", "site:\n  feed: false\n  feed_limit: invalid\n  feed_per_language: invalid", 1)
	writeFeedSource(t, ConfigFile, config)
	if err := generate(t, fullGen, noPluginsGen); err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(snapshotFeedDeploy(t), before) {
		t.Fatal("disabled feed changed existing output bytes")
	}
	assertDeployMissing(t, "feed.xml")
	if _, err := os.Stat(feedManifestPath); !os.IsNotExist(err) {
		t.Fatal("disabled feed created ownership manifest")
	}
}

func snapshotFeedDeploy(t *testing.T) map[string]string {
	t.Helper()
	outputs := make(map[string]string)
	if err := filepath.Walk(".zas/deploy", func(name string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			outputs[name] = feedHash(mustReadFeedFile(t, name))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return outputs
}

func mustReadFeedFile(t *testing.T, name string) []byte {
	t.Helper()
	contents, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

func TestFeedPagePluginExecutesOncePerRun(t *testing.T) {
	newTestSite(t, "feed-site")
	installStub(t, "zscountfeed", "#!/bin/sh\nprintf x >> .zas/feed-count\nprintf '<p>Plugin content</p>'\n")
	writeFeedSource(t, "posts/plugin.html", "<!--\narticle: true\ndate: 2026-01-01T00:00:00Z\n-->\n<h1>Plugin {{.Title}}</h1><script type=\"application/zas+countfeed\"></script>")
	for run := 1; run <= 2; run++ {
		if err := generate(t); err != nil {
			t.Fatal(err)
		}
		if got := string(mustReadFeedFile(t, ".zas/feed-count")); got != strings.Repeat("x", run) {
			t.Fatalf("run %d invoked plugin more than once: %q", run, got)
		}
		if !strings.Contains(decodeFeed(t, readDeploy(t, "feed.xml")).Channel.Items[0].Content, "Plugin content") {
			t.Fatal("plugin body absent from feed")
		}
	}
}

func TestFeedRejectsResidualPluginDirectives(t *testing.T) {
	newTestSite(t, "feed-site")
	installStub(t, "zsfeedfirst", "#!/bin/sh\nprintf x >> .zas/first-count\nprintf '<script type=APPLICATION/ZAS+feedsecond></script>'\n")
	installStub(t, "zsfeedsecond", "#!/bin/sh\nprintf x >> .zas/second-count\nprintf '<p>Second output</p>'\n")
	writeFeedSource(t, "posts/chain.html", "<!--\ndate: 2026-01-01T00:00:00Z\n-->\n<h1>Chain</h1><script type=application/zas+feedfirst></script>")
	if err := generate(t); err == nil || !strings.Contains(err.Error(), "residual Zas script") || !strings.Contains(err.Error(), "feed: false") {
		t.Fatalf("residual directive error: %v", err)
	}
	assertDeployMissing(t, "feed.xml")
	if _, err := os.Stat(".zas/second-count"); !os.IsNotExist(err) {
		t.Fatal("RSS validation executed residual directive")
	}
	replaceFeedConfig(t, "feed: true", "feed: true\n  feed_strict: false")
	if err := generate(t); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readDeploy(t, "feed.xml"), "<title>Chain</title>") || !strings.Contains(readDeploy(t, "posts/chain.html"), "Second output") || string(mustReadFeedFile(t, ".zas/second-count")) != "x" {
		t.Fatal("lenient skip changed normal chained page execution")
	}
	for _, body := range []string{`<embed src="later.html">`, `<script type=" Application/Zas+later "></script>`} {
		if _, err := feedHTML(body, "https://example.test/article/"); err == nil {
			t.Fatalf("residual directive accepted: %s", body)
		}
	}
	if _, err := feedHTML(`<pre><code>&lt;embed src="example"&gt;</code></pre><script type="application/ld+json">{}</script>`, "https://example.test/article/"); err != nil {
		t.Fatalf("ordinary script/code rejected: %v", err)
	}
}

func TestFeedCDATAAndDeterministicTie(t *testing.T) {
	date := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	state := feedState{config: feedConfig{title: "Site", baseURL: "https://example.test", language: "en", limit: 50}, entries: []feedEntry{
		{title: "B", canonical: "https://example.test/b", date: date, body: "<script>const x = ']]>';</script>", summary: "Literal <target> & &copy;."},
		{title: "A", canonical: "https://example.test/a", date: date},
	}}
	documents, err := state.documents()
	if err != nil {
		t.Fatal(err)
	}
	feed := decodeFeed(t, string(documents["feed.xml"]))
	if feed.Channel.Items[0].Title != "A" || feed.Channel.Items[1].Content != state.entries[1].body || feed.Channel.LastBuildDate != "" {
		t.Fatalf("CDATA/tie/no synthesized build time: %#v", feed.Channel)
	}
	if feed.Channel.Items[1].Description != html.EscapeString(state.entries[1].summary) || mustParseDoc(t, feed.Channel.Items[1].Description).Find("body").Text() != state.entries[1].summary {
		t.Fatal("resolved summary lost literal HTML/entity text")
	}
	state.entries[1].canonical = state.entries[0].canonical
	if _, err := state.documents(); err == nil {
		t.Fatal("duplicate GUID accepted")
	}
	state.entries[1].canonical = "https://example.test/b"
	state.entries[1].body = "invalid\x01"
	if _, err := state.documents(); err == nil {
		t.Fatal("invalid XML control character accepted")
	}
}

func TestFeedPartialWriteRecovery(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission failures require an unprivileged user")
	}
	newTestSite(t, "feed-site")
	if err := generate(t, noPluginsGen); err != nil {
		t.Fatal(err)
	}
	config, err := NewConfig()
	if err != nil {
		t.Fatal(err)
	}
	gen := &Generator{Config: config}
	if err := gen.prepareFeeds(); err != nil {
		t.Fatal(err)
	}
	gen.feed.entries = []feedEntry{
		{title: "Changed", canonical: "https://example.test/en-article", language: "en", date: time.Now()},
		{title: "Changed", canonical: "https://example.test/es-article", language: "es", date: time.Now()},
	}
	if err := os.Chmod(".zas/deploy/es", 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(".zas/deploy/es", 0o755) })
	if err := gen.finishFeeds(); err == nil {
		t.Fatal("write into readonly language directory succeeded")
	}
	manifest, err := readFeedManifest()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readDeploy(t, "en/feed.xml"), "Changed") || manifest.Files["en/feed.xml"] != feedHash(mustReadFeedFile(t, ".zas/deploy/en/feed.xml")) {
		t.Fatal("completed feed write was not retained in ownership")
	}
	if err := os.Chmod(".zas/deploy/es", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := generate(t, noPluginsGen); err != nil {
		t.Fatalf("retry after partial failure: %v", err)
	}
	// A failed private-manifest write restores the prior feed bytes.
	gen = &Generator{Config: config}
	if err := gen.prepareFeeds(); err != nil {
		t.Fatal(err)
	}
	gen.feed.entries = []feedEntry{{title: "Unowned change", canonical: "https://example.test/a", language: "en", date: time.Now()}}
	before := readDeploy(t, "en/feed.xml")
	if err := os.Chmod(".zas", 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(".zas", 0o755) })
	if err := gen.finishFeeds(); err == nil {
		t.Fatal("write into readonly manifest directory succeeded")
	}
	if readDeploy(t, "en/feed.xml") != before {
		t.Fatal("failed ownership write stranded changed feed bytes")
	}
	if err := os.Chmod(".zas", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := generate(t, noPluginsGen); err != nil {
		t.Fatalf("retry after ownership failure: %v", err)
	}
}

func TestFeedSourceTakeoverAndSkippedSymlink(t *testing.T) {
	for _, symlink := range []bool{false, true} {
		t.Run(strconv.FormatBool(symlink), func(t *testing.T) {
			newTestSite(t, "feed-site")
			if err := generate(t, noPluginsGen); err != nil {
				t.Fatal(err)
			}
			replaceFeedConfig(t, "feed: true", "feed: false")
			if symlink {
				if err := os.Symlink("posts/index.md", "feed.xml"); err != nil {
					t.Skip(err)
				}
			} else {
				writeFeedSource(t, "feed.xml", "user-owned feed")
				older := time.Now().Add(-24 * time.Hour)
				if err := os.Chtimes("feed.xml", older, older); err != nil {
					t.Fatal(err)
				}
			}
			if err := generate(t, noPluginsGen); err != nil {
				t.Fatal(err)
			}
			if symlink {
				assertDeployMissing(t, "feed.xml")
			} else if readDeploy(t, "feed.xml") != "user-owned feed" {
				t.Fatal("copied user feed removed during cleanup")
			}
		})
	}
}

func TestFeedConservativeSelectionWithoutSections(t *testing.T) {
	newTestSite(t, "feed-site")
	replaceFeedConfig(t, "  feed_sections: [posts]\n", "")
	// An unclassified root index remains an archive. In this mode nested
	// dated indexes are articles and undated utility pages remain ordinary.
	writeFeedSource(t, "posts/index.md", "# Archive\n")
	writeFeedSource(t, "posts/utility.md", "# Utility without date\n")
	writeFeedSource(t, "LICENSE", "ordinary asset")
	if err := generate(t, noPluginsGen); err != nil {
		t.Fatal(err)
	}
	assertDeployHas(t, "LICENSE")
	assertDeployHas(t, "posts/utility.html")
	if len(decodeFeed(t, readDeploy(t, "es/feed.xml")).Channel.Items) != 2 {
		t.Fatal("dated nested index was excluded")
	}
	replaceFeedConfig(t, "language: mul", "language: en")
	if err := generate(t, noPluginsGen); err != nil {
		t.Fatal(err)
	}
	assertDeployMissing(t, "en/feed.xml")
}

func TestFeedDynamicLanguageCollisionRetainsCopiedSource(t *testing.T) {
	newTestSite(t, "feed-site")
	writeFeedSource(t, "fr", "user source content")
	writeFeedSource(t, "posts/dynamic.html", "<!--\ndate: 2026-01-01T00:00:00Z\nlanguage: '{{.Extra \"/site/article_language\"}}'\n-->\n<h1>Dynamic language</h1>")
	replaceFeedConfig(t, "language: mul", "language: mul\n  article_language: fr")
	if err := generate(t, fullGen, noPluginsGen); err == nil {
		t.Fatal("dynamic language ancestor collision accepted")
	}
	if readDeploy(t, "fr") != "user source content" {
		t.Fatal("dynamic feed overwrote conflicting copied source")
	}
	assertDeployMissing(t, "feed.xml")
}

func TestFeedRejectsUnsafeManifest(t *testing.T) {
	newTestSite(t, "feed-site")
	writeFeedSource(t, ".zas/deploy/sentinel.txt", "keep")
	deploy, err := filepath.Abs(".zas/deploy")
	if err != nil {
		t.Fatal(err)
	}
	manifest := feedManifest{Version: 1, Deploy: deploy, Files: map[string]string{"../outside": strings.Repeat("0", 64)}}
	contents, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeFeedSource(t, feedManifestPath, string(contents))
	if err := generate(t, fullGen, noPluginsGen); err == nil {
		t.Fatal("unsafe ownership accepted")
	}
	if readDeploy(t, "sentinel.txt") != "keep" {
		t.Fatal("unsafe ownership caused full cleanup")
	}
}

func TestFeedConfigValidation(t *testing.T) {
	for key, value := range map[string]interface{}{"feed": "true", "feed_limit": 0, "feed_per_language": []string{"en"}, "feed_strict": "false", "feed_sections": []interface{}{3}, "feed_title": 1, "feed_description": 1, "baseurl": "//example.test", "language": "../en"} {
		t.Run(key, func(t *testing.T) {
			config := ConfigSection{"site": ConfigSection{"feed": true, "title": "Site", "baseurl": "https://example.test", "language": "en", key: value}}
			if _, err := readFeedConfig(config); err == nil {
				t.Fatalf("invalid %s accepted: %v", key, value)
			}
		})
	}
	for _, section := range []string{"../posts", "/posts", "posts/../other", "posts\\folder", ""} {
		config := ConfigSection{"site": ConfigSection{"feed": true, "title": "Site", "baseurl": "https://example.test", "language": "en", "feed_sections": []string{section}}}
		if _, err := readFeedConfig(config); err == nil {
			t.Fatalf("invalid section accepted: %q", section)
		}
	}
}

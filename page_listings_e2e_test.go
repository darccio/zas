package zas

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func assertPageListingsDeploy(t *testing.T, got, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("deploy file count = %d, want %d", len(got), len(want))
	}
	for path, contents := range want {
		if actual, ok := got[path]; !ok || actual != contents {
			t.Errorf("deploy output differs at %s\nwant: %s\ngot:  %s", path, contents, actual)
		}
	}
	for path := range got {
		if _, ok := want[path]; !ok {
			t.Errorf("unexpected deploy output: %s", path)
		}
	}
}

func TestPageListingsDisabledIsByteIdentical(t *testing.T) {
	newTestSite(t, "site")
	if err := generate(t, fullGen, noPluginsGen); err != nil {
		t.Fatal(err)
	}
	baseline := snapshotMarkdownDeploy(t)
	config, err := os.ReadFile(ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	writeMarkdownTestFile(t, ConfigFile, strings.Replace(string(config), "site:\n", "site:\n  page_listings: false\n", 1))
	if err := generate(t, fullGen, noPluginsGen); err != nil {
		t.Fatal(err)
	}
	assertPageListingsDeploy(t, snapshotMarkdownDeploy(t), baseline)
}

func TestPageListingsAvailableInBodyAndEscapedInLayout(t *testing.T) {
	gen := newPageListingsTestSite(t, "", map[string]string{
		"index.html":     `<h1>Home</h1><p id="body-count">{{len (.Pages "posts")}}</p>`,
		"posts/a & b.md": "<!--\ntitle: '<img src=x onerror=bad> & title'\nsummary: '<script>bad()</script> & summary'\ndate: 2025-01-02T00:00:00Z\n-->\n# Article\n",
	})
	writeMarkdownTestFile(t, filepath.Join(Dir, "layout.html"), `<html><head><title>{{.Title}}</title></head><body>{{.Body}}<ul id="listing">{{range .Pages "posts"}}<li><a href="{{.URL}}" data-canonical="{{.Canonical}}">{{.Title}}</a><p>{{.Summary}}</p><time>{{.Date.Format "2006-01-02"}}</time><i>{{.Language}}</i></li>{{end}}</ul></body></html>`)
	if err := gen.Run(); err != nil {
		t.Fatal(err)
	}
	out := readDeploy(t, "index.html")
	doc := mustParseDoc(t, out)
	if got := doc.Find("#body-count").Text(); got != "1" {
		t.Fatalf("page-body Pages() count = %q, want 1", got)
	}
	if doc.Find("#listing img, #listing script").Length() != 0 {
		t.Fatalf("listing metadata became markup: %s", out)
	}
	if got := doc.Find("#listing a").Text(); got != "<img src=x onerror=bad> & title" {
		t.Fatalf("listing title = %q", got)
	}
	if got := doc.Find("#listing p").Text(); got != "<script>bad()</script> & summary" {
		t.Fatalf("listing summary = %q", got)
	}
	if href, _ := doc.Find("#listing a").Attr("href"); href != "/base/posts/a%20&%20b.html" {
		t.Fatalf("listing href = %q, want root-relative escaped navigation URL", href)
	}
	if canonical, _ := doc.Find("#listing a").Attr("data-canonical"); canonical != "https://example.test/base/posts/a%20&%20b.html" {
		t.Fatalf("listing canonical = %q", canonical)
	}
	if got := doc.Find("#listing time").Text(); got != "2025-01-02" {
		t.Fatalf("listing Date.Format = %q", got)
	}
	if got := doc.Find("#listing i").Text(); got != "en" {
		t.Fatalf("listing language = %q", got)
	}
}

func TestPageListingsUsesStaticSourceWithoutExtraPluginExecution(t *testing.T) {
	gen := newPageListingsTestSite(t, "  image: Rendered heading\n", map[string]string{
		"index.html":          `<p id="body-count">{{len (.Pages "posts")}}</p>`,
		"posts/configured.md": "<!-- title: Literal title -->\n# {{.Site.Image}}\n",
		"posts/dynamic.md":    "# {{.Site.Image}}\n",
		"posts/embedded.html": `<embed src="../.zas/partial.html" type="text/html">`,
		"posts/literal.md":    "<!--\ntemplate: false\ntitle: '{{literal title}}'\nsummary: '{{literal summary}}'\n-->\n# Literal page\n",
		"posts/plugin.html":   `<p>Before plugin</p><script type="application/zas+listingcount"></script>`,
		".zas/partial.html":   "<h1>Embedded heading</h1>",
	})
	installStub(t, "zslistingcount", "#!/bin/sh\nprintf x >> .zas/listing-plugin-count\nprintf '<h1>Plugin heading</h1>'\n")
	gen.NoPlugins = false
	for run := 1; run <= 2; run++ {
		if err := gen.Run(); err != nil {
			t.Fatal(err)
		}
		calls, err := os.ReadFile(filepath.Join(Dir, "listing-plugin-count"))
		if err != nil || string(calls) != strings.Repeat("x", run) {
			t.Fatalf("run %d plugin calls = %q, error %v; want once per normal render", run, calls, err)
		}
		data := NewZasData("index.html", gen)
		pages, err := data.Pages("posts")
		if err != nil {
			t.Fatal(err)
		}
		assertPageListings(t, pages, []PageSummary{
			{Title: "Literal title", URL: "/base/posts/configured.html", Canonical: "https://example.test/base/posts/configured.html", Language: "en"},
			{URL: "/base/posts/dynamic.html", Canonical: "https://example.test/base/posts/dynamic.html", Language: "en"},
			{URL: "/base/posts/embedded.html", Canonical: "https://example.test/base/posts/embedded.html", Language: "en"},
			{Title: "{{literal title}}", Summary: "{{literal summary}}", URL: "/base/posts/literal.html", Canonical: "https://example.test/base/posts/literal.html", Language: "en"},
			{URL: "/base/posts/plugin.html", Canonical: "https://example.test/base/posts/plugin.html", Language: "en"},
		})
	}
	for name, heading := range map[string]string{"dynamic.html": "Rendered heading", "embedded.html": "Embedded heading", "plugin.html": "Plugin heading"} {
		if got := mustParseDoc(t, readDeploy(t, filepath.Join("posts", name))).Find("h1").First().Text(); got != heading {
			t.Errorf("rendered %s heading = %q, want %q from normal render", name, got, heading)
		}
	}
}

func TestPageListingsSkipsHiddenDeployLayoutAndSymlinkSources(t *testing.T) {
	gen := newPageListingsTestSite(t, "", map[string]string{
		"index.html":              "<h1>Home</h1>",
		"posts/visible.md":        "# Visible\n",
		".hidden/secret.md":       "<!-- title: [invalid] -->\n# Hidden\n",
		".hidden.html":            "<!-- title: [invalid] --><h1>Hidden file</h1>",
		"posts/.hidden/secret.md": "<!-- date: invalid -->\n# Nested hidden\n",
		"public/stale.md":         "<!-- summary: [invalid] -->\n# Deploy output\n",
		".zas/never-a-source.md":  "<!-- date: invalid -->\n# Internal\n",
	})
	config, err := os.ReadFile(ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(config), "layout: .zas/layout.html", "layout: layout.html", 1)
	updated = strings.Replace(updated, "deploy: .zas/deploy", "deploy: public", 1)
	writeMarkdownTestFile(t, ConfigFile, updated)
	writeMarkdownTestFile(t, "layout.html", "<h1>Layout heading</h1>{{.Body}}")
	outside := t.TempDir()
	writeMarkdownTestFile(t, filepath.Join(outside, "secret.md"), "<!-- title: [invalid] -->\n# External\n")
	if err := os.Symlink(filepath.Join(outside, "secret.md"), "posts/link.md"); err != nil {
		t.Logf("symlink file check unavailable: %v", err)
	}
	if err := os.Symlink(outside, "posts/linked-directory"); err != nil {
		t.Logf("symlink directory check unavailable: %v", err)
	}
	if err := gen.Run(); err != nil {
		t.Fatal(err)
	}
	data := NewZasData("index.html", gen)
	pages, err := data.Pages("")
	if err != nil {
		t.Fatal(err)
	}
	assertPageListings(t, pages, []PageSummary{{Title: "Visible", URL: "/base/posts/visible.html", Canonical: "https://example.test/base/posts/visible.html", Language: "en"}})
}

func TestPageListingsSourceCollisionsPreserveDeploy(t *testing.T) {
	for _, privateHTML := range []bool{false, true} {
		t.Run(fmt.Sprintf("private-competitor-%t", privateHTML), func(t *testing.T) {
			html := "<h1>HTML competitor</h1>"
			if privateHTML {
				html = "<!-- publish: false -->\n" + html
			}
			gen := newPageListingsTestSite(t, "", map[string]string{
				"posts/same.md":   "# Markdown competitor\n",
				"posts/same.html": html,
			})
			writeMarkdownTestFile(t, filepath.Join(Dir, "deploy", "marker.txt"), "KEEP\n")
			gen.Full = true
			err := gen.Run()
			if err == nil || !strings.Contains(err.Error(), "posts/same.md") || !strings.Contains(err.Error(), "posts/same.html") {
				t.Fatalf("collision error = %v, want both competing source paths", err)
			}
			if got := readDeploy(t, "marker.txt"); got != "KEEP\n" {
				t.Fatalf("collision preflight changed existing deploy output: %q", got)
			}
		})
	}
}

func TestPageListingsRejectTemplateGeneratedPrivateConfig(t *testing.T) {
	for _, tc := range []struct{ name, source string }{
		{"generated comment", `{{print "<!-- publish: false -->"}}` + "\n<h1>Private generated comment</h1>"},
		{"conditional comment", `{{if true}}<!-- publish: false -->{{end}}` + "\n<h1>Private conditional comment</h1>"},
		{"action in YAML comment", "<!--\n" + `# {{printf "\npublish: false\n"}}` + "\n-->\n<h1>Private YAML comment action</h1>"},
		{"action in unused scalar", "<!--\n" + `other: '{{printf "%c\npublish: false\nunused: %c" 39 39}}'` + "\n-->\n<h1>Private unknown-field action</h1>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gen := newPageListingsTestSite(t, "", map[string]string{"posts/private.html": tc.source})
			writeMarkdownTestFile(t, filepath.Join(Dir, "deploy", "marker.txt"), "KEEP\n")
			gen.Full = true
			err := gen.Run()
			if err == nil || !strings.Contains(err.Error(), "posts/private.html") || !strings.Contains(strings.ToLower(err.Error()), "static") {
				t.Fatalf("Run() = %v, want an unsupported static-metadata diagnostic before listing a private page", err)
			}
			if got := readDeploy(t, "marker.txt"); got != "KEEP\n" {
				t.Fatalf("unsupported generated config changed existing output: %q", got)
			}
		})
	}
}

func TestPageListingsDeployAliasPreservesPublicOutput(t *testing.T) {
	for _, full := range []bool{false, true} {
		t.Run(fmt.Sprintf("full-%t", full), func(t *testing.T) {
			gen := newPageListingsTestSite(t, "", map[string]string{
				"published/private.html": "<h1>Public target</h1>",
				"posts/private.html":     "<!-- publish: false -->\n<h1>Private source</h1>",
			})
			if err := gen.Run(); err != nil {
				t.Fatal(err)
			}
			publicBefore := readDeploy(t, "published/private.html")
			writeMarkdownTestFile(t, filepath.Join(Dir, "deploy", "marker.txt"), "KEEP\n")
			alias := filepath.Join(Dir, "deploy", "posts")
			if err := os.Symlink("published", alias); err != nil {
				t.Skipf("deployment symlink check unavailable: %v", err)
			}
			gen = &Generator{NoPlugins: true, Full: full}
			err := gen.Run()
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
				t.Fatalf("Run() = %v, want rejection of an internal deploy parent alias", err)
			}
			if got := readDeploy(t, "published/private.html"); got != publicBefore {
				t.Fatalf("private output cleanup changed aliased public output: %q", got)
			}
			if got := readDeploy(t, "marker.txt"); got != "KEEP\n" {
				t.Fatalf("symlink preflight changed existing deployment: %q", got)
			}
			if info, err := os.Lstat(alias); err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("symlink preflight changed alias: (%v, %v)", info, err)
			}
		})
	}
}

func TestPageListingsMarkdownMetadataMatchesRendering(t *testing.T) {
	for _, markdown := range []bool{false, true} {
		t.Run(fmt.Sprintf("publish-markdown-%t", markdown), func(t *testing.T) {
			gen := newPageListingsTestSite(t, fmt.Sprintf("  publish_markdown: %t\n", markdown), map[string]string{
				"posts/fenced.md":            "```\n<!-- publish: false -->\n```\n\n# Fenced example\n",
				"posts/indented-private.md":  "    <!-- publish: false -->\n\n# Public code example\n",
				"posts/indented-metadata.md": "    <!--\n    title: Raw configured title\n    summary: Raw configured summary\n    language: ca\n    -->\n\n# Visible heading\n",
				"posts/reference-private.md": "[unused]: /elsewhere\n\n<!-- publish: false -->\n\n# Private after reference\n",
			})
			writeMarkdownTestFile(t, filepath.Join(Dir, "layout.html"), `{{.Body}}<p id="page-metadata">{{.Title}}|{{.Language}}|{{.Summary}}</p>`)
			if err := gen.Run(); err != nil {
				t.Fatal(err)
			}
			title, summary, language := "Visible heading", "", "en"
			if markdown {
				title, summary, language = "Raw configured title", "Raw configured summary", "ca"
			}
			data := NewZasData("index.html", gen)
			pages, err := data.Pages("posts")
			if err != nil {
				t.Fatal(err)
			}
			assertPageListings(t, pages, []PageSummary{
				{Title: "Fenced example", URL: "/base/posts/fenced.html", Canonical: "https://example.test/base/posts/fenced.html", Language: "en"},
				{Title: title, Summary: summary, Language: language, URL: "/base/posts/indented-metadata.html", Canonical: "https://example.test/base/posts/indented-metadata.html"},
				{Title: "Public code example", URL: "/base/posts/indented-private.html", Canonical: "https://example.test/base/posts/indented-private.html", Language: "en"},
			})
			metadata := mustParseDoc(t, readDeploy(t, "posts/indented-metadata.html")).Find("#page-metadata").Text()
			if want := title + "|" + language + "|" + summary; metadata != want {
				t.Fatalf("rendered metadata = %q, want %q matching the listing", metadata, want)
			}
			for _, name := range []string{"fenced", "indented-private"} {
				code := mustParseDoc(t, readDeploy(t, "posts/"+name+".html")).Find("pre code").Text()
				if !strings.Contains(code, "<!-- publish: false -->") {
					t.Errorf("%s publication example was not retained as code: %q", name, code)
				}
			}
			assertDeployMissing(t, "posts/reference-private.html")
			assertDeployMissing(t, "posts/reference-private.md")
			assertDeployMissing(t, "posts/indented-private.md")
			if markdown {
				assertDeployHas(t, "posts/fenced.md")
				assertDeployHas(t, "posts/indented-metadata.md")
			}
		})
	}
}

func TestPageListingsRejectRenderedPrivateMetadataAfterIgnoredPreamble(t *testing.T) {
	for _, prefix := range []struct{ name, source string }{
		{"duplicate doctypes", "<!DOCTYPE html>\n<!DOCTYPE html>\n"},
		{"ignored closing tag", "</unknown>\n"},
	} {
		for _, flag := range []string{"publish: false", "draft: true"} {
			t.Run(prefix.name+"/"+flag, func(t *testing.T) {
				source := prefix.source + `{{print "<!-- ` + flag + ` -->"}}` + "\n<h1>Generated private metadata</h1>"
				gen := newPageListingsTestSite(t, "", map[string]string{"posts/private.html": source})
				err := gen.Run()
				if err == nil || !strings.Contains(err.Error(), "posts/private.html") || !strings.Contains(err.Error(), "template-generated publication metadata") || !strings.Contains(err.Error(), "static") {
					t.Fatalf("Run() = %v, want a final publication-state diagnostic for the static snapshot", err)
				}
				assertDeployMissing(t, "posts/private.html")
			})
		}
	}
}

const pageListingsRowsLayout = `{{.Body}}<ol id="listing">{{range .Pages "posts"}}<li>{{.Title}}|{{.URL}}|{{.Date.Format "2006-01-02T15:04:05Z07:00"}}|{{.Language}}|{{.Summary}}</li>{{end}}</ol>`

func assertPageListingsRows(t *testing.T, page string, want []string) {
	t.Helper()
	rows := mustParseDoc(t, readDeploy(t, page)).Find("#listing li")
	got := make([]string, rows.Length())
	for i := range got {
		got[i] = rows.Eq(i).Text()
	}
	if !slices.Equal(got, want) {
		t.Fatalf("%s listing rows = %q, want %q", page, got, want)
	}
}

func TestPageListingsIncrementalEditsMatchFullBuild(t *testing.T) {
	for _, markdown := range []bool{false, true} {
		t.Run(fmt.Sprintf("publish-markdown-%t", markdown), func(t *testing.T) {
			gen := newPageListingsTestSite(t, fmt.Sprintf("  publish_markdown: %t\n", markdown), map[string]string{
				"index.html": "<h1>Home</h1>",
				"about.html": "<h1>Unchanged consumer</h1>",
				"posts/a.md": "<!--\ndate: 2025-01-02T00:00:00Z\nsummary: Summary A\n-->\n# A\n",
				"posts/b.md": "<!-- date: 2025-01-01T00:00:00Z -->\n# B\n",
			})
			writeMarkdownTestFile(t, filepath.Join(Dir, "layout.html"), pageListingsRowsLayout)
			ageSources(t, -time.Hour)
			if err := gen.Run(); err != nil {
				t.Fatal(err)
			}
			assertPageListingsRows(t, "index.html", []string{
				"A|/base/posts/a.html|2025-01-02T00:00:00Z|en|Summary A",
				"B|/base/posts/b.html|2025-01-01T00:00:00Z|en|",
			})
			changedA := "<!--\ntitle: A edited\ndate: 2025-01-04T00:00:00Z\nlanguage: fr\nsummary: Edited summary\n-->\n# Heading\n"
			for _, step := range []struct {
				name   string
				change func()
				rows   []string
				absent []string
			}{
				{
					name: "create",
					change: func() {
						writeMarkdownTestFile(t, "posts/c.md", "<!--\ndate: 2025-01-03T00:00:00Z\nsummary: Summary C\n-->\n# C\n")
					},
					rows: []string{"C|/base/posts/c.html|2025-01-03T00:00:00Z|en|Summary C", "A|/base/posts/a.html|2025-01-02T00:00:00Z|en|Summary A", "B|/base/posts/b.html|2025-01-01T00:00:00Z|en|"},
				},
				{
					name:   "title-date-language-summary",
					change: func() { writeMarkdownTestFile(t, "posts/a.md", changedA) },
					rows:   []string{"A edited|/base/posts/a.html|2025-01-04T00:00:00Z|fr|Edited summary", "C|/base/posts/c.html|2025-01-03T00:00:00Z|en|Summary C", "B|/base/posts/b.html|2025-01-01T00:00:00Z|en|"},
				},
				{
					name:   "directory-language",
					change: func() { writeMarkdownTestFile(t, "posts/.zas.yml", "language: ca\n") },
					rows:   []string{"A edited|/base/posts/a.html|2025-01-04T00:00:00Z|fr|Edited summary", "C|/base/posts/c.html|2025-01-03T00:00:00Z|ca|Summary C", "B|/base/posts/b.html|2025-01-01T00:00:00Z|ca|"},
				},
				{
					name: "rename",
					change: func() {
						if err := os.Rename("posts/c.md", "posts/z.md"); err != nil {
							t.Fatal(err)
						}
					},
					rows:   []string{"A edited|/base/posts/a.html|2025-01-04T00:00:00Z|fr|Edited summary", "C|/base/posts/z.html|2025-01-03T00:00:00Z|ca|Summary C", "B|/base/posts/b.html|2025-01-01T00:00:00Z|ca|"},
					absent: []string{"posts/c.html", "posts/c.md"},
				},
				{
					name: "delete",
					change: func() {
						if err := os.Remove("posts/b.md"); err != nil {
							t.Fatal(err)
						}
					},
					rows:   []string{"A edited|/base/posts/a.html|2025-01-04T00:00:00Z|fr|Edited summary", "C|/base/posts/z.html|2025-01-03T00:00:00Z|ca|Summary C"},
					absent: []string{"posts/b.html", "posts/b.md"},
				},
				{
					name: "publish-false",
					change: func() {
						writeMarkdownTestFile(t, "posts/a.md", "<!--\npublish: false\n-->\n# Private\n")
					},
					rows:   []string{"C|/base/posts/z.html|2025-01-03T00:00:00Z|ca|Summary C"},
					absent: []string{"posts/a.html", "posts/a.md"},
				},
				{
					name:   "republish",
					change: func() { writeMarkdownTestFile(t, "posts/a.md", changedA) },
					rows:   []string{"A edited|/base/posts/a.html|2025-01-04T00:00:00Z|fr|Edited summary", "C|/base/posts/z.html|2025-01-03T00:00:00Z|ca|Summary C"},
				},
				{
					name: "draft",
					change: func() {
						writeMarkdownTestFile(t, "posts/z.md", "<!--\ndraft: true\ndate: 2025-01-03T00:00:00Z\n-->\n# Draft C\n")
					},
					rows: []string{"A edited|/base/posts/a.html|2025-01-04T00:00:00Z|fr|Edited summary"},
				},
			} {
				t.Run(step.name, func(t *testing.T) {
					step.change()
					if err := generate(t, noPluginsGen); err != nil {
						t.Fatalf("incremental generation: %v", err)
					}
					// Neither consumer's own source changes during these steps.
					for _, consumer := range []string{"index.html", "about.html"} {
						assertPageListingsRows(t, consumer, step.rows)
					}
					for _, path := range step.absent {
						assertDeployMissing(t, path)
					}
					incremental := snapshotMarkdownDeploy(t)
					if err := generate(t, fullGen, noPluginsGen); err != nil {
						t.Fatalf("full generation: %v", err)
					}
					assertPageListingsDeploy(t, incremental, snapshotMarkdownDeploy(t))
				})
			}
			assertDeployHas(t, "posts/z.html") // Draft remains standalone HTML.
			if markdown {
				assertDeployMissing(t, "posts/z.md")
			}
		})
	}
}

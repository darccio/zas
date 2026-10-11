package zas

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMarkdownOutputRenderedBodyAndMetadata(t *testing.T) {
	newTestSite(t, "markdown-output")
	source, err := os.ReadFile("article.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := generate(t, fullGen, noPluginsGen); err != nil {
		t.Fatal(err)
	}
	twin := readDeploy(t, "article.md")
	for _, want := range []string{"# Rendered heading", "https://example.com/article.html", "**Markdown**", "```go", `<embed src="does-not-exist"`, "## Details", "Nested **Markdown** content for Example site.", "<aside>Nested en HTML.</aside>"} {
		if !strings.Contains(twin, want) {
			t.Errorf("Markdown twin missing %q: %s", want, twin)
		}
	}
	for _, forbidden := range []string{"PRIVATE_CONFIG_SENTINEL", "NESTED_CONFIG_SENTINEL", "private_note:", "{{", `<embed src=".zas/partials`} {
		if strings.Contains(twin, forbidden) {
			t.Errorf("Markdown twin leaks %q: %s", forbidden, twin)
		}
	}
	output := readDeploy(t, "article.html")
	for _, want := range []string{"<title>Example site article</title>", `content="Public summary."`, "<strong>Markdown</strong>", "Literal *text* &amp; &lt;tags&gt;."} {
		if !strings.Contains(output, want) {
			t.Errorf("HTML missing %q: %s", want, output)
		}
	}
	after, err := os.ReadFile("article.md")
	if err != nil || string(after) != string(source) {
		t.Fatalf("source changed: %v", err)
	}
	contact := readDeploy(t, "contact.md")
	if !strings.Contains(contact, `<nav><a href="mailto:test@example.com">Example site</a></nav>`) || strings.Contains(contact, "HTML_CONFIG_SENTINEL") {
		t.Fatalf("HTML partial not processed or private config leaked: %s", contact)
	}
	preview := readDeploy(t, "preview.md")
	if !strings.Contains(preview, "The title preview is Static title and first heading is Static title.") {
		t.Fatalf("static Markdown title preview missing: %s", preview)
	}
	if !strings.Contains(readDeploy(t, "explicit.md"), "# Explicit title") {
		t.Fatal("explicit title did not populate templated heading")
	}
	if !strings.Contains(readDeploy(t, "literal.md"), "{{.Title}}") {
		t.Fatal("template:false literal documentation was not retained")
	}
	for _, path := range []string{"private.md", "private.html", "draft.md", "html-only.md", "page.md", filepath.Join("section", "INDEX.MD")} {
		assertDeployMissing(t, path)
	}
	for _, path := range []string{"draft.html", "html-only.html", "page.html", filepath.Join("section", "INDEX.md"), filepath.Join("section", "INDEX.html")} {
		assertDeployHas(t, path)
	}
	assertDeployMissing(t, "."+markdownManifestName)
	manifest, err := os.ReadFile(filepath.Join(Dir, markdownManifestName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manifest), "PRIVATE_CONFIG_SENTINEL") {
		t.Fatal("private config leaked into manifest")
	}
}

func TestMarkdownOutputDisabledIsByteIdentical(t *testing.T) {
	newTestSite(t, "site")
	if err := generate(t, fullGen); err != nil {
		t.Fatal(err)
	}
	baseline := snapshotMarkdownDeploy(t)
	config, err := os.ReadFile(ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	writeMarkdownTestFile(t, ConfigFile, strings.Replace(string(config), "site:\n", "site:\n  publish_markdown: false\n", 1))
	if err := generate(t, fullGen); err != nil {
		t.Fatal(err)
	}
	got := snapshotMarkdownDeploy(t)
	if len(got) != len(baseline) {
		t.Fatalf("disabled build changed file count: %d vs %d", len(got), len(baseline))
	}
	for path, content := range baseline {
		if got[path] != content {
			t.Errorf("disabled output differs at %s", path)
		}
	}
	if _, err := os.Stat(filepath.Join(Dir, markdownManifestName)); !os.IsNotExist(err) {
		t.Fatalf("never-enabled site gained manifest: %v", err)
	}
}

func TestMarkdownOutputIncrementalLifecycle(t *testing.T) {
	newTestSite(t, "markdown-output")
	ageSources(t, -time.Hour)
	if err := generate(t); err != nil {
		t.Fatal(err)
	}
	htmlPath := filepath.Join(Dir, "deploy", "contact.html")
	before, err := os.Stat(htmlPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := generate(t); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(htmlPath)
	if err != nil || !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("unchanged HTML was regenerated: %v", err)
	}
	if err := os.Remove(filepath.Join(Dir, "deploy", "contact.md")); err != nil {
		t.Fatal(err)
	}
	if err := generate(t); err != nil {
		t.Fatal(err)
	}
	assertDeployHas(t, "contact.md")
	if err := os.Rename("contact.md", "renamed.md"); err != nil {
		t.Fatal(err)
	}
	if err := generate(t); err != nil {
		t.Fatal(err)
	}
	assertDeployMissing(t, "contact.md")
	assertDeployMissing(t, "contact.html")
	assertDeployHas(t, "renamed.md")
	writeMarkdownTestFile(t, "renamed.md", "<!--\npublish: false\n-->\n# Now private\n")
	if err := generate(t); err != nil {
		t.Fatal(err)
	}
	assertDeployMissing(t, "renamed.md")
	assertDeployMissing(t, "renamed.html")
	writeMarkdownTestFile(t, "renamed.md", "# Public again\n")
	if err := generate(t); err != nil {
		t.Fatal(err)
	}
	assertDeployHas(t, "renamed.md")
	writeMarkdownTestFile(t, "renamed.md", "<!--\npublish_markdown: false\n-->\n# HTML retained\n")
	if err := generate(t); err != nil {
		t.Fatal(err)
	}
	assertDeployMissing(t, "renamed.md")
	assertDeployHas(t, "renamed.html")
	writeMarkdownTestFile(t, "renamed.md", "# Public again\n")
	if err := generate(t); err != nil {
		t.Fatal(err)
	}
	writeMarkdownTestFile(t, "renamed.md", "<!--\ndraft: true\n-->\n# Live draft\n")
	if err := generate(t); err != nil {
		t.Fatal(err)
	}
	assertDeployMissing(t, "renamed.md")
	assertDeployHas(t, "renamed.html")
	if err := os.Remove(filepath.Join("section", "INDEX.MD")); err != nil {
		t.Fatal(err)
	}
	if err := generate(t); err != nil {
		t.Fatal(err)
	}
	assertDeployMissing(t, filepath.Join("section", "INDEX.md"))
	assertDeployMissing(t, filepath.Join("section", "INDEX.html"))
}

func TestMarkdownOutputToggleAndEditedOwnership(t *testing.T) {
	newTestSite(t, "markdown-output")
	if err := generate(t); err != nil {
		t.Fatal(err)
	}
	writeMarkdownTestFile(t, filepath.Join(Dir, "deploy", "preview.md"), "Manual edited twin\n")
	writeMarkdownTestFile(t, filepath.Join(Dir, "deploy", "manual.md"), "Manual Markdown output\n")
	config, err := os.ReadFile(ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	writeMarkdownTestFile(t, ConfigFile, strings.Replace(string(config), "publish_markdown: true", "publish_markdown: false", 1))
	if err := generate(t); err != nil {
		t.Fatal(err)
	}
	assertDeployMissing(t, "article.md")
	assertDeployHas(t, "article.html")
	if readDeploy(t, "preview.md") != "Manual edited twin\n" || readDeploy(t, "manual.md") != "Manual Markdown output\n" {
		t.Fatal("cleanup touched files without matching ownership")
	}
	if _, err := os.Stat(filepath.Join(Dir, markdownManifestName)); !os.IsNotExist(err) {
		t.Fatalf("disabled manifest still exists: %v", err)
	}
}

func TestMarkdownOutputPreflightBeforeMutation(t *testing.T) {
	for _, scenario := range []string{"source collision", "unchanged collision", "manual twin", "edited twin", "directory collision", "source overlap", "root overlap", "parent overlap", "config overlap", "partial overlap", "git descendant overlap"} {
		t.Run(scenario, func(t *testing.T) {
			newTestSite(t, "markdown-output")
			if err := generate(t); err != nil {
				t.Fatal(err)
			}
			writeMarkdownTestFile(t, filepath.Join(Dir, "deploy", "marker.txt"), "KEEP\n")
			switch scenario {
			case "source collision", "unchanged collision":
				writeMarkdownTestFile(t, "contact.html", "<h1>Collision</h1>")
				if scenario == "unchanged collision" {
					when := time.Now().Add(-time.Hour)
					if err := os.Chtimes("contact.html", when, when); err != nil {
						t.Fatal(err)
					}
				}
			case "manual twin":
				writeMarkdownTestFile(t, "new.md", "# New\n")
				writeMarkdownTestFile(t, filepath.Join(Dir, "deploy", "new.md"), "Manual twin")
			case "edited twin":
				writeMarkdownTestFile(t, filepath.Join(Dir, "deploy", "contact.md"), "Manual edit")
			case "directory collision":
				if err := os.Mkdir("contact.html", 0o755); err != nil {
					t.Fatal(err)
				}
			case "source overlap", "root overlap", "parent overlap", "config overlap", "partial overlap", "git descendant overlap":
				deploy := map[string]string{"source overlap": "section", "root overlap": ".", "parent overlap": "..", "config overlap": ".zas", "partial overlap": ".zas/partials", "git descendant overlap": ".git/output"}[scenario]
				config, err := os.ReadFile(ConfigFile)
				if err != nil {
					t.Fatal(err)
				}
				writeMarkdownTestFile(t, ConfigFile, strings.Replace(string(config), "deploy: .zas/deploy", "deploy: "+deploy, 1))
			}
			before := snapshotMarkdownDeploy(t)
			if err := generate(t, fullGen); err == nil {
				t.Fatal("unsafe full build succeeded")
			}
			after := snapshotMarkdownDeploy(t)
			if len(after) != len(before) {
				t.Fatal("preflight failure changed deploy file count")
			}
			for path, content := range before {
				if after[path] != content {
					t.Errorf("preflight failure changed %s", path)
				}
			}
		})
	}
}

func TestMarkdownOutputUnsupportedContentAndOptOut(t *testing.T) {
	for _, input := range []string{
		"# Unsupported\n\n<embed src=\"article.md\" type=\"application/example\">\n",
		"# Unsupported\n\n<script type=\"application/zas+example\">payload</script>\n",
		"# Unsupported\n\n<script type=\" Application/Zas+Example \">payload</script>\n",
		"# Unsupported\n\n<script type=\"application/zas+example\" type=\"text/plain\">payload</script>\n",
		"# Unsupported\n\n<embed src=\"missing-first.html\" src=\".zas/partials/contact.html\" type=\"text/html\">\n",
		"# Unsupported\n\nInline <embed src=\"article.md\" type=\"text/markdown\"> content.\n",
		"# Unsupported\n\n<embed type=\"text/html\">\n",
		"# Unsupported\n\n{{\"{{.Title}}\"}}\n",
		"# Unsupported\n\n[local]: https://example.com\n\n<embed src=\".zas/partials/details.md\" type=\"text/markdown\">\n",
	} {
		t.Run(input, func(t *testing.T) {
			newTestSite(t, "markdown-output")
			writeMarkdownTestFile(t, "unsupported.md", input)
			err := generate(t, noPluginsGen)
			if err == nil || !strings.Contains(err.Error(), "publish_markdown: false") {
				t.Fatalf("unsupported content error = %v, want page opt-out guidance", err)
			}
			assertDeployMissing(t, "unsupported.md")
		})
	}
	// An explicit opt-out retains the legacy HTML path, including an
	// otherwise unsupported inline Markdown embed.
	newTestSite(t, "markdown-output")
	writeMarkdownTestFile(t, "unsupported.md", "<!--\npublish_markdown: false\n-->\n# HTML only\n\nInline <embed src=\".zas/partials/details.md\" type=\"text/markdown\"> content.\n")
	if err := generate(t, noPluginsGen); err != nil {
		t.Fatal(err)
	}
	assertDeployHas(t, "unsupported.html")
	assertDeployMissing(t, "unsupported.md")
}

func TestMarkdownOutputEmbedContainmentAndDepth(t *testing.T) {
	for _, kind := range []string{"traversal", "symlink", "cycle"} {
		t.Run(kind, func(t *testing.T) {
			newTestSite(t, "markdown-output")
			outside := filepath.Join(t.TempDir(), "secret.html")
			writeMarkdownTestFile(t, outside, "<p>Outside secret</p>")
			src := outside
			switch kind {
			case "symlink":
				src = filepath.Join(Dir, "partials", "outside.html")
				if err := os.Symlink(outside, src); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			case "cycle":
				src = filepath.Join(Dir, "partials", "cycle.md")
				writeMarkdownTestFile(t, src, "<embed src=\"cycle.md\" type=\"text/markdown\">\n")
			}
			typ := "text/html"
			if kind == "cycle" {
				typ = "text/markdown"
			}
			writeMarkdownTestFile(t, "unsafe.md", "# Unsafe\n\n<embed src=\""+src+"\" type=\""+typ+"\">\n")
			err := generate(t, noPluginsGen)
			if err == nil || (!strings.Contains(err.Error(), "escapes the site root") && !strings.Contains(err.Error(), "embed nesting")) {
				t.Fatalf("containment/depth error = %v", err)
			}
			assertDeployMissing(t, "unsafe.md")
		})
	}
}

func TestMarkdownOutputFullRebuildCleansOrdinaryOutputs(t *testing.T) {
	newTestSite(t, "markdown-output")
	writeMarkdownTestFile(t, "asset.txt", "Asset\n")
	if err := generate(t); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove("asset.txt"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove("page.html"); err != nil {
		t.Fatal(err)
	}
	if err := generate(t, fullGen); err != nil {
		t.Fatal(err)
	}
	assertDeployMissing(t, "asset.txt")
	assertDeployMissing(t, "page.html")
	assertDeployHas(t, "article.md")
}

func TestMarkdownOutputManifestDoesNotCleanOtherDeploy(t *testing.T) {
	newTestSite(t, "markdown-output")
	if err := generate(t); err != nil {
		t.Fatal(err)
	}
	old := readDeploy(t, "contact.md")
	config, err := os.ReadFile(ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(config), "publish_markdown: true", "publish_markdown: false", 1)
	updated = strings.Replace(updated, "deploy: .zas/deploy", "deploy: .zas/other", 1)
	writeMarkdownTestFile(t, ConfigFile, updated)
	if err := generate(t); err != nil {
		t.Fatal(err)
	}
	if readDeploy(t, "contact.md") != old {
		t.Fatal("another deploy's Markdown ownership was used for cleanup")
	}
}

func TestMarkdownOutputRejectsUnsafeManifestPaths(t *testing.T) {
	newTestSite(t, "markdown-output")
	deploy, err := resolvedOutputPath(filepath.Join(Dir, "deploy"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := markdownManifest{Version: 1, Deploy: deploy, Files: map[string]string{"../outside.md": strings.Repeat("0", 64)}}
	input, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeMarkdownTestFile(t, filepath.Join(Dir, markdownManifestName), string(input))
	if err := generate(t, fullGen); err == nil {
		t.Fatal("unsafe manifest was accepted")
	}
}

func TestMarkdownOutputCurrentHTMLSourceTakesOverOutput(t *testing.T) {
	newTestSite(t, "markdown-output")
	writeMarkdownTestFile(t, "takeover.md", "# Takeover\n")
	if err := generate(t); err != nil {
		t.Fatal(err)
	}
	before := readDeploy(t, "takeover.html")
	if err := os.Remove("takeover.md"); err != nil {
		t.Fatal(err)
	}
	writeMarkdownTestFile(t, "takeover.html", "<h1>Takeover</h1>\n")
	if err := generate(t); err != nil {
		t.Fatal(err)
	}
	if readDeploy(t, "takeover.html") != before {
		t.Fatal("current HTML source's identical output was removed or changed")
	}
	assertDeployMissing(t, "takeover.md")
	writeMarkdownTestFile(t, "takeover.html", "<h1>Edited takeover</h1>\n")
	if err := generate(t); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readDeploy(t, "takeover.html"), "Edited takeover") {
		t.Fatal("HTML takeover did not regenerate")
	}
	if err := os.Remove("takeover.html"); err != nil {
		t.Fatal(err)
	}
	if err := generate(t); err != nil {
		t.Fatal(err)
	}
	assertDeployMissing(t, "takeover.html")
}

func TestMarkdownOutputAllowlistedDirectoryManifest(t *testing.T) {
	newTestSite(t, "markdown-output")
	config, err := os.ReadFile(ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	writeMarkdownTestFile(t, ConfigFile, strings.Replace(string(config), "zas:\n", "zas:\n  allowed_dotdirs: [\".well-known\"]\n", 1))
	writeMarkdownTestFile(t, filepath.Join(".well-known", "page.md"), "# Public well-known page\n")
	for range 2 {
		if err := generate(t); err != nil {
			t.Fatal(err)
		}
		assertDeployHas(t, filepath.Join(".well-known", "page.md"))
		assertDeployHas(t, filepath.Join(".well-known", "page.html"))
	}
}

func TestMarkdownOutputCleanupPrecedesSitemap(t *testing.T) {
	newTestSite(t, "markdown-output")
	config, err := os.ReadFile(ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	writeMarkdownTestFile(t, ConfigFile, strings.Replace(string(config), "site:\n", "site:\n  sitemap: true\n", 1))
	if err := generate(t); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove("contact.md"); err != nil {
		t.Fatal(err)
	}
	if err := generate(t); err != nil {
		t.Fatal(err)
	}
	assertDeployMissing(t, "contact.html")
	if strings.Contains(readDeploy(t, "sitemap.xml"), "https://example.com/contact.html") {
		t.Fatal("sitemap advertises removed owned HTML")
	}
}

func TestMarkdownOutputSymlinkOutputParentPreflight(t *testing.T) {
	newTestSite(t, "markdown-output")
	if err := generate(t); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.RemoveAll(filepath.Join(Dir, "deploy", "section")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(Dir, "deploy", "section")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := generate(t); err == nil || !strings.Contains(err.Error(), "escapes deploy through a symlink") {
		t.Fatalf("output-parent containment error = %v", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("output-parent preflight wrote outside deploy: %v", err)
	}
}

func TestMarkdownOutputEmbedsSeeRenderedTitle(t *testing.T) {
	newTestSite(t, "markdown-output")
	writeMarkdownTestFile(t, filepath.Join(Dir, "partials", "title.html"), "<p>{{.Title}} / {{.FirstTitle}}</p>")
	writeMarkdownTestFile(t, "templated.md", "# {{.Extra \"/site/title\"}}\n\n<embed src=\".zas/partials/title.html\" type=\"text/html\">\n")
	writeMarkdownTestFile(t, "generated.md", "<!--\ntitle: '{{.Extra \"/site/title\"}} explicit'\n-->\n# {{.Extra \"/site/title\"}}\n\n<embed src=\".zas/partials/title.html\" type=\"text/html\">\n")
	if err := generate(t, noPluginsGen); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readDeploy(t, "templated.md"), "<p>Example site / Example site</p>") {
		t.Fatal("embedded template did not see the rendered H1")
	}
	if !strings.Contains(readDeploy(t, "generated.md"), "<p>Example site explicit / Example site</p>") {
		t.Fatal("embedded template did not see rendered explicit metadata")
	}
}

func TestMarkdownOutputFailureRecoveryAndPluginInvocation(t *testing.T) {
	newTestSite(t, "markdown-output")
	installStub(t, "zscount", "#!/bin/sh\nprintf 'call\\n' >> .zas/plugin-calls\nprintf '<p>Plugin output</p>'\n")
	writeMarkdownTestFile(t, "count.md", "# Count\n\n<script type=\" Application/Zas+Count \">payload</script>\n")
	if err := generate(t); err == nil {
		t.Fatal("unsupported script plugin twin succeeded")
	}
	if _, err := os.Stat(filepath.Join(Dir, "plugin-calls")); !os.IsNotExist(err) {
		t.Fatal("unsupported twin content invoked a plugin")
	}
	writeMarkdownTestFile(t, "count.md", "<!--\npublish_markdown: false\n-->\n# Count\n\n<script type=\" Application/Zas+Count \">payload</script>\n")
	if err := generate(t); err != nil {
		t.Fatalf("retry after partial failed build: %v", err)
	}
	calls, err := os.ReadFile(filepath.Join(Dir, "plugin-calls"))
	if err != nil || string(calls) != "call\n" {
		t.Fatalf("HTML-only plugin invocation count = %q, error %v; want one invocation", calls, err)
	}
	assertDeployMissing(t, "count.md")
	if !strings.Contains(readDeploy(t, "count.html"), "Plugin output") {
		t.Fatal("HTML-only plugin output missing")
	}
}

func writeMarkdownTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func snapshotMarkdownDeploy(t *testing.T) map[string]string {
	t.Helper()
	files := make(map[string]string)
	root := filepath.Join(Dir, "deploy")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		input, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[rel] = string(input)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

/*
 * Copyright (c) 2013 Dario Castañé.
 * This file is part of Zas.
 *
 * Zas is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * Zas is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with Zas.  If not, see <http://www.gnu.org/licenses/>.
 */

package zas

import (
	"strings"
	"testing"
)

// zsEchoStub wraps its argv (joined, --tag <name> selects the wrapping
// element) around a literal string, e.g. --tag b 'hello world' -> "<b>hello
// world</b>". zsWrapStub wraps its raw stdin in <pre>...</pre> - the
// generic "prove the plugin actually ran on real input" shape used across
// these tests. Installed per test via installStub (script_plugin_test.go).
const zsEchoStub = `#!/bin/sh
tag=span
text=
while [ $# -gt 0 ]; do
  case "$1" in
    --tag) tag="$2"; shift 2 ;;
    *) text="$1"; shift ;;
  esac
done
printf '<%s>%s</%s>' "$tag" "$text" "$tag"
`

const zsWrapStub = "#!/bin/sh\nprintf '<pre>'; cat; printf '</pre>'\n"

func TestGenerateRunsScriptPluginInPageBody(t *testing.T) {
	installStub(t, "zsecho", zsEchoStub)
	installStub(t, "zswrap", zsWrapStub)
	newTestSite(t, "script-plugin-site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	got := readDeploy(t, "index.html")
	if !strings.Contains(got, "<b>hello world</b>") {
		t.Fatalf("deployed index.html = %q, want the zsecho plugin's output", got)
	}
	if !strings.Contains(got, `<pre>{&#34;k&#34;: &#34;v&#34;}</pre>`) {
		t.Fatalf("deployed index.html = %q, want the zswrap plugin's output with its stdin intact", got)
	}
	if strings.Contains(got, "application/zas+") {
		t.Fatalf("deployed index.html = %q, want no zas script tag left behind", got)
	}
	if !strings.Contains(got, `var pageJS = "a<b";`) {
		t.Fatalf("deployed index.html = %q, want the page's real <script> untouched", got)
	}
	if !strings.Contains(got, `"@context": "https://schema.org"`) {
		t.Fatalf("deployed index.html = %q, want the layout's JSON-LD script untouched", got)
	}
}

func TestGenerateScriptPluginOutputSplicesWithoutNesting(t *testing.T) {
	installStub(t, "zsecho", "#!/bin/sh\necho '<html><head><title>t</title></head><body><p>x</p></body></html>'\n")
	installStub(t, "zswrap", zsWrapStub)
	newTestSite(t, "script-plugin-site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	got := readDeploy(t, "index.html")
	for _, tag := range []string{"<html", "<head", "<body"} {
		if n := strings.Count(got, tag); n != 1 {
			t.Fatalf("deployed index.html = %q, want exactly one %q (the outer page's own), got %d", got, tag, n)
		}
	}
	if !strings.Contains(got, "<p>x</p>") {
		t.Fatalf("deployed index.html = %q, want the plugin's own body content spliced in", got)
	}
}

func TestGenerateRunsScriptPluginInLayoutBody(t *testing.T) {
	installStub(t, "zsecho", zsEchoStub)
	installStub(t, "zsmeta", "#!/bin/sh\necho '<meta name=\"generated\" content=\"x\">'\n")
	newTestSite(t, "script-plugin-layout-site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	got := readDeploy(t, "index.html")
	if !strings.Contains(got, "from the layout itself") {
		t.Fatalf("deployed index.html = %q, want the layout body's own script plugin output", got)
	}
	if strings.Contains(got, "application/zas+") {
		t.Fatalf("deployed index.html = %q, want no zas script tag left in the layout", got)
	}
}

func TestGenerateRunsScriptPluginInLayoutHead(t *testing.T) {
	installStub(t, "zsecho", zsEchoStub)
	installStub(t, "zsmeta", "#!/bin/sh\necho '<meta name=\"generated\" content=\"x\">'\n")
	newTestSite(t, "script-plugin-layout-site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	got := readDeploy(t, "index.html")
	if !strings.Contains(got, `<meta name="generated" content="x"/>`) {
		t.Fatalf("deployed index.html = %q, want the layout head's script plugin output to have reached deployed output", got)
	}
}

func TestGenerateFailsOnHeadPlacedScriptInPage(t *testing.T) {
	newTestSite(t, "script-plugin-head-site")
	err := generate(t)
	if err == nil {
		t.Fatal("generate() for a page-opening zas script tag: want error, got nil")
	}
	if !strings.Contains(err.Error(), "<head>") {
		t.Fatalf("generate() error = %v, want it to mention <head>", err)
	}
	assertDeployMissing(t, "index.html")
}

func TestGenerateNoPluginsRefusesScriptPlugin(t *testing.T) {
	installStub(t, "zsecho", zsEchoStub)
	installStub(t, "zswrap", zsWrapStub)
	newTestSite(t, "script-plugin-site")
	err := generate(t, noPluginsGen)
	if err == nil {
		t.Fatal("generate() with NoPlugins set: want error, got nil")
	}
	if !strings.Contains(err.Error(), "-no-plugins") {
		t.Fatalf("generate() error = %v, want it to mention -no-plugins", err)
	}
	assertDeployMissing(t, "index.html")
}

func TestGenerateScriptPluginOutputEmbedResolves(t *testing.T) {
	installStub(t, "zsecho", zsEchoStub)
	installStub(t, "zswrap", "#!/bin/sh\necho '<embed src=\"partials/nav.html\" type=\"text/html\">'\n")
	newTestSite(t, "script-plugin-site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	got := readDeploy(t, "index.html")
	if !strings.Contains(got, "nav from a plugin-emitted embed") {
		t.Fatalf("deployed index.html = %q, want the plugin-emitted <embed> to have resolved", got)
	}
	if strings.Contains(got, "<embed") {
		t.Fatalf("deployed index.html = %q, want no <embed> tag left unresolved", got)
	}
}

func TestGenerateDoesNotRunScriptPluginInFencedCodeBlock(t *testing.T) {
	// No plugin is installed at all - PATH is left as the test's own,
	// which won't have a zsecho binary either. If the scanner ever
	// mistakenly reached into the fenced code block, this would fail with
	// a missing-binary error instead of succeeding.
	newTestSite(t, "script-plugin-codeblock-site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil (a fenced code block must never execute)", err)
	}
	got := readDeploy(t, "codeblock.html")
	if !strings.Contains(got, `&lt;script type=&#34;application/zas+echo&#34;&gt;`) {
		t.Fatalf("deployed codeblock.html = %q, want the script tag to still be escaped, unexecuted text", got)
	}
}

func TestGenerateRunsScriptPluginInMarkdownPage(t *testing.T) {
	installStub(t, "zswrap", zsWrapStub)
	newTestSite(t, "script-plugin-markdown-site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	got := readDeploy(t, "data.html")
	if !strings.Contains(got, "name,count") || !strings.Contains(got, "a,1") || !strings.Contains(got, "b,2") {
		t.Fatalf("deployed data.html = %q, want the multi-line inline data block piped to the plugin", got)
	}
}

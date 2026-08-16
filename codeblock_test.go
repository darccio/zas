package zas

import (
	"strings"
	"testing"
)

// renderMarkdown used to run goldmark's converter output through
// html.UnescapeString before handing it to the HTML5 parser in
// parseAndReplace. Goldmark escapes code block contents on the way out, so
// that unescape turned HTML entities inside a fenced or indented code block
// back into raw markup bytes right before they were re-parsed as HTML -
// corrupting <, >, and & in code samples, and turning a literal <script>
// tag typed inside a code fence into a live, executing script. These tests
// drive the full Generator.Run pipeline (not just markdownConverter, which
// cannot observe this bug) against testdata/site/codeblocks.md.

func TestGenerateEscapesHTMLInFencedCodeBlock(t *testing.T) {
	newTestSite(t, "site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	out := readDeploy(t, "codeblocks.html")
	if !strings.Contains(out, "&lt;b&gt;hi&lt;/b&gt;") {
		t.Fatalf("codeblocks.html = %q, want the fenced <b>hi</b> sample left escaped", out)
	}
	if strings.Contains(out, "<b>hi</b>") {
		t.Fatalf("codeblocks.html = %q, want no real <b> element from the code sample", out)
	}
}

func TestGenerateDoesNotExecuteScriptInFencedCodeBlock(t *testing.T) {
	newTestSite(t, "site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	out := readDeploy(t, "codeblocks.html")
	if !strings.Contains(out, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatalf("codeblocks.html = %q, want the fenced <script> sample left escaped", out)
	}
	if strings.Contains(out, "<script>alert(1)</script>") {
		t.Fatalf("codeblocks.html = %q, want no live <script> element from the code sample", out)
	}
}

func TestGenerateKeepsDoubleEscapedEntitiesInCodeBlock(t *testing.T) {
	newTestSite(t, "site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	out := readDeploy(t, "codeblocks.html")
	want := `<pre><code class="language-text">&amp;lt;script&amp;gt;`
	if !strings.Contains(out, want) {
		t.Fatalf("codeblocks.html = %q, want it to contain %q (literal &lt;script&gt; typed in source keeping its full escaping)", out, want)
	}
	degraded := `<pre><code class="language-text">&lt;script&gt;`
	if strings.Contains(out, degraded) {
		t.Fatalf("codeblocks.html = %q, the language-text block lost a level of escaping (want &amp;lt;..&amp;gt;, got &lt;..&gt;)", out)
	}
}

func TestGenerateEscapesHTMLInIndentedCodeBlock(t *testing.T) {
	newTestSite(t, "site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	out := readDeploy(t, "codeblocks.html")
	if !strings.Contains(out, "&lt;b&gt;indented&lt;/b&gt;") {
		t.Fatalf("codeblocks.html = %q, want the indented <b>indented</b> sample left escaped", out)
	}
	if strings.Contains(out, "<b>indented</b>") {
		t.Fatalf("codeblocks.html = %q, want no real <b> element from the indented sample", out)
	}
}

func TestGenerateFencedCodeBlockGetsLanguageClass(t *testing.T) {
	newTestSite(t, "site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	out := readDeploy(t, "codeblocks.html")
	if !strings.Contains(out, `<pre><code class="language-html">`) {
		t.Fatalf("codeblocks.html = %q, want a fenced code block with a language-html class", out)
	}
}

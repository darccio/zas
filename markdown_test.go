package zas

import (
	"bytes"
	"strings"
	"testing"
)

// Goldmark's default renderer replaces raw HTML blocks and inline HTML with
// an "raw HTML omitted" placeholder. That silently destroyed the per-page
// config comment and any <embed> tag written directly in a .md source, and
// dropped any other HTML an author wrote by hand - including the exact
// input from https://github.com/darccio/zas/issues/16, where a bare <div>
// around a Markdown page's content was expected to survive untouched.
//
// markdownConverter fixes this by overriding only the raw-HTML node kinds,
// rather than enabling goldmark's blanket "unsafe" mode - which would also
// disable link/image destination sanitization against javascript:/data:
// URLs. See TestMarkdownSanitizesDangerousLinkURL and
// TestMarkdownSanitizesDangerousImageURL below.

func convertMarkdown(t *testing.T, src string) string {
	t.Helper()
	var b bytes.Buffer
	if err := markdownConverter.Convert([]byte(src), &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestMarkdownKeepsRawHTMLBlock(t *testing.T) {
	out := convertMarkdown(t, "<div>\nHello world!\n</div>\n")
	if strings.Contains(out, "raw HTML omitted") {
		t.Fatalf("output = %q, raw HTML was replaced with a placeholder", out)
	}
	for _, want := range []string{"<div>", "Hello world!", "</div>"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output = %q, want it to contain %q", out, want)
		}
	}
}

func TestMarkdownDoesNotWrapBlockHTMLInParagraph(t *testing.T) {
	// The issue #16 reproducer: a bare <div> is a CommonMark HTML block, not
	// paragraph content, so it must never end up wrapped in <p></p>.
	out := convertMarkdown(t, "<div>\nHello world!\n</div>\n")
	if strings.Contains(out, "<p>") {
		t.Fatalf("output = %q, want no <p> wrapping the raw HTML block", out)
	}
}

func TestMarkdownKeepsLeadingConfigComment(t *testing.T) {
	out := convertMarkdown(t, "<!-- title: X -->\n\nHello\n")
	if !strings.Contains(out, "<!-- title: X -->") {
		t.Fatalf("output = %q, want the leading comment preserved", out)
	}
}

func TestMarkdownKeepsEmbedTag(t *testing.T) {
	// A blank line before the tag is required: an HTML block of this kind
	// cannot interrupt an open paragraph under CommonMark's rules.
	out := convertMarkdown(t, "Text before.\n\n<embed src=\"nav.md\" type=\"text/markdown\" />\n")
	if strings.Contains(out, "raw HTML omitted") {
		t.Fatalf("output = %q, embed tag was replaced with a placeholder", out)
	}
	if !strings.Contains(out, `<embed src="nav.md" type="text/markdown"`) {
		t.Fatalf("output = %q, want the embed tag preserved verbatim", out)
	}
}

func TestMarkdownEscapesEntitiesInFencedCode(t *testing.T) {
	out := convertMarkdown(t, "```\n&lt;script&gt;\n```\n")
	if !strings.Contains(out, "&amp;lt;script&amp;gt;") {
		t.Fatalf("output = %q, want literal entities in code left escaped, not interpreted", out)
	}
}

func TestMarkdownSanitizesDangerousLinkURL(t *testing.T) {
	out := convertMarkdown(t, "[x](javascript:alert(1))\n")
	if strings.Contains(out, "javascript:") {
		t.Fatalf("output = %q, want the javascript: destination stripped, not just raw HTML passed through", out)
	}
	if !strings.Contains(out, `href=""`) {
		t.Fatalf("output = %q, want an empty href for a sanitized destination", out)
	}
}

func TestMarkdownSanitizesDangerousImageURL(t *testing.T) {
	out := convertMarkdown(t, "![x](data:text/html;base64,x)\n")
	if strings.Contains(out, "data:text/html") {
		t.Fatalf("output = %q, want the data: destination stripped", out)
	}
	if !strings.Contains(out, `src=""`) {
		t.Fatalf("output = %q, want an empty src for a sanitized destination", out)
	}
}

func TestMarkdownFencedCodeBlockGetsLanguageClass(t *testing.T) {
	out := convertMarkdown(t, "```go\nfmt.Println(1)\n```\n")
	if !strings.Contains(out, `<pre><code class="language-go">`) {
		t.Fatalf("output = %q, want a fenced code block with a language-go class", out)
	}
}

func TestMarkdownIndentedCodeBlockRenders(t *testing.T) {
	out := convertMarkdown(t, "    fmt.Println(1)\n")
	if !strings.Contains(out, "<pre><code>") {
		t.Fatalf("output = %q, want an indented code block wrapped in <pre><code>", out)
	}
}

// markdownConverter enables extension.GFM (tables, strikethrough, task
// lists, autolinks) and extension.Footnote on top of goldmark's default
// CommonMark parsing. Previously neither was enabled, so this syntax
// rendered as plain, unprocessed text - most visibly a GFM table showing
// up as a literal paragraph of pipe characters instead of a <table>.

func TestMarkdownRendersGFMTable(t *testing.T) {
	out := convertMarkdown(t, "| A | B |\n|---|---|\n| 1 | 2 |\n")
	for _, want := range []string{"<table>", "<th>A</th>", "<th>B</th>", "<td>1</td>", "<td>2</td>"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output = %q, want it to contain %q", out, want)
		}
	}
	if strings.Contains(out, "| A | B |") {
		t.Fatalf("output = %q, table syntax was left as literal text", out)
	}
}

func TestMarkdownRendersStrikethrough(t *testing.T) {
	out := convertMarkdown(t, "~~struck~~\n")
	if !strings.Contains(out, "<del>struck</del>") {
		t.Fatalf("output = %q, want strikethrough syntax rendered as <del>", out)
	}
}

func TestMarkdownRendersTaskList(t *testing.T) {
	out := convertMarkdown(t, "- [ ] todo\n- [x] done\n")
	if !strings.Contains(out, `<input disabled="" type="checkbox">`) {
		t.Fatalf("output = %q, want an unchecked task list checkbox", out)
	}
	if !strings.Contains(out, `<input checked="" disabled="" type="checkbox">`) {
		t.Fatalf("output = %q, want a checked task list checkbox", out)
	}
}

func TestMarkdownRendersAutolink(t *testing.T) {
	out := convertMarkdown(t, "See http://example.com for more.\n")
	if !strings.Contains(out, `<a href="http://example.com">http://example.com</a>`) {
		t.Fatalf("output = %q, want the bare URL linkified", out)
	}
}

func TestMarkdownRendersFootnote(t *testing.T) {
	out := convertMarkdown(t, "Body text.[^1]\n\n[^1]: The footnote.\n")
	if !strings.Contains(out, "The footnote.") {
		t.Fatalf("output = %q, want the footnote definition rendered", out)
	}
	if !strings.Contains(out, `href="#fn:1"`) || !strings.Contains(out, `id="fn:1"`) {
		t.Fatalf("output = %q, want a footnote reference link and its matching definition anchor", out)
	}
}

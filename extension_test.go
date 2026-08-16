package zas

import "testing"

// sourceIsNewer, NewZasData, and reaper each used to swap .md/.html
// extensions with strings.Replace/ReplaceAll on the whole path string,
// instead of anchoring on the trailing extension - and disagreed with each
// other on occurrence count while doing it. swapExtension is the single
// helper all three now share.

func TestSwapExtension(t *testing.T) {
	cases := []struct {
		name string
		path string
		from string
		to   string
		want string
	}{
		{"simple extension swap", "page.md", ".md", ".html", "page.html"},
		{"only the trailing occurrence is swapped", "a.md.md", ".md", ".html", "a.md.html"},
		{"extension-like mid-name segment is untouched", "v1.mdx/page.md", ".md", ".html", "v1.mdx/page.html"},
		{"trailing occurrence swapped even when an earlier one matches to", "x.html.md", ".md", ".html", "x.html.html"},
		{"reverse swap also only touches the trailing occurrence", "x.html.html", ".html", ".md", "x.html.md"},
		{"path without the suffix is returned unchanged", "style.css", ".md", ".html", "style.css"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := swapExtension(c.path, c.from, c.to); got != c.want {
				t.Fatalf("swapExtension(%q, %q, %q) = %q, want %q", c.path, c.from, c.to, got, c.want)
			}
		})
	}
}

// TestSwapExtensionRoundTrip pins the property that makes the three call
// sites agree with each other: swapping .md -> .html and then .html -> .md
// must return the original path.
func TestSwapExtensionRoundTrip(t *testing.T) {
	paths := []string{"page.md", "a.md.md", "v1.mdx/page.md", "x.html.md"}
	for _, orig := range paths {
		t.Run(orig, func(t *testing.T) {
			html := swapExtension(orig, ".md", ".html")
			if back := swapExtension(html, ".html", ".md"); back != orig {
				t.Fatalf("round trip through %q = %q, want %q", html, back, orig)
			}
		})
	}
}

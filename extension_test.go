package zas

import (
	"os"
	"path/filepath"
	"testing"
)

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
		{"uppercase extension matches and is normalized to to's own case", "PAGE.MD", ".md", ".html", "PAGE.html"},
		{"mixed-case extension matches too", "page.Md", ".md", ".html", "page.html"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := swapExtension(c.path, c.from, c.to); got != c.want {
				t.Fatalf("swapExtension(%q, %q, %q) = %q, want %q", c.path, c.from, c.to, got, c.want)
			}
		})
	}
}

// renderAsync's dispatch switch used to match ".md"/".html" with
// strings.HasSuffix, case-sensitively, so a README.MD or page.Md source
// fell through to the copy branch and shipped into deploy verbatim,
// unrendered, keeping its original (unrecognized) extension. Both it and
// swapExtension now share hasExtension for a single, consistent notion of
// "does this path have extension X".
func TestHasExtension(t *testing.T) {
	cases := []struct {
		name string
		path string
		ext  string
		want bool
	}{
		{"exact case match", "page.md", ".md", true},
		{"all-uppercase extension matches", "PAGE.MD", ".md", true},
		{"mixed-case extension matches", "page.Md", ".md", true},
		{"non-matching extension", "page.html", ".md", false},
		{"extension-like mid-name segment doesn't count", "v1.mdx", ".md", false},
		{"path shorter than the extension", ".m", ".md", false},
		{"empty path", "", ".md", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := hasExtension(c.path, c.ext); got != c.want {
				t.Fatalf("hasExtension(%q, %q) = %v, want %v", c.path, c.ext, got, c.want)
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

// A case-varied extension deliberately does not round-trip: swapExtension
// always produces to's own casing, so PAGE.MD's deploy path is PAGE.html,
// not PAGE.HTML, and swapping that back yields PAGE.md, not the original
// PAGE.MD. This gives every consumer (walk, sourceIsNewer, reaper,
// NewZasData) one single, predictable extension casing to agree on,
// instead of each needing to separately preserve or normalize whatever
// casing the source file happened to use.
func TestSwapExtensionNormalizesCase(t *testing.T) {
	if got, want := swapExtension("PAGE.MD", ".md", ".html"), "PAGE.html"; got != want {
		t.Fatalf("swapExtension(%q, ...) = %q, want %q", "PAGE.MD", got, want)
	}
}

// reaper reconstructs a candidate source path from a deploy path via
// swapExtension, which - per TestSwapExtensionNormalizesCase above - always
// normalizes the guessed extension's own casing. Since a real source's
// casing might not be that guess (e.g. PAGE.MD's guessed source is
// "page.md", not "PAGE.MD"), reaper needs a case-insensitive existence
// check, not a plain os.Open on the exact guessed path.
func TestGeneratorExistsFold(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "PAGE.MD"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	gen := &Generator{}
	if !gen.existsFold(filepath.Join(dir, "page.md")) {
		t.Fatalf("existsFold(%q) = false, want true (PAGE.MD exists, compared case-insensitively)", filepath.Join(dir, "page.md"))
	}
	if gen.existsFold(filepath.Join(dir, "other.md")) {
		t.Fatal("existsFold(...) = true for a file that doesn't exist, want false")
	}
}

// existsFold caches each directory's entries in dirEntriesFoldCache after
// its first read, rather than re-reading and re-scanning the same
// directory listing on every call - otherwise a directory with many files
// pays for one full directory read per file instead of one total. This
// pins the caching itself: a file created after the first existsFold call
// for its directory must not suddenly become visible within the same
// Generator, since the whole point is that the directory is read only
// once per reap walk.
func TestGeneratorExistsFoldCachesDirectoryListing(t *testing.T) {
	dir := t.TempDir()
	gen := &Generator{}
	if gen.existsFold(filepath.Join(dir, "page.md")) {
		t.Fatal("existsFold(...) = true before the file was ever created")
	}
	if err := os.WriteFile(filepath.Join(dir, "page.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if gen.existsFold(filepath.Join(dir, "page.md")) {
		t.Fatal("existsFold(...) = true after the file appeared, want the cached (pre-creation) listing still in effect")
	}
}

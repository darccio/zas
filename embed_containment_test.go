package zas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// <embed src="..."> was read with os.ReadFile(src) and no check that the
// result stayed inside the site root, so an absolute src or a "../"
// traversal in site content could pull the contents of any file the build
// process can read into the published output. resolveEmbedSrc closes that
// off for all three embed handlers (Markdown, Plain, Html); these tests
// cover both escape shapes for each handler, and confirm a legitimate
// embed of a file in a site subdirectory still works.

type embedHandlerCase struct {
	name    string
	typ     string
	relPath string
	content string
	call    func(*Generator, *goquery.Selection, *goquery.Document, *ZasData) error
	want    string
}

var embedHandlerCases = []embedHandlerCase{
	{"Markdown", "text/markdown", "note.md", "# Note\n\nHello from markdown.\n", (*Generator).Markdown, "Hello from markdown."},
	{"Plain", "text/plain", "note.txt", "hello from plain", (*Generator).Plain, "hello from plain"},
	{"Html", "text/html", "note.html", "<p>hello from html</p>", (*Generator).Html, "hello from html"},
}

func embedDocFor(t *testing.T, src, typ string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<div><embed src="` + src + `" type="` + typ + `"></div>`))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestEmbedHandlersRejectAbsolutePathOutsideRoot(t *testing.T) {
	for _, tc := range embedHandlerCases {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			root := filepath.Join(base, "site")
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Fatal(err)
			}
			secret := filepath.Join(base, tc.relPath)
			if err := os.WriteFile(secret, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Chdir(root)

			doc := embedDocFor(t, secret, tc.typ)
			gen := &Generator{}
			err := tc.call(gen, doc.Find("embed"), doc, &ZasData{})
			if err == nil {
				t.Fatalf("%s() with an absolute src outside the site root: want error, got nil", tc.name)
			}
			if doc.Find("embed").Length() != 1 {
				t.Fatalf("%s() rejected the embed but still consumed the <embed> tag", tc.name)
			}
		})
	}
}

func TestEmbedHandlersRejectParentTraversal(t *testing.T) {
	for _, tc := range embedHandlerCases {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			root := filepath.Join(base, "site")
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(base, tc.relPath), []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Chdir(root)

			traversal := filepath.Join("..", tc.relPath)
			doc := embedDocFor(t, traversal, tc.typ)
			gen := &Generator{}
			err := tc.call(gen, doc.Find("embed"), doc, &ZasData{})
			if err == nil {
				t.Fatalf(`%s() with a src escaping the site root via "../": want error, got nil`, tc.name)
			}
			if doc.Find("embed").Length() != 1 {
				t.Fatalf("%s() rejected the embed but still consumed the <embed> tag", tc.name)
			}
		})
	}
}

func TestEmbedHandlersAllowNestedSubdirectory(t *testing.T) {
	for _, tc := range embedHandlerCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if err := os.Mkdir("partials", 0o755); err != nil {
				t.Fatal(err)
			}
			relPath := filepath.Join("partials", tc.relPath)
			if err := os.WriteFile(relPath, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}

			doc := embedDocFor(t, relPath, tc.typ)
			gen := &Generator{}
			if err := tc.call(gen, doc.Find("embed"), doc, &ZasData{}); err != nil {
				t.Fatalf("%s() for a file in a site subdirectory: error = %v, want nil", tc.name, err)
			}
			if got := doc.Find("div").Text(); !strings.Contains(got, tc.want) {
				t.Fatalf("%s() output = %q, want it to contain %q", tc.name, got, tc.want)
			}
		})
	}
}

// TestGenerateRejectsEmbedEscapingSiteRootViaAbsolutePath and
// TestGenerateRejectsEmbedEscapingSiteRootViaTraversal drive the full
// Generator.Run pipeline (as opposed to calling a handler method
// directly) to confirm a rejected embed surfaces as an ordinary per-file
// generation error - the same path any other render failure already takes
// - rather than a panic or a silent success with leaked file content in
// the deploy output.

func TestGenerateRejectsEmbedEscapingSiteRootViaAbsolutePath(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "site")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	copyFixture(t, "site", root)
	t.Chdir(root)
	saveGlobals(t)

	secret := filepath.Join(base, "secret.txt")
	if err := os.WriteFile(secret, []byte("do not leak"), 0o644); err != nil {
		t.Fatal(err)
	}
	leak := `<embed src="` + secret + `" type="text/plain">` + "\n"
	if err := os.WriteFile("leak.html", []byte(leak), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := generate(t); err == nil {
		t.Fatal("generate() with a page embedding an absolute path outside the site root: want error, got nil")
	}
	assertDeployMissing(t, "leak.html")
}

func TestGenerateRejectsEmbedEscapingSiteRootViaTraversal(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "site")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	copyFixture(t, "site", root)
	t.Chdir(root)
	saveGlobals(t)

	if err := os.WriteFile(filepath.Join(base, "secret.txt"), []byte("do not leak"), 0o644); err != nil {
		t.Fatal(err)
	}
	leak := `<embed src="../secret.txt" type="text/plain">` + "\n"
	if err := os.WriteFile("leak.html", []byte(leak), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := generate(t); err == nil {
		t.Fatal(`generate() with a page embedding "../secret.txt": want error, got nil`)
	}
	assertDeployMissing(t, "leak.html")
}

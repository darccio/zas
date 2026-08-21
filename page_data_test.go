package zas

import (
	thtml "html/template"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	ttext "text/template"

	"github.com/melvinmt/gt"
)

// text/template only exposes pointer-receiver methods (Title, URL, E, H,
// IsHome, Resolve, Extra) on an addressable value. render used to pass
// ZasData by value to the page template, so every one of those methods
// failed template execution outright when called from a page body - even
// though the identical expression works in the layout, which does receive a
// pointer.

func TestZasDataPointerMethodsNeedAddressableValue(t *testing.T) {
	tmpl, err := ttext.New("t").Parse(`{{.URL}}`)
	if err != nil {
		t.Fatal(err)
	}
	zd := ZasData{Path: "/index.html"}

	if err := tmpl.Execute(io.Discard, zd); err == nil {
		t.Fatal("Execute() against a value: want error, got nil")
	}
	if err := tmpl.Execute(io.Discard, &zd); err != nil {
		t.Fatalf("Execute() against a pointer: error = %v, want nil", err)
	}
}

// NewZasData builds data.Path from filepath.Walk's own path argument,
// which uses the OS's native separator - "/" on Unix, "\" on Windows. Since
// data.Path becomes a URL, not a filesystem path, it must always use "/"
// regardless of platform: a nested page's URL containing a literal
// backslash would be wrong, and IsHome's own hardcoded "/"-separated
// comparisons further down would never match a language-prefixed home
// page's path if it had backslashes in it instead.
//
// filepath.ToSlash is a documented no-op wherever the OS separator is
// already "/" - true for every platform this test suite actually runs on
// - so this only pins that ordinary Unix-style input is unaffected; the
// backslash-to-slash conversion itself only ever executes on a real
// Windows build, which isn't something this repo's test suite can
// exercise directly. GOOS=windows go build/vet at least confirm the
// change compiles and type-checks for that target.
func TestNewZasDataBuildsForwardSlashPath(t *testing.T) {
	gen := &Generator{
		Config: ConfigSection{},
		I18n:   &gt.Build{Index: gt.Strings{}, Origin: "en"},
	}
	data := NewZasData(filepath.Join("sub", "page.html"), gen)
	if want := "/sub/page.html"; data.Path != want {
		t.Fatalf("NewZasData(...).Path = %q, want %q", data.Path, want)
	}
}

func TestRenderPageBodyCanCallPointerReceiverMethod(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("deploy", 0o755); err != nil {
		t.Fatal(err)
	}
	gen := &Generator{
		Config: ConfigSection{
			"zas":  ConfigSection{"deploy": "deploy"},
			"site": ConfigSection{"baseurl": "http://example.com"},
		},
		I18n: &gt.Build{Index: gt.Strings{}, Origin: "en"},
	}
	layout, err := thtml.New("layout").Funcs(helpers).Parse(`{{.Body}}`)
	if err != nil {
		t.Fatal(err)
	}
	gen.Layout = layout

	if err := gen.render("page.html", []byte(`<body>{{.URL}}</body>`)); err != nil {
		t.Fatalf("render() error = %v, want nil", err)
	}

	out, err := os.ReadFile(filepath.Join("deploy", "page.html"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "http://example.com/page.html"; !strings.Contains(string(out), want) {
		t.Fatalf("deployed output = %q, want it to contain %q", out, want)
	}
}

// extractPageConfig used to only look at the document's literal first
// child, so a leading "<!DOCTYPE html>" (or anything else parsed as a
// sibling ahead of the comment) pushed the config comment out of that slot
// and its config was silently dropped - the page fell back to its <h1> text
// as the title instead of erroring. These two tests render otherwise
// identical pages, one with a leading doctype and one without, and confirm
// both pick up the title override from their config comment rather than
// falling back to the <h1>.

func TestExtractPageConfigFoundAfterLeadingDoctype(t *testing.T) {
	testExtractPageConfigTitleOverride(t, "<!DOCTYPE html>\n<!-- title: Overridden -->\n<h1>Ignored</h1>")
}

func TestExtractPageConfigFoundAsFirstLine(t *testing.T) {
	testExtractPageConfigTitleOverride(t, "<!-- title: Overridden -->\n<h1>Ignored</h1>")
}

func testExtractPageConfigTitleOverride(t *testing.T, source string) {
	t.Helper()
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("deploy", 0o755); err != nil {
		t.Fatal(err)
	}
	gen := &Generator{
		Config: ConfigSection{
			"zas": ConfigSection{"deploy": "deploy"},
		},
		I18n: &gt.Build{Index: gt.Strings{}, Origin: "en"},
	}
	layout, err := thtml.New("layout").Funcs(helpers).Parse(`{{.Title}}`)
	if err != nil {
		t.Fatal(err)
	}
	gen.Layout = layout

	if err := gen.render("page.html", []byte(source)); err != nil {
		t.Fatalf("render() error = %v, want nil", err)
	}

	out, err := os.ReadFile(filepath.Join("deploy", "page.html"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if !strings.Contains(got, "Overridden") {
		t.Fatalf("deployed output = %q, want it to contain the config comment's title %q", got, "Overridden")
	}
	if strings.Contains(got, "Ignored") {
		t.Fatalf("deployed output = %q, want it to not fall back to the <h1> title %q", got, "Ignored")
	}
}

// render used to populate data.Page and data.FirstTitle only after
// executing the page's own template - both are derived from that very
// execution's HTML5-parsed output (see extractPageConfig/getTitle in
// generate.go) - so {{.Title}}, {{.Page}}, and {{.FirstTitle}} always
// evaluated empty when used from inside a page's own body content, even
// though the identical expression works fine in the layout. These tests
// cover the best-effort early preview (earlyPageConfig/leadingH1Text in
// generate.go, unit-tested directly in raw_page_test.go) that now gives
// the page body a working view of each, the guard against a
// self-referential <h1>{{.Title}}</h1> heading, and confirm the layout's
// own canonical view stays exactly as before regardless of what the page
// body saw.

func TestRenderPageBodyCanReadTitleFromOwnConfigComment(t *testing.T) {
	gen := newRenderTestGenerator(t)
	src := "<!-- title: Custom Title -->\n<p>{{.Title}}</p>"

	if err := gen.render("page.html", []byte(src)); err != nil {
		t.Fatalf("render() error = %v, want nil", err)
	}
	out := readRenderedFile(t, "page.html")
	if !strings.Contains(out, "<p>Custom Title</p>") {
		t.Fatalf("deploy output = %q, want the page body's own {{.Title}} to resolve to %q instead of empty", out, "Custom Title")
	}
}

func TestRenderPageBodyCanIndexPageMapDirectly(t *testing.T) {
	gen := newRenderTestGenerator(t)
	src := "<!-- greeting: hello -->\n" + `<p>{{index .Page "greeting"}}</p>`

	if err := gen.render("page.html", []byte(src)); err != nil {
		t.Fatalf("render() error = %v, want nil", err)
	}
	out := readRenderedFile(t, "page.html")
	if !strings.Contains(out, "<p>hello</p>") {
		t.Fatalf(`deploy output = %q, want the page body's own {{index .Page "greeting"}} to resolve to %q instead of empty`, out, "hello")
	}
}

func TestRenderPageBodyCanReadFirstTitle(t *testing.T) {
	gen := newRenderTestGenerator(t)
	src := "<h1>Heading Text</h1>\n<p>{{.FirstTitle}}</p>"

	if err := gen.render("page.html", []byte(src)); err != nil {
		t.Fatalf("render() error = %v, want nil", err)
	}
	out := readRenderedFile(t, "page.html")
	if !strings.Contains(out, "<h1>Heading Text</h1>") {
		t.Fatalf("deploy output = %q, want the literal heading preserved", out)
	}
	if !strings.Contains(out, "<p>Heading Text</p>") {
		t.Fatalf("deploy output = %q, want the page body's own {{.FirstTitle}} to resolve to %q instead of empty", out, "Heading Text")
	}
}

// A heading that shows its own resolved title - <h1>{{.Title}}</h1> - is a
// natural pattern, but a raw pre-execution scan for the page's own H1 would
// otherwise capture the literal, unexecuted string "{{.Title}}" and hand it
// back as FirstTitle; since Title() falls back to FirstTitle, that string
// would then circularly reappear as the page's own "resolved" title.
// leadingH1Text guards against this (see TestLeadingH1Text in
// raw_page_test.go for the guard itself); this test confirms render's own
// pipeline actually benefits from it end to end.
func TestRenderFirstTitleGuardsAgainstSelfReferentialHeading(t *testing.T) {
	gen := newRenderTestGenerator(t)
	src := "<h1>{{.Title}}</h1>"

	if err := gen.render("page.html", []byte(src)); err != nil {
		t.Fatalf("render() error = %v, want nil", err)
	}
	out := readRenderedFile(t, "page.html")
	if strings.Contains(out, "{{.Title}}") {
		t.Fatalf("deploy output = %q, want no literal %q leaking out of the self-referential heading", out, "{{.Title}}")
	}
	if !strings.Contains(out, "<h1></h1>") {
		t.Fatalf("deploy output = %q, want an empty <h1></h1> (no page-config title override, FirstTitle unavailable) rather than a circular value", out)
	}
}

// The layout executes later, in Generate, using the canonical
// data.Title()/data.Page/data.FirstTitle that extractPageConfig and
// getTitle populate from the fully rendered, HTML5-parsed document - not
// the page body's own early, best-effort preview (render's later,
// unconditional extractPageConfig/getTitle calls overwrite both after the
// page's own template has already executed). This confirms both views
// agree in the normal case: the layout keeps seeing the exact same,
// correct value the page body's own {{.Title}} now also resolves to,
// i.e. the fix is additive and doesn't change the layout's own output.
func TestRenderLayoutTitleUnaffectedByPageBodyPreview(t *testing.T) {
	gen := newRenderTestGeneratorWithTitleLayout(t)
	src := "<!-- title: Shared -->\n<p>{{.Title}}</p>"

	if err := gen.render("page.html", []byte(src)); err != nil {
		t.Fatalf("render() error = %v, want nil", err)
	}
	out := readRenderedFile(t, "page.html")
	if !strings.Contains(out, "<title>Shared</title>") {
		t.Fatalf("deploy output = %q, want the layout's own {{.Title}} to still resolve to %q", out, "Shared")
	}
	if !strings.Contains(out, "<p>Shared</p>") {
		t.Fatalf("deploy output = %q, want the page body's own {{.Title}} to also resolve to %q", out, "Shared")
	}
}

// Body is, by definition, this very execution's own output - there is no
// way to preview a page's final rendered self ahead of rendering it, unlike
// Page and FirstTitle above (see the doc comment on ZasData.Body in
// data.go). {{.Body}} inside a page's own body content is expected to keep
// evaluating empty; this pins that intentional, documented limitation
// rather than a fix.
func TestRenderBodyStaysEmptyInsideOwnPageBody(t *testing.T) {
	gen := newRenderTestGenerator(t)
	src := "<p>before</p><span>{{.Body}}</span><p>after</p>"

	if err := gen.render("page.html", []byte(src)); err != nil {
		t.Fatalf("render() error = %v, want nil", err)
	}
	out := readRenderedFile(t, "page.html")
	if !strings.Contains(out, "<span></span>") {
		t.Fatalf("deploy output = %q, want {{.Body}} inside the page's own body to evaluate empty (<span></span>)", out)
	}
}

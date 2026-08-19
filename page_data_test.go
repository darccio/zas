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

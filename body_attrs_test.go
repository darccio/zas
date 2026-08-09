package zas

import (
	thtml "html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/melvinmt/gt"
)

// data.Body carries only a source page's inner HTML, not its <body>
// element, since nesting a full <body> inside the layout's own <body>
// produced doubled elements. That means any attributes on the source's own
// <body> tag (e.g. a per-page id or data attribute) need to be merged onto
// the layout's <body> explicitly, or they're silently lost. The layout's
// own attributes win on a colliding key.

func TestRenderPreservesSourceBodyAttributes(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("deploy", 0o755); err != nil {
		t.Fatal(err)
	}
	gen := &Generator{
		Config: ConfigSection{"zas": ConfigSection{"deploy": "deploy"}},
		I18n:   &gt.Build{Index: gt.Strings{}, Origin: "en"},
	}
	layout, err := thtml.New("layout").Funcs(helpers).Parse(`<body class="layout">{{.Body}}</body>`)
	if err != nil {
		t.Fatal(err)
	}
	gen.Layout = layout

	src := `<body id="special" data-x="1" class="source">content</body>`
	if err := gen.render("page.html", []byte(src), nil); err != nil {
		t.Fatalf("render() error = %v, want nil", err)
	}

	out, err := os.ReadFile(filepath.Join("deploy", "page.html"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, want := range []string{`id="special"`, `data-x="1"`, `class="layout"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want it to contain %q", got, want)
		}
	}
	if strings.Contains(got, `class="source"`) {
		t.Fatalf("output = %q, want the layout's class to win over the source's", got)
	}
}

func TestRenderWithoutSourceBodyAttributesLeavesLayoutUnchanged(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("deploy", 0o755); err != nil {
		t.Fatal(err)
	}
	gen := &Generator{
		Config: ConfigSection{"zas": ConfigSection{"deploy": "deploy"}},
		I18n:   &gt.Build{Index: gt.Strings{}, Origin: "en"},
	}
	layout, err := thtml.New("layout").Funcs(helpers).Parse(`<body class="layout">{{.Body}}</body>`)
	if err != nil {
		t.Fatal(err)
	}
	gen.Layout = layout

	if err := gen.render("page.html", []byte(`<body>content</body>`), nil); err != nil {
		t.Fatalf("render() error = %v, want nil", err)
	}

	out, err := os.ReadFile(filepath.Join("deploy", "page.html"))
	if err != nil {
		t.Fatal(err)
	}
	if want := `class="layout"`; !strings.Contains(string(out), want) {
		t.Fatalf("output = %q, want it to contain %q", out, want)
	}
}

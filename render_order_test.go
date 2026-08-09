package zas

import (
	thtml "html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/melvinmt/gt"
)

// The Go template pass must run over a Markdown page's *source*, before
// Markdown conversion - not over the already-converted HTML. Otherwise
// Markdown syntax coming out of a template action (e.g. a translated
// string) reaches the reader as literal text instead of being rendered,
// since Goldmark never gets a chance to see it.

func TestRenderMarkdownExecutesTemplateBeforeConversion(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("deploy", 0o755); err != nil {
		t.Fatal(err)
	}
	gen := &Generator{
		Config: ConfigSection{
			"zas":  ConfigSection{"deploy": "deploy"},
			"site": ConfigSection{"language": "en"},
		},
		I18n: &gt.Build{
			Index:  gt.Strings{"shout": {"en": "**important**"}},
			Origin: "en",
		},
	}
	layout, err := thtml.New("layout").Funcs(helpers).Parse(`{{.Body}}`)
	if err != nil {
		t.Fatal(err)
	}
	gen.Layout = layout

	if err := gen.render("page.md", []byte(`{{.E "shout"}} today`), markdownToHTML); err != nil {
		t.Fatalf("render() error = %v, want nil", err)
	}

	out, err := os.ReadFile(filepath.Join("deploy", "page.html"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "<strong>important</strong>"; !strings.Contains(string(out), want) {
		t.Fatalf("deployed output = %q, want the translated text converted as Markdown (%q)", out, want)
	}
}

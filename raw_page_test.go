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
	thtml "html/template"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/melvinmt/gt"
)

// utf16leBOM prepends a UTF-16LE byte-order mark and encodes s as
// UTF-16LE, reproducing the shape go.yaml.in/yaml transcodes internally -
// see the "utf-16le comment body still opts out" case below and
// mayDefineTemplateKey.
func utf16leBOM(s string) string {
	out := []byte("\xff\xfe")
	for _, r := range s {
		out = append(out, byte(r), 0)
	}
	return string(out)
}

// Every page used to be unconditionally parsed and executed as a
// text/template, with no way to opt out: a file containing literal {{ }}
// that isn't valid template syntax (e.g. `{{ message }}`, documenting Zas's
// own template syntax, or a Vue/Angular/Handlebars snippet using the same
// delimiters) failed the whole page with a template parse/execute error,
// and one that happened to parse as valid Go template syntax was silently
// executed against live data instead of shown literally. These tests cover
// pageOptsOutOfTemplating directly, then render() and the full Generator.Run
// pipeline for the "template: false" opt-out it implements.

func TestPageOptsOutOfTemplating(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "no comment at all",
			input: "<h1>Hi</h1>",
			want:  false,
		},
		{
			name:  "comment without a template key",
			input: "<!-- title: Hi -->\n<h1>Hi</h1>",
			want:  false,
		},
		{
			name:  "template explicitly true",
			input: "<!-- template: true -->\n<h1>Hi</h1>",
			want:  false,
		},
		{
			name:  "template false",
			input: "<!-- template: false -->\n<h1>Hi</h1>",
			want:  true,
		},
		{
			name:  "template false alongside other keys",
			input: "<!--\ntitle: Hi\ntemplate: false\n-->\n<h1>Hi</h1>",
			want:  true,
		},
		{
			name:  "template false past a leading doctype",
			input: "<!DOCTYPE html>\n<!-- template: false -->\n<h1>Hi</h1>",
			want:  true,
		},
		{
			name:  "template false past leading whitespace",
			input: "\n\n  <!-- template: false -->\n<h1>Hi</h1>",
			want:  true,
		},
		{
			name:  "malformed YAML comment is not an opt-out",
			input: "<!-- language: ru\n\tbad: true\n-->\n<h1>Hi</h1>",
			want:  false,
		},
		{
			name:  "template false later in the document doesn't count",
			input: "<h1>Hi</h1>\n<!-- template: false -->",
			want:  false,
		},
		// The remaining cases pin mayDefineTemplateKey's guards: each one
		// is a real go.yaml.in/yaml construct that can decode to a
		// "template" key without spelling the literal bytes "template" in
		// the comment, found while proving the fast path can't just check
		// for those bytes and a backslash. Deleting the '!' or BOM guard
		// would make the "still opts out" cases below silently stop
		// opting out.
		{
			name:  "!!binary-tagged key still opts out",
			input: "<!-- !!binary dGVtcGxhdGU=: false -->\n<h1>Hi</h1>",
			want:  true,
		},
		{
			name:  "double-quoted escaped key still opts out",
			input: `<!-- "\x74emplate": false -->` + "\n<h1>Hi</h1>",
			want:  true,
		},
		{
			name:  "utf-16le comment body still opts out",
			input: "<!--" + utf16leBOM("template: false") + "-->\n<h1>Hi</h1>",
			want:  true,
		},
		{
			name:  "capitalized Template is not an opt-out",
			input: "<!-- Template: false -->\n<h1>Hi</h1>",
			want:  false,
		},
		{
			name:  "template only inside a value stays an opt-in",
			input: "<!-- title: Template gallery -->\n<h1>Hi</h1>",
			want:  false,
		},
		{
			name:  "exclamation mark in an unrelated value",
			input: "<!-- title: Hi! -->\n<h1>Hi</h1>",
			want:  false,
		},
		{
			name:  "backslash in an unrelated value",
			input: `<!-- title: C:\docs -->` + "\n<h1>Hi</h1>",
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pageOptsOutOfTemplating([]byte(tt.input)); got != tt.want {
				t.Errorf("pageOptsOutOfTemplating(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// Without the opt-out, a page containing {{ message }} - the exact
// reproduction case: text that looks like a template action but isn't one
// Zas defines - fails the whole render with a template parse/execute error.
func TestRenderWithoutMarkerFailsOnUndefinedTemplateFunction(t *testing.T) {
	gen := newRenderTestGenerator(t)
	src := "<p>{{ message }}</p>"

	if err := gen.render("page.html", []byte(src)); err == nil {
		t.Fatal("render() error = nil, want a template error for undefined function \"message\"")
	}
}

// With the opt-out, the same content renders successfully and the literal
// {{ message }} text survives unexecuted into the deploy output.
func TestRenderWithMarkerSkipsTemplatingAndKeepsLiteralBraces(t *testing.T) {
	gen := newRenderTestGenerator(t)
	src := "<!-- template: false -->\n<p>{{ message }}</p>"

	if err := gen.render("page.html", []byte(src)); err != nil {
		t.Fatalf("render() error = %v, want nil", err)
	}
	out := readRenderedFile(t, "page.html")
	if !strings.Contains(out, "{{ message }}") {
		t.Fatalf("deploy output = %q, want it to contain the literal %q", out, "{{ message }}")
	}
}

// A page without the marker keeps being templated exactly as before: this
// is the regression check that the opt-out doesn't disable templating
// globally.
func TestRenderWithoutMarkerStillExecutesTemplate(t *testing.T) {
	gen := newRenderTestGenerator(t)
	src := "<p>{{.Path}}</p>"

	if err := gen.render("page.html", []byte(src)); err != nil {
		t.Fatalf("render() error = %v, want nil", err)
	}
	out := readRenderedFile(t, "page.html")
	if !strings.Contains(out, "/page.html") {
		t.Fatalf("deploy output = %q, want {{.Path}} substituted with /page.html", out)
	}
	if strings.Contains(out, "{{.Path}}") {
		t.Fatalf("deploy output = %q, want no literal {{.Path}} left over", out)
	}
}

// A raw page's other page-config fields (e.g. title) still work: only
// text/template execution is skipped, extractPageConfig and the rest of the
// pipeline run exactly as for any other page.
func TestRenderWithMarkerStillAppliesPageTitle(t *testing.T) {
	gen := newRenderTestGeneratorWithTitleLayout(t)
	// {{ undefinedFunc }} in the body proves templating was actually
	// skipped (it would fail template.Parse otherwise), while title still
	// comes from the very same config comment via extractPageConfig.
	src := "<!--\ntitle: Custom\ntemplate: false\n-->\n<h1>{{ undefinedFunc }}</h1>"

	if err := gen.render("page.html", []byte(src)); err != nil {
		t.Fatalf("render() error = %v, want nil", err)
	}
	out := readRenderedFile(t, "page.html")
	if !strings.Contains(out, "<title>Custom</title>") {
		t.Fatalf("deploy output = %q, want it to contain %q", out, "<title>Custom</title>")
	}
	if !strings.Contains(out, "{{ undefinedFunc }}") {
		t.Fatalf("deploy output = %q, want the literal %q preserved", out, "{{ undefinedFunc }}")
	}
}

// readRenderedFile reads rel from the "deploy" directory newRenderTestGenerator
// and newRenderTestGeneratorWithTitleLayout write into.
func readRenderedFile(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("deploy", rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// newRenderTestGeneratorWithTitleLayout is like newRenderTestGenerator, but
// its layout also renders {{.Title}}, so a test can confirm a page's own
// title config comment still reaches the deploy output.
func newRenderTestGeneratorWithTitleLayout(t *testing.T) *Generator {
	t.Helper()
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("deploy", 0o755); err != nil {
		t.Fatal(err)
	}
	gen := &Generator{
		Config: ConfigSection{"zas": ConfigSection{"deploy": "deploy"}},
		I18n:   &gt.Build{Index: gt.Strings{}, Origin: "en"},
	}
	layout, err := thtml.New("layout").Funcs(helpers).Parse(`<head><title>{{.Title}}</title></head><body>{{.Body}}</body>`)
	if err != nil {
		t.Fatal(err)
	}
	gen.Layout = layout
	return gen
}

// TestGenerateRawPageSkipsTemplating drives the full Generator.Run pipeline
// against testdata/site/raw.md, the exact reproduction case: a fenced code
// block containing literal {{ message }}. It must now build successfully
// (it used to fail the whole file with a template parse error) with the
// literal braces preserved in the deploy output, while the rest of the site
// - including pages that still rely on real template execution - is
// unaffected.
func TestGenerateRawPageSkipsTemplating(t *testing.T) {
	newTestSite(t, "site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	out := readDeploy(t, "raw.html")
	if !strings.Contains(out, "{{ message }}") {
		t.Fatalf("raw.html = %q, want it to contain the literal %q", out, "{{ message }}")
	}
	if !strings.Contains(out, "<title>Raw</title>") {
		t.Fatalf("raw.html = %q, want it to contain %q (page config still applied)", out, "<title>Raw</title>")
	}

	// A normal page's real template syntax - the layout's own {{.Title}} -
	// must still be executed, confirming the opt-out is scoped to raw.md
	// and doesn't disable templating globally.
	index := readDeploy(t, "index.html")
	if !strings.Contains(index, "<title>Home</title>") {
		t.Fatalf("index.html = %q, want %q (unrelated page still templated normally)", index, "<title>Home</title>")
	}
}

// earlyPageConfig and leadingH1Text are the raw, pre-execution
// counterparts render uses to give a page's own body a best-effort preview
// of {{.Page}}/{{.Title}} and {{.FirstTitle}}, mirroring
// leadingConfigComment's approach for the "template: false" opt-out above.
// Both are covered directly here, then through render() and the H1
// circularity guard in page_data_test.go.

func TestEarlyPageConfig(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  map[interface{}]interface{}
	}{
		{
			name:  "no comment at all",
			input: "<h1>Hi</h1>",
			want:  nil,
		},
		{
			name:  "simple title key",
			input: "<!-- title: Hi -->\n<h1>Hi</h1>",
			want:  map[interface{}]interface{}{"title": "Hi"},
		},
		{
			name:  "multiple keys",
			input: "<!--\ntitle: Hi\nlanguage: en\n-->\n<h1>Hi</h1>",
			want:  map[interface{}]interface{}{"title": "Hi", "language": "en"},
		},
		{
			name:  "found past a leading doctype",
			input: "<!DOCTYPE html>\n<!-- title: Hi -->\n<h1>Hi</h1>",
			want:  map[interface{}]interface{}{"title": "Hi"},
		},
		{
			name:  "malformed YAML comment yields nil, not an error",
			input: "<!-- language: ru\n\tbad: true\n-->\n<h1>Hi</h1>",
			want:  nil,
		},
		{
			name:  "comment later in the document doesn't count",
			input: "<h1>Hi</h1>\n<!-- title: Hi -->",
			want:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := earlyPageConfig([]byte(tt.input))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("earlyPageConfig(%q) = %#v, want %#v", tt.input, got, tt.want)
			}
		})
	}
}

func TestLeadingH1Text(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantText  string
		wantFound bool
	}{
		{
			name:      "no h1 at all",
			input:     "<p>Hi</p>",
			wantFound: false,
		},
		{
			name:      "simple h1",
			input:     "<h1>Hello</h1>",
			wantText:  "Hello",
			wantFound: true,
		},
		{
			name:      "h1 with attributes",
			input:     `<h1 class="title" id="top">Hello</h1>`,
			wantText:  "Hello",
			wantFound: true,
		},
		{
			name:      "only the first h1 counts",
			input:     "<h1>First</h1><h1>Second</h1>",
			wantText:  "First",
			wantFound: true,
		},
		{
			name:      "surrounding whitespace is trimmed",
			input:     "<h1>\n  Hello  \n</h1>",
			wantText:  "Hello",
			wantFound: true,
		},
		{
			name:      "self-referential title is refused",
			input:     "<h1>{{.Title}}</h1>",
			wantFound: false,
		},
		{
			name:      "self-referential FirstTitle is refused",
			input:     "<h1>{{.FirstTitle}}</h1>",
			wantFound: false,
		},
		{
			name:      "a template action alongside real text is still refused",
			input:     "<h1>Welcome, {{.Path}}</h1>",
			wantFound: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := leadingH1Text([]byte(tt.input))
			if ok != tt.wantFound {
				t.Fatalf("leadingH1Text(%q) ok = %v, want %v", tt.input, ok, tt.wantFound)
			}
			if ok && got != tt.wantText {
				t.Errorf("leadingH1Text(%q) = %q, want %q", tt.input, got, tt.wantText)
			}
		})
	}
}

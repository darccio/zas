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
	"strings"
	"testing"

	"github.com/melvinmt/gt"
)

// largeBody is the body every "large" case below shares: big enough that a
// full-input scan is measurable, and identical across cases so the only
// variable being benchmarked is the leading comment.
var largeBody = "<h1>Hi</h1>\n" + strings.Repeat("<p>filler paragraph content</p>\n", 2000)

// pageOptsOutOfTemplatingInputs are representative render() inputs, a slice
// (not a map) so benchstat output has a stable order to diff against.
// pageOptsOutOfTemplating runs once per source file on every render() call,
// so it needs to stay cheap regardless of what its comment looks like or
// how big the rest of the page is.
var pageOptsOutOfTemplatingInputs = []struct {
	name  string
	input string
}{
	// No leading comment: leadingConfigComment rejects on the very first
	// byte, so neither the key scan nor yaml ever runs.
	{"no_comment", "<h1>Hi</h1>"},
	// The ordinary case every ordinary page hits: a comment that sets
	// title/language but never mentions "template" - this is the shape
	// mayDefineTemplateKey exists to keep off the yaml.Unmarshal path.
	{"comment_without_template_key", "<!-- title: Hi -->\n<h1>Hi</h1>"},
	// A comment that really does carry the key, so the scan can't rule it
	// out and yaml still has to run - the case this change doesn't speed
	// up, benchmarked so a regression there would show.
	{"comment_with_template_key", "<!-- template: false -->\n<h1>Hi</h1>"},
	// A backslash defeats the scan's guard even with no "template" key
	// present, so this pays a full yaml parse it didn't strictly need -
	// the deliberate, documented cost of keeping the scan provably exact.
	{"comment_with_backslash", `<!-- title: C:\docs -->` + "\n<h1>Hi</h1>"},
	// Same shape as comment_without_template_key, but with a large body
	// after the comment: leadingConfigComment stops at the first "-->",
	// so this should cost about the same as the small case, not scale
	// with the body.
	{"large_body_without_template_key", "<!-- title: Hi -->\n" + largeBody},
}

func BenchmarkPageOptsOutOfTemplating(b *testing.B) {
	for _, tt := range pageOptsOutOfTemplatingInputs {
		in := []byte(tt.input)
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				pageOptsOutOfTemplating(in)
			}
		})
	}
}

// newRenderBenchGenerator is newRenderTestGenerator's benchmark
// counterpart: testing.B embeds the same TempDir/Chdir support as
// testing.T, so this builds an identical minimal Generator without
// depending on a *testing.T.
func newRenderBenchGenerator(b *testing.B) *Generator {
	b.Helper()
	b.Chdir(b.TempDir())
	if err := os.MkdirAll("deploy", 0o755); err != nil {
		b.Fatal(err)
	}
	gen := &Generator{
		Config: ConfigSection{"zas": ConfigSection{"deploy": "deploy"}},
		I18n:   &gt.Build{Index: gt.Strings{}, Origin: "en"},
	}
	layout, err := thtml.New("layout").Funcs(helpers).Parse(`<body>{{.Body}}</body>`)
	if err != nil {
		b.Fatal(err)
	}
	gen.Layout = layout
	return gen
}

// renderInputs are whole-page render() inputs standing in for a realistic
// page mix, so the win from skipping text/template on a page with no "{{"
// - invisible to a pageOptsOutOfTemplating-only benchmark, since it never
// gets called at all on that path - shows up here instead. Each shape has
// a small and a large variant: render's fixed per-call costs (HTML5 parse,
// the atomic write) dominate the large ones, so the small ones are what
// actually isolates the string(input) copy and parse/execute round trip
// this change avoids.
var renderInputs = []struct {
	name  string
	input string
}{
	// The common shape this change targets: a title comment, no template
	// actions anywhere. Never reaches pageOptsOutOfTemplating at all.
	{"plain_page/small", "<!-- title: Hi -->\n<h1>Hi</h1>"},
	{"plain_page/large", "<!-- title: Hi -->\n" + largeBody},
	// Actually uses template syntax, so the parse/execute path still
	// runs - benchmarked so a regression there would show.
	{"templated_page/small", "<!-- title: Hi -->\n<h1>{{.Path}}</h1>"},
	{"templated_page/large", "<!-- title: Hi -->\n<h1>{{.Path}}</h1>\n" + largeBody},
	// Explicitly opted out via "template: false" and full of literal
	// "{{" - the shape the opt-out exists for (Go-template documentation,
	// a Vue/Handlebars snippet), and the case that pays pageOptsOutOfTemplating's
	// real yaml.Unmarshal every time.
	{"opted_out_page/small", "<!-- template: false -->\n<h1>{{ message }}</h1>"},
	{"opted_out_page/large", "<!-- template: false -->\n<h1>{{ message }}</h1>\n" + largeBody},
}

func BenchmarkRender(b *testing.B) {
	for _, tt := range renderInputs {
		in := []byte(tt.input)
		b.Run(tt.name, func(b *testing.B) {
			gen := newRenderBenchGenerator(b)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := gen.render("page.html", in); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

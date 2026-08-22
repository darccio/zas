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
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/darccio/zas/internal/i18n"
)

// captureStderr is defined in symlink_test.go and reused here.

// newRenderTestGenerator builds a minimal Generator suitable for calling
// render() directly, deploying into ./deploy under a fresh chdir'd temp
// directory.
func newRenderTestGenerator(t *testing.T) *Generator {
	t.Helper()
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("deploy", 0o755); err != nil {
		t.Fatal(err)
	}
	gen := &Generator{
		Config: ConfigSection{"zas": ConfigSection{"deploy": "deploy"}},
		I18n:   &i18n.Build{Index: i18n.Strings{}, Origin: "en"},
	}
	layout, err := thtml.New("layout").Funcs(helpers).Parse(`<body>{{.Body}}</body>`)
	if err != nil {
		t.Fatal(err)
	}
	gen.Layout = layout
	return gen
}

// render's own call to parseAndReplace used to both print the error
// directly (gen.printLine(err)) and return it as render's error, which its
// caller renderAsync already aggregates into gen.errs and reports through
// the normal single-report-per-error path - so this one error type was
// reported twice. It must now be reported exactly once, by the caller.
func TestRenderParseAndReplaceErrorNotPrintedInline(t *testing.T) {
	gen := newRenderTestGenerator(t)
	// An embed type nothing resolves a plugin for makes handleEmbedTags -
	// and therefore render's own top-level parseAndReplace call - fail.
	src := `<body><embed src="x" type="text/x-nope"></body>`

	var err error
	stderr := captureStderr(t, func() {
		err = gen.render("page.html", []byte(src))
	})

	if err == nil {
		t.Fatal("render() error = nil, want the embed-plugin error propagated")
	}
	if !strings.Contains(err.Error(), "text/x-nope") {
		t.Fatalf("render() error = %v, want it to mention the embed type", err)
	}
	if strings.Contains(stderr, "text/x-nope") {
		t.Fatalf("render() printed the error directly (%q); it must be reported exactly once, by its caller", stderr)
	}
}

// extractPageConfig used to unconditionally discard its own
// yaml.Unmarshal error (_ = yaml.Unmarshal(...)), so its named err return
// could never be non-nil. It must now propagate a real error for malformed
// YAML in the page's config comment.
func TestExtractPageConfigPropagatesYAMLError(t *testing.T) {
	gen := &Generator{}
	src := "<!-- language: ru\n\tbad: true\n-->\n<h1>Hi</h1>"
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gen.extractPageConfig(doc); err == nil {
		t.Fatal("extractPageConfig() with a malformed YAML config comment: want error, got nil")
	}
}

// With extractPageConfig actually propagating its error, render's
// print-then-continue branch for it becomes live: the malformed comment
// must be reported, but must not abort rendering the rest of the page.
func TestRenderReportsMalformedPageConfigCommentButKeepsGoing(t *testing.T) {
	gen := newRenderTestGenerator(t)
	src := "<!-- language: ru\n\tbad: true\n-->\n<h1>Hi</h1>"

	var err error
	stderr := captureStderr(t, func() {
		err = gen.render("page.html", []byte(src))
	})

	if err != nil {
		t.Fatalf("render() error = %v, want nil (a malformed page config comment must not abort the render)", err)
	}
	if !strings.Contains(stderr, "page.html") {
		t.Fatalf("render() stderr = %q, want it to report the malformed page config comment", stderr)
	}

	out, readErr := os.ReadFile(filepath.Join("deploy", "page.html"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(out), "<h1>Hi</h1>") {
		t.Fatalf("deploy output = %q, want the page body still rendered despite the malformed config comment", out)
	}
}

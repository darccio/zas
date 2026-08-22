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
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// installStub writes an executable shell-script plugin stub named name
// into a fresh directory and prepends it to PATH - unlike
// mime_plugin_test.go's stubs, several of these scripts shell out to real
// utilities (cat) to read stdin, so the original PATH must stay reachable.
func installStub(t *testing.T, name, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script plugin stub is not portable to windows")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func mustParseDoc(t *testing.T, htmlStr string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestSplitArgs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"all whitespace", "   ", nil},
		{"simple fields", "a b  c", []string{"a", "b", "c"}},
		{"single quoted with space", `--title 'My Post'`, []string{"--title", "My Post"}},
		{"double quoted with space", `--title "My Post"`, []string{"--title", "My Post"}},
		{"empty single quotes", "''", []string{""}},
		{"empty double quotes", `""`, []string{""}},
		{"quote removal mid-field", `a''b`, []string{"ab"}},
		{"mixed quote removal", `-x'y'"z"`, []string{"-xyz"}},
		{"single quotes keep backslash literal", `'a\b'`, []string{`a\b`}},
		{"double quotes escape quote", `"a\"b"`, []string{`a"b`}},
		{"double quotes escape backslash", `"a\\b"`, []string{`a\b`}},
		{"double quotes keep other escapes literal", "\"a\\nb\"", []string{`a\nb`}},
		{"backslash escapes space", `a\ b`, []string{"a b"}},
		{"backslash escapes quote outside quotes", `a\'b`, []string{"a'b"}},
		{"shell metacharacters are literal", `a\|b;c&d`, []string{"a|b;c&d"}},
		{"utf8 argument", "café", []string{"café"}},
		{"glob is literal", "*.md", []string{"*.md"}},
		{"hash is literal", "#x", []string{"#x"}},
		{"newline separates fields", "a\nb", []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := splitArgs(tt.in)
			if err != nil {
				t.Fatalf("splitArgs(%q) error = %v, want nil", tt.in, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("splitArgs(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSplitArgsRejectsUnbalancedQuoting(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"unterminated single quote", `'x`},
		{"unterminated double quote", `"x`},
		{"trailing backslash", `x\`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, err := splitArgs(tt.in)
			if err == nil {
				t.Fatalf("splitArgs(%q) error = nil, want an error", tt.in)
			}
			if args != nil {
				t.Fatalf("splitArgs(%q) args = %#v, want nil", tt.in, args)
			}
		})
	}
}

func TestHandleScriptTagsRunsExternalCommand(t *testing.T) {
	installStub(t, "zstest", "#!/bin/sh\necho '<b>ok</b>'\n")
	doc := mustParseDoc(t, `<html><body><script type="application/zas+test"></script></body></html>`)
	gen := &Generator{}
	if err := gen.handleScriptTags(doc, headDropped); err != nil {
		t.Fatalf("handleScriptTags() error = %v, want nil", err)
	}
	if got := doc.Find("b").Text(); got != "ok" {
		t.Fatalf("plugin output not spliced into the document: got %q", got)
	}
	if doc.Find("script[type='application/zas+test']").Length() != 0 {
		t.Fatal("zas script tag still present after handleScriptTags()")
	}
}

func TestHandleScriptTagsPipesInnerContentToStdin(t *testing.T) {
	installStub(t, "zswrap", "#!/bin/sh\nprintf '<pre>'; cat; printf '</pre>'\n")
	doc := mustParseDoc(t, `<html><body><script type="application/zas+wrap">{"k":"v"}</script></body></html>`)
	gen := &Generator{}
	if err := gen.handleScriptTags(doc, headDropped); err != nil {
		t.Fatalf("handleScriptTags() error = %v, want nil", err)
	}
	if got := strings.TrimSpace(doc.Find("pre").Text()); got != `{"k":"v"}` {
		t.Fatalf("stdin not piped through: got %q", got)
	}
}

func TestHandleScriptTagsStdinIsNotEntityDecoded(t *testing.T) {
	// The point of this test: if the tokenizer entity-decoded <script>
	// content the way it does for ordinary text nodes, "&amp;lt;" would
	// reach the plugin as "<" instead of the literal six characters an
	// author wrote - defeating the whole premise of piping raw data
	// through a zas script tag.
	installStub(t, "zscheck", `#!/bin/sh
if [ "$(cat)" = '&amp;lt;' ]; then
  echo '<b>raw</b>'
else
  echo '<b>decoded</b>'
fi
`)
	doc := mustParseDoc(t, `<html><body><script type="application/zas+check">&amp;lt;</script></body></html>`)
	gen := &Generator{}
	if err := gen.handleScriptTags(doc, headDropped); err != nil {
		t.Fatalf("handleScriptTags() error = %v, want nil", err)
	}
	if got := doc.Find("b").Text(); got != "raw" {
		t.Fatalf("stdin content = %q outcome, want the raw (undecoded) bytes to reach the plugin", got)
	}
}

func TestHandleScriptTagsPassesDataArgs(t *testing.T) {
	installStub(t, "zstest", `#!/bin/sh
for a in "$@"; do printf '<li>%s</li>' "$a"; done
`)
	doc := mustParseDoc(t, `<html><body><script type="application/zas+test" data-args="--title 'My Post' plain"></script></body></html>`)
	gen := &Generator{}
	if err := gen.handleScriptTags(doc, headDropped); err != nil {
		t.Fatalf("handleScriptTags() error = %v, want nil", err)
	}
	var got []string
	doc.Find("li").Each(func(_ int, s *goquery.Selection) {
		got = append(got, s.Text())
	})
	want := []string{"--title", "My Post", "plain"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args spliced = %#v, want %#v", got, want)
	}
}

func TestHandleScriptTagsLeavesNonZasScriptsUntouched(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	src := `<html><head></head><body>` +
		`<script>var x = 1;</script>` +
		`<script type="text/javascript">var y = 2;</script>` +
		`<script type="application/ld+json">{"a":1}</script>` +
		`</body></html>`
	doc := mustParseDoc(t, src)
	before := doc.Find("script").Length()
	gen := &Generator{}
	if err := gen.handleScriptTags(doc, headDropped); err != nil {
		t.Fatalf("handleScriptTags() error = %v, want nil", err)
	}
	if got := doc.Find("script").Length(); got != before {
		t.Fatalf("script count = %d after handleScriptTags(), want unchanged %d", got, before)
	}
}

func TestHandleScriptTagsRejectsPathSeparatorInType(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	doc := mustParseDoc(t, `<html><body><script type="application/zas+../../evil"></script></body></html>`)
	gen := &Generator{}
	err := gen.handleScriptTags(doc, headDropped)
	if err == nil {
		t.Fatal("handleScriptTags() with a path-separator plugin name: want error, got nil")
	}
	if !strings.Contains(err.Error(), "no valid plugin") {
		t.Fatalf("error = %v, want the name rejected before exec", err)
	}
	if doc.Find("script").Length() != 1 {
		t.Fatal("script tag was removed despite the name being rejected")
	}
}

func TestHandleScriptTagsNoPluginsRefusesToExec(t *testing.T) {
	installStub(t, "zstest", "#!/bin/sh\necho '<b>ok</b>'\n")
	doc := mustParseDoc(t, `<html><body><script type="application/zas+test"></script></body></html>`)
	gen := &Generator{NoPlugins: true}
	err := gen.handleScriptTags(doc, headDropped)
	if err == nil {
		t.Fatal("handleScriptTags() with NoPlugins set: want error, got nil")
	}
	if !strings.Contains(err.Error(), "-no-plugins") {
		t.Fatalf("error = %v, want it to mention -no-plugins", err)
	}
	if !strings.Contains(err.Error(), "zstest") {
		t.Fatalf("error = %v, want it to name the plugin binary", err)
	}
	if doc.Find("script[type='application/zas+test']").Length() != 1 {
		t.Fatal("script tag was replaced despite NoPlugins - plugin executed anyway")
	}
}

func TestHandleScriptTagsRefusesHeadPlacementInPagePass(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	// A leading HTML comment (the shape of a page's own config comment)
	// does not force body insertion mode, so a script tag right after it
	// is still parsed into <head> - proving the constraint this test
	// exercises is real, not hypothetical.
	doc := mustParseDoc(t, `<!-- title: x --><script type="application/zas+test"></script><p>b</p>`)
	if doc.Find("head script").Length() != 1 {
		t.Fatal("test fixture invalid: expected the script to be parsed into <head>")
	}
	gen := &Generator{}
	err := gen.handleScriptTags(doc, headDropped)
	if err == nil {
		t.Fatal("handleScriptTags() for a head-placed script in the page pass: want error, got nil")
	}
	if !strings.Contains(err.Error(), "<head>") {
		t.Fatalf("error = %v, want it to mention <head>", err)
	}
	if doc.Find("script[type='application/zas+test']").Length() != 1 {
		t.Fatal("script tag was removed despite being refused")
	}
}

func TestHandleScriptTagsAllowsHeadEligibleOutputInLayoutPass(t *testing.T) {
	installStub(t, "zsmeta", "#!/bin/sh\necho '<meta name=\"generated\" content=\"x\">'\n")
	doc := mustParseDoc(t, `<html><head><script type="application/zas+meta"></script></head><body></body></html>`)
	gen := &Generator{}
	if err := gen.handleScriptTags(doc, headRendered); err != nil {
		t.Fatalf("handleScriptTags() error = %v, want nil", err)
	}
	if doc.Find(`head meta[name="generated"]`).Length() != 1 {
		t.Fatal("head-eligible plugin output did not land in <head>")
	}
}

func TestHandleScriptTagsRejectsNonEligibleHeadOutputInLayoutPass(t *testing.T) {
	installStub(t, "zsbad", "#!/bin/sh\necho '<b>ok</b>'\n")
	doc := mustParseDoc(t, `<html><head><script type="application/zas+bad"></script></head><body></body></html>`)
	gen := &Generator{}
	err := gen.handleScriptTags(doc, headRendered)
	if err == nil {
		t.Fatal("handleScriptTags() for non-head-eligible output in <head>: want error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot be placed") {
		t.Fatalf("error = %v, want it to explain the output cannot be placed", err)
	}
}

func TestHandleScriptTagsMissingBinaryReturnsError(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	doc := mustParseDoc(t, `<html><body><script type="application/zas+doesnotexist"></script></body></html>`)
	gen := &Generator{}
	if err := gen.handleScriptTags(doc, headDropped); err == nil {
		t.Fatal("handleScriptTags() with no matching binary on PATH: want error, got nil")
	}
}

func TestHandleScriptTagsInvalidDataArgsReturnsError(t *testing.T) {
	installStub(t, "zstest", "#!/bin/sh\necho '<b>ok</b>'\n")
	doc := mustParseDoc(t, `<html><body><script type="application/zas+test" data-args="--title 'oops"></script></body></html>`)
	gen := &Generator{}
	err := gen.handleScriptTags(doc, headDropped)
	if err == nil {
		t.Fatal("handleScriptTags() with invalid data-args: want error, got nil")
	}
	if !strings.Contains(err.Error(), "data-args") {
		t.Fatalf("error = %v, want it to mention data-args", err)
	}
	if doc.Find("script[type='application/zas+test']").Length() != 1 {
		t.Fatal("script tag was removed despite invalid data-args (plugin must not have run)")
	}
}

func TestHandleScriptTagsPluginIgnoringStdinSucceeds(t *testing.T) {
	installStub(t, "zstest", "#!/bin/sh\necho '<b>ok</b>'\n")
	inner := strings.Repeat("x", 1<<20)
	doc := mustParseDoc(t, `<html><body><script type="application/zas+test">`+inner+`</script></body></html>`)
	gen := &Generator{}
	if err := gen.handleScriptTags(doc, headDropped); err != nil {
		t.Fatalf("handleScriptTags() error = %v, want nil (plugin ignoring stdin must not fail the build)", err)
	}
}

func TestHandleScriptTagsFirstErrorAbortsRemainingTags(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	doc := mustParseDoc(t, `<html><body>`+
		`<script type="application/zas+first"></script>`+
		`<script type="application/zas+second"></script>`+
		`</body></html>`)
	gen := &Generator{}
	if err := gen.handleScriptTags(doc, headDropped); err == nil {
		t.Fatal("handleScriptTags() error = nil, want an error from the first missing binary")
	}
	if doc.Find("script[type='application/zas+second']").Length() != 1 {
		t.Fatal("second script tag was processed despite the first one erroring")
	}
}

func TestHandleScriptTagsMatchesTypeCaseInsensitively(t *testing.T) {
	installStub(t, "zstest", "#!/bin/sh\necho '<b>ok</b>'\n")
	doc := mustParseDoc(t, `<html><body><script type="APPLICATION/ZAS+Test"></script></body></html>`)
	gen := &Generator{}
	if err := gen.handleScriptTags(doc, headDropped); err != nil {
		t.Fatalf("handleScriptTags() error = %v, want nil", err)
	}
	if got := doc.Find("b").Text(); got != "ok" {
		t.Fatalf("plugin output not spliced into the document: got %q", got)
	}
}

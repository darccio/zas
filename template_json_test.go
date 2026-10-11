package zas

import (
	"bytes"
	"encoding/json"
	"html/template"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestLayoutJSONConstructors(t *testing.T) {
	const source = `<html><head><script type="application/ld+json">{{json (dict "@context" "https://schema.org" "person" (dict "name" . "sameAs" (list "https://example.org/a" "https://example.org/b")) "empty" (list) "count" 2 "active" true "missing" nil)}}</script></head><body></body></html>`
	name := "Dario Castañé"
	output, err := executeJSONLayout(t, source, name)
	if err != nil {
		t.Fatal(err)
	}
	scripts := jsonScriptContents(t, output)
	if len(scripts) != 1 {
		t.Fatalf("got %d scripts, want one JSON data block", len(scripts))
	}
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(scripts[0]), &got); err != nil {
		t.Fatalf("script is not a JSON object: %v; %s", err, scripts[0])
	}
	want := map[string]interface{}{
		"@context": "https://schema.org",
		"person": map[string]interface{}{
			"name": name, "sameAs": []interface{}{"https://example.org/a", "https://example.org/b"},
		},
		"empty": []interface{}{}, "count": float64(2), "active": true, "missing": nil,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded JSON = %#v, want %#v", got, want)
	}
	for _, tc := range []struct {
		source string
		want   string
	}{
		{`{{json (dict)}}`, `{}`},
		{`{{json (list)}}`, `[]`},
	} {
		got, err := executeJSONLayout(t, `<script type="application/ld+json">`+tc.source+`</script>`, nil)
		if err != nil || jsonScriptContents(t, got)[0] != tc.want {
			t.Fatalf("%s = (%q, %v), want %s", tc.source, got, err, tc.want)
		}
	}
}

func TestLayoutJSONPreventsScriptBreakout(t *testing.T) {
	payload := "\"quotes\" & < > </script><script>alert('injected')</script><!--\u2028\u2029 Castañé"
	for _, tc := range []struct {
		name  string
		value interface{}
		want  interface{}
	}{
		{"string property", map[string]string{"value": payload}, map[string]interface{}{"value": payload}},
		{"plain string", payload, payload},
		{"trusted JS input is still a string", template.JS(payload), payload},
		{"pre-encoded string is still a string", `{"admin":true}`, `{"admin":true}`},
		{"RawMessage", json.RawMessage(`{"value":"</script><script>alert(1)</script>&` + "\u2028\u2029" + `"}`), map[string]interface{}{"value": "</script><script>alert(1)</script>&\u2028\u2029"}},
		{"custom marshaler", templateJSONLiteral(`{"value":"</script><script>alert(1)</script>&` + "\u2028\u2029" + `"}`), map[string]interface{}{"value": "</script><script>alert(1)</script>&\u2028\u2029"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output, err := executeJSONLayout(t, `<html><head><script type="application/ld+json">{{json .}}</script></head><body></body></html>`, tc.value)
			if err != nil {
				t.Fatal(err)
			}
			scripts := jsonScriptContents(t, output)
			if len(scripts) != 1 {
				t.Fatalf("payload changed script count to %d: %s", len(scripts), output)
			}
			for _, unsafe := range []string{"<", ">", "&", "\u2028", "\u2029"} {
				if strings.Contains(scripts[0], unsafe) {
					t.Fatalf("script contains unescaped %q: %s", unsafe, scripts[0])
				}
			}
			var got interface{}
			if err := json.Unmarshal([]byte(scripts[0]), &got); err != nil {
				t.Fatalf("script is not JSON: %v; %s", err, scripts[0])
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("decoded JSON = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestLayoutJSONErrorsAbortExecution(t *testing.T) {
	cycle := map[string]interface{}{}
	cycle["self"] = cycle
	for _, tc := range []struct {
		name    string
		source  string
		value   interface{}
		wantErr string
	}{
		{"odd argument count", `{{json (dict "a")}}`, nil, "dict requires key/value pairs"},
		{"non-string key", `{{json (dict 1 "a")}}`, nil, "dict key 1 must be a string"},
		{"duplicate key", `{{json (dict "a" 1 "a" 2)}}`, nil, `dict has duplicate key "a"`},
		{"function", `{{json .}}`, func() {}, "unsupported type"},
		{"channel", `{{json .}}`, make(chan int), "unsupported type"},
		{"cycle", `{{json .}}`, cycle, "encountered a cycle"},
		{"NaN", `{{json .}}`, math.NaN(), "unsupported value"},
		{"invalid RawMessage", `{{json .}}`, json.RawMessage(`not JSON`), "error calling json"},
		{"invalid custom marshaler", `{{json .}}`, templateJSONLiteral(`alert(1)`), "error calling json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output, err := executeJSONLayout(t, `<script type="application/ld+json">`+tc.source+`</script>`, tc.value)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Execute() = (%q, %v), want error containing %q", output, err, tc.wantErr)
			}
			if strings.Contains(output, "</script>") {
				t.Fatalf("failed serializer completed data script: %s", output)
			}
		})
	}
}

func TestLayoutJSONOtherContextsRemainEscaped(t *testing.T) {
	payload := `</div><script>alert(1)</script>" onmouseover="alert(1)`
	output, err := executeJSONLayout(t, `<html><head></head><body><div data-json="{{json (dict "value" .)}}">{{json (dict "value" .)}}</div><a href="{{json .}}">link</a></body></html>`, payload)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := html.Parse(strings.NewReader(output))
	if err != nil {
		t.Fatal(err)
	}
	var divs, anchors int
	for node := range doc.Descendants() {
		if node.Type != html.ElementNode {
			continue
		}
		if node.Data == "script" {
			t.Fatal("non-script context allowed injected script")
		}
		switch node.Data {
		case "div":
			divs++
			if len(node.Attr) != 1 || node.Attr[0].Key != "data-json" {
				t.Fatalf("injected attributes: %#v", node.Attr)
			}
			var got map[string]string
			if err := json.Unmarshal([]byte(node.Attr[0].Val), &got); err != nil || got["value"] != payload {
				t.Fatalf("data-json = %#v, %v", got, err)
			}
		case "a":
			anchors++
			if len(node.Attr) != 1 || node.Attr[0].Key != "href" {
				t.Fatalf("injected link attributes: %#v", node.Attr)
			}
		}
	}
	if divs != 1 || anchors != 1 {
		t.Fatalf("payload changed DOM structure: %d divs, %d anchors", divs, anchors)
	}
	output, err = executeJSONLayout(t, `<a href="{{json .}}">link</a>`, "javascript:alert(1)")
	if err != nil || !strings.Contains(output, "#ZgotmplZ") {
		t.Fatalf("dangerous URL = (%q, %v), want html/template URL rejection", output, err)
	}
}

func TestLayoutJSONHelpersUnusedPreserveOutput(t *testing.T) {
	const source = `<html><head><title>{{.Title}}</title></head><body><p>{{.Summary}}</p>{{noescape .Body}}</body></html>`
	data := map[string]interface{}{"Title": `A & B`, "Summary": `<untrusted>`, "Body": `<p>Existing HTML</p>`}
	legacy := template.Must(template.New("legacy").Funcs(template.FuncMap{"noescape": noescape}).Parse(source))
	var old bytes.Buffer
	if err := legacy.Execute(&old, data); err != nil {
		t.Fatal(err)
	}
	got, err := executeJSONLayout(t, source, data)
	if err != nil || got != old.String() {
		t.Fatalf("unused helpers changed output: got %q, %v; want %q", got, err, old.String())
	}
}

func TestGenerateLayoutJSON(t *testing.T) {
	newTestSite(t, "template-json-site")
	if err := generate(t, fullGen, noPluginsGen); err != nil {
		t.Fatal(err)
	}
	output := readDeploy(t, "index.html")
	scripts := jsonScriptContents(t, output)
	if len(scripts) != 1 {
		t.Fatalf("generated page has %d scripts, want one data block", len(scripts))
	}
	var got struct {
		Context string `json:"@context"`
		Graph   []struct {
			Type          string   `json:"@type"`
			Name          string   `json:"name"`
			Headline      string   `json:"headline"`
			Description   string   `json:"description"`
			DatePublished string   `json:"datePublished"`
			DateModified  string   `json:"dateModified"`
			URL           string   `json:"url"`
			Keywords      []string `json:"keywords"`
		} `json:"@graph"`
	}
	if err := json.Unmarshal([]byte(scripts[0]), &got); err != nil {
		t.Fatalf("generated JSON-LD is not an object: %v; %s", err, scripts[0])
	}
	if got.Context != "https://schema.org" || len(got.Graph) != 2 || got.Graph[0].Type != "Person" || got.Graph[0].Name != "Dario Castañé" {
		t.Fatalf("unexpected graph: %#v", got)
	}
	article := got.Graph[1]
	if article.Type != "BlogPosting" || article.Headline != "Quoted & Unicode article" || article.URL != "https://example.org/" || article.DatePublished != "2026-10-11T10:00:00+02:00" || article.DateModified != "2026-10-11T11:00:00+02:00" {
		t.Fatalf("metadata did not survive generation: %#v", article)
	}
	if article.Description != `A "quote" & <tag> </script><script>alert(1)</script> Castañé` || !reflect.DeepEqual(article.Keywords, []string{"Go", "política"}) {
		t.Fatalf("metadata text corrupted: %#v", article)
	}
	if !strings.Contains(output, `<h1>Quoted &amp; Unicode article</h1>`) || !strings.Contains(output, "Ordinary article body.") {
		t.Fatalf("JSON helpers changed the ordinary body: %s", output)
	}
	if err := generate(t, fullGen); err != nil {
		t.Fatal(err)
	}
	if withPlugins := readDeploy(t, "index.html"); withPlugins != output {
		t.Fatalf("JSON data block changed when plugins are enabled: %s", withPlugins)
	}
}

func TestGenerateLayoutJSONConstructorError(t *testing.T) {
	newTestSite(t, "template-json-site")
	if err := os.WriteFile(filepath.Join(Dir, "layout.html"), []byte(`<html><head><script type="application/ld+json">{{json (dict "duplicate" 1 "duplicate" 2)}}</script></head><body>{{.Body}}</body></html>`), DefaultFilePerm); err != nil {
		t.Fatal(err)
	}
	if err := generate(t, fullGen, noPluginsGen); err == nil || !strings.Contains(err.Error(), "duplicate key") {
		t.Fatalf("generation error = %v, want constructor error", err)
	}
	assertDeployMissing(t, "index.html")
}

func TestPageTemplatesDoNotReceiveJSONHelpers(t *testing.T) {
	newTestSite(t, "template-json-site")
	if err := os.WriteFile("index.md", []byte(`# {{json "Page"}}`), DefaultFilePerm); err != nil {
		t.Fatal(err)
	}
	if err := generate(t, fullGen, noPluginsGen); err == nil || !strings.Contains(err.Error(), `function "json" not defined`) {
		t.Fatalf("page template error = %v, want layout-only helper diagnostic", err)
	}
}

type templateJSONLiteral string

func (value templateJSONLiteral) MarshalJSON() ([]byte, error) {
	return []byte(value), nil
}

func executeJSONLayout(t *testing.T, source string, data interface{}) (string, error) {
	t.Helper()
	tmpl := template.Must(template.New("layout").Funcs(helpers).Parse(source))
	var output bytes.Buffer
	err := tmpl.Execute(&output, data)
	return output.String(), err
}

func jsonScriptContents(t *testing.T, output string) []string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(output))
	if err != nil {
		t.Fatal(err)
	}
	var contents []string
	for node := range doc.Descendants() {
		if node.Type != html.ElementNode || node.Data != "script" {
			continue
		}
		if len(node.Attr) != 1 || node.Attr[0].Key != "type" || node.Attr[0].Val != "application/ld+json" || node.Parent.Data != "head" {
			t.Fatalf("unexpected executable or misplaced script: %#v", node)
		}
		var text strings.Builder
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Type != html.TextNode {
				t.Fatalf("script contains non-text child: %#v", child)
			}
			text.WriteString(child.Data)
		}
		contents = append(contents, text.String())
	}
	return contents
}

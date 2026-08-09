package zas

import (
	"reflect"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// B6: config data must not be able to pick an arbitrary Generator method via
// reflection and panic method.Call with a mismatched arity/signature.

func TestIsEmbedPluginMethod(t *testing.T) {
	gen := &Generator{}

	if valid := reflect.ValueOf(gen).MethodByName("Markdown"); !isEmbedPluginMethod(valid) {
		t.Fatal("isEmbedPluginMethod(Markdown) = false, want true")
	}
	// Run() error is a real exported Generator method, but with the wrong
	// arity (0 args) for embed dispatch - must be rejected, not panic.
	if wrongArity := reflect.ValueOf(gen).MethodByName("Run"); isEmbedPluginMethod(wrongArity) {
		t.Fatal("isEmbedPluginMethod(Run) = true, want false")
	}
	if notFound := reflect.ValueOf(gen).MethodByName("DoesNotExist"); isEmbedPluginMethod(notFound) {
		t.Fatal("isEmbedPluginMethod(missing method) = true, want false")
	}
}

func TestHandleEmbedTagsFallsBackOnWrongArityMethod(t *testing.T) {
	// Reproduces the audit repro: mimetypes: {text/x-t: run} resolves to the
	// real, exported Run() error method, which has the wrong signature for
	// embed dispatch. Must fall back to the external plugin path instead of
	// panicking in reflect.Value.Call. The fallback path itself is a no-op
	// here (see audit A1, out of scope), so simply completing without a
	// panic is the regression this guards.
	gen := &Generator{
		Config: ConfigSection{
			"mimetypes": ConfigSection{"text/x-t": "run"},
		},
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<html><body><embed src="x" type="text/x-t"></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	_ = gen.handleEmbedTags(doc, &ZasData{})
}

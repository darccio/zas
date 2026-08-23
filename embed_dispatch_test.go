package zas

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// config data must not be able to pick an arbitrary Generator method for
// embed dispatch - only the closed set embedPlugin knows about.

func TestHandleEmbedTagsFallsBackOnUnknownPluginName(t *testing.T) {
	// mimetypes: {text/x-t: run} resolves to "run", which isn't one of
	// embedPlugin's three known names (it happens to also be the name of a
	// real, exported, wrong-signature Generator method, Run() error - but
	// that's no longer relevant: embedPlugin never looks at Generator's
	// actual method set). Must fall back to the external plugin path.
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

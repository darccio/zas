package zas

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// The src/type attribute checks were inverted (returning early exactly when
// the attributes were present), so handleMIMETypePlugin could never reach
// the point of invoking an external plugin - the whole mzs* plugin feature
// was unreachable.

func newEmbedDoc(t *testing.T, src, typ string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<div><embed src="` + src + `" type="` + typ + `"></div>`))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestHandleMIMETypePluginRunsExternalCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script plugin stub is not portable to windows")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\necho '<b>ok</b>'\n"
	if err := os.WriteFile(filepath.Join(bin, "mzstest"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	gen := &Generator{Config: ConfigSection{
		"mimetypes": ConfigSection{"text/x-test": "test"},
	}}
	doc := newEmbedDoc(t, "x", "text/x-test")
	if err := gen.handleMIMETypePlugin(doc.Find("embed")); err != nil {
		t.Fatalf("handleMIMETypePlugin() error = %v, want nil", err)
	}
	if got := doc.Find("b").Text(); got != "ok" {
		t.Fatalf("plugin output not spliced into the document: got %q", got)
	}
	if doc.Find("embed").Length() != 0 {
		t.Fatal("embed tag still present after handleMIMETypePlugin()")
	}
}

func TestHandleMIMETypePluginMissingBinaryReturnsError(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	gen := &Generator{Config: ConfigSection{
		"mimetypes": ConfigSection{"text/x-missing": "doesnotexist"},
	}}
	doc := newEmbedDoc(t, "x", "text/x-missing")
	if err := gen.handleMIMETypePlugin(doc.Find("embed")); err == nil {
		t.Fatal("handleMIMETypePlugin() with no matching binary on PATH: want error, got nil")
	}
}

func TestHandleMIMETypePluginUnconfiguredTypeReturnsError(t *testing.T) {
	gen := &Generator{Config: ConfigSection{"mimetypes": ConfigSection{}}}
	doc := newEmbedDoc(t, "x", "text/x-nope")
	err := gen.handleMIMETypePlugin(doc.Find("embed"))
	if err == nil {
		t.Fatal("handleMIMETypePlugin() with an unconfigured type: want error, got nil")
	}
	if !strings.Contains(err.Error(), "text/x-nope") {
		t.Fatalf("error = %v, want it to mention the embed type", err)
	}
}

func TestHandleMIMETypePluginRejectsPathSeparator(t *testing.T) {
	gen := &Generator{Config: ConfigSection{
		"mimetypes": ConfigSection{"text/x-evil": "../../evil"},
	}}
	doc := newEmbedDoc(t, "x", "text/x-evil")
	err := gen.handleMIMETypePlugin(doc.Find("embed"))
	if err == nil {
		t.Fatal("handleMIMETypePlugin() with a path-separator plugin name: want error, got nil")
	}
	if !strings.Contains(err.Error(), "no valid plugin") {
		t.Fatalf("error = %v, want the name rejected before exec", err)
	}
}

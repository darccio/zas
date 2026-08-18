package zas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Exercises the full generation pipeline - Run, walk, renderMarkdown,
// render, parseAndReplace, Generate - against a minimal real site on disk,
// composing every fix in this series. index.md is the exact reproducer from
// https://github.com/darccio/zas/issues/16.

func TestGenerateFullSite(t *testing.T) {
	newTestSite(t, "i18n-site")

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	gen := &Generator{Full: true}
	if err := gen.Run(); err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}

	index, err := os.ReadFile(filepath.Join(ZAS_DIR, "deploy", "index.html"))
	must(err)
	got := string(index)
	for _, want := range []string{"<div>", "Hello world!", "</div>", "Hello World"} {
		if !strings.Contains(got, want) {
			t.Fatalf("index.html = %q, want it to contain %q", got, want)
		}
	}
	for _, unwanted := range []string{"raw HTML omitted", "<p></p>"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("index.html = %q, want it to not contain %q", got, unwanted)
		}
	}
	if n := strings.Count(got, "<body"); n != 1 {
		t.Fatalf("index.html has %d <body> tags, want 1: %q", n, got)
	}

	about, err := os.ReadFile(filepath.Join(ZAS_DIR, "deploy", "about.html"))
	must(err)
	if want := "<title>Overridden</title>"; !strings.Contains(string(about), want) {
		t.Fatalf("about.html = %q, want it to contain %q", about, want)
	}
}

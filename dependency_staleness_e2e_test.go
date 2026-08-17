package zas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// End-to-end tests proving sourceIsNewer invalidates pages when a shared
// dependency changes, not just when the page's own source file does.

func TestGenerateLayoutChangeInvalidatesEveryPage(t *testing.T) {
	newTestSite(t, "site")
	ageSources(t, -time.Hour)
	if err := generate(t); err != nil {
		t.Fatalf("first generate() error = %v, want nil", err)
	}

	layout := filepath.Join(".zas", "layout.html")
	data, err := os.ReadFile(layout)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(data), "<body>", `<body><p class="marker">layout-v2</p>`, 1)
	if updated == string(data) {
		t.Fatal("test fixture layout.html has no <body> tag to mark")
	}
	if err := os.WriteFile(layout, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	touchFuture(t, layout)

	if err := generate(t); err != nil {
		t.Fatalf("second generate() error = %v, want nil", err)
	}

	for _, page := range []string{"about.html", "index.html", filepath.Join("sub", "page.html")} {
		if out := readDeploy(t, page); !strings.Contains(out, "layout-v2") {
			t.Fatalf("%s = %q, want it to reflect the edited layout", page, out)
		}
	}
}

func TestGenerateConfigChangeInvalidatesEveryPage(t *testing.T) {
	newTestSite(t, "site")
	ageSources(t, -time.Hour)
	if err := generate(t); err != nil {
		t.Fatalf("first generate() error = %v, want nil", err)
	}
	if out := readDeploy(t, "about.html"); !strings.Contains(out, "http://example.com") {
		t.Fatalf("about.html = %q, want it to contain the original baseurl", out)
	}

	config := filepath.Join(".zas", "config.yml")
	data, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(data), "http://example.com", "http://updated.example.com", 1)
	if updated == string(data) {
		t.Fatal("test fixture config.yml has no baseurl to update")
	}
	if err := os.WriteFile(config, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	touchFuture(t, config)

	if err := generate(t); err != nil {
		t.Fatalf("second generate() error = %v, want nil", err)
	}

	for _, page := range []string{"about.html", "index.html", filepath.Join("sub", "page.html")} {
		out := readDeploy(t, page)
		if !strings.Contains(out, "http://updated.example.com") {
			t.Fatalf("%s = %q, want it to reflect the updated baseurl", page, out)
		}
	}
}

func TestGenerateI18nChangeInvalidatesEveryPage(t *testing.T) {
	newTestSite(t, "site")
	ageSources(t, -time.Hour)
	if err := generate(t); err != nil {
		t.Fatalf("first generate() error = %v, want nil", err)
	}
	if out := readDeploy(t, "about.html"); !strings.Contains(out, "Hello") {
		t.Fatalf("about.html = %q, want it to contain the original translation", out)
	}

	i18n := filepath.Join(".zas", "i18n.yml")
	if err := os.WriteFile(i18n, []byte("greeting:\n  en: Hi\n  es: Hola\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	touchFuture(t, i18n)

	if err := generate(t); err != nil {
		t.Fatalf("second generate() error = %v, want nil", err)
	}

	// about.html (root, language "en") and sub/page.html (its own
	// .zas.yml sets language "es") both embed the layout's {{.E
	// "greeting"}}, so both must pick up the new translation - proving
	// i18n.yml invalidation is global, not scoped to one page.
	if out := readDeploy(t, "about.html"); !strings.Contains(out, "Hi") || strings.Contains(out, "Hello") {
		t.Fatalf("about.html = %q, want the updated \"Hi\" translation", out)
	}
	if out := readDeploy(t, filepath.Join("sub", "page.html")); !strings.Contains(out, "Hola") {
		t.Fatalf("sub/page.html = %q, want it regenerated with the (unchanged) Hola translation", out)
	}
}

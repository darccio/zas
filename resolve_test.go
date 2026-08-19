package zas

import (
	"testing"

	"github.com/melvinmt/gt"
)

// B3: Resolve must error on a present-but-non-string value instead of
// panicking on the `.(string)` assertion, while still falling back to ""
// when the key is genuinely absent.

func TestResolveMissingKeyFallsBackToEmpty(t *testing.T) {
	zd := &ZasData{
		Page:   map[interface{}]interface{}{},
		config: ConfigSection{},
	}
	got, err := zd.Resolve("language")
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if got != "" {
		t.Fatalf("Resolve() = %q, want empty string", got)
	}
}

func TestResolveNilPageValueErrors(t *testing.T) {
	// e.g. <!-- language: --> (YAML null) in a page's config comment.
	zd := &ZasData{
		Page: map[interface{}]interface{}{"language": nil},
	}
	if _, err := zd.Resolve("language"); err == nil {
		t.Fatal("Resolve() with nil page value: want error, got nil")
	}
}

func TestResolveNonStringPageValueErrors(t *testing.T) {
	// e.g. language: 5 in a page's config comment.
	zd := &ZasData{
		Page: map[interface{}]interface{}{"language": 5},
	}
	if _, err := zd.Resolve("language"); err == nil {
		t.Fatal("Resolve() with numeric page value: want error, got nil")
	}
}

func TestResolveDirectoryFallback(t *testing.T) {
	zd := &ZasData{
		Page:      map[interface{}]interface{}{},
		Directory: ConfigSection{"language": "es"},
	}
	got, err := zd.Resolve("language")
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if got != "es" {
		t.Fatalf("Resolve() = %q, want %q", got, "es")
	}
}

func TestLanguagePropagatesResolveError(t *testing.T) {
	zd := &ZasData{Page: map[interface{}]interface{}{"language": 5}}
	if _, err := zd.Language(); err == nil {
		t.Fatal("Language(): want error, got nil")
	}
}

func TestIsHomePropagatesLanguageError(t *testing.T) {
	zd := &ZasData{Path: "/index.html", Page: map[interface{}]interface{}{"language": 5}}
	if _, err := zd.IsHome(); err == nil {
		t.Fatal("IsHome(): want error, got nil")
	}
}

func TestIsHomeOK(t *testing.T) {
	zd := &ZasData{
		Path:   "/index.html",
		Page:   map[interface{}]interface{}{},
		config: ConfigSection{},
	}
	home, err := zd.IsHome()
	if err != nil {
		t.Fatalf("IsHome() error = %v, want nil", err)
	}
	if !home {
		t.Fatal("IsHome() = false, want true for /index.html")
	}
}

// URL used to concatenate BaseURL and Path with no normalization, so a
// BaseURL configured with a trailing slash (an easy, unenforced mistake -
// the README says "without final slash" but nothing checks it) produced a
// double slash where the two met, since Path always starts with its own
// leading slash.

func TestURLTrimsTrailingSlashFromBaseURL(t *testing.T) {
	zd := &ZasData{
		Site: ZasSiteData{BaseURL: "http://example.com/"},
		Path: "/page.html",
	}
	if got, want := zd.URL(), "http://example.com/page.html"; got != want {
		t.Fatalf("URL() = %q, want %q", got, want)
	}
}

func TestURLWithoutTrailingSlashUnchanged(t *testing.T) {
	zd := &ZasData{
		Site: ZasSiteData{BaseURL: "http://example.com"},
		Path: "/page.html",
	}
	if got, want := zd.URL(), "http://example.com/page.html"; got != want {
		t.Fatalf("URL() = %q, want %q", got, want)
	}
}

func TestEPropagatesLanguageError(t *testing.T) {
	zd := &ZasData{
		Page: map[interface{}]interface{}{"language": 5},
		i18n: &gt.Build{},
	}
	if _, err := zd.E("greeting"); err == nil {
		t.Fatal("E(): want error, got nil")
	}
}

func TestEFallsBackOnMissingTranslation(t *testing.T) {
	// A missing translation key must still render as "**key**" rather than
	// being treated as an error - only a bad page-config value should abort
	// the render.
	zd := &ZasData{
		Page: map[interface{}]interface{}{"language": "en"},
		i18n: &gt.Build{Index: gt.Strings{}, Origin: "en"},
	}
	got, err := zd.E("missing.key")
	if err != nil {
		t.Fatalf("E() error = %v, want nil", err)
	}
	if want := "**missing.key**"; got != want {
		t.Fatalf("E() = %q, want %q", got, want)
	}
}

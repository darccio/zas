package zas

import (
	"testing"

	"github.com/darccio/zas/internal/i18n"
)

// Extra must error both when a keypath segment isn't itself a section and
// when the final key is missing or holds a non-string value - previously
// the final-key case silently "succeeded" with "", indistinguishable from a
// legitimately empty string (Resolve, Extra's only production caller,
// already discards this error deliberately - see
// TestResolveMissingKeyFallsBackToEmpty - so this doesn't change Resolve's
// own behavior).

func TestExtraReturnsValueForPresentStringKey(t *testing.T) {
	zd := &ZasData{
		config: ConfigSection{"site": ConfigSection{"language": "en"}},
	}
	got, err := zd.Extra("/site/language")
	if err != nil {
		t.Fatalf("Extra() error = %v, want nil", err)
	}
	if got != "en" {
		t.Fatalf("Extra() = %q, want %q", got, "en")
	}
}

func TestExtraErrorsOnMissingFinalKey(t *testing.T) {
	zd := &ZasData{
		config: ConfigSection{"site": ConfigSection{}},
	}
	got, err := zd.Extra("/site/language")
	if err == nil {
		t.Fatal("Extra() with a missing final key: want error, got nil")
	}
	if got != "" {
		t.Fatalf("Extra() = %q, want empty string", got)
	}
}

func TestExtraErrorsOnWrongTypeFinalKey(t *testing.T) {
	// e.g. a config value like "language: 5" where a string was expected.
	zd := &ZasData{
		config: ConfigSection{"site": ConfigSection{"language": 5}},
	}
	got, err := zd.Extra("/site/language")
	if err == nil {
		t.Fatal("Extra() with a non-string final value: want error, got nil")
	}
	if got != "" {
		t.Fatalf("Extra() = %q, want empty string", got)
	}
}

func TestExtraErrorsOnBadSectionSegment(t *testing.T) {
	zd := &ZasData{
		config: ConfigSection{"site": "not-a-section"},
	}
	if _, err := zd.Extra("/site/language"); err == nil {
		t.Fatal("Extra() with a non-section path segment: want error, got nil")
	}
}

// Resolve must error on a present-but-non-string value instead of
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
		i18n: &i18n.Build{},
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
		i18n: &i18n.Build{Index: i18n.Strings{}, Origin: "en"},
	}
	got, err := zd.E("missing.key")
	if err != nil {
		t.Fatalf("E() error = %v, want nil", err)
	}
	if want := "**missing.key**"; got != want {
		t.Fatalf("E() = %q, want %q", got, want)
	}
}

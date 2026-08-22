package zas

import (
	thtml "html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darccio/zas/internal/i18n"
)

// Coverage for the "publish: false" page-config key (upstream issue #15:
// "How can we exclude a file from the generation loop?"). It lets a file
// that only ever makes sense embedded elsewhere - a nav partial, for
// instance - opt out of also being written to the deploy directory as its
// own standalone page, without an underscore-prefix naming convention.

func TestPagePublishedDefaultsTrue(t *testing.T) {
	if !pagePublished(nil) {
		t.Fatal("pagePublished(nil) = false, want true")
	}
	if !pagePublished(map[interface{}]interface{}{}) {
		t.Fatal("pagePublished(empty) = false, want true")
	}
	if !pagePublished(map[interface{}]interface{}{"title": "Nav"}) {
		t.Fatal("pagePublished(no publish key) = false, want true")
	}
}

func TestPagePublishedExplicitTrue(t *testing.T) {
	if !pagePublished(map[interface{}]interface{}{"publish": true}) {
		t.Fatal("pagePublished(publish: true) = false, want true")
	}
}

func TestPagePublishedFalse(t *testing.T) {
	if pagePublished(map[interface{}]interface{}{"publish": false}) {
		t.Fatal("pagePublished(publish: false) = true, want false")
	}
}

func TestPagePublishedNonBoolValueDefaultsTrue(t *testing.T) {
	// A typo'd or unexpected value (e.g. a string) must not be silently
	// treated as an exclusion - only an actual boolean false does that.
	if !pagePublished(map[interface{}]interface{}{"publish": "false"}) {
		t.Fatal(`pagePublished(publish: "false") = false, want true (only a real bool false excludes)`)
	}
}

// TestRenderSkipsDeployOutputWhenPublishFalse drives render() directly
// (like TestRenderPageBodyCanCallPointerReceiverMethod in
// page_data_test.go) to confirm a page whose config comment sets
// "publish: false" never gets a deploy file written for it, while
// render() itself still returns no error.
func TestRenderSkipsDeployOutputWhenPublishFalse(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("deploy", 0o755); err != nil {
		t.Fatal(err)
	}
	gen := &Generator{
		Config: ConfigSection{
			"zas":  ConfigSection{"deploy": "deploy"},
			"site": ConfigSection{"baseurl": "http://example.com"},
		},
		I18n: &i18n.Build{Index: i18n.Strings{}, Origin: "en"},
	}
	layout, err := thtml.New("layout").Funcs(helpers).Parse(`{{.Body}}`)
	if err != nil {
		t.Fatal(err)
	}
	gen.Layout = layout

	source := "<!-- publish: false -->\n<nav><a href=\"/index.html\">Home</a></nav>\n"
	if err := gen.render("partials/nav.html", []byte(source)); err != nil {
		t.Fatalf("render() error = %v, want nil", err)
	}

	if _, err := os.Stat(filepath.Join("deploy", "partials", "nav.html")); !os.IsNotExist(err) {
		t.Fatalf("deploy/partials/nav.html stat err = %v, want a not-exist error", err)
	}
}

// TestGenerateExcludedPartialNotPublishedButStillEmbeds runs a full
// Generator.Run() over testdata/site, whose partials/nav.html carries
// "publish: false" and is embedded into index.html via <embed>. It
// confirms the exclusion actually reaches end to end: the partial itself
// never lands in the deploy tree, its content still shows up inside
// index.html's deployed output, and an ordinary page with no publish
// key (about.html) is generated exactly as before.
func TestGenerateExcludedPartialNotPublishedButStillEmbeds(t *testing.T) {
	newTestSite(t, "site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}

	// walk still creates the deploy-side "partials" directory itself (it
	// mirrors every source directory regardless of what ends up inside
	// it), so only the excluded file's own output is asserted missing.
	assertDeployMissing(t, filepath.Join("partials", "nav.html"))

	out := readDeploy(t, "index.html")
	if !strings.Contains(out, "<nav>") || !strings.Contains(out, "Home</a>") {
		t.Fatalf("index.html = %q, want it to still contain the embedded nav content", out)
	}

	assertDeployHas(t, "about.html")
	assertDeployHas(t, filepath.Join("sub", "page.html"))
}

// TestGenerateExcludedPartialFullRunAlsoOmitsIt confirms the exclusion
// holds under -full generation too, not just the incremental path.
func TestGenerateExcludedPartialFullRunAlsoOmitsIt(t *testing.T) {
	newTestSite(t, "site")
	if err := generate(t, fullGen); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}

	assertDeployMissing(t, filepath.Join("partials", "nav.html"))
	assertDeployHas(t, "index.html")
}

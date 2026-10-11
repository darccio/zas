package zas

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/darccio/zas/internal/i18n"
)

func TestMetadataDates(t *testing.T) {
	want := time.Date(2024, 11, 27, 22, 22, 0, 0, time.FixedZone("", 3600))
	for _, key := range []string{"date", "updated"} {
		for _, tc := range []struct {
			name    string
			value   interface{}
			wantErr bool
		}{
			{"RFC3339", "2024-11-27T22:22:00+01:00", false},
			{"typed time", want, false},
			{"day only", "2024-11-27", true},
			{"no timezone", "2024-11-27T22:22:00", true},
			{"one digit hour", "2024-11-27T2:22:00Z", true},
			{"comma fraction", "2024-11-27T22:22:00,5Z", true},
			{"offset hour", "2024-11-27T22:22:00+24:00", true},
			{"offset minute", "2024-11-27T22:22:00+01:60", true},
			{"invalid string", "yesterday", true},
			{"empty string", "", true},
			{"null", nil, true},
			{"number", 123, true},
		} {
			t.Run(key+"/"+tc.name, func(t *testing.T) {
				data := ZasData{Page: map[interface{}]interface{}{key: tc.value}}
				helper := data.Date
				if key == "updated" {
					helper = data.Updated
				}
				got, err := helper()
				if tc.wantErr {
					if err == nil || !strings.Contains(err.Error(), key) || !got.IsZero() {
						t.Fatalf("%s() = (%v, %v), want zero time and a diagnostic for %s", key, got, err, key)
					}
				} else if err != nil || !got.Equal(want) || got.Format(time.RFC3339) != want.Format(time.RFC3339) {
					t.Fatalf("%s() = (%v, %v), want (%v, nil) preserving its offset", key, got, err, want)
				}
			})
		}
	}

	data := ZasData{Page: map[interface{}]interface{}{
		"date_created": want, "date_updated": want, "description": "Legacy description",
	}}
	if got, err := data.Date(); err != nil || !got.IsZero() {
		t.Fatalf("absent Date() = (%v, %v), want zero time without a legacy-key fallback", got, err)
	}
	if got, err := data.Updated(); err != nil || !got.IsZero() {
		t.Fatalf("absent Updated() = (%v, %v), want zero time without a legacy-key fallback", got, err)
	}
	if got, err := data.Summary(); err != nil || got != "" {
		t.Fatalf("absent Summary() = (%q, %v), want empty without a description fallback", got, err)
	}
	if got, err := data.Tags(); err != nil || len(got) != 0 {
		t.Fatalf("absent Tags() = (%v, %v), want an empty list", got, err)
	}
}

func TestMetadataDatesAcceptFractionalSeconds(t *testing.T) {
	value := "2024-11-27T22:22:00.123456789+01:00"
	data := ZasData{Page: map[interface{}]interface{}{"date": value, "updated": value}}
	for _, helper := range []func() (time.Time, error){data.Date, data.Updated} {
		if got, err := helper(); err != nil || got.Format(time.RFC3339Nano) != value {
			t.Fatalf("date helper = (%v, %v), want fractional seconds and offset preserved", got, err)
		}
	}
}

func TestMetadataTags(t *testing.T) {
	for _, value := range []interface{}{[]string{"one", "two"}, []interface{}{"one", "two"}} {
		data := ZasData{Page: map[interface{}]interface{}{"tags": value}}
		got, err := data.Tags()
		if err != nil || !reflect.DeepEqual(got, []string{"one", "two"}) {
			t.Fatalf("Tags() = (%v, %v), want [one two] in order", got, err)
		}
		got[0] = "changed"
		original, err := data.Tags()
		if err != nil || original[0] != "one" {
			t.Fatal("Tags() exposes the Page list for mutation")
		}
	}
}

func TestMetadataCanonicalKeepsExistingPaths(t *testing.T) {
	gen := &Generator{
		Config: ConfigSection{"site": ConfigSection{"baseurl": "https://example.com/blog/"}},
		I18n:   &i18n.Build{},
	}
	for _, tc := range []struct{ source, path, canonical string }{
		{"index.md", "/index.html", "https://example.com/blog/"},
		{"dir/index.html", "/dir/index.html", "https://example.com/blog/dir/"},
		{"dir/index.md", "/dir/index.html", "https://example.com/blog/dir/"},
		{"dir/article.md", "/dir/article.html", "https://example.com/blog/dir/article.html"},
		{"dir/myindex.html", "/dir/myindex.html", "https://example.com/blog/dir/myindex.html"},
	} {
		data := NewZasData(filepath.FromSlash(tc.source), gen)
		if data.sourcePath != filepath.FromSlash(tc.source) {
			t.Fatalf("sourcePath = %q, want original %q", data.sourcePath, tc.source)
		}
		if data.Path != tc.path || data.URL() != "https://example.com/blog"+tc.path {
			t.Fatalf("source %s changed existing Path/URL: %q / %q", tc.source, data.Path, data.URL())
		}
		if got := data.Canonical(); got != tc.canonical {
			t.Fatalf("source %s Canonical() = %q, want %q", tc.source, got, tc.canonical)
		}
	}
	if got := (&ZasData{Path: "/index.html"}).Canonical(); got != "/" {
		t.Fatalf("Canonical() without baseurl = %q, want /", got)
	}
}

func TestMetadataHelpersUsePagePreviewAndFinalConfig(t *testing.T) {
	newTestSite(t, "metadata-site")
	if err := generate(t, fullGen, noPluginsGen); err != nil {
		t.Fatal(err)
	}
	out := readDeploy(t, "index.html")
	for _, stage := range []string{"preview", "layout"} {
		for _, field := range []struct{ name, value string }{
			{"date", "2024-11-27T22:22:00+01:00"},
			{"updated", "2025-01-02T03:04:05Z"},
			{"tags", "[one][two]"},
		} {
			want := `<p id="` + stage + `-` + field.name + `">` + field.value + `</p>`
			if !strings.Contains(out, want) {
				t.Fatalf("output misses %q: %s", want, out)
			}
		}
	}
	for _, want := range []string{
		`<p id="preview-summary">Summary {{.Extra &#34;/site/marker&#34;}}</p>`,
		`<p id="layout-summary">Summary final</p>`,
		`<p id="preview-canonical">https://example.com/blog/</p>`,
		`<link rel="canonical" href="https://example.com/blog/"/>`,
		`<p id="layout-path">/index.html</p>`,
		`<p id="layout-url">https://example.com/blog/index.html</p>`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output misses %q: %s", want, out)
		}
	}
}

func TestMetadataValidationIsLazyInBothRenderStages(t *testing.T) {
	for _, tc := range []struct{ name, config, helper string }{
		{"date day", "date: 2024-11-27", "Date"},
		{"date no zone", "date: 2024-11-27 22:22:00", "Date"},
		{"updated day", "updated: 2024-11-27", "Updated"},
		{"updated no zone", "updated: 2024-11-27 22:22:00", "Updated"},
		{"date one digit hour", `date: "2024-11-27T2:22:00Z"`, "Date"},
		{"date comma fraction", `date: "2024-11-27T22:22:00,5Z"`, "Date"},
		{"updated offset hour", `updated: "2024-11-27T22:22:00+24:00"`, "Updated"},
		{"updated offset minute", `updated: "2024-11-27T22:22:00+01:60"`, "Updated"},
		{"date alias", "old: &old 2024-11-27\ndate: *old", "Date"},
		{"date merged", "defaults: &defaults {date: 2024-11-27}\n<<: *defaults", "Date"},
		{"summary list", "summary: [one]", "Summary"},
		{"tags scalar", "tags: one", "Tags"},
		{"tags nonstring", "tags: [one, 2]", "Tags"},
		{"tags null item", "tags: [one, null]", "Tags"},
		{"tags null", "tags: null", "Tags"},
	} {
		for _, stage := range []string{"unused", "preview", "layout"} {
			t.Run(tc.name+"/"+stage, func(t *testing.T) {
				newTestSite(t, "metadata-site")
				source := "<!--\n" + tc.config + "\n-->\n<h1>Example</h1>"
				layout := "{{.Body}}"
				action := "{{." + tc.helper + "}}"
				switch stage {
				case "preview":
					source += "<p>" + action + "</p>"
				case "layout":
					layout += "<p>" + action + "</p>"
				}
				if err := os.WriteFile("index.html", []byte(source), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(".zas", "layout.html"), []byte(layout), 0o644); err != nil {
					t.Fatal(err)
				}
				err := generate(t, fullGen, noPluginsGen)
				if stage == "unused" {
					if err != nil {
						t.Fatalf("unused metadata changed the build: %v", err)
					}
					if got, want := readDeploy(t, "index.html"), "<html><head></head><body><h1>Example</h1></body></html>"; got != want {
						t.Fatalf("unused metadata output = %q, want unchanged %q", got, want)
					}
				} else if err == nil || !strings.Contains(err.Error(), "config value") {
					t.Fatalf("%s helper error = %v, want an explicit metadata diagnostic", stage, err)
				}
			})
		}
	}
}

func TestMetadataFinalTimestampSourceReplacesPreview(t *testing.T) {
	newTestSite(t, "metadata-site")
	// The raw comment cannot decode this template expression as a YAML
	// timestamp. Only the final rendered comment provides its valid scalar.
	source := `<!-- date: !!timestamp '{{.Extra "/site/date"}}' -->` + "\n<h1>Example</h1>"
	if err := os.WriteFile("index.html", []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generate(t, fullGen, noPluginsGen); err != nil {
		t.Fatalf("final layout used stale preview timestamp source: %v", err)
	}
	if got, want := readDeploy(t, "index.html"), `<p id="layout-date">2024-11-27T22:22:00+01:00</p>`; !strings.Contains(got, want) {
		t.Fatalf("output misses the final timestamp %q: %s", want, got)
	}
}

func TestUpdatedUsesOnlyCurrentlyTrackedOriginalSource(t *testing.T) {
	newTestSite(t, "site")
	// Keep local signing settings and hooks out of this temporary git repo.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	when := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	initGitRepoWithCommit(t, "about.md", when)
	gen := &Generator{Config: ConfigSection{}, I18n: &i18n.Build{}}
	data := NewZasData("about.md", gen)
	if got, err := data.Updated(); err != nil || !got.Equal(when) {
		t.Fatalf("Updated() = (%v, %v), want original Markdown source commit %v", got, err, when)
	}

	explicit := "2025-01-02T03:04:05Z"
	data.Page = map[interface{}]interface{}{"updated": explicit}
	if got, err := data.Updated(); err != nil || got.Format(time.RFC3339) != explicit {
		t.Fatalf("Updated() = (%v, %v), want explicit timestamp before git fallback", got, err)
	}
	data.Page["updated"] = "invalid"
	if _, err := data.Updated(); err == nil {
		t.Fatal("invalid explicit updated must error rather than use git")
	}
	data.Page = nil

	if out, err := exec.Command("git", "rm", "--cached", "about.md").CombinedOutput(); err != nil {
		t.Fatalf("git rm --cached: %v\n%s", err, out)
	}
	if got, err := data.Updated(); err != nil || !got.IsZero() {
		t.Fatalf("Updated() for untracked source with history = (%v, %v), want zero time", got, err)
	}
	if out, err := exec.Command("git", "add", "index.html").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	newSource := NewZasData("index.html", gen)
	if got, err := newSource.Updated(); err != nil || !got.IsZero() {
		t.Fatalf("Updated() for a new tracked source without history = (%v, %v), want zero time", got, err)
	}
}

func TestUpdatedWithoutGitReturnsZero(t *testing.T) {
	for _, missingBinary := range []bool{false, true} {
		t.Run(map[bool]string{false: "no repository", true: "no binary"}[missingBinary], func(t *testing.T) {
			newTestSite(t, "site")
			if missingBinary {
				t.Setenv("PATH", t.TempDir())
			}
			data := ZasData{sourcePath: "about.md"}
			if got, err := data.Updated(); err != nil || !got.IsZero() {
				t.Fatalf("Updated() = (%v, %v), want zero without falling back to source mtime", got, err)
			}
		})
	}
}

package zas

import "testing"

// ConfigSection.GetString/GetSection must return zero values instead of
// panicking on a missing or wrongly-typed key.

func TestGetStringMissingKey(t *testing.T) {
	cs := ConfigSection{}
	if got := cs.GetString("nope"); got != "" {
		t.Fatalf("GetString() = %q, want empty string", got)
	}
}

func TestGetStringNonStringValue(t *testing.T) {
	cs := ConfigSection{"baseurl": 123}
	if got := cs.GetString("baseurl"); got != "" {
		t.Fatalf("GetString() = %q, want empty string", got)
	}
}

func TestGetSectionMissingKey(t *testing.T) {
	cs := ConfigSection{}
	if got := cs.GetSection("nope"); got != nil {
		t.Fatalf("GetSection() = %v, want nil", got)
	}
}

func TestGetSectionScalarValue(t *testing.T) {
	cs := ConfigSection{"site": "not-a-section"}
	if got := cs.GetSection("site"); got != nil {
		t.Fatalf("GetSection() = %v, want nil", got)
	}
}

func TestGetSectionRawYAMLMap(t *testing.T) {
	// go.yaml.in/yaml/v3 decodes every nested section into ConfigSection
	// itself (see the type's doc comment in init.go), but GetSection must
	// still recognize a raw map[string]interface{} for any section built
	// by other means, e.g. a caller constructing one by hand.
	cs := ConfigSection{"site": map[string]interface{}{"language": "en"}}
	section := cs.GetSection("site")
	if section == nil {
		t.Fatal("GetSection() = nil, want a section")
	}
	if got := section.GetString("language"); got != "en" {
		t.Fatalf("GetString() = %q, want %q", got, "en")
	}
}

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
	// yaml.Unmarshal produces nested sections as map[interface{}]interface{},
	// not ConfigSection - GetSection must still recognize them.
	cs := ConfigSection{"site": map[interface{}]interface{}{"language": "en"}}
	section := cs.GetSection("site")
	if section == nil {
		t.Fatal("GetSection() = nil, want a section")
	}
	if got := section.GetString("language"); got != "en" {
		t.Fatalf("GetString() = %q, want %q", got, "en")
	}
}

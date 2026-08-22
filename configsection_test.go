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

// GetStringOK/GetSectionOK must let a caller tell "present" apart from
// "absent" or "wrong type" - the exact distinction GetString/GetSection
// themselves collapse into a single zero value.

func TestGetStringOKPresent(t *testing.T) {
	cs := ConfigSection{"language": "en"}
	got, ok := cs.GetStringOK("language")
	if !ok {
		t.Fatal("GetStringOK() ok = false, want true")
	}
	if got != "en" {
		t.Fatalf("GetStringOK() = %q, want %q", got, "en")
	}
}

func TestGetStringOKWrongType(t *testing.T) {
	cs := ConfigSection{"baseurl": 123}
	got, ok := cs.GetStringOK("baseurl")
	if ok {
		t.Fatal("GetStringOK() ok = true, want false for a non-string value")
	}
	if got != "" {
		t.Fatalf("GetStringOK() = %q, want empty string", got)
	}
}

func TestGetStringOKMissingKey(t *testing.T) {
	cs := ConfigSection{}
	got, ok := cs.GetStringOK("nope")
	if ok {
		t.Fatal("GetStringOK() ok = true, want false for a missing key")
	}
	if got != "" {
		t.Fatalf("GetStringOK() = %q, want empty string", got)
	}
}

func TestGetSectionOKPresent(t *testing.T) {
	cs := ConfigSection{"site": ConfigSection{"language": "en"}}
	section, ok := cs.GetSectionOK("site")
	if !ok {
		t.Fatal("GetSectionOK() ok = false, want true")
	}
	if got := section.GetString("language"); got != "en" {
		t.Fatalf("GetString() = %q, want %q", got, "en")
	}
}

func TestGetSectionOKRawYAMLMap(t *testing.T) {
	cs := ConfigSection{"site": map[string]interface{}{"language": "en"}}
	section, ok := cs.GetSectionOK("site")
	if !ok {
		t.Fatal("GetSectionOK() ok = false, want true for a raw map[string]interface{}")
	}
	if got := section.GetString("language"); got != "en" {
		t.Fatalf("GetString() = %q, want %q", got, "en")
	}
}

func TestGetSectionOKWrongType(t *testing.T) {
	cs := ConfigSection{"site": "not-a-section"}
	section, ok := cs.GetSectionOK("site")
	if ok {
		t.Fatal("GetSectionOK() ok = true, want false for a scalar value")
	}
	if section != nil {
		t.Fatalf("GetSectionOK() = %v, want nil", section)
	}
}

func TestGetSectionOKMissingKey(t *testing.T) {
	cs := ConfigSection{}
	section, ok := cs.GetSectionOK("nope")
	if ok {
		t.Fatal("GetSectionOK() ok = true, want false for a missing key")
	}
	if section != nil {
		t.Fatalf("GetSectionOK() = %v, want nil", section)
	}
}

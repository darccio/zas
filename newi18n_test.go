package zas

import (
	"os"
	"testing"
)

// B4: NewI18n must not panic on an entry with no translations, and must
// check the yaml.Unmarshal error before the backfill loop runs.

func TestNewI18nNoFile(t *testing.T) {
	t.Chdir(t.TempDir())
	i18n, err := NewI18n("en")
	if err != nil {
		t.Fatalf("NewI18n() error = %v, want nil", err)
	}
	if len(i18n) != 0 {
		t.Fatalf("NewI18n() = %v, want empty", i18n)
	}
}

func TestNewI18nNilMapEntry(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll(ZAS_DIR, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ZAS_I18N_FILE, []byte("Some key:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	i18n, err := NewI18n("en")
	if err != nil {
		t.Fatalf("NewI18n() error = %v, want nil", err)
	}
	if got := i18n["Some key"]["en"]; got != "Some key" {
		t.Fatalf("i18n[%q][%q] = %q, want %q", "Some key", "en", got, "Some key")
	}
}

func TestNewI18nMalformedYAML(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll(ZAS_DIR, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ZAS_I18N_FILE, []byte("not: valid: yaml: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewI18n("en"); err == nil {
		t.Fatal("NewI18n() with malformed YAML: want error, got nil")
	}
}

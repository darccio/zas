package zas

import (
	"os"
	"path/filepath"
	"testing"
)

// Init.Run must return an error instead of panicking on a write failure.

func TestInitRunWriteFailureReturnsError(t *testing.T) {
	t.Chdir(t.TempDir())
	// Make Dir a regular file so os.WriteFile(ConfigFile, ...) - which
	// treats it as a directory - fails deterministically.
	if err := os.WriteFile(Dir, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	i := &Init{}
	if err := i.Run(); err == nil {
		t.Fatal("Init.Run(): want error, got nil")
	}
}

func TestInitRunOK(t *testing.T) {
	t.Chdir(t.TempDir())
	i := &Init{}
	if err := i.Run(); err != nil {
		t.Fatalf("Init.Run() error = %v, want nil", err)
	}
	if _, err := os.Stat(filepath.Join(Dir, "config.yml")); err != nil {
		t.Fatalf("config.yml not written: %v", err)
	}
}

// Init.Run must scaffold a layout.html alongside config.yml, so that the
// documented "zas init && zas" quickstart succeeds without a manual step.
func TestInitRunScaffoldsLayout(t *testing.T) {
	t.Chdir(t.TempDir())
	i := &Init{}
	if err := i.Run(); err != nil {
		t.Fatalf("Init.Run() error = %v, want nil", err)
	}
	if _, err := os.Stat(LayoutFile); err != nil {
		t.Fatalf("layout.html not written: %v", err)
	}
}

// Without Force, Init.Run must not overwrite an existing config.yml: a
// second "zas init" must never silently discard hand-edited configuration.
func TestInitRunWithoutForceLeavesExistingConfigUntouched(t *testing.T) {
	t.Chdir(t.TempDir())
	i := &Init{}
	if err := i.Run(); err != nil {
		t.Fatalf("Init.Run() error = %v, want nil", err)
	}

	custom := []byte("zas:\n  layout: custom.html\nsite:\n  baseurl: https://example.org\n")
	if err := os.WriteFile(ConfigFile, custom, DefaultFilePerm); err != nil {
		t.Fatal(err)
	}

	if err := i.Run(); err != nil {
		t.Fatalf("Init.Run() error = %v, want nil", err)
	}

	got, err := os.ReadFile(ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(custom) {
		t.Fatalf("config.yml was overwritten without -force: got %q, want %q", got, custom)
	}
}

// Without Force, Init.Run must likewise leave an existing layout.html
// untouched.
func TestInitRunWithoutForceLeavesExistingLayoutUntouched(t *testing.T) {
	t.Chdir(t.TempDir())
	i := &Init{}
	if err := i.Run(); err != nil {
		t.Fatalf("Init.Run() error = %v, want nil", err)
	}

	custom := []byte("<html><body>custom layout {{.Body}}</body></html>")
	if err := os.WriteFile(LayoutFile, custom, DefaultFilePerm); err != nil {
		t.Fatal(err)
	}

	if err := i.Run(); err != nil {
		t.Fatalf("Init.Run() error = %v, want nil", err)
	}

	got, err := os.ReadFile(LayoutFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(custom) {
		t.Fatalf("layout.html was overwritten without -force: got %q, want %q", got, custom)
	}
}

// With Force set, Init.Run must overwrite an existing config.yml and
// layout.html with the scaffolded defaults.
func TestInitRunWithForceOverwritesExisting(t *testing.T) {
	t.Chdir(t.TempDir())
	i := &Init{}
	if err := i.Run(); err != nil {
		t.Fatalf("Init.Run() error = %v, want nil", err)
	}

	if err := os.WriteFile(ConfigFile, []byte("stale config"), DefaultFilePerm); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(LayoutFile, []byte("stale layout"), DefaultFilePerm); err != nil {
		t.Fatal(err)
	}

	forced := &Init{Force: true}
	if err := forced.Run(); err != nil {
		t.Fatalf("Init.Run() error = %v, want nil", err)
	}

	gotConfig, err := os.ReadFile(ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotConfig) == "stale config" {
		t.Fatal("config.yml was not overwritten with -force")
	}

	gotLayout, err := os.ReadFile(LayoutFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotLayout) == "stale layout" {
		t.Fatal("layout.html was not overwritten with -force")
	}
}

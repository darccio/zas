package zas

import (
	"os"
	"path/filepath"
	"testing"
)

// B1: Init.Run must return an error instead of panicking on a write failure.

func TestInitRunWriteFailureReturnsError(t *testing.T) {
	t.Chdir(t.TempDir())
	// Make ZAS_DIR a regular file so os.WriteFile(ZAS_CONF_FILE, ...) - which
	// treats it as a directory - fails deterministically.
	if err := os.WriteFile(ZAS_DIR, []byte("not a directory"), 0o644); err != nil {
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
	if _, err := os.Stat(filepath.Join(ZAS_DIR, "config.yml")); err != nil {
		t.Fatalf("config.yml not written: %v", err)
	}
}

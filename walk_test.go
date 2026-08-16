package zas

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// B5: walk/reaper must return the incoming filepath.Walk error immediately,
// instead of touching a possibly-nil FileInfo first.

func TestWalkPropagatesErr(t *testing.T) {
	gen := &Generator{}
	wantErr := errors.New("permission denied")
	if err := gen.walk("noperm", nil, wantErr); !errors.Is(err, wantErr) {
		t.Fatalf("walk() error = %v, want %v", err, wantErr)
	}
}

func TestReaperPropagatesErr(t *testing.T) {
	gen := &Generator{}
	wantErr := errors.New("permission denied")
	if err := gen.reaper("noperm", nil, wantErr); !errors.Is(err, wantErr) {
		t.Fatalf("reaper() error = %v, want %v", err, wantErr)
	}
}

func TestWalkSkipsHiddenDirectory(t *testing.T) {
	gen := &Generator{}
	info, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("sect", ".hidden")
	if err := gen.walk(path, info, nil); !errors.Is(err, filepath.SkipDir) {
		t.Fatalf("walk(%q) error = %v, want %v", path, err, filepath.SkipDir)
	}
}

func TestWalkSkipsDeployDirectory(t *testing.T) {
	gen := &Generator{}
	info, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("public", ZAS_DIR, "deploy")
	if err := gen.walk(path, info, nil); !errors.Is(err, filepath.SkipDir) {
		t.Fatalf("walk(%q) error = %v, want %v", path, err, filepath.SkipDir)
	}
}

func TestWalkSkipsHiddenFileWithoutSkipDir(t *testing.T) {
	gen := &Generator{}
	file := filepath.Join(t.TempDir(), "x")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("sect", ".gitignore")
	if err := gen.walk(path, info, nil); err != nil {
		t.Fatalf("walk(%q) error = %v, want nil", path, err)
	}
}

func TestWalkDoesNotSkipDirAtRoot(t *testing.T) {
	gen := &Generator{}
	info, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.walk(".", info, nil); err != nil {
		t.Fatalf(`walk(".") error = %v, want nil`, err)
	}
}

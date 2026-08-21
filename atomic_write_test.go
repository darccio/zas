package zas

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// atomicWriteFile must never truncate or partially write the final
// destination path: it only ever touches a temporary file in the same
// directory, renaming it onto the destination once write fully succeeds.

// assertNoStrayTempFiles confirms atomicWriteFile cleaned up after itself:
// no leftover ".*.tmp" entry in dir.
func assertNoStrayTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("stray temp file left behind in %s: %s", dir, e.Name())
		}
	}
}

func TestAtomicWriteFileLeavesExistingContentOnError(t *testing.T) {
	gen := &Generator{}
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	original := "original content, must survive a failed write"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	wantErr := errors.New("boom")
	err := gen.atomicWriteFile(path, func(w io.Writer) error {
		if _, werr := io.WriteString(w, "partial-new-bytes-that-must-never-land"); werr != nil {
			return werr
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("atomicWriteFile() error = %v, want %v", err, wantErr)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Fatalf("destination content = %q, want untouched original %q", got, original)
	}
	assertNoStrayTempFiles(t, dir)
}

func TestAtomicWriteFileLeavesNoFileOnErrorWhenAbsent(t *testing.T) {
	gen := &Generator{}
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	wantErr := errors.New("boom")
	err := gen.atomicWriteFile(path, func(w io.Writer) error {
		_, _ = io.WriteString(w, "partial-bytes-that-must-never-land")
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("atomicWriteFile() error = %v, want %v", err, wantErr)
	}

	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("destination exists after a failed write (stat err = %v), want absent", statErr)
	}
	assertNoStrayTempFiles(t, dir)
}

func TestAtomicWriteFileSuccessSetsDefaultPermissionsAndContent(t *testing.T) {
	gen := &Generator{}
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	want := "hello atomic world"

	err := gen.atomicWriteFile(path, func(w io.Writer) error {
		_, werr := io.WriteString(w, want)
		return werr
	})
	if err != nil {
		t.Fatalf("atomicWriteFile() error = %v, want nil", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != DefaultFilePerm {
		t.Fatalf("permissions = %o, want %o", perm, DefaultFilePerm)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
	assertNoStrayTempFiles(t, dir)
}

func TestAtomicWriteFileOverwritesExistingContentOnSuccess(t *testing.T) {
	gen := &Generator{}
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(path, []byte("stale content"), 0o644); err != nil {
		t.Fatal(err)
	}

	want := "fresh content"
	err := gen.atomicWriteFile(path, func(w io.Writer) error {
		_, werr := io.WriteString(w, want)
		return werr
	})
	if err != nil {
		t.Fatalf("atomicWriteFile() error = %v, want nil", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
	assertNoStrayTempFiles(t, dir)
}

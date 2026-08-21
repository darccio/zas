package zas

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// walk/reaper must return the incoming filepath.Walk error immediately,
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
	path := filepath.Join("public", Dir, "deploy")
	if err := gen.walk(path, info, nil); !errors.Is(err, filepath.SkipDir) {
		t.Fatalf("walk(%q) error = %v, want %v", path, err, filepath.SkipDir)
	}
}

func TestWalkSkipsConfiguredDeployPathOutsideZasDir(t *testing.T) {
	gen := &Generator{Config: ConfigSection{Name: ConfigSection{"deploy": "public"}}}
	info, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := "public"
	if err := gen.walk(path, info, nil); !errors.Is(err, filepath.SkipDir) {
		t.Fatalf("walk(%q) error = %v, want %v", path, err, filepath.SkipDir)
	}
}

func TestWalkSkipsConfiguredLayoutPathOutsideZasDir(t *testing.T) {
	gen := &Generator{Config: ConfigSection{Name: ConfigSection{"layout": "mylayout.html"}}}
	file := filepath.Join(t.TempDir(), "x")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	path := "mylayout.html"
	if err := gen.walk(path, info, nil); err != nil {
		t.Fatalf("walk(%q) error = %v, want nil", path, err)
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

// TestWalkDoesNotSkipDirEndingInZasDir is a regression test: a directory
// whose name merely ends in Dir (".zas"), such as "docs.zas", must not
// be treated as the real ".zas" directory.
func TestWalkDoesNotSkipDirEndingInZasDir(t *testing.T) {
	t.Chdir(t.TempDir())
	gen := &Generator{}
	if err := os.Mkdir("docs.zas", 0o755); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat("docs.zas")
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.walk("docs.zas", info, nil); err != nil {
		t.Fatalf(`walk("docs.zas") error = %v, want nil`, err)
	}
}

// TestWalkRecordsCollisionAndSkipsSecondGoroutine is a regression test: a
// source file whose deploy output path was already claimed by another
// source (e.g. foo.html claiming foo.html before foo.md tries to, since
// swapExtension maps both to the same output) must not get a second
// renderAsync goroutine spawned for it - that's what let two goroutines
// race to write the same output file. It must instead record an error
// naming both files and return without touching gen.wg.
func TestWalkRecordsCollisionAndSkipsSecondGoroutine(t *testing.T) {
	t.Chdir(t.TempDir())
	gen := &Generator{claimedOutputs: map[string]string{"foo.html": "foo.html"}}
	if err := os.WriteFile("foo.md", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat("foo.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.walk("foo.md", info, nil); err != nil {
		t.Fatalf("walk(%q) error = %v, want nil", "foo.md", err)
	}
	gen.wg.Wait()
	if len(gen.errs) != 1 {
		t.Fatalf("errs = %v, want exactly 1 collision error", gen.errs)
	}
	if msg := gen.errs[0].Error(); !strings.Contains(msg, "foo.md") || !strings.Contains(msg, "foo.html") {
		t.Fatalf("errs[0] = %q, want it to mention both foo.md and foo.html", msg)
	}
}

func TestPathHasComponent(t *testing.T) {
	tests := []struct {
		path      string
		component string
		want      bool
	}{
		{Dir, Dir, true},
		{filepath.Join("sect", Dir), Dir, true},
		{filepath.Join(Dir, "config.yml"), Dir, true},
		{"docs.zas", Dir, false},
		{filepath.Join("docs.zas", "page.md"), Dir, false},
		{"myzasdir", Dir, false},
		{"foo.zaster", Dir, false},
	}
	for _, tt := range tests {
		if got := pathHasComponent(tt.path, tt.component); got != tt.want {
			t.Errorf("pathHasComponent(%q, %q) = %v, want %v", tt.path, tt.component, got, tt.want)
		}
	}
}

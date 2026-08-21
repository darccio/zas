package zas

import (
	"fmt"
	"os"
	"sync"
	"testing"
)

// cachedZasDirectoryConfigs is read and written by every renderAsync
// goroutine; run this under `go test -race` to prove there's no longer a
// concurrent map read/write.
func TestLoadZasDirectoryConfigConcurrent(t *testing.T) {
	t.Chdir(t.TempDir())
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll("a/sub", 0o755))
	must(os.MkdirAll("b", 0o755))
	must(os.WriteFile("a/"+DirConfigFile, []byte("lang: a\n"), 0o644))
	must(os.WriteFile("b/"+DirConfigFile, []byte("lang: b\n"), 0o644))

	gen := &Generator{}
	// a/sub has no .zas.yml of its own, so it must recurse up to a/'s config.
	cases := map[string]string{
		"a/page1.html":     "a",
		"a/sub/page2.html": "a",
		"b/page3.html":     "b",
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(cases)*50)
	for path, wantLang := range cases {
		for range 50 {
			wg.Add(1)
			go func(path, wantLang string) {
				defer wg.Done()
				cfg, _, err := gen.loadZasDirectoryConfig(path)
				if err != nil {
					errCh <- fmt.Errorf("%s: %w", path, err)
					return
				}
				if got := cfg.GetString("lang"); got != wantLang {
					errCh <- fmt.Errorf("%s: lang = %q, want %q", path, got, wantLang)
				}
			}(path, wantLang)
		}
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

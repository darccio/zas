package zas

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratorExcludesConfiguredFiles(t *testing.T) {
	withTempSite(t)
	writeFile(t, ".zas/layout.html", "<html><body>{{.Body}}</body></html>")
	writeFile(t, ".zas/config.yml", `zas:
  layout: .zas/layout.html
  deploy: .zas/deploy
  exclude:
    - README.md
    - drafts/*
site:
  baseurl: http://example.com
  language: en
mimetypes:
  text/markdown: markdown
  text/plain: plain
  text/html: html
`)
	writeFile(t, "index.md", "# Home")
	writeFile(t, "README.md", "# Internal notes")
	writeFile(t, "drafts/hidden.md", "# Draft")

	gen := Generator{Full: true}
	if err := gen.Run(); err != nil {
		t.Fatal(err)
	}

	assertExists(t, ".zas/deploy/index.html")
	assertNotExists(t, ".zas/deploy/README.html")
	assertNotExists(t, ".zas/deploy/drafts/hidden.html")
}

func TestGeneratorReapsExcludedFiles(t *testing.T) {
	withTempSite(t)
	writeFile(t, ".zas/layout.html", "<html><body>{{.Body}}</body></html>")
	writeConfig(t, nil)
	writeFile(t, "index.md", "# Home")
	writeFile(t, "README.md", "# Internal notes")

	gen := Generator{Full: true}
	if err := gen.Run(); err != nil {
		t.Fatal(err)
	}
	assertExists(t, ".zas/deploy/README.html")

	writeConfig(t, []string{"README.md"})
	gen = Generator{}
	if err := gen.Run(); err != nil {
		t.Fatal(err)
	}

	assertNotExists(t, ".zas/deploy/README.html")
}

func withTempSite(t *testing.T) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatal(err)
		}
	})
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	(&Init{}).Run()
}

func writeConfig(t *testing.T, excludes []string) {
	t.Helper()
	config := `zas:
  layout: .zas/layout.html
  deploy: .zas/deploy
`
	if excludes != nil {
		config += "  exclude:\n"
		for _, exclude := range excludes {
			config += "    - " + exclude + "\n"
		}
	}
	config += `site:
  baseurl: http://example.com
  language: en
mimetypes:
  text/markdown: markdown
  text/plain: plain
  text/html: html
`
	writeFile(t, ".zas/config.yml", config)
}

func writeFile(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), os.FileMode(ZAS_DEFAULT_DIR_PERM)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), os.FileMode(ZAS_DEFAULT_FILE_PERM)); err != nil {
		t.Fatal(err)
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func assertNotExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s not to exist", path)
	}
}

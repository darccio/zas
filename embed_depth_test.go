package zas

import (
	"os"
	"strings"
	"testing"

	"github.com/darccio/zas/internal/i18n"
)

// A file that embeds itself (directly, or through a cycle of mutually-
// embedding files) recurses through parseAndReplace/handleEmbedTags without
// bound now that raw HTML - including <embed> tags written in a .md source -
// is no longer dropped by the Markdown converter. This must fail with an
// error instead of exhausting the goroutine's stack.

func TestRenderMarkdownSelfEmbedReturnsError(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("self.md", []byte(`<embed src="self.md" type="text/markdown" />`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gen := &Generator{
		Config: ConfigSection{"mimetypes": ConfigSection{"text/markdown": "markdown"}},
		I18n:   &i18n.Build{Index: i18n.Strings{}, Origin: "en"},
	}
	err := gen.renderMarkdown("self.md")
	if err == nil {
		t.Fatal("renderMarkdown() on a self-embedding file: want error, got nil")
	}
	if !strings.Contains(err.Error(), "embed nesting") {
		t.Fatalf("error = %v, want it to report excessive embed nesting", err)
	}
}

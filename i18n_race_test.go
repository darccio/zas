package zas

import (
	"fmt"
	"sync"
	"testing"

	"github.com/melvinmt/gt"
)

// C2: every render used to share one *gt.Build, racing on SetTarget/
// Translate's internal fields and letting one page's language bleed into
// another's. Run under `go test -race`; also asserts each goroutine only
// ever observes its own language's translation.
func TestZasDataTranslationConcurrentNoRace(t *testing.T) {
	index := gt.Strings{
		"greeting": {"en": "Hello", "es": "Hola", "fr": "Bonjour"},
	}
	gen := &Generator{I18n: &gt.Build{Index: index, Origin: "en"}}
	want := map[string]string{"en": "Hello", "es": "Hola", "fr": "Bonjour"}

	var wg sync.WaitGroup
	errCh := make(chan error, len(want)*100)
	for range 100 {
		for lang, translation := range want {
			wg.Add(1)
			go func(lang, translation string) {
				defer wg.Done()
				data := NewZasData("page.html", gen)
				data.i18n.SetTarget(lang)
				got, err := data.i18n.Translate("greeting")
				if err != nil {
					errCh <- fmt.Errorf("lang=%s: %w", lang, err)
					return
				}
				if got != translation {
					errCh <- fmt.Errorf("lang=%s: Translate() = %q, want %q", lang, got, translation)
				}
			}(lang, translation)
		}
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

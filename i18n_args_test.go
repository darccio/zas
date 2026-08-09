package zas

import (
	thtml "html/template"
	"testing"

	"github.com/melvinmt/gt"
)

// Translate's arguments must be spread into gt.Translate's variadic
// parameter, not passed as a single []interface{} value - otherwise fmt's
// verbs only ever see one argument (the slice itself), and any string with
// more than one verb fails gt's verb/argument count check entirely.

func TestESpreadsSingleArgument(t *testing.T) {
	zd := &ZasData{
		Page: map[interface{}]interface{}{"language": "en"},
		i18n: &gt.Build{
			Index:  gt.Strings{"Hello %s": {"en": "Hello %s"}},
			Origin: "en",
		},
	}
	got, err := zd.E("Hello %s", "World")
	if err != nil {
		t.Fatalf("E() error = %v, want nil", err)
	}
	if want := "Hello World"; got != want {
		t.Fatalf("E() = %q, want %q", got, want)
	}
}

func TestESpreadsMultipleArguments(t *testing.T) {
	zd := &ZasData{
		Page: map[interface{}]interface{}{"language": "en"},
		i18n: &gt.Build{
			Index: gt.Strings{
				"greeting": {"en": "Hello %s, you have %s messages"},
			},
			Origin: "en",
		},
	}
	got, err := zd.E("greeting", "World", "3")
	if err != nil {
		t.Fatalf("E() error = %v, want nil", err)
	}
	if want := "Hello World, you have 3 messages"; got != want {
		t.Fatalf("E() = %q, want %q", got, want)
	}
}

func TestHSpreadsArguments(t *testing.T) {
	zd := &ZasData{
		Page: map[interface{}]interface{}{"language": "en"},
		i18n: &gt.Build{
			Index:  gt.Strings{"Hello %s": {"en": "Hello %s"}},
			Origin: "en",
		},
	}
	got, err := zd.H("Hello %s", "World")
	if err != nil {
		t.Fatalf("H() error = %v, want nil", err)
	}
	if want := thtml.HTML("Hello World"); got != want {
		t.Fatalf("H() = %q, want %q", got, want)
	}
}

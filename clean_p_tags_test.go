package zas

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// html.Node.Data holds the tag name for an ElementNode, not its text
// content, so reading it to decide whether a <p> was "visually empty" always
// saw the literal string "p" - the removal branch below could never run.

func parseFragment(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestCleanUnnecessaryPTagsRemovesChildlessParagraph(t *testing.T) {
	// The artifact HTML5 parser error recovery leaves behind when a block
	// element inline inside a Markdown paragraph implicitly closes it.
	doc := parseFragment(t, `<p></p><div>Hello world!</div><p></p>`)
	gen := &Generator{}
	gen.cleanUnnecessaryPTags(doc)
	if doc.Find("p").Length() != 0 {
		t.Fatalf("p elements = %d, want 0", doc.Find("p").Length())
	}
	if doc.Find("div").Length() != 1 {
		t.Fatalf("div elements = %d, want 1 (must survive untouched)", doc.Find("div").Length())
	}
}

func TestCleanUnnecessaryPTagsKeepsTextParagraph(t *testing.T) {
	doc := parseFragment(t, `<p>Hello world!</p>`)
	gen := &Generator{}
	gen.cleanUnnecessaryPTags(doc)
	if doc.Find("p").Length() != 1 {
		t.Fatalf("p elements = %d, want 1", doc.Find("p").Length())
	}
}

func TestCleanUnnecessaryPTagsKeepsSingleChildParagraph(t *testing.T) {
	// A <p> whose only child is an element (not a removal target) must
	// survive: unwrapping it would change ordinary Markdown paragraph
	// semantics such as <p><strong>bold sentence</strong></p>.
	doc := parseFragment(t, `<p><strong>bold sentence</strong></p>`)
	gen := &Generator{}
	gen.cleanUnnecessaryPTags(doc)
	if doc.Find("p").Length() != 1 {
		t.Fatalf("p elements = %d, want 1", doc.Find("p").Length())
	}
	if doc.Find("strong").Length() != 1 {
		t.Fatalf("strong elements = %d, want 1", doc.Find("strong").Length())
	}
}

func TestCleanUnnecessaryPTagsKeepsImageOnlyParagraph(t *testing.T) {
	doc := parseFragment(t, `<p><img src="x.png"></p>`)
	gen := &Generator{}
	gen.cleanUnnecessaryPTags(doc)
	if doc.Find("p").Length() != 1 {
		t.Fatalf("p elements = %d, want 1", doc.Find("p").Length())
	}
	if doc.Find("img").Length() != 1 {
		t.Fatalf("img elements = %d, want 1", doc.Find("img").Length())
	}
}

func TestCleanUnnecessaryPTagsKeepsNbspParagraph(t *testing.T) {
	// A non-breaking space is a real (if invisible) text node, and a common
	// spacer idiom - it must not be treated the same as a childless <p>.
	doc := parseFragment(t, `<p>&nbsp;</p>`)
	gen := &Generator{}
	gen.cleanUnnecessaryPTags(doc)
	if doc.Find("p").Length() != 1 {
		t.Fatalf("p elements = %d, want 1", doc.Find("p").Length())
	}
}

package zas

import (
	"os"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// Plain embeds must insert the rendered file as a text node. html.Node.Data
// holds the tag name for an ElementNode, not its text content, so writing
// straight into the parent's Data field renamed the parent tag instead of
// inserting text.

func TestPlainEmbedDoesNotRenameParentTag(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("note.txt", []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<div><embed src="note.txt" type="text/plain"></div>`))
	if err != nil {
		t.Fatal(err)
	}
	gen := &Generator{}
	if err := gen.Plain(doc.Find("embed"), doc, &ZasData{}); err != nil {
		t.Fatalf("Plain() error = %v, want nil", err)
	}
	div := doc.Find("div")
	if div.Length() != 1 {
		t.Fatalf("div elements = %d, want 1 (parent tag must survive)", div.Length())
	}
	if got := goquery.NodeName(div); got != "div" {
		t.Fatalf("parent tag = %q, want %q", got, "div")
	}
}

func TestPlainEmbedInsertsEscapedText(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("note.txt", []byte("Hello <b>World</b>"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<div><embed src="note.txt" type="text/plain"></div>`))
	if err != nil {
		t.Fatal(err)
	}
	gen := &Generator{}
	if err := gen.Plain(doc.Find("embed"), doc, &ZasData{}); err != nil {
		t.Fatalf("Plain() error = %v, want nil", err)
	}
	div := doc.Find("div")
	if div.Children().Length() != 0 {
		t.Fatalf("div has %d child elements, want 0 - embedded text must not become markup", div.Children().Length())
	}
	if got, want := div.Text(), "Hello <b>World</b>"; got != want {
		t.Fatalf("div.Text() = %q, want %q", got, want)
	}
}

func TestPlainEmbedExecutesTemplate(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("note.txt", []byte("path is {{.Path}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<div><embed src="note.txt" type="text/plain"></div>`))
	if err != nil {
		t.Fatal(err)
	}
	gen := &Generator{}
	data := &ZasData{Path: "/about.html"}
	if err := gen.Plain(doc.Find("embed"), doc, data); err != nil {
		t.Fatalf("Plain() error = %v, want nil", err)
	}
	if want, got := "path is /about.html", doc.Find("div").Text(); got != want {
		t.Fatalf("div.Text() = %q, want %q", got, want)
	}
}

func TestPlainEmbedRemovesEmbedTag(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("note.txt", []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<div><embed src="note.txt" type="text/plain"></div>`))
	if err != nil {
		t.Fatal(err)
	}
	gen := &Generator{}
	if err := gen.Plain(doc.Find("embed"), doc, &ZasData{}); err != nil {
		t.Fatalf("Plain() error = %v, want nil", err)
	}
	if doc.Find("embed").Length() != 0 {
		t.Fatal("embed tag still present after Plain()")
	}
}

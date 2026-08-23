package zas

import (
	"bytes"
	"encoding/xml"
	"testing"
)

// TestSitemapXhtmlLinkMarshalsLiteralPrefix pins the exact byte output of
// marshaling a populated xhtml:link: encoding/xml treats a colon-joined
// struct tag ("xhtml:link", "xmlns:xhtml") as a literal element/attribute
// name rather than a namespace+prefix pair (that requires a *space*-
// separated tag), so this is the standard workaround for emitting a fixed
// "xhtml:" prefix without fighting encoding/xml's own auto-generated-
// prefix namespace handling - see the doc comment on sitemapXhtmlLink.
func TestSitemapXhtmlLinkMarshalsLiteralPrefix(t *testing.T) {
	urlset := sitemapURLSet{
		Xmlns:      sitemapNS,
		XmlnsXhtml: sitemapXhtmlNS,
		URLs: []sitemapURLEntry{
			{
				Loc:     "https://example.com/index.html",
				Lastmod: "2020-01-02T03:04:05Z",
				Links: []sitemapXhtmlLink{
					{Rel: "alternate", Hreflang: "es", Href: "https://example.com/es/index.html"},
				},
			},
		},
	}
	var buf bytes.Buffer
	if err := writeSitemapXML(&buf, &urlset); err != nil {
		t.Fatalf("writeSitemapXML() error = %v, want nil", err)
	}
	want := xml.Header +
		`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">` +
		`<url><loc>https://example.com/index.html</loc><lastmod>2020-01-02T03:04:05Z</lastmod>` +
		`<xhtml:link rel="alternate" hreflang="es" href="https://example.com/es/index.html"></xhtml:link>` +
		`</url></urlset>`
	if got := buf.String(); got != want {
		t.Fatalf("writeSitemapXML() =\n%q\nwant\n%q", got, want)
	}
}

// TestSitemapURLSetOmitsXhtmlNamespaceWhenNoLinks confirms a plain,
// single-language sitemap (no hreflang clusters at all) stays a bare
// <urlset xmlns="..."> with no xmlns:xhtml attribute - XmlnsXhtml is only
// ever set by writeSitemapShard when at least one real cluster exists.
func TestSitemapURLSetOmitsXhtmlNamespaceWhenNoLinks(t *testing.T) {
	urlset := sitemapURLSet{
		Xmlns: sitemapNS,
		URLs:  []sitemapURLEntry{{Loc: "https://example.com/index.html"}},
	}
	var buf bytes.Buffer
	if err := writeSitemapXML(&buf, &urlset); err != nil {
		t.Fatalf("writeSitemapXML() error = %v, want nil", err)
	}
	want := xml.Header +
		`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` +
		`<url><loc>https://example.com/index.html</loc></url></urlset>`
	if got := buf.String(); got != want {
		t.Fatalf("writeSitemapXML() =\n%q\nwant\n%q", got, want)
	}
}

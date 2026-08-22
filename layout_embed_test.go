/*
 * Copyright (c) 2013 Dario Castañé.
 * This file is part of Zas.
 *
 * Zas is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * Zas is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with Zas.  If not, see <http://www.gnu.org/licenses/>.
 */

package zas

import (
	"strings"
	"testing"
)

// render only ever parses a page's own source. An <embed> tag written
// directly into layout.html - outside {{.Body}} - is never seen by that
// parse; the only place it can resolve is Generate's second parseAndReplace
// pass over the fully assembled page (layout + body). This pins that the
// second pass is load-bearing, not redundant work: dropping it, or
// replacing it with something that only walks the page's own body, would
// silently stop layout-level embeds from working.
//
// This assertion is deliberately loose about exact markup shape: Html's
// handler (generate.go) splices in the embedded file's whole parsed
// document - <html><head>...<body>... - rather than just its body's
// contents, same pre-existing gap F5 (F5-embed-injects-body-html.md)
// already tracks for page-body-level embeds. Page-body embeds only look
// clean today because they get a second, further re-parse (this same
// Generate pass) whose HTML5 error recovery quietly drops the duplicate
// html/body tags; a layout-level embed like this one has no such further
// pass to hide behind, so the nesting survives into deployed output
// verbatim - confirmed live and noted on F5's own file. Once F5's
// remaining sub-claims land, this test should keep passing unchanged and
// could be tightened to assert the exact clean markup.
func TestGenerateResolvesEmbedInLayoutItself(t *testing.T) {
	newTestSite(t, "layout-embed-site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	got := readDeploy(t, "index.html")
	if !strings.Contains(got, "footer from the layout itself") {
		t.Fatalf("deployed index.html = %q, want it to contain the layout's own embedded footer", got)
	}
	if strings.Contains(got, "<embed") {
		t.Fatalf("deployed index.html = %q, want the layout's <embed> tag replaced, not left in place", got)
	}
}

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

// <embed src="..."> used to be read with os.ReadFile(src) resolved against
// the process's current directory (the site root), regardless of where the
// embedding page itself lived. A page written inside a subdirectory that
// embedded a same-named file present at both the site root and its own
// directory would silently get the root copy - no error, no warning, just
// the wrong content. resolveEmbedSrc now resolves a page-body embed
// relative to that page's own directory instead (see ZasData.embedBaseDir
// in data.go). This fixture places different content in a same-named
// nav.md/nav.txt/nav.html at both the site root and in "section/", and
// "section/index.html" embeds all three by their bare filename: if
// resolution were still root-relative, every one of these assertions would
// see the "ROOT ..." text instead.
func TestGenerateEmbedResolvesRelativeToPage(t *testing.T) {
	newTestSite(t, "embed-page-relative-site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	got := readDeploy(t, "section/index.html")
	for _, want := range []string{"SECTION markdown nav", "SECTION plain nav", "SECTION html nav"} {
		if !strings.Contains(got, want) {
			t.Fatalf("deployed section/index.html = %q, want it to contain %q (the section-local file)", got, want)
		}
	}
	for _, unwanted := range []string{"ROOT markdown nav", "ROOT plain nav", "ROOT html nav"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("deployed section/index.html = %q, want it not to contain %q (the site-root file with the same name)", got, unwanted)
		}
	}
}

// A chain of embeds - a page embedding a file which itself embeds a
// further file via a relative src - should have each src resolve against
// the file that wrote it, not against the outermost page. This fixture's
// root "index.html" embeds "partials/nav.md", which in turn embeds
// "footer.md" using only the bare filename (no "partials/" prefix): that
// only resolves if the embed base directory follows the embed chain down
// into "partials/" rather than staying pinned to the outer page's own
// directory (the site root). If it stayed pinned to the root, this would
// fail generation entirely with a missing-file error, since there is no
// "footer.md" at the site root.
func TestGenerateChainedEmbedResolvesRelativeToEachFile(t *testing.T) {
	newTestSite(t, "embed-chained-relative-site")
	if err := generate(t); err != nil {
		t.Fatalf("generate() error = %v, want nil", err)
	}
	got := readDeploy(t, "index.html")
	for _, want := range []string{"partial nav", "partial footer"} {
		if !strings.Contains(got, want) {
			t.Fatalf("deployed index.html = %q, want it to contain %q", got, want)
		}
	}
}

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
	"io"
	"os"
	"strings"
)

// robotsFileName is the deploy-root file appendSitemapToRobots reads and
// writes. A site-authored robots.txt already reaches deploy unmodified via
// renderAsync's ordinary unmatched-extension copy path (gen.copy) - this
// file only ever adds or confirms a "Sitemap:" directive on top of that.
const robotsFileName = "robots.txt"

// minimalRobotsTxt is what appendSitemapToRobots writes when a site has
// sitemap generation on but no robots.txt of its own: a permissive
// crawl policy (nothing here should be more restrictive than a site that
// never thought about robots.txt at all) plus the Sitemap directive
// itself, so the sitemap stays discoverable with zero configuration
// beyond turning site.sitemap on.
const minimalRobotsTxt = "User-agent: *\nAllow: /\n"

// appendSitemapToRobots ensures the deploy-side robots.txt declares
// sitemapURL via a "Sitemap:" directive: idempotently appending one to a
// site-authored file if it's missing, or synthesizing a minimal robots.txt
// from scratch if the site has none at all. It runs unconditionally on
// every Run (independent of incremental staleness), so it self-corrects
// if a fresh copy of an edited source robots.txt overwrote deploy's
// previously-appended line.
func (gen *Generator) appendSitemapToRobots(sitemapURL string) error {
	path := gen.BuildDeployPath(robotsFileName)
	existing, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		content := minimalRobotsTxt + "Sitemap: " + sitemapURL + "\n"
		return gen.atomicWriteFile(path, func(w io.Writer) error {
			_, writeErr := io.WriteString(w, content)
			return writeErr
		})
	}
	if robotsHasSitemapDirective(string(existing), sitemapURL) {
		return nil
	}
	content := string(existing)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "Sitemap: " + sitemapURL + "\n"
	return gen.atomicWriteFile(path, func(w io.Writer) error {
		_, writeErr := io.WriteString(w, content)
		return writeErr
	})
}

// robotsHasSitemapDirective reports whether content already has a
// "Sitemap:" line (matched case-insensitively, per the sitemaps.org
// convention that the directive name itself isn't case-sensitive) whose
// trimmed value equals sitemapURL exactly.
func robotsHasSitemapDirective(content, sitemapURL string) bool {
	for _, line := range strings.Split(content, "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(key), "sitemap") {
			continue
		}
		if strings.TrimSpace(value) == sitemapURL {
			return true
		}
	}
	return false
}

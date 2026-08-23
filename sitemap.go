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
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	sitemapNS            = "http://www.sitemaps.org/schemas/sitemap/0.9"
	sitemapXhtmlNS       = "http://www.w3.org/1999/xhtml"
	sitemapFileName      = "sitemap.xml"
	sitemapIndexFileName = "sitemap-index.xml"
)

// sitemapMaxURLsPerFile and sitemapMaxBytesPerFile bound how many <url>
// entries (and how many estimated bytes) a single sitemap file may hold
// before writeSitemapFiles splits the sitemap into an index plus numbered
// shards. Both stay comfortably under the sitemaps.org hard limits (50,000
// URLs / 50MB uncompressed) rather than sitting right at them - the byte
// count in particular is an estimate, not a measured value (see
// estimateSitemapEntryBytes). They're vars, not consts, purely so an e2e
// test can shrink sitemapMaxURLsPerFile to exercise sharding without
// writing tens of thousands of real files to disk.
var (
	sitemapMaxURLsPerFile  = 45000
	sitemapMaxBytesPerFile = 45 * 1000 * 1000
)

// sitemapURLSet is the root element of a plain (non-sharded) sitemap.
type sitemapURLSet struct {
	XMLName    xml.Name          `xml:"urlset"`
	Xmlns      string            `xml:"xmlns,attr"`
	XmlnsXhtml string            `xml:"xmlns:xhtml,attr,omitempty"`
	URLs       []sitemapURLEntry `xml:"url"`
}

// sitemapURLEntry is one <url> block: a page's absolute location, its best
// available last-modification date, and (for a page that's part of a
// multilingual translation cluster) its reciprocal hreflang alternates.
type sitemapURLEntry struct {
	Loc     string             `xml:"loc"`
	Lastmod string             `xml:"lastmod,omitempty"`
	Links   []sitemapXhtmlLink `xml:"xhtml:link,omitempty"`
}

// sitemapXhtmlLink is one <xhtml:link rel="alternate" ...> hreflang
// annotation. The "xhtml:" prefix in the struct tags below is written by
// encoding/xml.Marshal as a literal element/attribute name rather than
// parsed as a namespace prefix - encoding/xml only treats a *space*
// separated tag ("uri local") as a namespace+name pair, so a colon-joined
// tag like "xhtml:link" or "xmlns:xhtml" passes straight through as-is.
// That's exactly the fixed, well-known "xhtml:" prefix the sitemap hreflang
// convention expects, and it sidesteps encoding/xml's namespace support,
// which has no way to choose a prefix on marshal (only an opaque
// auto-generated one). Attribute values are still fully escaped normally;
// only the name token bypasses namespace resolution.
type sitemapXhtmlLink struct {
	Rel      string `xml:"rel,attr"`
	Hreflang string `xml:"hreflang,attr"`
	Href     string `xml:"href,attr"`
}

// sitemapIndex is the root element of a sitemap index, used once the
// deployed site's URL count or estimated size crosses sitemapMaxURLsPerFile
// / sitemapMaxBytesPerFile and the sitemap has to be split into shards.
type sitemapIndex struct {
	XMLName  xml.Name            `xml:"sitemapindex"`
	Xmlns    string              `xml:"xmlns,attr"`
	Sitemaps []sitemapIndexEntry `xml:"sitemap"`
}

// sitemapIndexEntry is one <sitemap> entry in a sitemap index, pointing at
// one shard.
type sitemapIndexEntry struct {
	Loc     string `xml:"loc"`
	Lastmod string `xml:"lastmod,omitempty"`
}

// sitemapEntry is generateSitemap's internal working representation of one
// deployed page, before XML marshaling.
type sitemapEntry struct {
	// deployRelPath is slash-separated, relative to the deploy root, e.g.
	// "es/index.html".
	deployRelPath string
	// sourcePath is the best-effort guess at the source file that produced
	// this deploy output (see resolveSitemapSourcePath), or "" if none
	// could be found.
	sourcePath string
	// loc is the entry's absolute URL.
	loc string
	// lastmod is RFC3339-formatted, or "" if it couldn't be determined.
	lastmod string
	// language is this page's resolved language, or "" if undeterminable.
	language string
	// clusterKey groups this entry with its translations, if any - see
	// sitemapClusterKey. Entries with no translation cluster get a
	// clusterKey equal to their own deployRelPath, which is never shared
	// by another entry.
	clusterKey string
}

// joinSiteURL builds an absolute URL from baseurl and a deploy-relative
// path (e.g. "es/index.html", or ZasData.Path's own leading-"/" form).
// baseurl is documented as being configured without a trailing slash, but
// nothing enforces that, so any trailing slash is trimmed here - matching
// ZasData.URL's own trim-then-join rule exactly, so a sitemap's <loc>
// values are always identical to what the corresponding page's own
// {{.URL}} would render.
func joinSiteURL(baseurl, path string) string {
	return strings.TrimRight(baseurl, "/") + "/" + strings.TrimLeft(path, "/")
}

// generateSitemap builds and writes sitemap.xml (or, past
// sitemapMaxURLsPerFile/sitemapMaxBytesPerFile, sitemap-index.xml plus
// numbered shards) to the deploy root, returning the absolute URL of
// whichever file robots.txt should point at. It returns ("", nil), writing
// nothing, when site.sitemap is off or the deploy tree has no .html pages
// at all - there's no point publishing an empty or disabled sitemap.
func (gen *Generator) generateSitemap() (sitemapURL string, err error) {
	if !gen.Config.GetSection("site").GetBool("sitemap") {
		return "", nil
	}
	entries, err := gen.collectSitemapEntries()
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", nil
	}
	clusters, err := buildHreflangClusters(entries)
	if err != nil {
		return "", err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].loc < entries[j].loc })
	shards := shardSitemapEntries(entries)
	return gen.writeSitemapFiles(shards, clusters)
}

// collectSitemapEntries walks the finished deploy tree (like reaper, not
// like the concurrent source walk in walk/renderAsync) collecting one
// sitemapEntry per deployed .html file. Walking deploy output rather than
// reusing claimedOutputs (populated only for pages actually re-rendered
// this run) is what makes this correct on an incremental run: it sees
// every page currently in deploy, not just the ones this particular Run
// touched. A page excluded via "publish: false" never reaches deploy in
// the first place, so it's already absent from this walk with no special
// casing needed.
func (gen *Generator) collectSitemapEntries() ([]*sitemapEntry, error) {
	baseurl := gen.Config.GetSection("site").GetString("baseurl")
	deployPath := gen.GetDeployPath()
	gitAvailable := gen.sitemapGitAvailable()
	var entries []*sitemapEntry
	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !hasExtension(path, ".html") {
			return nil
		}
		deployRelPath := filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(path, deployPath), string(filepath.Separator)))
		sourcePath := gen.resolveSitemapSourcePath(path)
		entry := &sitemapEntry{
			deployRelPath: deployRelPath,
			sourcePath:    sourcePath,
			loc:           joinSiteURL(baseurl, deployRelPath),
			clusterKey:    deployRelPath,
		}
		if sourcePath != "" {
			entry.lastmod = sitemapLastmod(sourcePath, gitAvailable)
			language, langDir, hasLangDir, metaErr := gen.sitemapPageMeta(sourcePath)
			if metaErr != nil {
				gen.recordErr(fmt.Errorf("%s: %w", sourcePath, metaErr))
			} else {
				entry.language = language
				entry.clusterKey = sitemapClusterKey(deployRelPath, langDir, hasLangDir)
			}
		}
		entries = append(entries, entry)
		return nil
	}
	if err := filepath.Walk(deployPath, walkFn); err != nil {
		return nil, err
	}
	return entries, nil
}

// resolveSitemapSourcePath reconstructs the likely source file for a
// deployed .html path, mirroring reaper's own source-path reconstruction:
// swap the deploy root for ".", then, since swapExtension always
// normalizes to ".md"'s own casing, fall back to a case-insensitive
// directory scan if a plain os.Stat for the guessed ".md" name doesn't
// find it. Returns "" if neither the ".html" source nor any case variant
// of the ".md" guess actually exists.
func (gen *Generator) resolveSitemapSourcePath(deployPath string) string {
	sourcePath := "." + strings.TrimPrefix(deployPath, gen.GetDeployPath())
	if _, err := os.Stat(sourcePath); err == nil {
		return sourcePath
	}
	mdGuess := swapExtension(sourcePath, ".html", ".md")
	if _, err := os.Stat(mdGuess); err == nil {
		return mdGuess
	}
	if actual, ok := findFoldedName(mdGuess); ok {
		return actual
	}
	return ""
}

// findFoldedName looks for a file in name's directory whose name matches
// name's basename case-insensitively, returning its real, on-disk name and
// casing. Unlike Generator.existsFold (which only remembers *whether* a
// lowercased name exists, discarding the real casing - fine for reaper,
// which never opens the file it's asking about), resolveSitemapSourcePath
// needs an actually openable path, since sitemapPageMeta/sitemapLastmod go
// on to read and stat it. This is its own small, uncached directory scan:
// the sitemap pass runs once, sequentially, over the finished deploy tree,
// so the rare case-mismatch this handles doesn't need the render path's
// per-run cache.
func findFoldedName(name string) (string, bool) {
	dir := filepath.Dir(name)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	target := strings.ToLower(filepath.Base(name))
	for _, e := range entries {
		if strings.ToLower(e.Name()) == target {
			return filepath.Join(dir, e.Name()), true
		}
	}
	return "", false
}

// writeSitemapXML writes the XML declaration followed by v's marshaled
// form to w.
func writeSitemapXML(w io.Writer, v any) error {
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	return xml.NewEncoder(w).Encode(v)
}

// sitemapGitAvailable reports whether the current directory is inside a
// git working tree with a usable git binary on PATH, checked once per Run
// (not once per file) via a single `git rev-parse`, so a non-git site pays
// one process spawn total instead of one per page for a lookup that could
// never succeed.
func (gen *Generator) sitemapGitAvailable() bool {
	return exec.Command("git", "rev-parse", "--is-inside-work-tree").Run() == nil
}

// gitCommitDate returns sourcePath's most recent commit's committer date
// (git's %cI, strict ISO 8601 = time.RFC3339) as reported by `git log -1
// --format=%cI -- <sourcePath>`. ok is false, uniformly and without
// distinguishing why, whenever this can't produce a usable date: git isn't
// installed, the site isn't inside a git working tree, sourcePath isn't
// tracked, or (defensively) git's own output doesn't parse. This is
// intentional - see sitemapLastmod's fallback chain and the CI/shallow-
// clone caveat documented in README.md.
func gitCommitDate(sourcePath string) (time.Time, bool) {
	out, err := exec.Command("git", "log", "-1", "--format=%cI", "--", sourcePath).Output()
	if err != nil {
		return time.Time{}, false
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, s)
	return t, err == nil
}

// sitemapLastmod resolves sourcePath's <lastmod>: git commit date (when
// gitAvailable), else sourcePath's own mtime, else "" - which omits
// <lastmod> for that URL entirely rather than publish a guessed value.
func sitemapLastmod(sourcePath string, gitAvailable bool) string {
	if gitAvailable {
		if t, ok := gitCommitDate(sourcePath); ok {
			return t.Format(time.RFC3339)
		}
	}
	if info, err := os.Stat(sourcePath); err == nil {
		return info.ModTime().Format(time.RFC3339)
	}
	return ""
}

// sitemapPageMeta re-derives, directly from sourcePath's own leading config
// comment and its directory ancestry, the same "language" ZasData.Resolve
// would produce for a live render of this page: page config comment >
// nearest ancestor .zas.yml (loadZasDirectoryConfig's own single, non-
// merging nearest-ancestor lookup - stopping there even if that file
// doesn't itself declare "language", exactly like Resolve does) > site
// default.
//
// It separately reports whether the nearest ancestor .zas.yml (if any) is
// what supplied the language - langDir/hasLangDir - which is what
// sitemapClusterKey uses to detect the i18n language-subdirectory
// convention documented in README.md. This is deliberately independent of
// whichever source actually won for language: a page can sit inside a
// declared language subdirectory and still override its own language via
// a leading comment, and per the confirmed scope for this feature, it's
// still part of that subdirectory's translation cluster - clustering is
// about directory structure, not about which precedence tier resolved
// this particular page's own value.
//
// This deliberately re-implements Resolve's precedence rather than
// reusing ZasData/render: it runs once per deployed URL on the sequential
// post-build sitemap pass, not on render's hot, concurrent, per-goroutine
// path, and building a full ZasData here would need most of Run's other
// startup state (I18n, ...) for no benefit.
func (gen *Generator) sitemapPageMeta(sourcePath string) (language, langDir string, hasLangDir bool, err error) {
	pageLanguage, pageHasLanguage, err := pageLanguageFromComment(sourcePath)
	if err != nil {
		return "", "", false, err
	}
	dirConfig, _, _ := gen.loadZasDirectoryConfig(sourcePath)
	var dirLanguage string
	var dirHasLanguage bool
	if dirConfig != nil {
		if value, present := dirConfig["language"]; present {
			s, isString := value.(string)
			if !isString {
				return "", "", false, fmt.Errorf("config value %q must be a string, got %T", "language", value)
			}
			dirLanguage, dirHasLanguage = s, true
		}
	}
	if dirHasLanguage {
		if dir, ok := nearestZasDirectoryConfigDir(sourcePath); ok && dir != "." {
			langDir, hasLangDir = dir, true
		}
	}
	switch {
	case pageHasLanguage:
		language = pageLanguage
	case dirHasLanguage:
		language = dirLanguage
	default:
		language = gen.Config.GetSection("site").GetString("language")
	}
	return language, langDir, hasLangDir, nil
}

// pageLanguageFromComment extracts the "language" key from sourcePath's
// own leading config comment, reusing earlyPageConfig - the same silent,
// best-effort extractor render's own page-body preview uses - so an
// unreadable file, a missing comment, or a comment that isn't valid YAML
// all quietly produce a nil map here exactly as they do there, with no
// error: each of those is already reported (or isn't an error at all)
// elsewhere in the normal render path, and this standalone lookup doesn't
// duplicate that diagnostic. Only a "language" value that's present but
// isn't a string is treated as an error, matching ZasData.Resolve's own
// validation.
func pageLanguageFromComment(sourcePath string) (language string, ok bool, err error) {
	input, _ := os.ReadFile(sourcePath)
	value, present := earlyPageConfig(input)["language"]
	if !present {
		return "", false, nil
	}
	s, isString := value.(string)
	if !isString {
		return "", false, fmt.Errorf("config value %q must be a string, got %T", "language", value)
	}
	return s, true, nil
}

// nearestZasDirectoryConfigDir finds the directory holding the nearest
// ancestor DirConfigFile (.zas.yml) for sourcePath, walking upward exactly
// like loadZasDirectoryConfig's own recursion (stopping at ".", the site
// root, either way). It's a separate, uncached walk rather than a reuse of
// loadZasDirectoryConfig's cache, which only stores a directory's resolved
// config content and mtime, not which directory supplied it - this only
// runs once per deployed URL on the sequential post-build sitemap pass,
// not on render's hot path.
func nearestZasDirectoryConfigDir(sourcePath string) (dir string, ok bool) {
	dir = filepath.Dir(sourcePath)
	for {
		if _, err := os.Stat(filepath.Join(dir, DirConfigFile)); err == nil {
			return dir, true
		}
		if dir == "." {
			return "", false
		}
		dir = filepath.Dir(dir)
	}
}

// sitemapClusterKey strips langDir (if hasLangDir) from deployRelPath,
// giving the key translations of the same content share: es/faq.html and
// ca/faq.html and root faq.html all reduce to "faq.html". Only the
// nearest declaring ancestor's own path component(s) are stripped - a
// nested directory within a language keeps its own structure, so
// es/blog/post.html -> "blog/post.html", matching root-language
// blog/post.html's own "blog/post.html". A page with no qualifying
// ancestor (hasLangDir false) keeps its full deployRelPath as its
// clusterKey - a guaranteed singleton "cluster" of one, since no other
// entry can share the exact same deployRelPath.
func sitemapClusterKey(deployRelPath, langDir string, hasLangDir bool) string {
	if !hasLangDir {
		return deployRelPath
	}
	prefix := filepath.ToSlash(langDir)
	prefix = strings.TrimPrefix(prefix, "./")
	if prefix == "." || prefix == "" {
		return deployRelPath
	}
	prefix += "/"
	if !strings.HasPrefix(deployRelPath, prefix) {
		return deployRelPath
	}
	return strings.TrimPrefix(deployRelPath, prefix)
}

// buildHreflangClusters groups entries by clusterKey, and attaches a
// reciprocal xhtml:link set (including a self-reference) to every member
// of any cluster with 2 or more entries. A singleton cluster (a translated
// page with no sibling yet, or a page not in any directory-based i18n
// cluster at all) is not an error - partial translation coverage is
// expected - it's simply excluded from hreflang for that URL. Two members
// of one cluster resolving to the *same* language is a hard error: there's
// no way to say which is canonical, so the whole sitemap step fails rather
// than ship a self-contradictory hreflang set.
func buildHreflangClusters(entries []*sitemapEntry) (map[string][]*sitemapEntry, error) {
	byKey := make(map[string][]*sitemapEntry)
	for _, e := range entries {
		if e.language == "" {
			continue
		}
		byKey[e.clusterKey] = append(byKey[e.clusterKey], e)
	}
	clusters := make(map[string][]*sitemapEntry)
	for key, members := range byKey {
		if len(members) < 2 {
			continue
		}
		seen := make(map[string]string, len(members))
		for _, m := range members {
			if other, dup := seen[m.language]; dup {
				return nil, fmt.Errorf("sitemap: %s and %s resolve to the same language %q within one translation cluster - can't build reciprocal hreflang", other, m.deployRelPath, m.language)
			}
			seen[m.language] = m.deployRelPath
		}
		clusters[key] = members
	}
	return clusters, nil
}

// estimateSitemapEntryBytes conservatively estimates the serialized size
// of entry's <url> block, without actually marshaling it: sharding is a
// defensive backstop for sites nowhere near common Zas site sizes, so an
// approximate-but-safe estimate is preferred over the complexity of
// incremental XML encoding+measuring.
func estimateSitemapEntryBytes(entry *sitemapEntry, clusters map[string][]*sitemapEntry) int {
	const perURLOverhead = 64 // "<url>", "</url>", "<loc>", "</loc>", "<lastmod>", "</lastmod>"
	const perLinkOverhead = 64
	n := perURLOverhead + len(entry.loc) + len(entry.lastmod)
	n += len(clusters[entry.clusterKey]) * (perLinkOverhead + len(entry.loc) + 16)
	return n
}

// shardSitemapEntries splits entries (already sorted by loc) into
// consecutive shards, each capped at sitemapMaxURLsPerFile URLs and an
// estimated sitemapMaxBytesPerFile serialized bytes. Stable shard
// boundaries across runs (as long as the URL set itself doesn't change)
// fall out of entries already being sorted before this is called.
func shardSitemapEntries(entries []*sitemapEntry) [][]*sitemapEntry {
	return shardSitemapEntriesWithClusters(entries, nil)
}

func shardSitemapEntriesWithClusters(entries []*sitemapEntry, clusters map[string][]*sitemapEntry) [][]*sitemapEntry {
	if len(entries) == 0 {
		return nil
	}
	var shards [][]*sitemapEntry
	var current []*sitemapEntry
	var currentBytes int
	for _, e := range entries {
		size := estimateSitemapEntryBytes(e, clusters)
		if len(current) > 0 && (len(current) >= sitemapMaxURLsPerFile || currentBytes+size > sitemapMaxBytesPerFile) {
			shards = append(shards, current)
			current = nil
			currentBytes = 0
		}
		current = append(current, e)
		currentBytes += size
	}
	if len(current) > 0 {
		shards = append(shards, current)
	}
	return shards
}

// writeSitemapFiles writes shards to the deploy root: a single sitemap.xml
// when there's only one shard, or sitemap-1.xml, sitemap-2.xml, ... plus a
// sitemap-index.xml when there's more than one. It returns the absolute
// URL of whichever file robots.txt should reference - the plain sitemap's
// own URL, or the index's, never an individual shard's.
func (gen *Generator) writeSitemapFiles(shards [][]*sitemapEntry, clusters map[string][]*sitemapEntry) (string, error) {
	baseurl := gen.Config.GetSection("site").GetString("baseurl")
	hasHreflang := len(clusters) > 0
	if len(shards) == 1 {
		if err := gen.writeSitemapShard(sitemapFileName, shards[0], clusters, hasHreflang); err != nil {
			return "", err
		}
		return joinSiteURL(baseurl, sitemapFileName), nil
	}
	index := sitemapIndex{Xmlns: sitemapNS}
	for i, shard := range shards {
		name := fmt.Sprintf("sitemap-%d.xml", i+1)
		if err := gen.writeSitemapShard(name, shard, clusters, hasHreflang); err != nil {
			return "", err
		}
		index.Sitemaps = append(index.Sitemaps, sitemapIndexEntry{
			Loc:     joinSiteURL(baseurl, name),
			Lastmod: maxLastmod(shard),
		})
	}
	if err := gen.atomicWriteFile(gen.BuildDeployPath(sitemapIndexFileName), func(w io.Writer) error {
		return writeSitemapXML(w, &index)
	}); err != nil {
		return "", err
	}
	return joinSiteURL(baseurl, sitemapIndexFileName), nil
}

// writeSitemapShard writes one sitemap file (either the only one, or one
// numbered shard among several) to name under the deploy root.
func (gen *Generator) writeSitemapShard(name string, entries []*sitemapEntry, clusters map[string][]*sitemapEntry, hasHreflang bool) error {
	urlset := sitemapURLSet{Xmlns: sitemapNS}
	if hasHreflang {
		urlset.XmlnsXhtml = sitemapXhtmlNS
	}
	for _, e := range entries {
		entry := sitemapURLEntry{Loc: e.loc, Lastmod: e.lastmod}
		if members := clusters[e.clusterKey]; len(members) > 1 {
			for _, m := range members {
				entry.Links = append(entry.Links, sitemapXhtmlLink{
					Rel:      "alternate",
					Hreflang: m.language,
					Href:     m.loc,
				})
			}
		}
		urlset.URLs = append(urlset.URLs, entry)
	}
	return gen.atomicWriteFile(gen.BuildDeployPath(name), func(w io.Writer) error {
		return writeSitemapXML(w, &urlset)
	})
}

// maxLastmod returns the latest non-empty lastmod among entries, or "" if
// none has one, for a sitemap index's own per-shard <lastmod>.
func maxLastmod(entries []*sitemapEntry) string {
	var latest string
	for _, e := range entries {
		if e.lastmod == "" {
			continue
		}
		if latest == "" || e.lastmod > latest {
			latest = e.lastmod
		}
	}
	return latest
}

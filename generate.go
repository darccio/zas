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
	"bytes"
	"errors"
	"fmt"
	thtml "html/template"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	ttext "text/template"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/melvinmt/gt"
	markdown "github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
	html5 "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	yaml "gopkg.in/yaml.v2"
)

var helpers = thtml.FuncMap{
	"noescape": noescape,
}

// rawHTMLRenderer overrides only goldmark's raw-HTML node kinds (block and
// inline) to pass their source through unchanged, instead of the default
// renderer's "<!-- raw HTML omitted -->" placeholder. Unlike
// html.WithUnsafe(), this leaves every other renderer - including the ones
// that sanitize link and image destinations against javascript:/data:/etc.
// URLs - on their default, safe behavior. It's registered at a lower
// priority number than the default HTML renderer (1000), which wins ties.
type rawHTMLRenderer struct{}

func (r *rawHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindHTMLBlock, r.renderHTMLBlock)
	reg.Register(ast.KindRawHTML, r.renderRawHTML)
}

func (r *rawHTMLRenderer) renderHTMLBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.HTMLBlock)
	if entering {
		for i := 0; i < n.Lines().Len(); i++ {
			line := n.Lines().At(i)
			_, _ = w.Write(line.Value(source))
		}
	} else if n.HasClosure() {
		_, _ = w.Write(n.ClosureLine.Value(source))
	}
	return ast.WalkContinue, nil
}

func (r *rawHTMLRenderer) renderRawHTML(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	n := node.(*ast.RawHTML)
	for i := 0; i < n.Segments.Len(); i++ {
		segment := n.Segments.At(i)
		_, _ = w.Write(segment.Value(source))
	}
	return ast.WalkSkipChildren, nil
}

// markdownConverter passes raw HTML through instead of dropping it - a
// leading HTML comment (per-page config) and an <embed> tag written
// directly in a .md file would otherwise never reach Zas. Built once at
// package init and shared across renderAsync goroutines: goldmark builds a
// fresh parse context per Convert call, so this is safe for concurrent use.
//
// extension.GFM (tables, strikethrough, task lists, autolinks) and
// extension.Footnote are enabled on top of goldmark's default CommonMark
// parsing - previously neither was, so e.g. a GFM table rendered as a
// literal paragraph of pipe characters instead of a <table>. Both are
// purely additive: they recognize syntax CommonMark alone leaves as plain
// text, so existing content that doesn't use any of it renders exactly as
// before.
var markdownConverter = markdown.New(
	markdown.WithExtensions(extension.GFM, extension.Footnote),
	markdown.WithRendererOptions(
		renderer.WithNodeRenderers(util.Prioritized(&rawHTMLRenderer{}, 100)),
	),
)

// Generator groups relevant rendering info.
type Generator struct {
	// Verbose output.
	Verbose bool
	// Full generation (non-incremental mode).
	Full bool
	// Config from ZAS_CONF_FILE.
	Config ConfigSection
	// Default layout from Config[ZAS]["layout"].
	Layout *thtml.Template
	// i18n helper.
	I18n *gt.Build
	// layoutModTime, configModTime, and i18nModTime are the shared
	// dependency files' mtimes, each captured once during Run's startup
	// phase (parseLayout, Run, loadI18N) instead of being stat'd per page,
	// since a site can have thousands of pages all depending on the same
	// three files. sourceIsNewer treats a page as stale if any is newer
	// than the page's deploy output. Zero value (e.g. i18n.yml absent)
	// never counts as newer than a real deploy output.
	layoutModTime time.Time
	configModTime time.Time
	i18nModTime   time.Time
	// ZasDirectoryConfigs cache
	cachedZasDirectoryConfigs map[string]dirConfigEntry
	// Guards cachedZasDirectoryConfigs, read and written from many renderAsync goroutines.
	dirConfigMu sync.Mutex

	wg sync.WaitGroup

	// sem bounds how many renderAsync goroutines may run at once (see
	// renderConcurrency), so a large site doesn't fan out one goroutine
	// per source file with no ceiling. Lazily sized on first use in walk,
	// whose own invocations are sequential like claimedOutputs and
	// reapedDirs below, so no mutex guards the initialization itself.
	sem chan struct{}

	// active and peakActive track how many renderAsync goroutines are
	// concurrently doing real rendering work. Nothing in production code
	// reads peakActive; it exists purely so tests can assert the bound
	// enforced by sem is actually respected.
	active     atomic.Int64
	peakActive atomic.Int64

	// Guards errs, which collects per-file render errors from renderAsync goroutines.
	mu   sync.Mutex
	errs []error

	// printMu serializes stdout writes from renderAsync goroutines (the
	// verbose "+" line in walk and the error lines in render) so
	// concurrent goroutines can't interleave mid-line.
	printMu sync.Mutex

	// reapedDirs holds deploy paths reaper has RemoveAll'd during the
	// current reap walk. The reap walk is single-threaded, so no mutex.
	reapedDirs map[string]struct{}

	// dirEntriesFoldCache caches, per directory, the lowercased basenames
	// of its entries - built lazily by existsFold on first use and reused
	// for every other file existsFold is asked about in the same
	// directory during the current reap walk. Without it, a directory
	// with many files (case-insensitive extension matching means reaper
	// can no longer trust a single exact-path os.Open, see existsFold)
	// would re-read and re-scan the same directory listing once per file
	// instead of once total. Like reapedDirs, the reap walk that fills
	// this is single-threaded, so no mutex.
	dirEntriesFoldCache map[string]map[string]struct{}

	// claimedOutputs maps each deploy output path claimed so far to the
	// source path that claimed it, so two sources that render to the same
	// output (e.g. foo.md and foo.html) don't both spawn a renderAsync
	// goroutine and race to write it. Like reapedDirs, this is only
	// touched from walk, whose own invocations are sequential, so no
	// mutex is needed.
	claimedOutputs map[string]string
}

// renderConcurrency bounds how many renderAsync goroutines may run at
// once. Each one interleaves CPU-bound work (HTML5 parse, template
// execution) with blocking file I/O (reading the source, then an atomic
// write via temp-file-then-rename), so sizing purely on GOMAXPROCS would
// leave CPUs idle while goroutines wait on I/O; the x4 multiplier keeps
// them busy without still spawning thousands of concurrent goroutines
// (and open file descriptors) for a large site.
func renderConcurrency() int {
	return runtime.GOMAXPROCS(0) * 4
}

// printLine writes args to stderr like fmt.Println, but serialized
// against every other printLine call so concurrent renderAsync
// goroutines (and walk itself) never interleave mid-line. Every message
// printLine carries - progress lines, symlink-skip notices, per-page
// diagnostics - is either gated behind -verbose or reports a problem, not
// data the tool produces; stderr keeps stdout free for a future
// machine-consumable output format and matches the top-level fatal error
// (cmd/zas/main.go) already going there.
func (gen *Generator) printLine(args ...interface{}) {
	gen.printMu.Lock()
	defer gen.printMu.Unlock()
	fmt.Fprintln(os.Stderr, args...)
}

/*
 * Records err for later aggregation, safe for concurrent use.
 */
func (gen *Generator) recordErr(err error) {
	gen.mu.Lock()
	gen.errs = append(gen.errs, err)
	gen.mu.Unlock()
}

// GetDeployPath returns deployment base path in config.
func (gen *Generator) GetDeployPath() string {
	return gen.Config.GetZString("deploy")
}

// BuildDeployPath builds deployment path for specific file pointed by path.
func (gen *Generator) BuildDeployPath(path string) string {
	return filepath.Join(gen.GetDeployPath(), path)
}

// hasExtension reports whether path ends in ext, matched case-insensitively
// so PAGE.MD and page.Md are both recognized the same as page.md - a
// filesystem that preserves the case a file was created with doesn't mean
// every file on it was typed in the same case.
func hasExtension(path, ext string) bool {
	return len(path) >= len(ext) && strings.EqualFold(path[len(path)-len(ext):], ext)
}

// swapExtension replaces path's trailing from extension with to, matched
// case-insensitively via hasExtension (so PAGE.MD swaps just like
// page.md - always producing to's own casing, never from's). Paths not
// ending in from are returned unchanged. Unlike strings.Replace/ReplaceAll,
// this only ever touches the extension, never a from/to occurrence earlier
// in path.
func swapExtension(path, from, to string) string {
	if !hasExtension(path, from) {
		return path
	}
	return path[:len(path)-len(from)] + to
}

// existsFold reports whether a file matching name exists in name's
// directory, compared case-insensitively against the directory's actual
// entries. reaper reconstructs a candidate source path from a deploy path
// via swapExtension, which always normalizes to's own casing - so a guess
// like "./page.md" needs to still find a real "PAGE.MD" source on a
// case-sensitive filesystem. Each directory's entries are read at most
// once per reap walk and cached in dirEntriesFoldCache: a directory with
// many files would otherwise pay for a full re-read and re-scan of the
// same listing once per file instead of once total.
func (gen *Generator) existsFold(name string) bool {
	dir := filepath.Dir(name)
	names, ok := gen.dirEntriesFoldCache[dir]
	if !ok {
		names = map[string]struct{}{}
		if entries, err := os.ReadDir(dir); err == nil {
			for _, e := range entries {
				names[strings.ToLower(e.Name())] = struct{}{}
			}
		}
		if gen.dirEntriesFoldCache == nil {
			gen.dirEntriesFoldCache = make(map[string]map[string]struct{})
		}
		gen.dirEntriesFoldCache[dir] = names
	}
	_, found := names[strings.ToLower(filepath.Base(name))]
	return found
}

// atomicWriteFile calls write with a temporary file in path's own directory
// (same filesystem, so the rename below is atomic), then renames it onto
// path once write fully succeeds. path itself is never opened or truncated,
// so an interruption at any point - panic, crash, SIGKILL, power loss -
// before the rename leaves it holding either its previous complete content
// or nothing, never a partial write. On any error the temporary file is
// removed instead of left behind.
//
// path's directory is created here, on demand, rather than upfront by
// walk for every source directory it visits: a source directory whose
// entire content is skipped (e.g. one holding only a .zas.yml) would
// otherwise get a permanently empty counterpart in deploy, since nothing
// else ever writes into it or cleans it up. Creating it only when there's
// an actual file to place in it means a directory with no deployable
// content never gets a deploy-side counterpart at all. os.MkdirAll is
// safe to call concurrently for the same path - a race between two
// renderAsync goroutines both wanting the same parent directory ends with
// one creating it and the other finding it already there, neither an
// error.
func (gen *Generator) atomicWriteFile(path string, write func(io.Writer) error) (err error) {
	if err = os.MkdirAll(filepath.Dir(path), os.FileMode(ZAS_DEFAULT_DIR_PERM)); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := f.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpPath)
		}
	}()
	if err = write(f); err != nil {
		_ = f.Close()
		return err
	}
	// os.CreateTemp always creates with mode 0600, regardless of
	// ZAS_DEFAULT_FILE_PERM, so the final file's permissions must be set
	// explicitly before it replaces path.
	if err = f.Chmod(os.FileMode(ZAS_DEFAULT_FILE_PERM)); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// Generate renders and writes the file at data.Path using the given
// template context.
func (gen *Generator) Generate(_ string, data *ZasData) (err error) {
	var processed bytes.Buffer
	if err = gen.Layout.Execute(&processed, data); err != nil {
		return
	}
	doc, err := gen.parseAndReplace(&processed, data)
	if err != nil {
		return
	}
	if len(data.bodyAttrs) > 0 {
		// Body only carries the source page's inner HTML, not its <body>
		// element, so its attributes need to be merged onto the layout's
		// <body> explicitly. Non-colliding only: the layout's own attribute
		// wins on a key both define.
		layoutBody := doc.Find(atom.Body.String())
		for key, val := range data.bodyAttrs {
			if _, exists := layoutBody.Attr(key); !exists {
				layoutBody.SetAttr(key, val)
			}
		}
	}
	return gen.atomicWriteFile(gen.BuildDeployPath(data.Path), func(w io.Writer) error {
		return html5.Render(w, doc.Get(0))
	})
}

// maxEmbedDepth bounds how many levels of <embed> an entry file may nest.
// Markdown and HTML embed handlers call back into parseAndReplace for
// whatever they embed, so a file that embeds itself (directly, or through a
// cycle of mutually-embedding files) would otherwise recurse until the
// goroutine's stack is exhausted.
const maxEmbedDepth = 20

func (gen *Generator) parseAndReplace(processed io.Reader, data *ZasData) (doc *goquery.Document, err error) {
	if data.embedDepth >= maxEmbedDepth {
		return nil, fmt.Errorf("embed nesting deeper than %d levels; check for a self- or mutually-embedding file", maxEmbedDepth)
	}
	data.embedDepth++
	defer func() { data.embedDepth-- }()
	// Here we manipulate its result.
	doc, err = goquery.NewDocumentFromReader(processed)
	if err != nil {
		return
	}
	err = gen.handleEmbedTags(doc, data)
	return
}

// Run performs a full generation pass over the current directory,
// rendering every source file into the configured deploy path.
func (gen *Generator) Run() error {
	cfg, err := NewConfig()
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("not a valid Zas repository: %w", err)
		}
		return err
	}
	gen.Config = cfg
	if info, statErr := os.Stat(ZAS_CONF_FILE); statErr == nil {
		gen.configModTime = info.ModTime()
	}
	gen.wg.Add(3)
	go gen.parseLayout()
	go gen.loadI18N()
	go gen.handleDeployPath(gen.Full)
	gen.wg.Wait()
	if len(gen.errs) > 0 {
		return errors.Join(gen.errs...)
	}
	// Walking function. It allows to bubble up any error from generator.
	walk := func(path string, info os.FileInfo, err error) error {
		return gen.walk(path, info, err)
	}
	walkErr := filepath.Walk(".", walk)
	gen.wg.Wait()
	if walkErr != nil {
		gen.recordErr(walkErr)
	}
	if !gen.Full {
		// TODO Can we go parallel?
		// This removes deleted source files in deploy path
		reapwalk := func(path string, info os.FileInfo, err error) error {
			return gen.reaper(path, info, err)
		}
		if err = filepath.Walk(gen.GetDeployPath(), reapwalk); err != nil {
			return err
		}
	}
	if len(gen.errs) > 0 {
		return errors.Join(gen.errs...)
	}
	return nil
}

func (gen *Generator) parseLayout() {
	var err error
	defer gen.wg.Done()

	layout := gen.Config.GetZString("layout")
	if info, statErr := os.Stat(layout); statErr == nil {
		gen.layoutModTime = info.ModTime()
	}
	if gen.Layout, err = thtml.New(filepath.Base(layout)).Funcs(helpers).ParseFiles(layout); err != nil {
		gen.recordErr(err)
	}
}

func (gen *Generator) loadI18N() {
	defer gen.wg.Done()

	mainlang := gen.Config.GetSection("site").GetString("language")
	i18nStrings, err := NewI18n(mainlang)
	if err != nil {
		gen.recordErr(err)
		return
	}
	gen.I18n = &gt.Build{
		Index:  i18nStrings,
		Origin: mainlang,
	}
	if info, statErr := os.Stat(ZAS_I18N_FILE); statErr == nil {
		gen.i18nModTime = info.ModTime()
	}
}

func (gen *Generator) handleDeployPath(full bool) {
	defer gen.wg.Done()

	deployPath := gen.GetDeployPath()
	// If deployment path already exists, it must be deleted.
	if _, err := os.Stat(deployPath); err == nil && full {
		if err = os.RemoveAll(deployPath); err != nil {
			gen.recordErr(err)
			return
		}
	}
	if err := os.MkdirAll(deployPath, os.FileMode(ZAS_DEFAULT_DIR_PERM)); err != nil {
		gen.recordErr(err)
	}
}

func pathHasComponent(path, component string) bool {
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		if part == component {
			return true
		}
	}
	return false
}

/*
 * Real walking function. Handles all supported files and copy not supported ones in current deployment path.
 */
func (gen *Generator) walk(path string, info os.FileInfo, err error) (ierr error) {
	if err != nil {
		return err
	}
	if strings.HasPrefix(path, ".") || strings.HasPrefix(filepath.Base(path), ".") || pathHasComponent(path, ZAS_DIR) ||
		path == gen.GetDeployPath() || path == gen.Config.GetZString("layout") {
		if path != "." && info.IsDir() {
			return filepath.SkipDir
		}
		return nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		// filepath.Walk calls os.Lstat, so a symlinked directory arrives
		// here with info.IsDir() == false - Lstat reports the link itself,
		// not its target - which used to fall through to the file branch
		// below and reach copy(), whose os.Open follows the link and then
		// fails writing a regular file where a directory belongs. A
		// symlinked file has the same Lstat shape and used to be copied by
		// silently dereferencing it, with no indication in the output that
		// the source was ever a link. Skip both explicitly instead: a
		// symlinked directory can point anywhere on the filesystem the
		// build process can read - the same containment concern
		// resolveEmbedSrc already handles for <embed src> - and following
		// it would additionally need its own cycle detection, since
		// filepath.Walk doesn't resolve symlinks on its own.
		gen.printLine("~", path, "(symlink, not followed)")
		return nil
	}
	// A directory itself needs nothing done for it here: atomicWriteFile
	// creates a source directory's deploy-side counterpart lazily, only
	// once something actually needs to be written into it, so a directory
	// whose entire content is skipped never gets an empty one in deploy.
	if !info.IsDir() && gen.sourceIsNewer(path, info) {
		outputPath := swapExtension(path, ".md", ".html")
		if claimant, ok := gen.claimedOutputs[outputPath]; ok {
			gen.recordErr(fmt.Errorf("%s: output path %q already claimed by %s, skipping", path, outputPath, claimant))
			return
		}
		if gen.claimedOutputs == nil {
			gen.claimedOutputs = make(map[string]string)
		}
		gen.claimedOutputs[outputPath] = path
		if gen.Verbose {
			gen.printLine("+", path)
		}
		if gen.sem == nil {
			gen.sem = make(chan struct{}, renderConcurrency())
		}
		gen.wg.Add(1)
		// Blocks once renderConcurrency() goroutines are already in
		// flight, throttling walk itself until one finishes and releases
		// its slot - the actual fan-out cap.
		gen.sem <- struct{}{}
		go gen.renderAsync(path)
	}
	return
}

func (gen *Generator) renderAsync(path string) {
	var err error
	defer gen.wg.Done()
	defer func() { <-gen.sem }()

	n := gen.active.Add(1)
	defer gen.active.Add(-1)
	for {
		peak := gen.peakActive.Load()
		if n <= peak || gen.peakActive.CompareAndSwap(peak, n) {
			break
		}
	}

	switch {
	case hasExtension(path, ".md"):
		err = gen.renderMarkdown(path)
	case hasExtension(path, ".html"):
		err = gen.renderHTML(path)
	default:
		err = gen.copy(gen.BuildDeployPath(path), path)
	}

	if err != nil {
		gen.mu.Lock()
		gen.errs = append(gen.errs, fmt.Errorf("%s: %w", path, err))
		gen.mu.Unlock()
	}
}

/*
 * Real reaping function. Reaps all missing source files in current deployment path.
 *
 * This only reaps deploy output whose source file no longer exists; it does
 * not detect a source file that still exists but newly opted out of
 * standalone publishing via "publish: false" (see pagePublished). That
 * page's previous deploy output is left behind by an incremental run - a
 * -full run, which clears the whole deploy directory upfront, is needed to
 * remove it.
 */
func (gen *Generator) reaper(path string, _ os.FileInfo, err error) (ierr error) {
	if err != nil {
		// filepath.Walk lists a directory's children before invoking this
		// callback for the directory itself, so RemoveAll below leaves Walk
		// holding a stale child list for path's parent; the lstat it then
		// retries on each child surfaces here as a not-exist error we
		// already caused by reaping the parent.
		if _, ok := gen.reapedDirs[filepath.Dir(path)]; ok && os.IsNotExist(err) {
			return nil
		}
		return err
	}
	sourcePath := strings.Replace(path, gen.GetDeployPath(), ".", 1)
	source, err := os.Open(sourcePath)
	// TODO it must clean directories too
	if err != nil {
		reap := true
		if hasExtension(sourcePath, ".html") {
			// swapExtension always normalizes to's own casing, so this
			// guess (e.g. "./page.md") can only ever be lowercase,
			// regardless of what case the real source used - a plain
			// os.Open here would never find a differently-cased source
			// like "PAGE.MD" on a case-sensitive filesystem. Note this
			// isn't needed above: an .html-sourced deploy path is never
			// extension-swapped at all (swapExtension's from is always
			// ".md"), so it keeps the source's exact original casing, and
			// plain os.Open above already matches it correctly.
			sourcePath = swapExtension(sourcePath, ".html", ".md")
			if gen.existsFold(sourcePath) {
				reap = false
			}
		}
		if reap {
			if gen.Verbose {
				gen.printLine("-", sourcePath)
			}
			if rmErr := os.RemoveAll(path); rmErr != nil {
				gen.recordErr(rmErr)
			} else {
				if gen.reapedDirs == nil {
					gen.reapedDirs = make(map[string]struct{})
				}
				gen.reapedDirs[path] = struct{}{}
			}
		}
	} else {
		_ = source.Close()
	}
	return
}

func (gen *Generator) sourceIsNewer(path string, sourceInfo os.FileInfo) bool {
	// Shortcut
	if gen.Full {
		return true
	}
	realpath := swapExtension(path, ".md", ".html")
	destination, err := os.Open(gen.BuildDeployPath(realpath))
	if err != nil {
		return true
	}
	defer func() { _ = destination.Close() }()
	destinationInfo, err := destination.Stat()
	if err != nil {
		return true
	}
	destModTime := destinationInfo.ModTime()
	if sourceInfo.ModTime().UnixNano() >= destModTime.UnixNano() {
		return true
	}
	// .Before, not UnixNano: a dependency that was never stat'd (e.g. no
	// i18n.yml, or no .zas.yml anywhere in path's ancestry) leaves its
	// mtime at time.Time's zero value, and zero.UnixNano() is documented
	// as undefined for dates this far out - .Before/.After stay well
	// defined and correctly treat "never stat'd" as "not newer".
	if !gen.layoutModTime.Before(destModTime) || !gen.configModTime.Before(destModTime) || !gen.i18nModTime.Before(destModTime) {
		return true
	}
	_, dirModTime, _ := gen.loadZasDirectoryConfig(path)
	return !dirModTime.Before(destModTime)
}

/*
 * Renders a Markdown file.
 */
func (gen *Generator) renderMarkdown(path string) (err error) {
	input, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var b bytes.Buffer
	if err := markdownConverter.Convert(input, &b); err != nil {
		return err
	}
	// Do not unescape the converter's output here: goldmark escapes code
	// block contents (and rawHTMLRenderer above already passes raw HTML
	// through verbatim), so unescaping would let HTML entities inside a
	// fenced or indented code block turn back into real elements once
	// parseAndReplace re-parses this as HTML - including a <script> tag
	// becoming a live, executing script.
	return gen.render(path, b.Bytes())
}

/*
 * Renders a HTML file.
 */
func (gen *Generator) renderHTML(path string) (err error) {
	input, err := os.ReadFile(path)
	if err != nil {
		return
	}
	return gen.render(path, input)
}

// dirConfigEntry is a cached loadZasDirectoryConfig resolution: config is
// nil when no ZAS_DIR_CONF_FILE exists anywhere in the queried directory's
// ancestry, and modTime is its zero value in that case too.
type dirConfigEntry struct {
	config  ConfigSection
	modTime time.Time
}

/*
 * Loads ZAS_DIR_CONF_FILE (as defined in constants.go) from current
 * directory or previously found ones, along with that file's own mtime.
 * It must be a YAML file.
 *
 * Every directory visited while resolving currentpath is cached, not just
 * the one where ZAS_DIR_CONF_FILE was actually found - including a miss
 * that bottoms out at ".". Without that, a directory with no directory
 * config anywhere in its ancestry (the common case) would redo the whole
 * upward walk on every call instead of hitting the cache after the first.
 */
func (gen *Generator) loadZasDirectoryConfig(currentpath string) (config ConfigSection, modTime time.Time, err error) {
	path := filepath.Dir(currentpath)
	if entry, ok := gen.getCachedDirConfig(path); ok {
		if entry.config == nil {
			return nil, time.Time{}, os.ErrNotExist
		}
		return entry.config, entry.modTime, nil
	}
	confPath := fmt.Sprintf("%s/%s", path, ZAS_DIR_CONF_FILE)
	data, err := os.ReadFile(confPath)
	if err != nil {
		// Maybe .zas.yml is in an upper directory (already cached or not),
		// so we call this recursively. Unless we are at current working
		// directory.
		var entry dirConfigEntry
		if path != "." {
			entry.config, entry.modTime, err = gen.loadZasDirectoryConfig(path)
		}
		gen.setCachedDirConfig(path, entry)
		return entry.config, entry.modTime, err
	}
	config = make(ConfigSection)
	if yamlErr := yaml.Unmarshal(data, &config); yamlErr != nil {
		// A malformed .zas.yml would otherwise look like a completely
		// successful, merely empty, config load (err == nil here), and that
		// empty config is about to be memoized for the rest of this run: no
		// page under this directory would ever see a diagnostic for it.
		// Route it through the same aggregation/reporting path every other
		// config-loading failure (parseLayout, loadI18N, ...) already uses,
		// so it surfaces once, without aborting the rest of the build.
		err = fmt.Errorf("%s: %w", confPath, yamlErr)
		gen.recordErr(err)
	}
	if info, statErr := os.Stat(confPath); statErr == nil {
		modTime = info.ModTime()
	}
	gen.setCachedDirConfig(path, dirConfigEntry{config: config, modTime: modTime})
	return config, modTime, err
}

/*
 * Reads cachedZasDirectoryConfigs, safe for concurrent use.
 */
func (gen *Generator) getCachedDirConfig(path string) (entry dirConfigEntry, ok bool) {
	gen.dirConfigMu.Lock()
	defer gen.dirConfigMu.Unlock()
	entry, ok = gen.cachedZasDirectoryConfigs[path]
	return
}

/*
 * Writes cachedZasDirectoryConfigs, safe for concurrent use.
 */
func (gen *Generator) setCachedDirConfig(path string, entry dirConfigEntry) {
	gen.dirConfigMu.Lock()
	defer gen.dirConfigMu.Unlock()
	if gen.cachedZasDirectoryConfigs == nil {
		gen.cachedZasDirectoryConfigs = make(map[string]dirConfigEntry)
	}
	gen.cachedZasDirectoryConfigs[path] = entry
}

// doctypePrefix and commentOpen/commentClose are the literal delimiters
// leadingConfigComment scans for. Matching regexp's (?is) \s class exactly,
// not unicode.IsSpace, is what makes isConfigSpace its own function below
// rather than a reuse of a stdlib whitespace check.
const (
	doctypePrefix = "<!DOCTYPE"
	commentOpen   = "<!--"
)

var commentClose = []byte("-->")

// leadingConfigComment extracts a page's leading config comment straight
// from its raw source bytes, tolerating a single leading <!DOCTYPE ...>
// ahead of it (the same tolerance extractPageConfig has once the document
// is fully HTML5-parsed) and any whitespace before either. It exists only
// so pageOptsOutOfTemplating can decide, before text/template ever runs,
// whether it should run at all - see that function and the comment on
// render's ttext.New call below for why extractPageConfig itself can't be
// used for this.
//
// It walks input by hand as a small state machine - skip whitespace, try
// an optional doctype, skip whitespace again, require the comment to open
// immediately, then scan for its close - instead of using regexp, since
// this runs once per source file on every render() call and is on the hot
// path for a large site. It reproduces
// `(?is)\A\s*(?:<!DOCTYPE[^>]*>\s*)?<!--(.*?)-->` byte for byte: in
// particular the scan for "-->" stops at the *first* occurrence (a later
// comment further into input must never count), and any mismatch anchored
// at the very start of input - after whitespace/doctype - means there is
// no leading comment at all.
func leadingConfigComment(input []byte) ([]byte, bool) {
	i := skipConfigSpace(input, 0)
	if hasFoldPrefix(input[i:], doctypePrefix) {
		end := bytes.IndexByte(input[i:], '>')
		if end < 0 {
			// No closing '>' for the doctype: the optional group can't
			// have matched, but input at i still starts with "<!DOCTYPE",
			// which can never also start with "<!--", so there is no
			// leading comment either way.
			return nil, false
		}
		i += end + 1
		i = skipConfigSpace(input, i)
	}
	if !hasPrefix(input[i:], commentOpen) {
		return nil, false
	}
	i += len(commentOpen)
	end := bytes.Index(input[i:], commentClose)
	if end < 0 {
		return nil, false
	}
	return input[i : i+end], true
}

// skipConfigSpace advances i past any run of configSpace bytes.
func skipConfigSpace(input []byte, i int) int {
	for i < len(input) && isConfigSpace(input[i]) {
		i++
	}
	return i
}

// isConfigSpace reports whether b is one of regexp's \s bytes
// ([\t\n\f\r ]), matching the whitespace class the original
// pageConfigCommentRe relied on exactly.
func isConfigSpace(b byte) bool {
	switch b {
	case '\t', '\n', '\f', '\r', ' ':
		return true
	}
	return false
}

// hasPrefix reports whether b starts with prefix. Comparing a sliced
// []byte-to-string conversion with == is a case the compiler recognizes
// and doesn't allocate for.
func hasPrefix(b []byte, prefix string) bool {
	return len(b) >= len(prefix) && string(b[:len(prefix)]) == prefix
}

// hasFoldPrefix is hasPrefix's ASCII case-insensitive counterpart, used
// only for the (case-insensitive) "<!DOCTYPE" tag.
func hasFoldPrefix(b []byte, prefix string) bool {
	if len(b) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		c := b[i]
		if 'a' <= c && c <= 'z' {
			c -= 'a' - 'A'
		}
		if c != prefix[i] {
			return false
		}
	}
	return true
}

// templateKey is the one map key pageOptsOutOfTemplating cares about.
// bomUTF16LE/bomUTF16BE are the two byte-order marks yaml.v2 recognizes at
// the very start of a stream - see mayDefineTemplateKey, the only place
// either is used.
var templateKey = []byte("template")

const (
	bomUTF16LE = "\xff\xfe"
	bomUTF16BE = "\xfe\xff"
)

// mayDefineTemplateKey reports whether content could possibly decode to a
// YAML mapping containing the exact key "template", without actually
// running the parser to find out. It exists to keep yaml.Unmarshal off the
// hot path: gopkg.in/yaml.v2's yaml_parser_initialize allocates the
// parser's two I/O buffers (512 and 1536 bytes) on every single Unmarshal
// call regardless of input size, so asking an eleven-byte
// "<!-- title: Hi -->" comment for a key it doesn't have costs ~4.5us and
// ~5.5KB - once per page, on every render, across renderConcurrency()
// goroutines.
//
// A false positive here only costs the yaml parse that would have run
// anyway, so the bar is one-sided: this must never say false for content
// yaml would in fact decode with a "template" key. Every YAML 1.1
// construct that can put a key into the map either reproduces that key's
// bytes verbatim or needs one of the three markers checked below - verified
// against gopkg.in/yaml.v2 v2.4.0's actual source, not just the spec:
//
//   - Plain and single-quoted scalars reproduce their bytes. A line break
//     inside one folds to a space, so "temp\nlate" decodes as "temp late"
//     and can never be rejoined into "template"; block scalars (| and >),
//     explicit "? " keys and flow mappings are just other ways to spell
//     those same bytes. Anchors, aliases and "<<" merge keys can only
//     reach a key some node in this very document already spells out, and
//     yaml.Unmarshal is handed nothing but the comment's own bytes.
//   - A double-quoted scalar can synthesize the key ("\x74emplate", or a
//     backslash-newline continuation joining "temp" and "late"), but every
//     such form needs a backslash.
//   - So can a tag: yaml.v2 base64-decodes !!binary into a Go string
//     (decode.go's scalar case deliberately excludes yaml_BINARY_TAG from
//     resolvableTag in resolve.go, so it skips normal resolution and hits
//     the base64 branch instead), which makes
//     "<!-- !!binary dGVtcGxhdGU=: false -->" - base64 of "template" - a
//     real opt-out today that never spells the key in the comment. Every
//     way to name that tag - the !! shorthand, a verbatim !<...>, or a
//     %TAG-remapped handle - puts a '!' somewhere in the document, because
//     a tag handle is delimited by them.
//   - The bytes need not be UTF-8 at all. yaml.v2 sniffs a UTF-16 byte
//     order mark and transcodes (readerc.go's
//     yaml_parser_determine_encoding), so a comment body opening with
//     FF FE spells the key as 74 00 65 00 ... instead of literal ASCII.
//     That sniff happens once, at the very start of the stream and
//     nowhere else, so a two-byte prefix check is exact rather than
//     another scan.
//
// The key is matched case-sensitively on purpose: the lookup this feeds is
// config["template"], an exact Go string comparison, so "Template: false"
// is not an opt-out today and must not become one here.
func mayDefineTemplateKey(content []byte) bool {
	if hasPrefix(content, bomUTF16LE) || hasPrefix(content, bomUTF16BE) {
		return true
	}
	return bytes.Contains(content, templateKey) ||
		bytes.IndexByte(content, '\\') >= 0 ||
		bytes.IndexByte(content, '!') >= 0
}

// pageOptsOutOfTemplating reports whether input's leading config comment
// sets "template: false", letting a whole page skip text/template
// parsing/execution so its content - including any literal {{ }} it
// contains - reaches the rest of the pipeline unexecuted. This is for
// pages that use {{ }} for something other than Zas's own templating: a
// Vue/Angular/Handlebars snippet, Go-template documentation, or a code
// sample demonstrating Zas's own template syntax.
//
// It has to inspect input's raw bytes directly, before ttext.New(...).Parse
// runs. extractPageConfig reads this same leading comment for every other
// page-config key (title, language, ...), but only runs later in render, on
// the document produced by executing that very template - so by the time
// it could report "template: false", template execution has already
// happened (or already failed). A comment that isn't valid YAML is treated
// the same as one without a "template" key (templating proceeds as
// before): extractPageConfig will report the same malformed comment once
// the page reaches it further down the pipeline, so it doesn't need to be
// reported twice.
func pageOptsOutOfTemplating(input []byte) bool {
	content, ok := leadingConfigComment(input)
	if !ok {
		return false
	}
	if !mayDefineTemplateKey(content) {
		// yaml could only confirm what the scan above already
		// established, and it is by far the most expensive thing this
		// function does - see mayDefineTemplateKey.
		return false
	}
	var config map[interface{}]interface{}
	if err := yaml.Unmarshal(content, &config); err != nil {
		return false
	}
	runTemplate, ok := config["template"].(bool)
	return ok && !runTemplate
}

// earlyPageConfig extracts a best-effort preview of a page's own Page
// config map straight from input's raw source bytes, before text/template
// ever runs - the same raw, pre-execution approach leadingConfigComment
// (and pageOptsOutOfTemplating, built on it) already take for the
// "template: false" opt-out. It exists so a page's own body can read
// {{.Page}} (and, through Title's fallback, {{.Title}}) as something other
// than always empty: render otherwise only populates data.Page from
// extractPageConfig, which runs on the fully rendered, HTML5-parsed
// document - strictly after the page's own template has already executed
// - so the exact same expression that works in the layout used to fail
// silently inside the page's own body.
//
// This is deliberately only a preview. render's later, unconditional call
// to extractPageConfig - itself untouched by this function - always
// overwrites data.Page once the document is fully HTML5-parsed, so the
// layout's own view of data.Page is completely unaffected by whatever this
// returns. The two extractions are separate implementations of "find the
// leading config comment" and can in principle disagree on some edge case
// neither was built to handle identically; that is an acceptable rough
// edge for a page's own internal, best-effort view, but would not be for
// the canonical one extractPageConfig produces.
//
// A malformed or absent leading comment yields a nil map, matching
// data.Page's own zero value and today's pre-fix empty behavior, rather
// than surfacing a duplicate diagnostic: extractPageConfig already reports
// a malformed comment once the page reaches it later in render.
func earlyPageConfig(input []byte) map[interface{}]interface{} {
	content, ok := leadingConfigComment(input)
	if !ok {
		return nil
	}
	var config map[interface{}]interface{}
	if err := yaml.Unmarshal(content, &config); err != nil {
		return nil
	}
	return config
}

// leadingH1Re matches the first <h1>...</h1> element in a document,
// capturing its inner content. Used only by leadingH1Text.
var leadingH1Re = regexp.MustCompile(`(?is)<h1[^>]*>(.*?)</h1>`)

// leadingH1Text extracts the first <h1>...</h1> element's inner content
// from input's raw source bytes, before text/template or HTML5 parsing
// ever run - the same raw, pre-execution spirit as earlyPageConfig above.
// It exists only to give a page's own body a best-effort preview of
// {{.FirstTitle}} (and, through Title's fallback, {{.Title}}); render's
// later, unconditional getTitle(doc) call - run on the fully HTML5-parsed
// document, unchanged by this function - always overwrites data.FirstTitle
// with the canonical value afterward, so the layout's own view is
// completely unaffected by whatever preview this returns.
//
// Unlike getTitle, which walks a parsed tree and so sees plain text, any
// inner HTML tags here are left exactly as written rather than stripped:
// stripping them correctly needs the very HTML5 parse this function exists
// to run ahead of. A raw preview that still contains markup (e.g. an <em>
// inside the heading) is an acceptable rough edge for an early,
// best-effort value that gets fully replaced moments later.
//
// It refuses to return content containing "{{": a heading written as
// <h1>{{.Title}}</h1> - a natural pattern for showing the resolved title
// in the page's own heading - would otherwise hand back the literal,
// unexecuted string "{{.Title}}" as FirstTitle, and since Title() falls
// back to FirstTitle when Page["title"] isn't set, that string would
// circularly render as its own placeholder instead of either failing
// loudly or resolving. Leaving FirstTitle unset in that case reproduces
// today's safe, empty behavior instead of exposing template syntax as if
// it were real content.
func leadingH1Text(input []byte) (string, bool) {
	m := leadingH1Re.FindSubmatch(input)
	if m == nil {
		return "", false
	}
	content := m[1]
	if bytes.Contains(content, templateActionOpen) {
		return "", false
	}
	return strings.TrimSpace(string(content)), true
}

// templateActionOpen is text/template's default left delimiter, and the
// only one render's template ever uses - nothing in this repo calls
// ttext.New(...).Delims(...). A page containing no "{{" anywhere therefore
// has no template actions at all: not a "{{- -}}" trim marker, not a
// "{{/* */}}" comment, since text/template's lexer (lexText in
// text/template/parse/lex.go) only reaches either past this same
// delimiter. With none present, Parse builds a tree holding a single text
// node and Execute copies it back out byte for byte - so render's
// no-templating branch below produces the identical result without paying
// for string(input)'s full-file copy or the parse/execute round trip.
var templateActionOpen = []byte("{{")

/*
 * Generic render function. It expects input to be a valid HTML document.
 * Input can be a valid Go template, unless its leading config comment
 * opts out with "template: false" (see pageOptsOutOfTemplating).
 */
func (gen *Generator) render(path string, input []byte) (err error) {
	var processed bytes.Buffer
	// Building context and rendering template.
	data := NewZasData(path, gen)
	data.Directory, _, _ = gen.loadZasDirectoryConfig(path)
	// The "{{" check goes first, and that order is load-bearing rather
	// than stylistic: when input has no "{{" at all, templating and the
	// opt-out branch below produce byte-identical output regardless of
	// what the page's config comment says, so pageOptsOutOfTemplating's
	// answer cannot change the outcome, and || short-circuits it away
	// entirely. That matters because mayDefineTemplateKey's guards are
	// deliberately one-sided (an innocuous "!" or backslash anywhere in
	// the comment - "Hi!", a Windows path in a title - still falls
	// through to a real yaml.Unmarshal); putting bytes.Contains first
	// means a plain page never pays for that parse over a question whose
	// answer was never going to matter.
	if !bytes.Contains(input, templateActionOpen) || pageOptsOutOfTemplating(input) {
		// input flows through unchanged: no text/template parsing or
		// execution at all, so a literal {{ }} anywhere in it - including
		// inside a fenced code block - survives verbatim into the rest of
		// the pipeline below (HTML5 parsing, embeds, page config, ...),
		// exactly like every other byte of a normal page would.
		_, _ = processed.Write(input)
	} else {
		var template *ttext.Template
		if template, err = ttext.New("current").Parse(string(input)); err != nil {
			return
		}
		// data.Page and data.FirstTitle are normally only populated after
		// this very template executes (see the extractPageConfig/getTitle
		// calls below), since both are derived from the page's own
		// rendered, HTML5-parsed output - so a page body referencing
		// {{.Page}}, {{.Title}}, or {{.FirstTitle}} always saw them empty,
		// even though the identical expression works fine in the layout
		// (which only runs once render has already finished, via
		// Generate). Give the page body a best-effort preview of each,
		// extracted straight from input's raw bytes before this Execute
		// call runs. The canonical extractPageConfig/getTitle calls below
		// still run unconditionally afterward and overwrite both with the
		// real value, so the layout's own view is completely unaffected by
		// whatever preview the page body saw.
		data.Page = earlyPageConfig(input)
		if title, ok := leadingH1Text(input); ok {
			data.FirstTitle = title
		}
		// Pass a pointer: text/template only sees pointer-receiver methods
		// (Title, E, URL, ...) on an addressable value, and a plain "data" here
		// is not addressable.
		if err = template.Execute(&processed, &data); err != nil {
			return
		}
	}
	doc, err := gen.parseAndReplace(&processed, &data)
	if err != nil {
		return
	}
	gen.cleanUnnecessaryPTags(doc)
	var pageErr error
	data.Page, pageErr = gen.extractPageConfig(doc)
	if pageErr != nil {
		// A malformed page-config comment is reported but doesn't abort the
		// render: it's kept out of the named err return (unlike every other
		// error in this function) so the rest of the page still renders.
		gen.printLine(path, "=>", pageErr)
	}
	data.FirstTitle = gen.getTitle(doc)
	body := doc.Find(atom.Body.String())
	if body.Size() > 0 {
		bodyHTML, bodyErr := body.Html()
		if bodyErr != nil {
			err = bodyErr
			return
		}
		data.Body = thtml.HTML(strings.TrimSpace(bodyHTML))
		if attrs := body.Get(0).Attr; len(attrs) > 0 {
			data.bodyAttrs = make(map[string]string, len(attrs))
			for _, a := range attrs {
				data.bodyAttrs[a.Key] = a.Val
			}
		}
	}
	if !pagePublished(data.Page) {
		// This is the answer to upstream issue #15 ("How can we exclude a
		// file from the generation loop?"): a page opts out of being
		// written to the deploy directory as its own standalone file with
		// "publish: false" in its config comment - e.g. a partial that only
		// ever makes sense embedded into another page via <embed>. The file
		// is still fully parsed above (so its own template/embeds/page
		// config all work exactly as before) and stays fully readable and
		// processable when pulled in elsewhere: Markdown/Plain/Html read
		// and process the target file directly via os.ReadFile and
		// resolveEmbedSrc, independent of this decision.
		//
		// The flag only lives inside the page's own content, so walk can't
		// know about it before render parses this far - meaning an
		// excluded page never has a deploy output for sourceIsNewer to
		// compare mtimes against, and so is treated as stale (fully
		// re-parsed, though never written) on every incremental run. This
		// is a minor inefficiency, not a correctness bug.
		//
		// It also means reaper - which only removes deploy output whose
		// source no longer exists - won't notice a page that keeps
		// existing but newly opts out with "publish: false": a
		// previously-published file's stale output under .zas/deploy
		// survives an incremental run and needs a -full run (which clears
		// the whole deploy directory upfront) to actually disappear.
		return nil
	}
	return gen.Generate(path, &data)
}

// pagePublished reports whether a page should be written to the deploy
// directory as its own standalone file. A page opts out with "publish:
// false" in its config comment; any other value, or the key's absence,
// keeps the pre-existing default of publishing every page render
// produces.
func pagePublished(page map[interface{}]interface{}) bool {
	publish, ok := page["publish"].(bool)
	return !ok || publish
}

/*
 * Removes <p> elements left completely empty by HTML5 parser error
 * recovery: a block element written inline inside a Markdown paragraph
 * implicitly closes it, and the parser synthesizes an empty <p></p> from
 * the orphaned closing tag.
 */
func (gen *Generator) cleanUnnecessaryPTags(doc *goquery.Document) {
	doc.Find(atom.P.String()).Each(func(_ int, p *goquery.Selection) {
		if p.Nodes[0].FirstChild == nil {
			p.Remove()
		}
	})
}

/*
 * Returns first H1 tag as page title.
 */
func (gen *Generator) getTitle(doc *goquery.Document) (title string) {
	result := doc.Find(atom.H1.String())
	if result.Size() > 0 {
		title = result.First().Text()
	}
	return
}

/*
 * Extracts first HTML commend as map. It expects it as a valid YAML map.
 *
 * The config comment is looked for among the document's top-level nodes
 * (Doctype, comments, the <html> element, ...), not just the literal first
 * one: a leading "<!DOCTYPE html>" - or anything else parsed as a sibling
 * ahead of the comment - would otherwise push the comment out of the
 * FirstChild slot and make its config silently disappear. This still stops
 * at the first CommentNode it finds walking top-level siblings in document
 * order, so the established "config comment as the very first line" layout
 * keeps working exactly as before.
 */
func (gen *Generator) extractPageConfig(doc *goquery.Document) (config map[interface{}]interface{}, err error) {
	var comment *html5.Node
	for _, root := range doc.Nodes {
		for child := root.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html5.CommentNode {
				comment = child
				break
			}
		}
		if comment != nil {
			break
		}
	}
	if comment != nil {
		err = yaml.Unmarshal([]byte(comment.Data), &config)
	}
	return
}

/*
 * Copies a file, preserving the source's permission bits (so an
 * executable asset stays executable in deploy - atomicWriteFile would
 * otherwise leave every copy at the fixed ZAS_DEFAULT_FILE_PERM).
 * Source mtimes are deliberately not preserved: sourceIsNewer's
 * incremental staleness check treats an equal source/deploy mtime as
 * stale (its ">=", not ">", is itself a deliberate safe-direction
 * choice), so copying the source's mtime onto the deploy file would
 * make every asset look stale, and get recopied, on every incremental
 * run.
 */
func (gen *Generator) copy(dstPath, srcPath string) (err error) {
	src, err := os.Open(srcPath)
	if err != nil {
		return
	}
	defer func() { _ = src.Close() }()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	if err = gen.atomicWriteFile(dstPath, func(w io.Writer) error {
		_, err := io.Copy(w, src)
		return err
	}); err != nil {
		return err
	}
	return os.Chmod(dstPath, info.Mode())
}

// resolveEmbedSrc resolves an <embed src="..."> attribute to an absolute
// path and rejects any result that falls outside the site root - the
// process's current directory, the same base every embed src is already
// read relative to (see walk's use of "." and BuildDeployPath). Without
// this check, an absolute src or a "../" traversal in site content lets a
// page pull the contents of any file the build process can read into the
// published output.
func (gen *Generator) resolveEmbedSrc(src string) (string, error) {
	root, err := filepath.Abs(".")
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(src)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("embed src %q escapes the site root", src)
	}
	return target, nil
}

// Markdown embeds a Markdown file.
func (gen *Generator) Markdown(e *goquery.Selection, _ *goquery.Document, data *ZasData) (err error) {
	if src, ok := e.Attr(atom.Src.String()); ok {
		resolved, err := gen.resolveEmbedSrc(src)
		if err != nil {
			return err
		}
		mdInput, err := os.ReadFile(resolved)
		if err != nil {
			return err
		}
		var b bytes.Buffer
		if err := markdownConverter.Convert(mdInput, &b); err != nil {
			return err
		}
		mdDoc, err := gen.parseAndReplace(&b, data)
		if err != nil {
			return err
		}
		e.ReplaceWithSelection(mdDoc.Find(atom.Body.String()))
	}
	return
}

// Plain embeds a plain text file.
func (gen *Generator) Plain(e *goquery.Selection, _ *goquery.Document, data *ZasData) (err error) {
	if src, ok := e.Attr(atom.Src.String()); ok {
		resolved, err := gen.resolveEmbedSrc(src)
		if err != nil {
			return err
		}
		var input []byte
		input, err = os.ReadFile(resolved)
		if err != nil {
			return err
		}
		var template *ttext.Template
		template, err = ttext.New("current").Parse(string(input))
		if err != nil {
			return err
		}
		var processed bytes.Buffer
		if err = template.Execute(&processed, data); err != nil {
			return err
		}
		e.ReplaceWithNodes(&html5.Node{Type: html5.TextNode, Data: processed.String()})
	}
	return
}

// Html embeds a HTML file.
func (gen *Generator) Html(e *goquery.Selection, _ *goquery.Document, data *ZasData) (err error) {
	if src, ok := e.Attr(atom.Src.String()); ok {
		resolved, err := gen.resolveEmbedSrc(src)
		if err != nil {
			return err
		}
		var input []byte
		input, err = os.ReadFile(resolved)
		if err != nil {
			return err
		}
		var htmlDoc *goquery.Document
		htmlDoc, err = gen.parseAndReplace(bytes.NewBuffer(input), data)
		if err != nil {
			return err
		}
		e.ReplaceWithSelection(htmlDoc.Children())
	}
	return
}

/*
 * Handles <embed> tags.
 *
 * They can be handled with MIME type plugins or internal exported methods like Markdown.
 */
func (gen *Generator) handleEmbedTags(doc *goquery.Document, data *ZasData) (err error) {
	doc.Find(atom.Embed.String()).EachWithBreak(func(_ int, e *goquery.Selection) bool {
		if src, ok := e.Attr(atom.Src.String()); ok {
			var typ string
			if typ, ok = e.Attr(atom.Type.String()); !ok {
				err = fmt.Errorf("missing type attribute for embed '%s'", src)
				return false
			}
			plugin := gen.resolveMIMETypePlugin(typ)
			method := reflect.ValueOf(gen).MethodByName(cases.Title(language.English).String(plugin))
			if !isEmbedPluginMethod(method) {
				err = gen.handleMIMETypePlugin(e)
			} else {
				args := make([]reflect.Value, 3)
				args[0] = reflect.ValueOf(e)
				args[1] = reflect.ValueOf(doc)
				args[2] = reflect.ValueOf(data)
				r := method.Call(args)
				rerr := r[0].Interface()
				if ierr, ok := rerr.(error); ok {
					err = ierr
				}
			}
			if err != nil {
				return false
			}
		}
		return true
	})
	return
}

/*
 * Reports whether method is a valid embed-plugin dispatch target: a method
 * with the exact (e *goquery.Selection, doc *goquery.Document, data *ZasData) error
 * signature. Config data chooses the method name (see resolveMIMETypePlugin),
 * so any exported Generator method is reachable by MethodByName and must be
 * shape-checked before Call to avoid a reflect panic on arity/type mismatch.
 */
func isEmbedPluginMethod(method reflect.Value) bool {
	if !method.IsValid() {
		return false
	}
	want := []reflect.Type{
		reflect.TypeFor[*goquery.Selection](),
		reflect.TypeFor[*goquery.Document](),
		reflect.TypeFor[*ZasData](),
	}
	t := method.Type()
	if t.NumIn() != len(want) || t.NumOut() != 1 {
		return false
	}
	for i, w := range want {
		if t.In(i) != w {
			return false
		}
	}
	return true
}

// pluginNameRe restricts resolved plugin names to safe exec.Command argv[0]
// suffixes. Without it, a mimetypes entry like {text/x: "../../evil"} would
// make exec.Command skip PATH lookup and run a path relative to the working
// directory - and mimetypes config is repo content, so this closes a code
// execution hole for anyone who can send a pull request.
var pluginNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

/*
 * Invokes a MIME type plugin based on current node's type attribute, passing
 * src attribute's value as argument. Subcommand's stdout replaces the embed
 * tag as HTML; stderr is passed through to the user's shell.
 */
func (gen *Generator) handleMIMETypePlugin(e *goquery.Selection) error {
	src, ok := e.Attr(atom.Src.String())
	if !ok {
		return errors.New("missing src attribute for embed")
	}
	typ, ok := e.Attr(atom.Type.String())
	if !ok {
		return fmt.Errorf("missing type attribute for embed %q", src)
	}
	cmdname := gen.resolveMIMETypePlugin(typ)
	if !pluginNameRe.MatchString(cmdname) {
		return fmt.Errorf("no valid plugin configured for embed type %q (src %q)", typ, src)
	}
	cmd := exec.Command(fmt.Sprintf("m%s%s", ZAS_PREFIX, cmdname), src)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("plugin m%s%s failed for %q: %w", ZAS_PREFIX, cmdname, src, err)
	}
	e.ReplaceWithHtml(string(out))
	return nil
}

/*
 * Returns registered plugin (without ZAS_PREFIX) from config.
 */
func (gen *Generator) resolveMIMETypePlugin(typ string) string {
	return gen.Config.GetSection("mimetypes").GetString(typ)
}

func noescape(data string) thtml.HTML {
	return thtml.HTML(data)
}

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
	"strings"
	"sync"
	ttext "text/template"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/melvinmt/gt"
	markdown "github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
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
	"eq":       eq,
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
var markdownConverter = markdown.New(markdown.WithRendererOptions(
	renderer.WithNodeRenderers(util.Prioritized(&rawHTMLRenderer{}, 100)),
))

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

	// Guards errs, which collects per-file render errors from renderAsync goroutines.
	mu   sync.Mutex
	errs []error

	// reapedDirs holds deploy paths reaper has RemoveAll'd during the
	// current reap walk. The reap walk is single-threaded, so no mutex.
	reapedDirs map[string]struct{}

	// claimedOutputs maps each deploy output path claimed so far to the
	// source path that claimed it, so two sources that render to the same
	// output (e.g. foo.md and foo.html) don't both spawn a renderAsync
	// goroutine and race to write it. Like reapedDirs, this is only
	// touched from walk, whose own invocations are sequential, so no
	// mutex is needed.
	claimedOutputs map[string]string
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

// swapExtension replaces path's trailing from extension with to. Paths not
// ending in from are returned unchanged. Unlike strings.Replace/ReplaceAll,
// this only ever touches the extension, never a from/to occurrence earlier
// in path.
func swapExtension(path, from, to string) string {
	if !strings.HasSuffix(path, from) {
		return path
	}
	return strings.TrimSuffix(path, from) + to
}

// Generate renders and writes the file at data.Path using the given
// template context.
func (gen *Generator) Generate(_ string, data *ZasData) (err error) {
	var processed bytes.Buffer
	if err = gen.Layout.Execute(&processed, data); err != nil {
		return
	}
	doc, err := gen.parseAndReplace(processed, data)
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
	f, err := os.OpenFile(gen.BuildDeployPath(data.Path), os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.FileMode(ZAS_DEFAULT_FILE_PERM))
	if err != nil {
		return
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	err = html5.Render(f, doc.Get(0))
	if err != nil {
		return
	}
	return
}

// maxEmbedDepth bounds how many levels of <embed> an entry file may nest.
// Markdown and HTML embed handlers call back into parseAndReplace for
// whatever they embed, so a file that embeds itself (directly, or through a
// cycle of mutually-embedding files) would otherwise recurse until the
// goroutine's stack is exhausted.
const maxEmbedDepth = 20

func (gen *Generator) parseAndReplace(processed bytes.Buffer, data *ZasData) (doc *goquery.Document, err error) {
	if data.embedDepth >= maxEmbedDepth {
		return nil, fmt.Errorf("embed nesting deeper than %d levels; check for a self- or mutually-embedding file", maxEmbedDepth)
	}
	data.embedDepth++
	defer func() { data.embedDepth-- }()
	// Here we manipulate its result.
	doc, err = goquery.NewDocumentFromReader(&processed)
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
	if err := filepath.Walk(".", walk); err != nil {
		return err
	}
	gen.wg.Wait()
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
	if info.IsDir() {
		ierr = os.MkdirAll(gen.BuildDeployPath(path), os.FileMode(ZAS_DEFAULT_DIR_PERM))
	} else if gen.sourceIsNewer(path, info) {
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
			fmt.Println("+", path)
		}
		gen.wg.Add(1)
		go gen.renderAsync(path)
	}
	return
}

func (gen *Generator) renderAsync(path string) {
	var err error
	defer gen.wg.Done()

	switch {
	case strings.HasSuffix(path, ".md"):
		err = gen.renderMarkdown(path)
	case strings.HasSuffix(path, ".html"):
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
		if strings.HasSuffix(sourcePath, ".html") {
			sourcePath = swapExtension(sourcePath, ".html", ".md")
			sourceNew, err := os.Open(sourcePath)
			if err == nil {
				_ = sourceNew.Close()
				reap = false
			}
		}
		if reap {
			if gen.Verbose {
				fmt.Println("-", sourcePath)
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
	_ = yaml.Unmarshal(data, &config)
	if info, statErr := os.Stat(confPath); statErr == nil {
		modTime = info.ModTime()
	}
	gen.setCachedDirConfig(path, dirConfigEntry{config: config, modTime: modTime})
	return config, modTime, nil
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

/*
 * Generic render function. It expects input to be a valid HTML document.
 * Input can be a valid Go template.
 */
func (gen *Generator) render(path string, input []byte) (err error) {
	template, err := ttext.New("current").Parse(string(input))
	if err != nil {
		return
	}
	var processed bytes.Buffer
	// Building context and rendering template.
	data := NewZasData(path, gen)
	data.Directory, _, _ = gen.loadZasDirectoryConfig(path)
	// Pass a pointer: text/template only sees pointer-receiver methods
	// (Title, E, URL, ...) on an addressable value, and a plain "data" here
	// is not addressable.
	if err = template.Execute(&processed, &data); err != nil {
		return
	}
	doc, err := gen.parseAndReplace(processed, &data)
	if err != nil {
		fmt.Println(err)
		return
	}
	gen.cleanUnnecessaryPTags(doc)
	data.Page, err = gen.extractPageConfig(doc)
	if err != nil {
		fmt.Println(path, "=>", err)
		err = nil
	}
	data.FirstTitle = gen.getTitle(doc)
	body := doc.Find(atom.Body.String())
	if err != nil {
		return
	}
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
	return gen.Generate(path, &data)
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
 */
func (gen *Generator) extractPageConfig(doc *goquery.Document) (config map[interface{}]interface{}, err error) {
	var comment *html5.Node
	for _, child := range doc.Nodes {
		if child.FirstChild == nil {
			continue
		}
		if child.FirstChild.Type == html5.CommentNode {
			comment = child.FirstChild
			break
		}
	}
	if comment != nil {
		_ = yaml.Unmarshal([]byte(comment.Data), &config)
	}
	return
}

/*
 * Copies a file.
 */
func (gen *Generator) copy(dstPath, srcPath string) (err error) {
	src, err := os.Open(srcPath)
	if err != nil {
		return
	}
	defer func() { _ = src.Close() }()
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, os.FileMode(ZAS_DEFAULT_FILE_PERM))
	if err != nil {
		return
	}
	defer func() {
		if cerr := dst.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	_, err = io.Copy(dst, src)
	return
}

// Markdown embeds a Markdown file.
func (gen *Generator) Markdown(e *goquery.Selection, _ *goquery.Document, data *ZasData) (err error) {
	if src, ok := e.Attr(atom.Src.String()); ok {
		mdInput, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		var b bytes.Buffer
		if err := markdownConverter.Convert(mdInput, &b); err != nil {
			return err
		}
		mdDoc, err := gen.parseAndReplace(b, data)
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
		var input []byte
		input, err = os.ReadFile(src)
		if err != nil {
			return err
		}
		var template *ttext.Template
		template, err = ttext.New("current").Parse(string(input))
		if err != nil {
			return
		}
		var processed bytes.Buffer
		if err = template.Execute(&processed, data); err != nil {
			return
		}
		e.ReplaceWithNodes(&html5.Node{Type: html5.TextNode, Data: processed.String()})
	}
	return
}

// Html embeds a HTML file.
func (gen *Generator) Html(e *goquery.Selection, _ *goquery.Document, data *ZasData) (err error) {
	if src, ok := e.Attr(atom.Src.String()); ok {
		var input []byte
		input, err = os.ReadFile(src)
		if err != nil {
			return err
		}
		var htmlDoc *goquery.Document
		htmlDoc, err = gen.parseAndReplace(*bytes.NewBuffer(input), data)
		if err != nil {
			return
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

func eq(a, b interface{}) bool {
	return a == b
}

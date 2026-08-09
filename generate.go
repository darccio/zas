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
	"html"
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

	"github.com/PuerkitoBio/goquery"
	"github.com/melvinmt/gt"
	markdown "github.com/yuin/goldmark"
	htmlrenderer "github.com/yuin/goldmark/renderer/html"
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

/*
 * Convenience type to group relevant rendering info.
 */
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
	// ZasDirectoryConfigs cache
	cachedZasDirectoryConfigs map[string]ConfigSection
	// Guards cachedZasDirectoryConfigs, read and written from many renderAsync goroutines.
	dirConfigMu sync.Mutex

	wg sync.WaitGroup

	// Guards errs, which collects per-file render errors from renderAsync goroutines.
	mu   sync.Mutex
	errs []error
}

/*
 * Records err for later aggregation, safe for concurrent use.
 */
func (gen *Generator) recordErr(err error) {
	gen.mu.Lock()
	gen.errs = append(gen.errs, err)
	gen.mu.Unlock()
}

/*
 * Returns deployment base path in config.
 */
func (gen *Generator) GetDeployPath() string {
	return gen.Config.GetZString("deploy")
}

/*
 * Builds deployment path for specific file pointed by path.
 */
func (gen *Generator) BuildDeployPath(path string) string {
	return filepath.Join(gen.GetDeployPath(), path)
}

/*
 * Renders and writes current file "path" with context "data".
 */
func (gen *Generator) Generate(path string, data *ZasData) (err error) {
	var processed bytes.Buffer
	if err = gen.Layout.Execute(&processed, data); err != nil {
		return
	}
	doc, err := gen.parseAndReplace(processed, data)
	if err != nil {
		return
	}
	f, err := os.OpenFile(gen.BuildDeployPath(data.Path), os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.FileMode(ZAS_DEFAULT_FILE_PERM))
	if err != nil {
		return
	}
	defer f.Close()
	err = html5.Render(f, doc.Get(0))
	if err != nil {
		return
	}
	return
}

func (gen *Generator) parseAndReplace(processed bytes.Buffer, data *ZasData) (doc *goquery.Document, err error) {
	// Here we manipulate its result.
	doc, err = goquery.NewDocumentFromReader(&processed)
	if err != nil {
		return
	}
	err = gen.handleEmbedTags(doc, data)
	return
}

func (gen *Generator) Run() error {
	cfg, err := NewConfig()
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("not a valid Zas repository: %w", err)
		}
		return err
	}
	gen.Config = cfg
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

/*
 * Real walking function. Handles all supported files and copy not supported ones in current deployment path.
 */
func (gen *Generator) walk(path string, info os.FileInfo, err error) (ierr error) {
	if err != nil {
		return err
	}
	if strings.HasPrefix(path, ".") || strings.HasPrefix(filepath.Base(path), ".") || strings.Contains(path, fmt.Sprintf("%s/", ZAS_DIR)) {
		return
	}
	if info.IsDir() {
		ierr = os.MkdirAll(gen.BuildDeployPath(path), os.FileMode(ZAS_DEFAULT_DIR_PERM))
	} else {
		if gen.sourceIsNewer(path, info) {
			if gen.Verbose {
				fmt.Println("+", path)
			}
			gen.wg.Add(1)
			go gen.renderAsync(path)
		}
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
func (gen *Generator) reaper(path string, info os.FileInfo, err error) (ierr error) {
	if err != nil {
		return err
	}
	sourcePath := strings.Replace(path, gen.GetDeployPath(), ".", 1)
	source, err := os.Open(sourcePath)
	// TODO it must clean directories too
	if err != nil {
		reap := true
		if strings.HasSuffix(sourcePath, ".html") {
			sourcePath = strings.Replace(sourcePath, ".html", ".md", 1)
			sourceNew, err := os.Open(sourcePath)
			if err == nil {
				sourceNew.Close()
				reap = false
			}
		}
		if reap {
			if gen.Verbose {
				fmt.Println("-", sourcePath)
			}
			os.RemoveAll(path)
		}
	} else {
		source.Close()
	}
	return
}

func (gen *Generator) sourceIsNewer(path string, sourceInfo os.FileInfo) bool {
	// Shortcut
	if gen.Full {
		return true
	}
	realpath := string(path)
	if strings.HasSuffix(path, ".md") {
		realpath = strings.Replace(path, ".md", ".html", 1)
	}
	destination, err := os.Open(gen.BuildDeployPath(realpath))
	if err != nil {
		return true
	}
	defer destination.Close()
	destinationInfo, err := destination.Stat()
	if err != nil {
		return true
	}
	return sourceInfo.ModTime().UnixNano() >= destinationInfo.ModTime().UnixNano()
}

/*
 * Renders a Markdown file.
 */
func (gen *Generator) renderMarkdown(path string) (err error) {
	input, err := os.ReadFile(path)
	if err != nil {
		return
	}
	// This is going to haunt me for a while.
	var b bytes.Buffer
	if err := markdown.New(markdown.WithRendererOptions(htmlrenderer.WithUnsafe())).Convert(input, &b); err != nil {
		return err
	}
	md := []byte(html.UnescapeString(b.String()))
	return gen.render(path, md)
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

/*
 * Loads ZAS_DIR_CONF_FILE (as defined in constants.go) from current
 * directory or previously found ones.
 * It must be a YAML file.
 */
func (gen *Generator) loadZasDirectoryConfig(currentpath string) (config ConfigSection, err error) {
	path := filepath.Dir(currentpath)
	if config, ok := gen.getCachedDirConfig(path); ok {
		return config, nil
	}
	data, err := os.ReadFile(fmt.Sprintf("%s/%s", path, ZAS_DIR_CONF_FILE))
	if err != nil {
		// Maybe .zas.yml is in an upper directory (already cached or not),
		// so we call this recursively.
		// Unless we are at current working directory.
		if path == "." {
			return nil, err
		}
		return gen.loadZasDirectoryConfig(path)
	}
	config = make(ConfigSection)
	_ = yaml.Unmarshal(data, &config)
	gen.setCachedDirConfig(path, config)
	return config, nil
}

/*
 * Reads cachedZasDirectoryConfigs, safe for concurrent use.
 */
func (gen *Generator) getCachedDirConfig(path string) (config ConfigSection, ok bool) {
	gen.dirConfigMu.Lock()
	defer gen.dirConfigMu.Unlock()
	config, ok = gen.cachedZasDirectoryConfigs[path]
	return
}

/*
 * Writes cachedZasDirectoryConfigs, safe for concurrent use.
 */
func (gen *Generator) setCachedDirConfig(path string, config ConfigSection) {
	gen.dirConfigMu.Lock()
	defer gen.dirConfigMu.Unlock()
	if gen.cachedZasDirectoryConfigs == nil {
		gen.cachedZasDirectoryConfigs = make(map[string]ConfigSection)
	}
	gen.cachedZasDirectoryConfigs[path] = config
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
	data.Directory, _ = gen.loadZasDirectoryConfig(path)
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
	}
	return gen.Generate(path, &data)
}

/*
 * Removes unnecessary paragraph HTML tags generated during Markdown processing by
 * deleting any <p> without child text nodes (just to avoid deletion if semantic tags
 * are inside).
 */
func (gen *Generator) cleanUnnecessaryPTags(doc *goquery.Document) {
	doc.Find(atom.P.String()).Each(func(ix int, p *goquery.Selection) {
		hasText := false
		// Little heuristic to remove nodes with visually empty content.
		content := strings.TrimSpace(p.Nodes[0].Data)
		if content != "" {
			hasText = true
		}
		// If current <p> tag doesn't have any child text node, extract children and add to its parent.
		if !hasText {
			p.ReplaceWithSelection(p.Children())
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
	defer src.Close()
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, os.FileMode(ZAS_DEFAULT_FILE_PERM))
	if err != nil {
		return
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return
}

/*
 * Embeds a Markdown file.
 */
func (gen *Generator) Markdown(e *goquery.Selection, doc *goquery.Document, data *ZasData) (err error) {
	if src, ok := e.Attr(atom.Src.String()); ok {
		mdInput, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		var b bytes.Buffer
		if err := markdown.New(markdown.WithRendererOptions(htmlrenderer.WithUnsafe())).Convert(mdInput, &b); err != nil {
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

/*
 * Embeds a plain text file.
 */
func (gen *Generator) Plain(e *goquery.Selection, doc *goquery.Document, data *ZasData) (err error) {
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

/*
 * Embeds a HTML file.
 */
func (gen *Generator) Html(e *goquery.Selection, doc *goquery.Document, data *ZasData) (err error) {
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
	doc.Find(atom.Embed.String()).EachWithBreak(func(ix int, e *goquery.Selection) bool {
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

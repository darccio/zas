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
	"fmt"
	"html"
	thtml "html/template"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	ttext "text/template"

	"github.com/PuerkitoBio/goquery"
	"github.com/melvinmt/gt"
	markdown "github.com/yuin/goldmark"
	html5 "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	yaml "gopkg.in/yaml.v2"
)

var (
	helpers = thtml.FuncMap{
		"noescape": noescape,
		"eq":       eq,
	}
)

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
	// Errors channel for async actions
	errs chan error
	// Done channel for async actions
	done chan bool
	// Counter for started goroutines
	expectedFiles int
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
			fmt.Printf("fatal: Not a valid Zas repository: %s\n", err)
			return err
		}
		return err
	}
	gen.Config = cfg
	gen.errs = make(chan error)
	gen.done = make(chan bool)
	go gen.parseLayout()
	go gen.loadI18N()
	go gen.handleDeployPath(gen.Full)
	gen.wait(3)
	// Walking function. It allows to bubble up any error from generator.
	walk := func(path string, info os.FileInfo, err error) error {
		return gen.walk(path, info, err)
	}
	// TODO optimize for lesser hits on disk
	if err := filepath.Walk(".", gen.countwalk); err != nil {
		return err
	}
	if err := filepath.Walk(".", walk); err != nil {
		return err
	}
	gen.wait(gen.expectedFiles)
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
	return nil
}

/*
 * Waits for the last N started goroutines.
 */
func (gen *Generator) wait(goroutines int) (err error) {
	for i := 0; i < goroutines; i++ {
		select {
		case <-gen.done:
			// NOOP
		case err = <-gen.errs:
			// TODO improve error handling showing which file caused a given error
			if err != nil {
				return
			}
		}
	}
	return
}

func (gen *Generator) parseLayout() {
	var err error
	layout := gen.Config.GetZString("layout")
	if gen.Layout, err = thtml.New(filepath.Base(layout)).Funcs(helpers).ParseFiles(layout); err != nil {
		panic(err)
	}
	gen.done <- true
}

func (gen *Generator) loadI18N() {
	mainlang := gen.Config.GetSection("site").GetString("language")
	i18nStrings, err := NewI18n(mainlang)
	if err != nil {
		panic(err)
	}
	gen.I18n = &gt.Build{
		Index:  i18nStrings,
		Origin: mainlang,
	}
	gen.done <- true
}

func (gen *Generator) handleDeployPath(full bool) {
	deployPath := gen.GetDeployPath()
	// If deployment path already exists, it must be deleted.
	if _, err := os.Stat(deployPath); err == nil && full {
		if err = os.RemoveAll(deployPath); err != nil {
			panic(err)
		}
	}
	if err := os.MkdirAll(deployPath, os.FileMode(ZAS_DEFAULT_DIR_PERM)); err != nil {
		panic(err)
	}
	gen.done <- true
}

/*
 * Count walking function. Counts all files affected by walk().
 */
func (gen *Generator) countwalk(path string, info os.FileInfo, err error) (ierr error) {
	if strings.HasPrefix(path, ".") || strings.HasPrefix(filepath.Base(path), ".") {
		return
	}
	if !info.IsDir() {
		if gen.sourceIsNewer(path, info) {
			gen.expectedFiles++
		}
	}
	return
}

/*
 * Real walking function. Handles all supported files and copy not supported ones in current deployment path.
 */
func (gen *Generator) walk(path string, info os.FileInfo, err error) (ierr error) {
	if strings.HasPrefix(path, ".") || strings.HasPrefix(filepath.Base(path), ".") {
		return
	}
	if info.IsDir() {
		ierr = os.MkdirAll(gen.BuildDeployPath(path), os.FileMode(ZAS_DEFAULT_DIR_PERM))
	} else {
		if gen.sourceIsNewer(path, info) {
			if gen.Verbose {
				fmt.Println("+", path)
			}
			go gen.renderAsync(path)
		}
	}
	return
}

func (gen *Generator) renderAsync(path string) {
	var err error
	switch {
	case strings.HasSuffix(path, ".md"):
		err = gen.renderMarkdown(path)
	case strings.HasSuffix(path, ".html"):
		err = gen.renderHTML(path)
	default:
		err = gen.copy(gen.BuildDeployPath(path), path)
	}
	if err != nil {
		gen.errs <- err
	}
	gen.done <- true
}

/*
 * Real reaping function. Reaps all missing source files in current deployment path.
 */
func (gen *Generator) reaper(path string, info os.FileInfo, err error) (ierr error) {
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
	destinationInfo, _ := destination.Stat()
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
	if err := markdown.Convert(input, &b); err != nil {
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
	var ok bool
	path := filepath.Dir(currentpath)
	if config, ok = gen.cachedZasDirectoryConfigs[path]; !ok {
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
		if gen.cachedZasDirectoryConfigs == nil {
			gen.cachedZasDirectoryConfigs = make(map[string]ConfigSection)
		}
		gen.cachedZasDirectoryConfigs[path] = config
	}
	return
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
	if err = template.Execute(&processed, data); err != nil {
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
		var bbody bytes.Buffer
		err = html5.Render(&bbody, body.Get(0))
		if err != nil {
			return
		}
		data.Body = thtml.HTML(bbody.Bytes())
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
func (gen *Generator) copy(dstPath string, srcPath string) (err error) {
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
		if err := markdown.Convert(mdInput, &b); err != nil {
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
		e.Parent().Nodes[0].Data = processed.String()
		e.Remove()
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
			if method == reflect.ValueOf(nil) {
				err = gen.handleMIMETypePlugin(e, doc)
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

type bufErr struct {
	buffer []byte
	err    error
}

/*
 * Invokes a MIME type plugin based on current node's type attribute, passing src attribute's value
 * as argument. Subcommand's output is piped to Gokogiri through a buffer.
 */
func (gen *Generator) handleMIMETypePlugin(e *goquery.Selection, doc *goquery.Document) (err error) {
	var (
		src, typ string
		ok       bool
	)
	if src, ok = e.Attr(atom.Src.String()); ok {
		return
	}
	if typ, ok = e.Attr(atom.Type.String()); ok {
		return
	}
	cmdname := gen.resolveMIMETypePlugin(typ)
	if cmdname == "" {
		return
	}
	cmd := exec.Command(fmt.Sprintf("m%s%s", ZAS_PREFIX, cmdname), src)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	cmd.Stderr = os.Stderr
	c := make(chan bufErr)
	go func() {
		data, err := io.ReadAll(stdout)
		c <- bufErr{data, err}
	}()
	if err = cmd.Start(); err != nil {
		return
	}
	be := <-c
	if err = cmd.Wait(); err != nil {
		return
	}
	if be.err != nil {
		return be.err
	}
	e.ReplaceWithHtml(string(be.buffer))
	return
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

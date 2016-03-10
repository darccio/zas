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
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/bevik/etree"
	"github.com/melvinmt/gt"
	markdown "github.com/russross/blackfriday"
	yaml "gopkg.in/yaml.v2"
	thtml "html/template"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	ttext "text/template"
)

var (
	cmdGenerate = &Subcommand{
		UsageLine: "generate",
	}
	helpers = thtml.FuncMap{
		"noescape": noescape,
		"eq":       eq,
	}
)

/*
 * Convenience type to group relevant rendering info.
 */
type Generator struct {
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
	f, err := os.OpenFile(gen.BuildDeployPath(data.Path), O_RDWR|O_CREATE|O_TRUNC, os.FileMode(ZAS_DEFAULT_FILE_PERM))
	if err != nil {
		return
	}
	w := bufio.NewWriter(f)
	defer f.Close()
	defer w.Flush()
	_, err = doc.WriteTo(w)
	if err != nil {
		return
	}
	return
}

func (gen *Generator) parseAndReplace(processed bytes.Buffer, data *ZasData) (doc *etree.Document, err error) {
	// Here we manipulate its result.
	doc = etree.NewDocument()
	err = doc.ReadFrom(processed)
	if err != nil {
		return
	}
	err = gen.handleEmbedTags(doc, data)
	return
}

var verbose = cmdGenerate.Flag.Bool("verbose", false, "Verbose output")
var full = cmdGenerate.Flag.Bool("full", false, "Full generation (non-incremental mode)")

func init() {
	cmdGenerate.Run = func() {
		var (
			gen Generator
			err error
		)
		if gen.Config, err = NewConfig(); err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("fatal: Not a valid Zas repository: %s\n", err)
				return
			}
			panic(err)
		}
		gen.errs = make(chan error)
		gen.done = make(chan bool)
		go gen.parseLayout()
		go gen.loadI18N()
		go gen.handleDeployPath(*full)
		gen.wait(3)
		// Walking function. It allows to bubble up any error from generator.
		walk := func(path string, info os.FileInfo, err error) error {
			return gen.walk(path, info, err)
		}
		if err := filepath.Walk(".", gen.countwalk); err != nil {
			panic(err)
		}
		if err := filepath.Walk(".", walk); err != nil {
			panic(err)
		}
		gen.wait(gen.expectedFiles)
		if !*full {
			// TODO Can we go parallel?
			// This removes deleted source files in deploy path
			reapwalk := func(path string, info os.FileInfo, err error) error {
				return gen.reaper(path, info, err)
			}
			if err = filepath.Walk(gen.GetDeployPath(), reapwalk); err != nil {
				panic(err)
			}
		}
	}
	cmdGenerate.Init()
}

/*
 * Waits for the last N started goroutines.
 */
func (gen *Generator) wait(goroutines int) {
	for i := 0; i < goroutines; i++ {
		<-gen.done
	}
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
			if *verbose {
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
	gen.done <- true
	if err != nil {
		gen.errs <- err
	}
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
			if *verbose {
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
	if *full {
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
	input, err := ioutil.ReadFile(path)
	if err != nil {
		return
	}
	md := markdown.MarkdownCommon(input)
	return gen.render(path, md)
}

/*
 * Renders a HTML file.
 */
func (gen *Generator) renderHTML(path string) (err error) {
	input, err := ioutil.ReadFile(path)
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
		data, err := ioutil.ReadFile(fmt.Sprintf("%s/%s", path, ZAS_DIR_CONF_FILE))
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
		err = yaml.Unmarshal(data, &config)
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
		return
	}
	gen.cleanUnnecessaryPTags(doc)
	data.Page, err = gen.extractPageConfig(doc)
	if err != nil {
		fmt.Println(path, "=>", err)
		err = nil
	}
	data.FirstTitle = gen.getTitle(doc)
	body, err := doc.FindElements("//body")
	if err != nil {
		return
	}
	if len(body) > 0 {
		var bbody bytes.Buffer
		_, err = body[0].WriteTo(bbody)
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
func (gen *Generator) cleanUnnecessaryPTags(doc *etree.Document) (err error) {
	ps, err := doc.FindElements("//p")
	if err != nil {
		return
	}
	for _, p := range ps {
		hasText := false
		for child := range p.Child {
			if cd, ok := child.(*etree.CharData); ok {
				// Little heuristic to remove nodes with visually empty content.
				content := strings.TrimSpace(cd.Data)
				if content != "" {
					hasText = true
					break
				}
			}
		}
		// If current <p> tag doesn't have any child text node, extract children and add to its parent.
		if !hasText {
			parent := p.Parent
			for child := range p.Child {
				parent.Child = append(parent.Child, child)
			}
			parent.RemoveElement(p)
		}
	}
	return
}

/*
 * Returns first H1 tag as page title.
 */
func (gen *Generator) getTitle(doc *etree.Document) (title string) {
	result, _ := doc.FindElements("//h1")
	if len(result) > 0 {
		title = result[0].Child[0].Text()
	}
	return
}

/*
 * Extracts first HTML commend as map. It expects it as a valid YAML map.
 */
func (gen *Generator) extractPageConfig(doc *etree.Document) (config map[interface{}]interface{}, err error) {
	var comment *etree.Comment
	for child := range doc.Child {
		if c, ok := child.(*etree.Comment); ok {
			comment = c
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
func (gen *Generator) Markdown(e *etree.Element, doc *etree.Document, data *ZasData) (err error) {
	src := e.SelectAttribute("src").Value
	mdInput, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}
	md := markdown.MarkdownCommon(mdInput)
	mdDoc, err := gen.parseAndReplace(*bytes.NewBuffer(md), data)
	if err != nil {
		return err
	}
	partial, err := mdDoc.FindElements("//body")
	if err != nil {
		return err
	}
	parent := e.Parent
	gen.appendChildren(parent, partial[0].Child)
	parent.RemoveElement(e)
	return
}

/*
 * Embeds a plain text file.
 */
func (gen *Generator) Plain(e *etree.Element, doc *etree.Document, data *ZasData) (err error) {
	src := e.SelectAttribute("src").Value
	input, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}
	template, err := ttext.New("current").Parse(string(input))
	if err != nil {
		return
	}
	var processed bytes.Buffer
	if err = template.Execute(&processed, data); err != nil {
		return
	}
	parent := e.Parent
	parent.CreateCharData(string(processed.Bytes()))
	parent.RemoveElement(e)
	return
}

/*
 * Embeds a HTML file.
 */
func (gen *Generator) Html(e *etree.Element, doc *etree.Document, data *ZasData) (err error) {
	src := e.SelectAttribute("src").Value
	input, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}
	parent := e.Parent()
	htmlDoc, err := gen.parseAndReplace(*bytes.NewBuffer(input), data)
	nodes, err := gen.coerce(htmlDoc.WriteToBytes())
	if err != nil {
		return err
	}
	gen.appendChildren(parent, nodes)
	parent.RemoveElement(e)
	return
}

/*
 * Handles <embed> tags.
 *
 * They can be handled with MIME type plugins or internal exported methods like Markdown.
 */
func (gen *Generator) handleEmbedTags(doc *etree.Document, data *ZasData) (err error) {
	result, err := doc.FindElements("//embed")
	if err != nil {
		return
	}
	for _, e := range result {
		plugin := gen.resolveMIMETypePlugin(e.SelectAttribute("type").Value)
		method := reflect.ValueOf(gen).MethodByName(strings.Title(plugin))
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
			return
		}
	}
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
func (gen *Generator) handleMIMETypePlugin(e *etree.Element, doc *etree.Document) (err error) {
	src := e.SelectAttribute("src").Value
	typ := e.SelectAttribute("type").Value
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
		data, err := ioutil.ReadAll(stdout)
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
	parent := e.Parent
	child, err := gen.coerce(be.buffer)
	if err != nil {
		return
	}
	gen.appendChildren(parent, child)
	parent.RemoveElement(e)
	return
}

func (gen *Generator) coerce(data []byte) (els []*Element, err error) {
	doc := etree.NewDocument()
	err = doc.ReadFromBytes(data)
	if err != nil {
		return
	}
	els = doc.ChildElements()
	return
}

func (gen *Generator) appendChildren(parent *Element, children []*Element) {
	tokens := make([]Token, len(children))
	for ix, child := range children {
		tokens[ix] = child.(Token)
	}
	parent.Child = append(parent.Child, tokens...)
}

/*
 * Returns registered plugin (without ZAS_PREFIX) from config.
 */
func (gen *Generator) resolveMIMETypePlugin(typ string) string {
	return gen.Config.GetSection("mimetypes").GetString(typ)
}

func noescape(text string) thtml.HTML {
	return thtml.HTML(text)
}

func eq(a, b interface{}) bool {
	return a == b
}

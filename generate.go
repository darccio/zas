/*
 * Copyright (c) 2013 Dario Castañé.
 * This file is part of Zingy.
 *
 * Zingy is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * Zingy is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with Zingy.  If not, see <http://www.gnu.org/licenses/>.
 */
package main

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/moovweb/gokogiri"
	"github.com/moovweb/gokogiri/html"
	"github.com/moovweb/gokogiri/xml"
	markdown "github.com/russross/blackfriday"
	thtml "html/template"
	"io"
	"io/ioutil"
	yaml "launchpad.net/goyaml"
	"os"
	"os/exec"
	pathes "path"
	"path/filepath"
	"reflect"
	"strings"
	ttext "text/template"
)

var cmdGenerate = &Subcommand{
	UsageLine: "generate",
}

type Generator struct {
	Config ConfigSection
	Layout *thtml.Template
}

func (gen *Generator) GetDeployPath() string {
	return gen.Config.GetZString("deploy")
}

func (gen *Generator) BuildDeployPath(path string) string {
	return filepath.Join(gen.GetDeployPath(), path)
}

func (gen *Generator) Generate(path string, data *ZingyData) (err error) {
	writer, err := os.OpenFile(gen.BuildDeployPath(data.Path), os.O_CREATE|os.O_TRUNC|os.O_RDWR, os.FileMode(ZNG_DEFAULT_FILE_PERM))
	if err != nil {
		return
	}
	defer writer.Close()
	return gen.Layout.Execute(writer, data)
}

type ZingyData struct {
	Body   thtml.HTML
	Title  string
	Path   string
	Site   ZingySiteData
	Page   map[interface{}]interface{}
	config ConfigSection
}

type ZingySiteData struct {
	BaseURL string
	Image   string
}

func (zd *ZingyData) URL() string {
	return fmt.Sprintf("%s%s", zd.Site.BaseURL, zd.Path)
}

func (zd *ZingyData) Extra(path string) (value string, err error) {
	path = pathes.Clean(path)
	if pathes.IsAbs(path) {
		path = path[1:]
	}
	steps := strings.Split(path, "/")
	last := len(steps) - 1
	key, steps := steps[last], steps[:last]
	section := zd.config
	for _, step := range steps {
		section = section.GetSection(step)
		if section == nil {
			err = errors.New("not found")
			return
		}
	}
	value = section.GetString(key)
	return
}

func NewZingyData(path string, config ConfigSection) (data ZingyData) {
	if strings.HasSuffix(path, ".md") {
		path = strings.Replace(path, ".md", ".html", -1)
	}
	data.Path = fmt.Sprintf("/%s", path)
	data.config = config
	data.Site.BaseURL = config.GetSection("site").GetString("baseurl")
	data.Site.Image = config.GetSection("site").GetString("image")
	return
}

func init() {
	cmdGenerate.Run = func() {
		var (
			gen Generator
			err error
		)
		if gen.Config, err = NewConfig(); err != nil {
			panic(err)
		}
		if gen.Layout, err = thtml.ParseFiles(gen.Config.GetZString("layout")); err != nil {
			panic(err)
		}
		deployPath := gen.GetDeployPath()
		if _, err := os.Stat(deployPath); err == nil {
			if err = os.RemoveAll(deployPath); err != nil {
				panic(err)
			}
		}
		if err = os.Mkdir(deployPath, os.FileMode(ZNG_DEFAULT_DIR_PERM)); err != nil {
			panic(err)
		}
		walk := func(path string, info os.FileInfo, err error) error {
			return gen.walk(path, info, err)
		}
		if err := filepath.Walk(".", walk); err != nil {
			panic(err)
		}
	}
	cmdGenerate.Init()
}

func (gen *Generator) walk(path string, info os.FileInfo, err error) (ierr error) {
	if strings.HasPrefix(path, ".") {
		return nil
	}
	switch {
	case info.IsDir():
		ierr = os.Mkdir(gen.BuildDeployPath(path), os.FileMode(ZNG_DEFAULT_DIR_PERM))
	case strings.HasSuffix(path, ".md"):
		ierr = gen.renderMarkdown(path)
	case strings.HasSuffix(path, ".html"):
		ierr = gen.renderHTML(path)
	default:
		ierr = gen.copy(gen.BuildDeployPath(path), path)
	}
	return
}

func (gen *Generator) renderMarkdown(path string) (err error) {
	input, err := ioutil.ReadFile(path)
	if err != nil {
		return
	}
	md := markdown.MarkdownCommon(input)
	return gen.render(path, md)
}

func (gen *Generator) renderHTML(path string) (err error) {
	input, err := ioutil.ReadFile(path)
	if err != nil {
		return
	}
	return gen.render(path, input)
}

func (gen *Generator) render(path string, input []byte) (err error) {
	template, err := ttext.New("current").Parse(string(input))
	if err != nil {
		return
	}
	var processed bytes.Buffer
	data := NewZingyData(path, gen.Config)
	if err = template.Execute(&processed, &data); err != nil {
		return
	}
	doc, err := gokogiri.ParseHtml(processed.Bytes())
	if err != nil {
		return
	}
	defer doc.Free()
	if err = gen.handleEmbedTags(doc); err != nil {
		return
	}
	gen.cleanUnnecessaryPTags(doc)
	data.Page, err = gen.extractPageConfig(doc)
	if err != nil {
		fmt.Println(path, "=>", err)
		err = nil
	}
	data.Title = gen.getTitle(doc)
	body, err := doc.Search("//body")
	if err != nil {
		return
	}
	data.Body = thtml.HTML(body[0].InnerHtml())
	return gen.Generate(path, &data)
}

func (gen *Generator) cleanUnnecessaryPTags(doc *html.HtmlDocument) (err error) {
	ps, err := doc.Search("//p")
	if err != nil {
		return
	}
	for _, p := range ps {
		hasText := false
		child := p.FirstChild()
		for child != nil {
			typ := child.NodeType()
			if typ == xml.XML_TEXT_NODE {
				hasText = true
				break
			}
			child = child.NextSibling()
		}
		if !hasText {
			parent := p.Parent()
			child = p.FirstChild()
			for child != nil {
				parent.AddChild(child)
				child = child.NextSibling()
			}
			p.Remove()
		}
	}
	return
}

func (gen *Generator) getTitle(doc *html.HtmlDocument) (title string) {
	result, _ := doc.Search("//h1")
	if len(result) > 0 {
		title = result[0].FirstChild().Content()
	}
	return
}

func (gen *Generator) extractPageConfig(doc *html.HtmlDocument) (config map[interface{}]interface{}, err error) {
	result, _ := doc.Search("//comment()")
	if len(result) > 0 {
		err = yaml.Unmarshal([]byte(result[0].Content()), &config)
	}
	return
}

func (gen *Generator) copy(dstPath string, srcPath string) (err error) {
	src, err := os.Open(srcPath)
	if err != nil {
		return
	}
	defer src.Close()
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, os.FileMode(ZNG_DEFAULT_FILE_PERM))
	if err != nil {
		return
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return
}

func (gen *Generator) Markdown(e xml.Node, doc *html.HtmlDocument) (err error) {
	src := e.Attribute("src").Value()
	mdInput, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}
	md := markdown.MarkdownCommon(mdInput)
	mdDoc, err := gokogiri.ParseHtml(md)
	if err != nil {
		return err
	}
	partial, err := mdDoc.Search("//body")
	if err != nil {
		return err
	}
	parent := e.Parent()
	child := partial[0].FirstChild()
	for child != nil {
		parent.AddChild(child)
		child = child.NextSibling()
	}
	e.Remove()
	return
}

func (gen *Generator) handleEmbedTags(doc *html.HtmlDocument) (err error) {
	result, err := doc.Search("//embed")
	if err != nil {
		return
	}
	for _, e := range result {
		plugin := gen.resolveMIMETypePlugin(e.Attribute("type").Value())
		method := reflect.ValueOf(gen).MethodByName(strings.Title(plugin))
		if method == reflect.ValueOf(nil) {
			err = gen.handleMIMETypePlugin(e, doc)
		} else {
			args := make([]reflect.Value, 2)
			args[0] = reflect.ValueOf(e)
			args[1] = reflect.ValueOf(doc)
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

func (gen *Generator) handleMIMETypePlugin(e xml.Node, doc *html.HtmlDocument) (err error) {
	src := e.Attribute("src").Value()
	typ := e.Attribute("type").Value()
	cmd := exec.Command(fmt.Sprintf("m%s%s", ZNG_PREFIX, gen.resolveMIMETypePlugin(typ)), src)
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
	parent := e.Parent()
	child, err := doc.Coerce(be.buffer)
	if err != nil {
		return
	}
	parent.AddChild(child)
	e.Remove()
	return
}

func (gen *Generator) resolveMIMETypePlugin(typ string) string {
	return gen.Config.GetSection("mimetypes").GetString(typ)
}

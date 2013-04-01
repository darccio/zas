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
	"github.com/moovweb/gokogiri"
	"github.com/moovweb/gokogiri/html"
	markdown "github.com/russross/blackfriday"
	"html/template"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

var cmdGenerate = &Subcommand{
	UsageLine: "generate",
}

type Generator struct {
	Config ConfigSection
	Layout *template.Template
}

func (gen *Generator) GetDeployPath() string {
	return gen.Config.GetZString("deploy")
}

func (gen *Generator) BuildDeployPath(path string) string {
	return filepath.Join(gen.GetDeployPath(), path)
}

func (gen *Generator) Generate(path string, data *ZingyData) {
	writer, err := os.OpenFile(gen.BuildDeployPath(strings.Replace(path, ".md", ".html", -1)), os.O_CREATE|os.O_TRUNC|os.O_RDWR, os.FileMode(ZNG_DEFAULT_FILE_PERM))
	if err != nil {
		return
	}
	defer writer.Close()
	gen.Layout.Execute(writer, struct{ Zingy ZingyData }{*data})
}

type ZingyData struct {
	Body  template.HTML
	Title string
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
		if gen.Layout, err = template.ParseFiles(gen.Config.GetZString("layout")); err != nil {
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
		ierr = gen.render(path)
	case strings.HasSuffix(path, ".html"):
		ierr = gen.renderHTML(path)
	default:
		ierr = gen.copy(gen.BuildDeployPath(path), path)
	}
	return
}

func (gen *Generator) render(path string) (err error) {
	input, err := ioutil.ReadFile(path)
	if err != nil {
		return
	}
	body := markdown.MarkdownCommon(input)
	doc, err := gokogiri.ParseHtml(body)
	if err != nil {
		return
	}
	defer doc.Free()
	var data ZingyData
	data.Body = template.HTML(body)
	data.Title = gen.getTitle(doc)
	gen.Generate(path, &data)
	return nil
}

func (gen *Generator) renderHTML(path string) (err error) {
	input, err := ioutil.ReadFile(path)
	if err != nil {
		return
	}
	doc, err := gokogiri.ParseHtml(input)
	if err != nil {
		return
	}
	defer doc.Free()
	result, _ := doc.Search("//embed")
	for _, e := range result {
		if e.Attribute("type").Value() != "text/markdown" {
			continue
		}
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
		parent := e.Parent()
		partial, _ := mdDoc.Search("//body")
		child, _ := doc.Coerce(partial[0].InnerHtml())
		parent.AddChild(child)
		e.Remove()
	}
	var data ZingyData
	body, _ := doc.Search("//body")
	data.Body = template.HTML(body[0].InnerHtml())
	data.Title = gen.getTitle(doc)
	gen.Generate(path, &data)
	return nil
}

func (gen *Generator) getTitle(doc *html.HtmlDocument) (title string) {
	result, _ := doc.Search("//h1")
	if len(result) > 0 {
		title = result[0].FirstChild().Content()
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

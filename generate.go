package main

import (
	"fmt"
	"github.com/moovweb/gokogiri"
	"html/template"
	"io"
	"io/ioutil"
	markdown "github.com/russross/blackfriday"
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

type ZingyData struct {
	Body template.HTML
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
		if ierr = os.Mkdir(gen.BuildDeployPath(path), os.FileMode(ZNG_DEFAULT_DIR_PERM)); ierr != nil {
			return
		}
	case strings.HasSuffix(path, ".md"):
		if ierr = gen.render(path); ierr != nil {
			return
		}
	case strings.HasSuffix(path, ".html"):
		fmt.Println("html")
	default:
		if ierr = gen.copy(gen.BuildDeployPath(path), path); ierr != nil {
			return
		}
	}
	return nil
}

func (gen *Generator) render(path string) (err error) {
	input, err := ioutil.ReadFile(path)
	if err != nil {
		return
	}
	var data ZingyData
	body := markdown.MarkdownCommon(input)
	doc, err := gokogiri.ParseHtml(body)
	if err != nil {
		return
	}
	defer doc.Free()
	data.Body = template.HTML(body)
	result, _ := doc.Search("//h1")
	data.Title = "undefined"
	if len(result) > 0 {
		data.Title = result[0].FirstChild().Content()
	}
	dst, err := os.OpenFile(gen.BuildDeployPath(strings.Replace(path, ".md", ".html", -1)), os.O_CREATE|os.O_TRUNC|os.O_RDWR, os.FileMode(ZNG_DEFAULT_FILE_PERM))
	if err != nil {
		return
	}
	defer dst.Close()
	gen.Layout.Execute(dst, struct { Zingy ZingyData }{ data, })
	return nil
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

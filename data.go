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
	"errors"
	"fmt"
	thtml "html/template"
	"path"
	"strings"
	"github.com/melvinmt/gt"
)

/*
 * Context data store used in templates.
 */
type ZasData struct {
	// Template used as body from current file.
	Body thtml.HTML
	// Current path (usable in URLs).
	Path string
	// Title from first level header (H1).
	FirstTitle string
	// Site configuration, as found in ZAS_CONF_FILE.
	Site ZasSiteData
	// In-page configuration, from first HTML comment (expected as YAML map).
	Page map[interface{}]interface{}
	// Current directory configuration, from ZAS_DIR_CONF_FILE.
	Directory ConfigSection
	// Config loaded from ZAS_CONF_FILE.
	config ConfigSection
	// i18n helper
	i18n *gt.Build
}

/*
 * Site configuration.
 *
 * They are required fields in order to complete social/semantic meta tags.
 */
type ZasSiteData struct {
	BaseURL string
	Image   string
}

/*
 * Current title, from page's config and first level header (H1), in this order.
 */
func (zd *ZasData) Title() (title string) {
	title, ok := zd.Page["title"].(string)
	if !ok {
		title = zd.FirstTitle
	}
	return
}

/*
 * Builds URL from current configuration.
 */
func (zd *ZasData) URL() string {
	return fmt.Sprintf("%s%s", zd.Site.BaseURL, zd.Path)
}

/*
 * Helper template method to get any value from ZasData.config using pathes.
 */
func (zd *ZasData) Extra(keypath string) (value string, err error) {
	keypath = path.Clean(keypath)
	if path.IsAbs(keypath) {
		keypath = keypath[1:]
	}
	steps := strings.Split(keypath, "/")
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

func (zd *ZasData) Language() (language string) {
	return zd.Resolve("language")
}

func (zd *ZasData) Resolve(id string) string {
	var (
		value interface{}
		ok bool
	)
	value, ok = zd.Page[id]
	if !ok {
		if zd.Directory != nil {
			value, ok = zd.Directory[id]
		}
		if !ok {
			value, _ = zd.Extra(fmt.Sprintf("/site/%s", id))
		}
	}
	return value.(string)
}

func (zd *ZasData) E(s string, a ...interface{}) (t string) {
	var err error
	target := zd.Language()
	if (zd.config.GetSection("site").GetString("language") == target) {
		return s
	}
	zd.i18n.SetTarget(target)
	if len(a) == 0 {
		fmt.Println(s)
		t, err = zd.i18n.Translate(s)
	} else {
		t, err = zd.i18n.Translate(s, a)
	}
	if err != nil {
		t = "**" + s + "**"
		err = nil
	}
	return
}

func (zd *ZasData) IsHome() bool {
	return zd.Path == "/index.html" || zd.Path == fmt.Sprintf("/%s/index.html", zd.Language())
}

func NewZasData(filepath string, gen *Generator) (data ZasData) {
	// Any path must finish in ".html".
	if strings.HasSuffix(filepath, ".md") {
		filepath = strings.Replace(filepath, ".md", ".html", -1)
	}
	data.Path = fmt.Sprintf("/%s", filepath)
	data.config = gen.Config
	data.i18n = gen.I18n
	data.Site.BaseURL = gen.Config.GetSection("site").GetString("baseurl")
	data.Site.Image = gen.Config.GetSection("site").GetString("image")
	return
}

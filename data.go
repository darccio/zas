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
	"errors"
	"fmt"
	thtml "html/template"
	"path"
	"strings"

	"github.com/melvinmt/gt"
)

// ZasData is the context data store used in templates.
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
	// Attributes from the source page's own <body> tag (if any), merged
	// onto the layout's <body> element since Body only carries the source
	// body's inner HTML, not the element itself.
	bodyAttrs map[string]string
	// Tracks embed nesting depth for this render, guarding against a self-
	// or mutually-embedding file recursing without bound.
	embedDepth int
}

// ZasSiteData is the site configuration.
//
// They are required fields in order to complete social/semantic meta tags.
type ZasSiteData struct {
	BaseURL string
	Image   string
}

// Title returns the current title, from page's config and first level
// header (H1), in this order.
func (zd *ZasData) Title() (title string) {
	title, ok := zd.Page["title"].(string)
	if !ok {
		title = zd.FirstTitle
	}
	return
}

// URL builds the URL from current configuration.
func (zd *ZasData) URL() string {
	return fmt.Sprintf("%s%s", zd.Site.BaseURL, zd.Path)
}

// Extra is a helper template method to get any value from ZasData.config
// using paths.
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

// Language returns the page's resolved language.
func (zd *ZasData) Language() (string, error) {
	return zd.Resolve("language")
}

// Resolve resolves id from Page, then Directory, then site-wide Extra
// config, in that order. It returns an error only when id is present in
// Page or Directory but isn't a string (e.g. a page-config typo like
// "language:" with no value, or a numeric value) — a genuinely absent key
// still falls back to Extra's own lenient "" default.
func (zd *ZasData) Resolve(id string) (string, error) {
	var (
		value interface{}
		ok    bool
	)
	value, ok = zd.Page[id]
	if !ok {
		if zd.Directory != nil {
			value, ok = zd.Directory[id]
		}
		if !ok {
			s, _ := zd.Extra("/site/" + id)
			return s, nil
		}
	}
	s, isString := value.(string)
	if !isString {
		return "", fmt.Errorf("config value %q must be a string, got %T", id, value)
	}
	return s, nil
}

// E translates s for the page's resolved language, falling back to
// "**s**" when no translation is found.
func (zd *ZasData) E(s string, a ...interface{}) (t string, err error) {
	lang, err := zd.Language()
	if err != nil {
		return "", err
	}
	zd.i18n.SetTarget(lang)
	t, err = zd.i18n.Translate(s, a...)
	if err != nil {
		t = "**" + s + "**"
		err = nil
	}
	return
}

// H is like E but returns the translation as trusted HTML.
func (zd *ZasData) H(s string, a ...interface{}) (h thtml.HTML, err error) {
	t, err := zd.E(s, a...)
	return thtml.HTML(t), err
}

// IsHome reports whether the current page is the site's home page.
func (zd *ZasData) IsHome() (bool, error) {
	lang, err := zd.Language()
	if err != nil {
		return false, err
	}
	return zd.Path == "/index.html" || zd.Path == fmt.Sprintf("/%s/index.html", lang), nil
}

// NewZasData builds a ZasData for the page at filepath.
func NewZasData(filepath string, gen *Generator) (data ZasData) {
	// Any path must finish in ".html".
	filepath = swapExtension(filepath, ".md", ".html")
	data.Path = "/" + filepath
	data.config = gen.Config
	// Each ZasData gets its own gt.Build sharing the (read-only, post-init)
	// Index, so per-render SetTarget/Translate calls don't race or bleed
	// across languages on a Build shared by every render goroutine.
	data.i18n = &gt.Build{
		Index:  gen.I18n.Index,
		Origin: gen.I18n.Origin,
	}
	data.Site.BaseURL = gen.Config.GetSection("site").GetString("baseurl")
	data.Site.Image = gen.Config.GetSection("site").GetString("image")
	return
}

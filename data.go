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
	"errors"
	"fmt"
	thtml "html/template"
	"path"
	"strings"
)

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

func (zd *ZingyData) Extra(keypath string) (value string, err error) {
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

func NewZingyData(filepath string, config ConfigSection) (data ZingyData) {
	if strings.HasSuffix(filepath, ".md") {
		filepath = strings.Replace(filepath, ".md", ".html", -1)
	}
	data.Path = fmt.Sprintf("/%s", filepath)
	data.config = config
	data.Site.BaseURL = config.GetSection("site").GetString("baseurl")
	data.Site.Image = config.GetSection("site").GetString("image")
	return
}

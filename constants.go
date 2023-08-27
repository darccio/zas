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
	"fmt"
	"path/filepath"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var ZAS = "zas"
var ZAS_PREFIX = "zs"
var ZAS_NAME = cases.Title(language.English).String(ZAS)
var ZAS_DIR = fmt.Sprintf(".%s", ZAS)
var ZAS_CONF_FILE = filepath.Join(ZAS_DIR, "config.yml")
var ZAS_I18N_FILE = filepath.Join(ZAS_DIR, "i18n.yml")
var ZAS_DIR_CONF_FILE = fmt.Sprintf(".%s.yml", ZAS)
var ZAS_DEFAULT_DIR_PERM = 0755
var ZAS_DEFAULT_FILE_PERM = 0644
var ZAS_DEFAULT_CONF = ConfigSection{
	ZAS: ConfigSection{
		"layout": filepath.Join(ZAS_DIR, "layout.html"),
		"deploy": filepath.Join(ZAS_DIR, "deploy"),
	},
	"site": ConfigSection{
		"baseurl":  "http://example.com",
		"language": "en",
	},
	"mimetypes": ConfigSection{
		"text/markdown": "markdown",
		"text/plain":    "plain",
		"text/html":     "html",
	},
}

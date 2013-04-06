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
	"fmt"
	"path/filepath"
	"strings"
)

var ZNG = "zingy"
var ZNG_PREFIX = "zg"
var ZNG_NAME = strings.Title(ZNG)
var ZNG_DIR = fmt.Sprintf(".%s", ZNG)
var ZNG_CONF_FILE = filepath.Join(ZNG_DIR, "config.yml")
var ZNG_DEFAULT_DIR_PERM = 0755
var ZNG_DEFAULT_FILE_PERM = 0644
var ZNG_DEFAULT_CONF = ConfigSection{
	ZNG: ConfigSection {
		"layout": filepath.Join(ZNG_DIR, "layout.html"),
		"deploy": filepath.Join(ZNG_DIR, "deploy"),
	},
	"site": ConfigSection {
		"baseurl": "http://example.com",
	},
	"mimetypes": ConfigSection {
		"text/markdown": "markdown",
	},
}

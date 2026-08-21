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
	"os"
	"path/filepath"

	"dario.cat/mergo"
)

// Name is the application/binary name.
const Name = "zas"

// PluginPrefix is prepended to a mimetype's Title-cased name to build the
// external plugin command dispatched for it (see generate.go), e.g. the
// "markdown" mimetype's "Markdown" title becomes the "zsMarkdown" command.
const PluginPrefix = "zs"

// DisplayName is the Title-cased, human-readable form of Name, as printed in
// the CLI's help and version output. It is a hardcoded literal rather than
// computed from Name via cases.Title at init time: Name never changes, so
// title-casing it can never produce anything other than "Zas", and hardcoding
// it drops this file's runtime dependency on golang.org/x/text/cases and
// golang.org/x/text/language.
const DisplayName = "Zas"

// Dir is the name of the per-site zas directory (".zas") holding a site's
// config, i18n strings, layout template and deploy output.
const Dir = "." + Name

// DirConfigFile is the name of the optional per-directory override file
// (".zas.yml") zas looks for while walking a site's directory tree.
const DirConfigFile = "." + Name + ".yml"

// DefaultDirPerm and DefaultFilePerm are the permissions zas uses when
// creating directories and files of its own: the .zas directory, deploy
// output, generated pages, and the scaffolded config.yml.
const (
	DefaultDirPerm  os.FileMode = 0o755
	DefaultFilePerm os.FileMode = 0o644
)

// ConfigFile and I18nFile are a site's config and i18n file paths, relative
// to its root. They can't be Go constants because filepath.Join isn't a
// constant expression - but building them with filepath.Join, rather than
// hand-concatenating with "/", is what keeps them correct on Windows, where
// the OS path separator is "\\".
var (
	ConfigFile = filepath.Join(Dir, "config.yml")
	I18nFile   = filepath.Join(Dir, "i18n.yml")
)

// defaultConfig is the built-in configuration merged into every site's own
// config.yml (see NewConfig in init.go). It is unexported, with DefaultConfig
// as the only way to read it, so external code can neither reassign it nor -
// since DefaultConfig hands back a deep copy rather than the map itself -
// mutate it (or one of its nested section maps) through the returned value.
var defaultConfig = ConfigSection{
	Name: ConfigSection{
		"layout": filepath.Join(Dir, "layout.html"),
		"deploy": filepath.Join(Dir, "deploy"),
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

// DefaultConfig returns a deep copy of the built-in default configuration,
// safe for callers to read or even mutate without affecting subsequent calls
// or any other code in the process. It reuses the same mergo-based technique
// NewConfig uses in init.go rather than a hand-rolled deep-copy helper:
// pre-seeding a fresh, empty map for every nested section gives mergo.Merge's
// own recursive merge somewhere to copy each value into that nothing else
// references.
func DefaultConfig() ConfigSection {
	dst := ConfigSection{}
	for key, value := range defaultConfig {
		if _, ok := value.(ConfigSection); ok {
			dst[key] = ConfigSection{}
		}
	}
	if err := mergo.Merge(&dst, defaultConfig); err != nil {
		panic("zas: default configuration failed to merge: " + err.Error())
	}
	return dst
}

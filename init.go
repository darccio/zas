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
	"os"
	"path/filepath"

	"dario.cat/mergo"
	"github.com/melvinmt/gt"
	yaml "go.yaml.in/yaml/v3"
)

// ConfigSection is a section of zas configuration: a string-keyed map,
// as produced by decoding a YAML mapping and by every ConfigSection{...}
// literal in this package. go.yaml.in/yaml/v3 special-cases a named map
// type whose keys are strings and whose values are interface{}: once it
// decodes the outermost mapping into a ConfigSection, it keeps using that
// same named type for every nested mapping found inside an interface{}
// value, at any depth (see stringMapType propagation in its decoder) -
// so a config.yml with several levels of nested sections decodes into
// nested ConfigSection values all the way down, with no custom
// UnmarshalYAML method required. (That propagation requires every key in
// the mapping to be a string; GetSection's map[string]interface{}
// fallback below covers a section that arrives some other way.)
type ConfigSection map[string]interface{}

// NewConfig loads ConfigFile (as defined in constants.go).
// It must be a YAML file.
func NewConfig() (ConfigSection, error) {
	data, err := os.ReadFile(ConfigFile)
	if err != nil {
		return nil, err
	}

	var config ConfigSection
	if err = yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	if config == nil {
		config = ConfigSection{}
	}
	// mergo.Merge only recursively merges a nested map's own keys when
	// dst already has a real (even if empty) map value to merge into. If
	// a whole top-level section like "mimetypes" is absent from config,
	// mergo instead falls back to assigning defaults' own map object for
	// that section directly - config and defaults would then share the
	// exact same underlying map, so mutating config's defaulted section
	// (e.g. through GetSection) would corrupt defaults, and any other
	// holder of it, for as long as either is still referenced. Giving
	// every missing section a fresh, empty map first means mergo's own
	// merge does the actual copying, one key at a time, into a map
	// nothing else references - no separate deep-copy logic needed.
	defaults := DefaultConfig()
	for key, value := range defaults {
		if _, ok := value.(ConfigSection); !ok {
			continue
		}
		if _, exists := config[key]; !exists {
			config[key] = ConfigSection{}
		}
	}

	if err = mergo.Merge(&config, defaults); err != nil {
		return nil, err
	}

	return config, nil
}

// NewI18n loads I18nFile (as defined in constants.go).
// It must be a YAML file.
func NewI18n(mainlang string) (i18n gt.Strings, err error) {
	data, err := os.ReadFile(I18nFile)
	if err != nil {
		if os.IsNotExist(err) {
			return make(gt.Strings), nil
		}
		return nil, err
	}
	i18n = make(gt.Strings)
	if err = yaml.Unmarshal(data, &i18n); err != nil {
		return nil, err
	}
	for k, v := range i18n {
		if v == nil {
			v = make(map[string]string)
			i18n[k] = v
		}
		if _, ok := v[mainlang]; !ok {
			v[mainlang] = k
		}
	}
	return i18n, nil
}

// GetString returns a string value from current section, or "" if key is
// missing or not a string. Callers that need to tell "absent"/"wrong type"
// apart from a legitimately empty string should use GetStringOK instead.
func (cs ConfigSection) GetString(key string) (value string) {
	value, _ = cs.GetStringOK(key)
	return
}

// GetStringOK returns a string value from current section, and whether key
// was present and held a string value. ok is false both when key is absent
// and when it holds a non-string value; callers that need to tell those two
// cases apart should check key's presence themselves beforehand.
func (cs ConfigSection) GetStringOK(key string) (value string, ok bool) {
	value, ok = cs[key].(string)
	return
}

// GetSection returns a subsection from current section, or nil if key is
// missing or not a section.
func (cs ConfigSection) GetSection(key string) (value ConfigSection) {
	value, _ = cs.GetSectionOK(key)
	return
}

// GetSectionOK returns a subsection from current section, and whether key
// resolved to a real section.
func (cs ConfigSection) GetSectionOK(key string) (value ConfigSection, ok bool) {
	switch raw := cs[key].(type) {
	case ConfigSection:
		value, ok = raw, true
	case map[string]interface{}:
		// A section built by some means other than decoding YAML into a
		// ConfigSection-typed destination (e.g. a caller constructing one
		// by hand as a plain map[string]interface{}) - see
		// TestGetSectionRawYAMLMap.
		value, ok = ConfigSection(raw), true
	}
	return
}

// GetZString returns a string value from default Zas section.
func (cs ConfigSection) GetZString(key string) string {
	s := cs.GetSection(Name)
	return s.GetString(key)
}

// defaultLayout is the minimal html/template document scaffolded as
// LayoutFile by Init.Run: just enough for "zas init && zas" to succeed
// standalone, matching the README's quickstart. Modeled on the smallest
// layouts under testdata (e.g. testdata/walk-error-base/.zas/layout.html),
// it renders each page's title and body and nothing else - it's a
// starting point, not a real design.
const defaultLayout = `<!DOCTYPE html>
<html>
<head><title>{{.Title}}</title></head>
<body>
{{.Body}}
</body>
</html>
`

// Init implements the "init" subcommand, which scaffolds a new Zas
// repository in the current directory.
type Init struct {
	// Force, when true, makes Run overwrite an existing ConfigFile or
	// LayoutFile with its scaffolded default. When false (the default),
	// Run leaves either file alone if it already exists, so re-running
	// "zas init" on a site never silently discards hand-edited content.
	Force bool
}

// Run scaffolds Dir, and writes a default ConfigFile and LayoutFile.
// Without Force set, an existing ConfigFile or LayoutFile is left
// untouched (Run says so rather than staying silent about it); with
// Force set, both are overwritten unconditionally.
func (i *Init) Run() error {
	path, err := filepath.Abs(Dir)
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(Dir); os.IsNotExist(statErr) {
		if err := os.Mkdir(Dir, DefaultDirPerm); err != nil {
			return err
		}
		fmt.Printf("Initialized empty %s repository in %s\n", DisplayName, path)
	} else {
		fmt.Printf("Reinitialized existing %s repository in %s\n", DisplayName, path)
	}
	config, err := yaml.Marshal(DefaultConfig())
	if err != nil {
		return err
	}
	if err := i.writeScaffold(ConfigFile, config); err != nil {
		return err
	}
	return i.writeScaffold(LayoutFile, []byte(defaultLayout))
}

// writeScaffold writes data to path, creating or overwriting it, unless
// path already exists and Force is false - in which case it leaves the
// existing file untouched and prints that it did, instead of silently
// discarding whatever is there.
func (i *Init) writeScaffold(path string, data []byte) error {
	if !i.Force {
		switch _, statErr := os.Stat(path); {
		case statErr == nil:
			fmt.Printf("%s already exists, not overwriting (use -force to overwrite)\n", path)
			return nil
		case !os.IsNotExist(statErr):
			return statErr
		}
	}
	return os.WriteFile(path, data, DefaultFilePerm)
}

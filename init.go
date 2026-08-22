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
	"github.com/darccio/zas/internal/i18n"
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
func NewI18n(mainlang string) (strs i18n.Strings, err error) {
	data, err := os.ReadFile(I18nFile)
	if err != nil {
		if os.IsNotExist(err) {
			return make(i18n.Strings), nil
		}
		return nil, err
	}
	strs = make(i18n.Strings)
	if err = yaml.Unmarshal(data, &strs); err != nil {
		return nil, err
	}
	for k, v := range strs {
		if v == nil {
			v = make(map[string]string)
			strs[k] = v
		}
		if _, ok := v[mainlang]; !ok {
			v[mainlang] = k
		}
	}
	return strs, nil
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

// Init implements the "init" subcommand, which scaffolds a new Zas
// repository in the current directory.
type Init struct{}

// Run scaffolds Dir and writes a default ConfigFile, overwriting
// any existing one.
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
	// Stores DefaultConfig() as ConfigFile (as defined in
	// constants.go). Overwrites every time we invoke the init subcommand.
	data, err := yaml.Marshal(DefaultConfig())
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigFile, data, DefaultFilePerm)
}

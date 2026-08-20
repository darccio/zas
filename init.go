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
	yaml "gopkg.in/yaml.v2"
)

// ConfigSection aliases goyaml's default map type.
type ConfigSection map[interface{}]interface{}

// NewConfig loads ZAS_CONF_FILE (as defined in constants.go).
// It must be a YAML file.
func NewConfig() (ConfigSection, error) {
	data, err := os.ReadFile(ZAS_CONF_FILE)
	if err != nil {
		return nil, err
	}

	var config ConfigSection
	if err = yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	if err = mergo.Merge(&config, ZAS_DEFAULT_CONF); err != nil {
		return nil, err
	}

	return config, nil
}

// NewI18n loads ZAS_I18N_FILE (as defined in constants.go).
// It must be a YAML file.
func NewI18n(mainlang string) (i18n gt.Strings, err error) {
	data, err := os.ReadFile(ZAS_I18N_FILE)
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

// GetString returns a string value from current section.
func (cs ConfigSection) GetString(key string) (value string) {
	value, _ = cs[key].(string)
	return
}

// GetSection returns a subsection from current section, or nil if key is
// missing or not a section.
func (cs ConfigSection) GetSection(key string) (value ConfigSection) {
	switch raw := cs[key].(type) {
	case ConfigSection:
		value = raw
	case map[interface{}]interface{}:
		value = ConfigSection(raw)
	}
	return
}

// GetZString returns a string value from default Zas section.
func (cs ConfigSection) GetZString(key string) string {
	s := cs.GetSection(ZAS)
	return s.GetString(key)
}

// Init implements the "init" subcommand, which scaffolds a new Zas
// repository in the current directory.
type Init struct{}

// Run scaffolds ZAS_DIR and writes a default ZAS_CONF_FILE, overwriting
// any existing one.
func (i *Init) Run() error {
	path, err := filepath.Abs(ZAS_DIR)
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(ZAS_DIR); os.IsNotExist(statErr) {
		if err := os.Mkdir(ZAS_DIR, os.FileMode(ZAS_DEFAULT_DIR_PERM)); err != nil {
			return err
		}
		fmt.Printf("Initialized empty %s repository in %s\n", ZAS_NAME, path)
	} else {
		fmt.Printf("Reinitialized existing %s repository in %s\n", ZAS_NAME, path)
	}
	// Stores ZAS_DEFAULT_CONF as ZAS_CONF_FILE (as defined in
	// constants.go). Overwrites every time we invoke the init subcommand.
	data, err := yaml.Marshal(&ZAS_DEFAULT_CONF)
	if err != nil {
		return err
	}
	return os.WriteFile(ZAS_CONF_FILE, data, os.FileMode(ZAS_DEFAULT_FILE_PERM))
}

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
	"io/ioutil"
	yaml "launchpad.net/goyaml"
	"os"
	"path/filepath"
)

var cmdInit = &Subcommand{
	UsageLine: "init",
}

/*
 * Aliasing goyaml's default map type.
 */
type ConfigSection map[interface{}]interface{}

/*
 * Loads ZNG_CONF_FILE (as defined in constants.go).
 * It must be a YAML file.
 */
func NewConfig() (config ConfigSection, err error) {
	data, err := ioutil.ReadFile(ZNG_CONF_FILE)
	if err != nil {
		return
	}
	config = make(ConfigSection)
	err = yaml.Unmarshal(data, &config)
	return
}

/*
 * Returns a string value from current section.
 */
func (cs ConfigSection) GetString(key string) (value string) {
	raw := cs[key]
	if raw == nil {
		value = ""
	} else {
		value = raw.(string)
	}
	return
}

/*
 * Returns a subsection from current section.
 */
func (cs ConfigSection) GetSection(key string) ConfigSection {
	value := cs[key].(map[interface{}]interface{})
	return ConfigSection(value)
}

/*
 * Returns a string value from default zingy section.
 */
func (cs ConfigSection) GetZString(key string) string {
	return cs.GetSection(ZNG).GetString(key)
}

func init() {
	cmdInit.Run = func() {
		path, _ := filepath.Abs(ZNG_DIR)
		if _, err := os.Stat(ZNG_DIR); os.IsNotExist(err) {
			os.Mkdir(ZNG_DIR, os.FileMode(ZNG_DEFAULT_DIR_PERM))
			fmt.Printf("Initialized empty %s repository in %s\n", ZNG_NAME, path)
		} else {
			fmt.Printf("Reinitialized existing %s repository in %s\n", ZNG_NAME, path)
		}
		var (
			data []byte
			err  error
		)
		// If default config variable has fields, we store it as ZNG_CONF_FILE (as defined in constants.go).
		// It overwrites every time we invoke init subcommand.
		if len(ZNG_DEFAULT_CONF) > 0 {
			if data, err = yaml.Marshal(&ZNG_DEFAULT_CONF); err != nil {
				panic(err)
			}
		}
		if err := ioutil.WriteFile(ZNG_CONF_FILE, data, os.FileMode(ZNG_DEFAULT_FILE_PERM)); err != nil {
			panic(err)
		}
	}
	cmdInit.Init()
}

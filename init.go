package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	yaml "launchpad.net/goyaml"
)

var cmdInit = &Subcommand{
	UsageLine: "init",
}

type ConfigSection map[interface{}]interface{}

func NewConfig() (config ConfigSection, err error) {
	data, err := ioutil.ReadFile(ZNG_CONF_FILE)
	if err != nil {
		return
	}
	config = make(ConfigSection)
	err = yaml.Unmarshal(data, &config)
	return
}

func (cs ConfigSection) GetString(key string) string {
	return cs[key].(string)
}

func (cs ConfigSection) GetSection(key string) ConfigSection {
	value := cs[key].(map[interface{}]interface{})
	return ConfigSection(value)
}

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
			err error
		)
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

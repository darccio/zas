package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

var ZNG = "zingy"
var ZNG_NAME = strings.Title(ZNG)
var ZNG_DIR = fmt.Sprintf(".%s", ZNG)
var ZNG_CONF_FILE = filepath.Join(ZNG_DIR, "config.yml")
var ZNG_DEFAULT_DIR_PERM = 0755
var ZNG_DEFAULT_FILE_PERM = 0644
var ZNG_DEFAULT_CONF = ConfigSection {
	ZNG: ConfigSection {
		"layout": filepath.Join(ZNG_DIR, "layout.html"),
		"deploy": filepath.Join(ZNG_DIR, "deploy"),
	},
}

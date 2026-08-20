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
	"bytes"
	thtml "html/template"
	"os"
	"strings"
	"testing"

	"github.com/melvinmt/gt"
)

// helpers used to register its own "eq", shadowing html/template's builtin
// (present since Go 1.6) with a plain a == b comparison: no numeric-kind
// unification (e.g. int(5) vs int64(5) never matched, even though both are
// integers of the same value) and a raw panic - recovered by the template
// engine into an execution error - on uncomparable dynamic types like
// slices or maps. Removing the registration lets the builtin apply in the
// layout, the only place "helpers" is ever used (pages render via plain
// text/template with no Funcs call, so they already got the builtin
// regardless).

func TestLayoutEqUsesBuiltinNumericUnification(t *testing.T) {
	// A template integer literal like 5 evaluates as a plain Go int when
	// passed to an interface{} parameter - confirmed directly, not
	// assumed - so an explicitly int64-typed value is used here to force a
	// genuine cross-type comparison within the same numeric kind. A bare
	// a == b comparison never matches two different concrete Go types
	// (interface{}(int64(5)) != interface{}(int(5))), but the builtin eq's
	// int-kind unification does.
	tmpl := thtml.Must(thtml.New("t").Funcs(helpers).Parse(
		`{{if eq . 5}}match{{else}}nomatch{{end}}`))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, int64(5)); err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}
	got := buf.String()
	if !strings.Contains(got, "match") || strings.Contains(got, "nomatch") {
		t.Fatalf("output = %q, want \"match\" (builtin eq unifies int64(5) and a template literal 5), not \"nomatch\"", got)
	}
}

func TestLayoutEqOnUncomparableTypesReturnsCleanError(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("deploy", 0o755); err != nil {
		t.Fatal(err)
	}
	gen := &Generator{
		Config: ConfigSection{"zas": ConfigSection{"deploy": "deploy"}},
		I18n:   &gt.Build{Index: gt.Strings{}, Origin: "en"},
	}
	// eq on two slices is still an error either way (Execute aborts on
	// error same as on a recovered panic), but the builtin rejects
	// uncomparable types with its own error instead of attempting a raw ==
	// and panicking - so the message should read as a normal template
	// error, not a recovered Go runtime panic. Two YAML flow-sequence
	// page-config values (decoded as []interface{}, uncomparable via ==)
	// reproduce this exactly as the finding itself does.
	layout, err := thtml.New("layout").Funcs(helpers).Parse(
		`{{if eq (index .Page "list") (index .Page "list2")}}match{{end}}`)
	if err != nil {
		t.Fatal(err)
	}
	gen.Layout = layout

	src := "<!--\nlist: [1, 2]\nlist2: [1, 2]\n-->\n<body>content</body>"
	err = gen.render("page.html", []byte(src))
	if err == nil {
		t.Fatal("render() error = nil, want a non-nil error comparing two uncomparable slices")
	}
	if strings.Contains(err.Error(), "runtime error") {
		t.Fatalf("render() error = %q, want the builtin's own uncomparable-type error, not a recovered Go runtime panic", err.Error())
	}
}

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
	"strings"
	"testing"
)

// A page whose source opens with a <script> (or any other head-eligible
// element - meta, link, base, style, title) before any real body content
// used to have it silently vanish: HTML5's own tree construction routes
// such elements into <head> rather than <body>, and render only ever keeps
// doc.Find("body").Html() as data.Body. A leading "<!-- key: value -->"
// config comment doesn't help either - it doesn't force body insertion
// mode. Nothing surfaced this: no error, no warning, no trace. render must
// now fail loudly instead.
func TestRenderRejectsContentLostToHead(t *testing.T) {
	gen := newRenderTestGenerator(t)
	src := "<!-- title: T -->\n<script>var a = 1;</script>\n<p>hi</p>\n"

	err := gen.render("index.html", []byte(src))
	if err == nil {
		t.Fatal("render() error = nil, want an error for content parsed into <head>")
	}
	if !strings.Contains(err.Error(), "script") || !strings.Contains(err.Error(), "<head>") {
		t.Fatalf("render() error = %v, want it to name the lost element and mention <head>", err)
	}

	if _, statErr := os.Stat(filepath.Join("deploy", "index.html")); !os.IsNotExist(statErr) {
		t.Fatalf("deploy output stat error = %v, want no output written for a rejected page", statErr)
	}
}

// A <script> (or other head-eligible element) that comes after real body
// content is parsed as ordinary body content, not routed into <head>, so it
// must render exactly as written - this is the control case for
// TestRenderRejectsContentLostToHead.
func TestRenderKeepsScriptAfterRealBodyContent(t *testing.T) {
	gen := newRenderTestGenerator(t)
	src := "<!-- title: T -->\n<h1>Hi</h1>\n<script>var a = 1;</script>\n"

	if err := gen.render("index.html", []byte(src)); err != nil {
		t.Fatalf("render() error = %v, want nil", err)
	}

	out, err := os.ReadFile(filepath.Join("deploy", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "<script>var a = 1;</script>") {
		t.Fatalf("deploy output = %q, want the script tag preserved", out)
	}
}

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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/PuerkitoBio/goquery"
	html5 "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// scriptPluginType is the <script type="..."> value prefix that marks a
// tag as a build-time plugin invocation: everything after it is the
// plugin name, so type="application/zas+myplugin" runs zsmyplugin.
const scriptPluginType = "application/" + Name + "+"

// dataArgsAttr holds a zas script tag's command-line arguments. It has no
// html/atom entry - atom only covers standard attribute names - so unlike
// type/src it's spelled out as a literal rather than taken from atom.
const dataArgsAttr = "data-args"

/*
 * Handles <script type="application/zas+name"> tags by running the
 * zs<name> plugin (see handleScriptPlugin).
 *
 * Every other <script> - real JavaScript, application/ld+json, a bare
 * <script> with no type at all - is left completely alone: not rewritten,
 * not reparsed, not even touched. A <script> typed inside a fenced code
 * block never reaches here either; goldmark escapes it, so it's a text
 * node, not an element, and Find never sees it (see codeblock_test.go).
 *
 * doc.Find returns a snapshot of the matching nodes, so anything this
 * splices in is never visited by this loop - a plugin cannot recursively
 * trigger itself, and no depth guard is needed (unlike Markdown/Html,
 * which recurse through parseAndReplace and are bounded by
 * maxEmbedDepth). One exception worth knowing: a script tag in a page's
 * own body fires during render's pass, and its output is then re-parsed
 * by Generate's own pass - so output containing another zas script tag
 * runs once more there. A layout-level tag gets no such second chance.
 */
func (gen *Generator) handleScriptTags(doc *goquery.Document, head headFate) (err error) {
	doc.Find(atom.Script.String()).EachWithBreak(func(_ int, e *goquery.Selection) bool {
		typ, ok := e.Attr(atom.Type.String())
		if !ok {
			return true
		}
		name, ok := strings.CutPrefix(strings.ToLower(strings.TrimSpace(typ)), scriptPluginType)
		if !ok {
			return true
		}
		err = gen.handleScriptPlugin(e, typ, name, head)
		return err == nil
	})
	return
}

/*
 * Invokes a zs<name> plugin named by a script tag's type attribute. The
 * tag's data-args attribute supplies argv (see splitArgs), its raw inner
 * text is piped to the plugin's stdin, and its stdout replaces the whole
 * tag as HTML; stderr is passed through to the user's shell.
 *
 * <script> is an HTML5 raw-text element, so the tokenizer never entity-
 * decodes its content: an inline JSON/CSV/DOT/YAML data block reaches the
 * plugin byte for byte, which is the whole point of hanging this on
 * <script> rather than on another, parsed element - that's a deliberate
 * extension beyond a script tag's literal, argument-only README spec.
 *
 * async is deliberately not supported: every tag runs synchronously, in
 * document order. It's accepted as an attribute and simply ignored - a
 * second fan-out layer inside already-concurrent page rendering is
 * exactly where this codebase has had its race and goroutine-leak bugs
 * (see the C1-C6 audit history).
 */
func (gen *Generator) handleScriptPlugin(e *goquery.Selection, typ, name string, head headFate) error {
	if !pluginNameRe.MatchString(name) {
		return fmt.Errorf("no valid plugin named by script type %q", typ)
	}
	if gen.NoPlugins {
		return fmt.Errorf("plugin execution disabled (-no-plugins): script type %q needs plugin %s%s", typ, PluginPrefix, name)
	}
	inHead := e.Closest(atom.Head.String()).Length() > 0
	if inHead && head == headDropped {
		return fmt.Errorf("script type %q was parsed into <head>, whose content is discarded for this page: put the tag after the page's first body content (a leading config comment does not count)", typ)
	}
	args, err := splitArgs(e.AttrOr(dataArgsAttr, ""))
	if err != nil {
		return fmt.Errorf("invalid %s for script type %q: %w", dataArgsAttr, typ, err)
	}
	cmd := exec.Command(PluginPrefix+name, args...)
	cmd.Stdin = strings.NewReader(e.Text())
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("plugin %s%s failed for script type %q: %w", PluginPrefix, name, typ, err)
	}
	// Parse with the tag's own parent as context - the same thing
	// ReplaceWithHtml does internally - but keep the resulting node slice
	// so its length is observable: in <head>, HTML5 only has real
	// insertion-mode cases for a handful of elements (meta, link, base,
	// style, title, script); anything else silently produces zero nodes
	// instead of an error (verified against x/net/html/parse.go: the
	// implied </head> empties the fragment's node stack, and the
	// subsequent implied <body> is appended to the document itself, not
	// to the fragment root, because addChild's insertion point falls back
	// to the document when the stack is empty). That would otherwise be
	// silent, unrecoverable data loss - the exact failure mode this
	// feature must not have.
	nodes, err := html5.ParseFragment(strings.NewReader(string(out)), e.Get(0).Parent)
	if err != nil {
		return fmt.Errorf("plugin %s%s produced unparseable output for script type %q: %w", PluginPrefix, name, typ, err)
	}
	if len(nodes) == 0 && strings.TrimSpace(string(out)) != "" {
		return fmt.Errorf("plugin %s%s produced output that cannot be placed here (script type %q); inside <head> only meta, link, base, style and title are valid", PluginPrefix, name, typ)
	}
	e.ReplaceWithNodes(nodes...)
	return nil
}

// argSeparators is the byte set splitArgs treats as unquoted field
// separators: HTML5's own attribute whitespace plus \v, so a data-args
// value wrapped across source lines splits the way it reads.
const argSeparators = " \t\n\r\f\v"

// splitArgs splits a data-args attribute value into argv using a small
// POSIX-shell-like quoting grammar: unquoted whitespace separates fields,
// 'literal quotes' preserve everything including whitespace and
// backslashes verbatim, "quotes allow \" and \\ escapes", and a bare
// backslash escapes the next byte outside quotes. Quotes are removal-only
// (a"b"c splits to one field, abc); an empty quoted pair - single or
// double - is one empty-string field.
//
// This is hand-rolled rather than a dependency (google/shlex is untagged
// and unmaintained since 2019, and this package just finished getting off
// unmaintained YAML dependencies) or a shell (nothing here is ever handed
// to sh -c - exec.Command runs the plugin directly - so unlike a real
// shell, none of $, `, *, ~, #, |, ;, &, <, > carry any special meaning;
// they're always literal). An unterminated quote or a trailing backslash
// is an error rather than a best-effort guess, matching this package's
// reject-before-exec posture elsewhere (see pluginNameRe): guessing here
// would mean handing a wrong argv to a program that executes.
func splitArgs(s string) ([]string, error) {
	var (
		args    []string
		cur     strings.Builder
		started bool // a field has begun, even if still empty: "" must yield one empty arg
	)
	flush := func() {
		if started {
			args = append(args, cur.String())
			cur.Reset()
			started = false
		}
	}
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case strings.IndexByte(argSeparators, c) >= 0:
			flush()
		case c == '\'':
			started = true
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				return nil, errors.New("unterminated single quote")
			}
			cur.WriteString(s[i+1 : i+1+j])
			i += j + 1
		case c == '"':
			started = true
			i++
			for {
				if i >= len(s) {
					return nil, errors.New("unterminated double quote")
				}
				if s[i] == '"' {
					break
				}
				if s[i] == '\\' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\\') {
					i++
				}
				cur.WriteByte(s[i])
				i++
			}
		case c == '\\':
			if i+1 >= len(s) {
				return nil, errors.New("trailing backslash")
			}
			started = true
			i++
			cur.WriteByte(s[i])
		default:
			started = true
			cur.WriteByte(c)
		}
	}
	flush()
	return args, nil
}

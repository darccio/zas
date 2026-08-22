// Copyright (c) 2013 Melvin Tercan, https://github.com/melvinmt
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of this
// software and associated documentation files (the "Software"), to deal in the Software
// without restriction, including without limitation the rights to use, copy, modify,
// merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit
// persons to whom the Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all copies or
// substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED,
// INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR
// PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE
// FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR
// OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

// Package i18n is a vendored, trimmed copy of github.com/melvinmt/gt v1.0.1
// (unmaintained upstream since 2013). Only the Build/Strings surface zas
// actually uses is kept; gt's own SetOrigin and T helpers were dropped as
// dead code. Behavior is otherwise unchanged from upstream, including its
// key-or-literal-string lookup, %verb argument swapping, and #tag stripping.
package i18n

import (
	"errors"
	"fmt"
	"regexp"
)

// Strings is a map type in the form of map[key]map[language][translation]
type Strings map[string]map[string]string

// Build sets up the translation environment. It must not be shared across
// concurrent Translate/SetTarget calls for different targets: Target is an
// unsynchronized field, and Translate's regex caches are lazily initialized
// without locking.
type Build struct {
	Origin     string         // the origin env
	Target     string         // the target env
	Index      Strings        // the index which contains all keys and strings
	regexVerbs *regexp.Regexp // caching regex Compiles
	regexTags  *regexp.Regexp
}

// SetTarget is a shorthand method to set Target.
func (b *Build) SetTarget(lang string) {
	b.Target = lang
}

// Translate translates a key or string from origin to target.
// Parses augmented sprintf format when additional arguments are given.
func (b *Build) Translate(str string, args ...interface{}) (t string, err error) {
	if b.Origin == "" {
		b.Origin = "xx" // default
	}

	if b.Target == "" {
		b.Target = "xx" // default
	}

	// Try to find origin string by key or key[:2]
	var o string // origin string
	key := str   // key can differ from str

	if val, ok := b.Index[key][b.Origin]; ok {
		o = val
	} else if val, ok := b.Index[key][b.Origin[:2]]; ok {
		o = val
	} else {
		// If key is not found, try matching strings in origin.
		for k, m := range b.Index {
			for l, v := range m {
				if (l == b.Origin || l == b.Origin[:2]) && key == v {
					o, key = v, k
					break
				}
			}
		}
	}

	// Try to find target string by key or key[:2]
	if val, ok := b.Index[key][b.Target]; ok {
		t = val
	} else if val, ok := b.Index[key][b.Target[:2]]; ok {
		t = val
	}

	if o == "" {
		return b.parseString(str, args...), errors.New("couldn't find origin string")
	}

	if t == "" {
		return b.parseString(o, args...), errors.New("couldn't find target string")
	}

	// When no additional arguments are given, there's nothing left to do.
	if len(args) == 0 {
		return b.cleanTags(t), err
	}

	// Find verbs in both strings.
	if b.regexVerbs == nil {
		b.regexVerbs = regexp.MustCompile(`%(?:\d+\$)?[+-]?(?:[ 0]|'.{1})?-?\d*(?:\.\d+)?#?[bcdeEfFgGopqstTuUvxX%]?[#[\w0-9-_]+]?`)
	}
	oVerbs := b.regexVerbs.FindAllStringSubmatch(o, -1)
	tVerbs := b.regexVerbs.FindAllStringSubmatch(t, -1)

	if len(oVerbs) != len(args) || len(tVerbs) != len(args) {
		return b.parseString(o, args...), errors.New("arguments count is different than verbs count")
	}

	// Check if verbs are unique.
	uniqueVerbs := true
	q := make(map[string]int, len(tVerbs))
	for _, v := range tVerbs {
		if len(v) > 0 {
			q[v[0]]++
			if q[v[0]] > 1 {
				uniqueVerbs = false
				break
			}
		}
	}

	// Swap argument positions when swapping is necessary and verbs are unique.
	if uniqueVerbs {
		for i, v1 := range tVerbs {
			for j, v2 := range oVerbs {
				if len(args)-1 == i {
					break
				}
				if v1[0] == v2[0] && args[i] != args[j] {
					args[j], args[i] = args[i], args[j]
					break
				}
			}
		}
	} else if h1, h2 := fmt.Sprintf("%v", oVerbs), fmt.Sprintf("%v", tVerbs); h1 != h2 {
		return b.parseString(o, args...), errors.New("verbs have to be swapped but are not unique")
	}

	return b.parseString(t, args...), err
}

// parseString removes tags and parses arguments.
func (b *Build) parseString(str string, args ...interface{}) (s string) {
	s = b.cleanTags(str)
	// Parse string.
	s = fmt.Sprintf(s, args...)
	return s
}

// cleanTags only removes tags.
func (b *Build) cleanTags(str string) (s string) {
	if b.regexTags == nil {
		b.regexTags = regexp.MustCompile(`#[\w0-9-_]+`)
	}
	s = b.regexTags.ReplaceAllLiteralString(str, "")
	return s
}

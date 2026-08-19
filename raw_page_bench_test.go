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
	"strings"
	"testing"
)

// pageOptsOutOfTemplatingInputs are representative render() inputs:
// pageOptsOutOfTemplating runs once per source file on every render() call,
// so it needs to stay cheap regardless of how big the rest of the page is.
var pageOptsOutOfTemplatingInputs = map[string]string{
	"small_with_comment":  "<!-- title: Hi -->\n<h1>Hi</h1>",
	"large_body_no_match": "<!-- title: Hi -->\n<h1>Hi</h1>" + strings.Repeat("<p>filler paragraph content</p>\n", 2000),
	"no_comment_at_all":   strings.Repeat("<p>filler paragraph content</p>\n", 2000),
}

func BenchmarkPageOptsOutOfTemplating(b *testing.B) {
	for name, input := range pageOptsOutOfTemplatingInputs {
		in := []byte(input)
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				pageOptsOutOfTemplating(in)
			}
		})
	}
}

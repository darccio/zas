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
	"strings"
	"testing"
)

// renderConcurrencyFixtureNPages is large enough (many times over any
// plausible renderConcurrency() cap) that walk keeps every worker slot
// busy for most of the run, instead of finishing before concurrency ever
// builds up.
const renderConcurrencyFixtureNPages = 800

// TestRenderAsyncConcurrencyIsBounded is the regression test for the
// unbounded fan-out: walk used to spawn one renderAsync goroutine per
// source file with no cap at all, so a large site could have thousands of
// goroutines running at once, each holding open file descriptors and doing
// a full HTML5 parse/template pass simultaneously. It builds a fixture with
// hundreds of pages, runs a real Generator.Run, and checks gen.peakActive -
// incremented/decremented around each renderAsync's real work, purely for
// this kind of test - never exceeded renderConcurrency()'s cap. That bound
// is actually structural (sem's capacity limits how many goroutines can
// ever hold a permit at once, and active/peakActive can't exceed permits
// held), but this proves it holds for gen.Run() end to end and that real
// concurrency happens at all (peak > 1), so the test isn't vacuously
// passing because everything happened to run one at a time.
func TestRenderAsyncConcurrencyIsBounded(t *testing.T) {
	newTestSite(t, "walk-error-base")

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := range renderConcurrencyFixtureNPages {
		name := fmt.Sprintf("page-%04d.md", i)
		body := fmt.Sprintf("# Page %d\n\n%s\n", i, strings.Repeat("Lorem ipsum dolor sit amet. ", 40))
		must(os.WriteFile(name, []byte(body), 0o644))
	}

	gen := &Generator{Full: true}
	if err := gen.Run(); err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}

	limit := renderConcurrency()
	peak := gen.peakActive.Load()
	if peak > int64(limit) {
		t.Fatalf("peak concurrent renderAsync goroutines = %d, want <= %d (renderConcurrency cap)", peak, limit)
	}
	if peak <= 1 {
		t.Fatalf("peak concurrent renderAsync goroutines = %d, want > 1 - fixture didn't exercise real concurrency", peak)
	}
	t.Logf("peak concurrent renderAsync goroutines = %d (cap %d)", peak, limit)

	// Every page must still have been rendered - the cap must throttle
	// concurrency, not drop work.
	for i := range renderConcurrencyFixtureNPages {
		assertDeployHas(t, fmt.Sprintf("page-%04d.html", i))
	}
}

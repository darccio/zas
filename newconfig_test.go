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
	"testing"
)

// mergo.Merge only recursively merges a nested map's own keys when dst
// already has a real (even if empty) map value to merge into. If a whole
// top-level section like "mimetypes" is absent from a user's config.yml
// entirely, mergo used to fall back to assigning the built-in default's own
// map object for that section directly, so mutating the returned config's
// defaulted section (e.g. through GetSection) corrupted the built-in default
// for every later NewConfig call in the process. NewConfig now pre-seeds a
// fresh, empty map for every missing default section before merging, so
// mergo's own recursive merge copies each value into a map nothing else
// references - on top of that, DefaultConfig() itself now also hands back a
// deep copy, so no separate deep-copy logic is needed here either way.

func TestNewConfigDoesNotAliasDefaultSections(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// No "mimetypes" section at all - the exact case that used to alias.
	yaml := "zas:\n  layout: .zas/layout.html\n  deploy: .zas/deploy\nsite:\n  language: en\n"
	if err := os.WriteFile(ConfigFile, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig() error = %v, want nil", err)
	}

	mimetypes := cfg.GetSection("mimetypes")
	if mimetypes == nil {
		t.Fatal(`cfg.GetSection("mimetypes") = nil, want the defaulted section`)
	}
	mimetypes["text/markdown"] = "corrupted"

	if got := DefaultConfig().GetSection("mimetypes").GetString("text/markdown"); got != "markdown" {
		t.Fatalf(`DefaultConfig()["mimetypes"]["text/markdown"] = %q, want %q (mutating the returned config must not affect the global default)`, got, "markdown")
	}

	cfg2, err := NewConfig()
	if err != nil {
		t.Fatalf("second NewConfig() error = %v, want nil", err)
	}
	if got := cfg2.GetSection("mimetypes").GetString("text/markdown"); got != "markdown" {
		t.Fatalf(`fresh NewConfig().GetSection("mimetypes").GetString("text/markdown") = %q, want %q`, got, "markdown")
	}
}

// A section already present in config.yml, even partially, was never
// affected by the aliasing bug (mergo only aliases on total absence) -
// this pins that the fix doesn't disturb the already-correct case.
func TestNewConfigMergesPartialSection(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "zas:\n  layout: .zas/layout.html\n  deploy: .zas/deploy\nmimetypes:\n  text/markdown: custom\n"
	if err := os.WriteFile(ConfigFile, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig() error = %v, want nil", err)
	}
	mimetypes := cfg.GetSection("mimetypes")
	if got := mimetypes.GetString("text/markdown"); got != "custom" {
		t.Fatalf(`GetSection("mimetypes").GetString("text/markdown") = %q, want the user's own value %q`, got, "custom")
	}
	if got := mimetypes.GetString("text/plain"); got != "plain" {
		t.Fatalf(`GetSection("mimetypes").GetString("text/plain") = %q, want the default value %q`, got, "plain")
	}
}

// GetSection must recognize a subsection as a section regardless of how
// deeply it is nested. go.yaml.in/yaml/v3 only keeps decoding nested
// mappings into the named ConfigSection type (rather than some other map
// type GetSection wouldn't recognize) by propagating that named type
// downward from the enclosing map on every decode - a bug here would
// typically only show up two or more levels deep, since the outermost
// mapping always decodes correctly on its own.
func TestNewConfigNestedSections(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "zas:\n" +
		"  layout: .zas/layout.html\n" +
		"  deploy: .zas/deploy\n" +
		"site:\n" +
		"  language: en\n" +
		"  analytics:\n" +
		"    provider: acme\n" +
		"    settings:\n" +
		"      id: UA-123\n" +
		"      nested:\n" +
		"        flag: enabled\n"
	if err := os.WriteFile(ConfigFile, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig() error = %v, want nil", err)
	}

	site := cfg.GetSection("site")
	if site == nil {
		t.Fatal(`cfg.GetSection("site") = nil, want a section`)
	}
	analytics := site.GetSection("analytics")
	if analytics == nil {
		t.Fatal(`site.GetSection("analytics") = nil, want a section (2 levels deep)`)
	}
	if got := analytics.GetString("provider"); got != "acme" {
		t.Fatalf(`analytics.GetString("provider") = %q, want %q`, got, "acme")
	}
	settings := analytics.GetSection("settings")
	if settings == nil {
		t.Fatal(`analytics.GetSection("settings") = nil, want a section (3 levels deep)`)
	}
	if got := settings.GetString("id"); got != "UA-123" {
		t.Fatalf(`settings.GetString("id") = %q, want %q`, got, "UA-123")
	}
	nested := settings.GetSection("nested")
	if nested == nil {
		t.Fatal(`settings.GetSection("nested") = nil, want a section (4 levels deep)`)
	}
	if got := nested.GetString("flag"); got != "enabled" {
		t.Fatalf(`nested.GetString("flag") = %q, want %q`, got, "enabled")
	}
}

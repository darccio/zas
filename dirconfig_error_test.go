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

// loadZasDirectoryConfig used to discard yaml.Unmarshal's error entirely
// (_ = yaml.Unmarshal(...)), so a malformed .zas.yml silently produced an
// empty config with err == nil, which then got memoized for the rest of the
// run with no diagnostic anywhere. It must now surface that error, exactly
// once per misconfigured directory thanks to the existing cache.

// malformedDirConfig is invalid YAML: a tab character is not a legal
// indentation character, so this reliably fails to parse rather than merely
// producing an unexpected structure.
const malformedDirConfig = "language: ru\n\tbad: true\n"

func TestLoadZasDirectoryConfigMalformedYAMLSurfacesError(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("sub", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("sub", DirConfigFile), []byte(malformedDirConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	gen := &Generator{}
	if _, _, err := gen.loadZasDirectoryConfig(filepath.Join("sub", "page.html")); err == nil {
		t.Fatal("loadZasDirectoryConfig() with malformed .zas.yml: want error, got nil")
	} else if wantPath := filepath.Join("sub", DirConfigFile); !strings.Contains(err.Error(), wantPath) {
		t.Fatalf("error = %v, want it to mention the malformed file's path %q", err, wantPath)
	}
	if len(gen.errs) != 1 {
		t.Fatalf("gen.errs = %v, want exactly one recorded error", gen.errs)
	}

	// A second call for a different file under the same directory must hit
	// the cache and NOT re-report the same parse failure a second time.
	if _, _, err := gen.loadZasDirectoryConfig(filepath.Join("sub", "other.html")); err != nil {
		t.Fatalf("second loadZasDirectoryConfig() call: error = %v, want nil (cached)", err)
	}
	if len(gen.errs) != 1 {
		t.Fatalf("gen.errs after second call = %v, want still exactly one recorded error (cached, not re-reported)", gen.errs)
	}
}

// TestGenerateReportsMalformedDirectoryConfig is the end-to-end regression
// test: a malformed sub/.zas.yml must fail the overall build (so the
// mistake can't go unnoticed) while the page under it still renders,
// falling back to the site-default language instead of quietly and
// permanently losing sub/'s directory config for the rest of the run.
func TestGenerateReportsMalformedDirectoryConfig(t *testing.T) {
	newTestSite(t, "site")
	if err := os.WriteFile(filepath.Join("sub", DirConfigFile), []byte(malformedDirConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	err := generate(t)
	if err == nil {
		t.Fatal("generate() with a malformed sub/.zas.yml: want error, got nil")
	}
	if wantPath := filepath.Join("sub", DirConfigFile); !strings.Contains(err.Error(), wantPath) {
		t.Fatalf("generate() error = %v, want it to mention %q", err, wantPath)
	}

	out := readDeploy(t, filepath.Join("sub", "page.html"))
	if !strings.Contains(out, "Hello") {
		t.Fatalf("sub/page.html = %q, want it to fall back to the site default language (%q)", out, "Hello")
	}
	if strings.Contains(out, "Hola") {
		t.Fatalf("sub/page.html = %q, want it NOT to have picked up sub/'s (unparseable) language", out)
	}
}

package zas

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const feedManifestPath = Dir + "/feed-outputs.json"

// The private manifest binds ownership to one deployment directory and the
// exact bytes Zas wrote. It is never itself copied to the public output.
type feedManifest struct {
	Version int               `json:"version"`
	Deploy  string            `json:"deploy"`
	Files   map[string]string `json:"files"`
}

func feedOutputName(name string) bool {
	if name == "feed.xml" {
		return true
	}
	parts := strings.Split(name, "/")
	return len(parts) == 2 && parts[1] == "feed.xml" && feedLanguagePattern.MatchString(parts[0])
}

func feedHash(contents []byte) string {
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}

func (gen *Generator) prepareFeeds() error {
	options, err := readFeedConfig(gen.Config)
	if err != nil {
		return err
	}
	manifest, err := readFeedManifest()
	if err != nil {
		return err
	}
	if !options.enabled && manifest == nil {
		gen.feed = nil
		return nil
	}
	deploy, err := filepath.Abs(gen.GetDeployPath())
	if err != nil {
		return err
	}
	state := &feedState{
		config: options, deploy: deploy, candidates: make(map[string]bool),
		manifest: feedManifest{Version: 1, Deploy: deploy, Files: make(map[string]string)},
	}
	if manifest != nil {
		if manifest.Deploy != deploy {
			return fmt.Errorf("feed: %s belongs to a different deploy path; remove the old managed feeds and manifest before changing deploy", feedManifestPath)
		}
		state.manifest = *manifest
	}
	gen.feed = state
	// Check ownership before -full can clear the deployment directory.
	if err := state.verifyOwned(false); err != nil {
		return err
	}
	if !options.enabled {
		return nil
	}
	if err := state.checkTarget("feed.xml"); err != nil {
		return err
	}
	if options.perLanguage {
		// Check existing language feed paths before a full build removes any
		// output, including languages resolved only by page templates later.
		entries, err := os.ReadDir(deploy)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		for _, entry := range entries {
			if (entry.IsDir() || entry.Type()&os.ModeSymlink != 0) && feedLanguagePattern.MatchString(entry.Name()) {
				name := entry.Name() + "/feed.xml"
				if _, err := os.Lstat(filepath.Join(deploy, filepath.FromSlash(name))); err == nil {
					if err := state.checkTarget(name); err != nil {
						return err
					}
				} else if !os.IsNotExist(err) {
					return err
				}
			}
		}
	}
	return filepath.Walk(".", func(source string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		dotPath := strings.HasPrefix(filepath.Base(source), ".") && !gen.allowedDotDir(source, info)
		if dotPath || pathHasComponent(source, Dir) || source == gen.GetDeployPath() || source == gen.Config.GetZString("layout") {
			if source != "." && info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		// Language feeds reserve this namespace even when a template supplies
		// a language that cannot be known until the page executes.
		if options.perLanguage && feedOutputName(filepath.ToSlash(source)) {
			return fmt.Errorf("feed: output %q collides with a source file", source)
		}
		if info.IsDir() || !options.contains(source) || (!hasExtension(source, ".md") && !hasExtension(source, ".html")) {
			return nil
		}
		page, templated, err := readFeedPreview(source)
		if err != nil {
			return err
		}
		if !feedExcluded(page) && (options.selected(source, page) || templated) {
			state.candidates[source] = true
			if options.perLanguage {
				data := ZasData{config: gen.Config}
				data.Page = page
				data.Directory, _, _ = gen.loadZasDirectoryConfig(source)
				language, err := data.Language()
				if err == nil && feedLanguagePattern.MatchString(language) && language != options.language {
					if err := state.checkTarget(language + "/feed.xml"); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
}

func readFeedManifest() (*feedManifest, error) {
	info, err := os.Lstat(feedManifestPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("feed: reading ownership: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("feed: ownership manifest must be a regular file")
	}
	contents, err := os.ReadFile(feedManifestPath)
	if err != nil {
		return nil, err
	}
	var manifest feedManifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		return nil, fmt.Errorf("feed: invalid ownership manifest: %w", err)
	}
	if manifest.Version != 1 || !filepath.IsAbs(manifest.Deploy) || filepath.Clean(manifest.Deploy) != manifest.Deploy || manifest.Files == nil {
		return nil, errors.New("feed: invalid ownership manifest")
	}
	for name, hash := range manifest.Files {
		digest, err := hex.DecodeString(hash)
		if !feedOutputName(name) || err != nil || len(digest) != sha256.Size {
			return nil, fmt.Errorf("feed: invalid ownership entry %q", name)
		}
	}
	return &manifest, nil
}

// Reject symlinks in the output path, including its parent directories.
// Neither writing nor cleanup may follow a link out of the deploy tree.
func (state *feedState) safePath(name string) (string, error) {
	if !feedOutputName(name) {
		return "", fmt.Errorf("feed: invalid output path %q", name)
	}
	output := filepath.Join(state.deploy, filepath.FromSlash(name))
	for current := output; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err == nil && (info.Mode()&os.ModeSymlink != 0 || (current != output && !info.IsDir())) {
			return "", fmt.Errorf("feed: unsafe output path %q", name)
		}
		if current == filepath.Dir(current) {
			break
		}
	}
	return output, nil
}

func (state *feedState) verifyOwned(allowSourceTakeover bool) error {
	for name, expected := range state.manifest.Files {
		if allowSourceTakeover && state.sourceTakeover(name) {
			continue
		}
		output, err := state.safePath(name)
		if err != nil {
			return err
		}
		contents, err := os.ReadFile(output)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("feed: reading managed %q: %w", name, err)
		}
		if feedHash(contents) != expected {
			return fmt.Errorf("feed: managed output %q was modified; move it aside before rebuilding", name)
		}
	}
	return nil
}

// A former feed is relinquished only when a real source file has actually
// been copied into its output path. Skipped symlinks or directories do not
// acquire ownership merely by occupying a matching source name.
func (state *feedState) sourceTakeover(name string) bool {
	source := filepath.FromSlash(name)
	info, err := os.Lstat(source)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	for parent := filepath.Dir(source); parent != "."; parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	contents, err := os.ReadFile(source)
	if err != nil {
		return false
	}
	output, err := state.safePath(name)
	if err != nil {
		return false
	}
	copied, err := os.ReadFile(output)
	return err == nil && feedHash(contents) == feedHash(copied)
}

func (state *feedState) checkTarget(name string) error {
	if _, err := os.Lstat(filepath.FromSlash(name)); err == nil {
		return fmt.Errorf("feed: output %q collides with a source file or directory", name)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("feed: checking source target %q: %w", name, err)
	}
	output, err := state.safePath(name)
	if err != nil {
		return err
	}
	if info, err := os.Lstat(output); err == nil {
		if _, owned := state.manifest.Files[name]; !owned || !info.Mode().IsRegular() {
			return fmt.Errorf("feed: output %q already exists without feed ownership", name)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Protect managed feeds and their ancestor directories from the general
// source reaper until finishFeeds can remove them using verified ownership.
func (gen *Generator) feedProtects(path string) bool {
	if gen.feed == nil {
		return false
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for name := range gen.feed.manifest.Files {
		output := filepath.Join(gen.feed.deploy, filepath.FromSlash(name))
		if absolute == output || strings.HasPrefix(output, absolute+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func (gen *Generator) finishFeeds() error {
	state := gen.feed
	if state == nil {
		return nil
	}
	documents := map[string][]byte{}
	if state.config.enabled {
		var err error
		documents, err = state.documents()
		if err != nil {
			return err
		}
	}
	if err := state.verifyOwned(true); err != nil {
		return err
	}
	names := make([]string, 0, len(documents))
	for name := range documents {
		if err := state.checkTarget(name); err != nil {
			return err
		}
		names = append(names, name)
	}
	sort.Strings(names)
	manifest := feedManifest{Version: 1, Deploy: state.deploy, Files: maps.Clone(state.manifest.Files)}
	for _, name := range names {
		output, err := state.safePath(name)
		if err != nil {
			return err
		}
		previous, readErr := os.ReadFile(output)
		if readErr != nil && !os.IsNotExist(readErr) {
			return readErr
		}
		contents := documents[name]
		if err := gen.atomicWriteFile(output, func(writer io.Writer) error {
			_, err := writer.Write(contents)
			return err
		}); err != nil {
			return fmt.Errorf("feed: writing %s: %w", name, err)
		}
		manifest.Files[name] = feedHash(contents)
		// Record every completed write immediately. If a later feed fails,
		// retries still recognize the bytes already published by this run.
		if err := gen.writeFeedManifest(manifest); err != nil {
			// A failed ownership write must not strand new feed bytes under
			// an old hash. Restore the previously verified file (or absence).
			var restoreErr error
			if os.IsNotExist(readErr) {
				restoreErr = os.Remove(output)
			} else {
				restoreErr = gen.atomicWriteFile(output, func(writer io.Writer) error {
					_, err := writer.Write(previous)
					return err
				})
			}
			return errors.Join(fmt.Errorf("feed: writing ownership: %w", err), restoreErr)
		}
		state.manifest = manifest
	}
	for name := range state.manifest.Files {
		if _, retained := documents[name]; retained {
			continue
		}
		// A source may take over a former feed path after feeds are disabled.
		// Keep its copied output and relinquish ownership rather than deleting it.
		if state.sourceTakeover(name) {
			delete(manifest.Files, name)
			continue
		}
		output, err := state.safePath(name)
		if err != nil {
			return err
		}
		if err := os.Remove(output); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("feed: removing %s: %w", name, err)
		}
		delete(manifest.Files, name)
		// Only empty feed directories are removed; unrelated output remains.
		if filepath.Dir(output) != state.deploy {
			_ = os.Remove(filepath.Dir(output))
		}
	}
	return gen.writeFeedManifest(manifest)
}

func (gen *Generator) writeFeedManifest(manifest feedManifest) error {
	if len(manifest.Files) == 0 {
		if err := os.Remove(feedManifestPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	contents, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')
	return gen.atomicWriteFile(feedManifestPath, func(writer io.Writer) error {
		_, err := writer.Write(contents)
		return err
	})
}

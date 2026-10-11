package zas

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"

	"github.com/PuerkitoBio/goquery"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	yaml "go.yaml.in/yaml/v3"
	html5 "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const markdownManifestName = "markdown-outputs.json"

type markdownManifest struct {
	Version int               `json:"version"`
	Deploy  string            `json:"deploy"`
	Files   map[string]string `json:"files"`
}

type markdownOutputState struct {
	enabled  bool
	deploy   string
	previous map[string]string
	wanted   map[string]bool
	current  map[string]string
	mu       sync.Mutex
}

// prepareMarkdownOutputs runs before handleDeployPath can clear anything.
// The legacy path stays unchanged unless this site opts in, or owns twins
// from an earlier opt-in run.
func (gen *Generator) prepareMarkdownOutputs() error {
	gen.markdownState = nil
	section := gen.Config.GetSection("site")
	enabled := section.GetBool("publish_markdown")
	if value, exists := section["publish_markdown"]; exists {
		if _, ok := value.(bool); !ok {
			return errors.New("site.publish_markdown must be a boolean")
		}
	}
	manifestPath := filepath.Join(Dir, markdownManifestName)
	input, err := os.ReadFile(manifestPath)
	if os.IsNotExist(err) && !enabled {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	deploy, err := resolvedOutputPath(gen.GetDeployPath())
	if err != nil {
		return err
	}
	previous := make(map[string]string)
	matchedManifest := false
	if len(input) > 0 {
		var manifest markdownManifest
		if err := json.Unmarshal(input, &manifest); err != nil {
			return fmt.Errorf("read Markdown ownership manifest: %w", err)
		}
		if manifest.Version != 1 {
			return fmt.Errorf("unsupported Markdown ownership manifest version %d", manifest.Version)
		}
		if manifest.Deploy == deploy {
			matchedManifest = true
			for path, digest := range manifest.Files {
				if !safeMarkdownOutputPath(path) || len(digest) != sha256.Size*2 {
					return fmt.Errorf("invalid Markdown ownership record %q", path)
				}
				if _, err := hex.DecodeString(digest); err != nil {
					return fmt.Errorf("invalid Markdown ownership digest for %q", path)
				}
				resolved, err := resolvedOutputPath(gen.BuildDeployPath(filepath.FromSlash(path)))
				if err != nil {
					return err
				}
				if !pathWithin(deploy, resolved) {
					return fmt.Errorf("owned Markdown output %q escapes deploy through a symlink", path)
				}
				previous[path] = digest
			}
		} else if !enabled {
			return nil // Never clean another deploy directory.
		}
	}
	if err := gen.checkMarkdownDeployPath(deploy, matchedManifest); err != nil {
		return err
	}
	state := &markdownOutputState{enabled: enabled, deploy: deploy, previous: previous, wanted: make(map[string]bool), current: make(map[string]string)}
	claims := make(map[string]string)
	directories := make(map[string]bool)
	err = filepath.Walk(".", func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		dotPath := strings.HasPrefix(filepath.Base(path), ".") && !gen.allowedDotDir(path, info)
		if path != "." && (dotPath || pathHasComponent(path, Dir) || pathWithin(deploy, absolute) || path == gen.Config.GetZString("layout")) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if info.IsDir() {
			directories[filepath.ToSlash(path)] = true
			return nil
		}
		output := filepath.ToSlash(swapExtension(path, ".md", ".html"))
		if other, exists := claims[output]; exists {
			return fmt.Errorf("%s: output path %q already claimed by %s", path, output, other)
		}
		claims[output] = path
		state.wanted[output] = true
		if !hasExtension(path, ".md") {
			return nil
		}
		input, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		page := earlyPageConfig(input)
		if value, exists := page["publish_markdown"]; exists {
			if _, boolean := value.(bool); !boolean {
				return fmt.Errorf("%s: publish_markdown must be a boolean", path)
			}
		}
		if !pagePublished(page) {
			delete(state.wanted, output)
			return nil
		}
		if enabled && markdownPageEligible(page) {
			twin := markdownTwinPath(path)
			if other, exists := claims[twin]; exists {
				return fmt.Errorf("%s: Markdown output path %q already claimed by %s", path, twin, other)
			}
			claims[twin] = path
			state.wanted[twin] = true
			if _, err := os.Lstat(gen.BuildDeployPath(filepath.FromSlash(twin))); err == nil {
				old, owned := previous[twin]
				digest, hashErr := markdownOutputHash(gen.BuildDeployPath(filepath.FromSlash(twin)))
				if hashErr != nil {
					return hashErr
				}
				if !owned || digest != old {
					return fmt.Errorf("%s: Markdown output %q already exists without matching generator ownership; move it or set publish_markdown: false on this page", path, twin)
				}
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for output := range claims {
		resolved, err := resolvedOutputPath(gen.BuildDeployPath(filepath.FromSlash(output)))
		if err != nil {
			return err
		}
		if !pathWithin(deploy, resolved) {
			return fmt.Errorf("output path %q escapes deploy through a symlink", output)
		}
		if directories[output] {
			return fmt.Errorf("output path %q collides with a source directory", output)
		}
		for parent := filepath.ToSlash(filepath.Dir(output)); parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
			if other, exists := claims[parent]; exists {
				return fmt.Errorf("output path %q is inside file output %q claimed by %s", output, parent, other)
			}
		}
	}
	gen.markdownState = state
	return nil
}

func safeMarkdownOutputPath(path string) bool {
	return path != "." && !filepath.IsAbs(path) && !strings.Contains(path, "\\") && filepath.ToSlash(filepath.Clean(path)) == path && !strings.HasPrefix(path, "../") && !pathHasComponent(filepath.FromSlash(path), Dir) && !pathHasComponent(filepath.FromSlash(path), ".git") && (hasExtension(path, ".md") || hasExtension(path, ".html"))
}

func pathWithin(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Resolve existing parent symlinks as well as a not-yet-created destination.
func resolvedOutputPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	var missing []string
	current := absolute
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) || filepath.Dir(current) == current {
			return "", err
		}
		missing = append(missing, filepath.Base(current))
		current = filepath.Dir(current)
	}
}

func (gen *Generator) checkMarkdownDeployPath(deploy string, previouslyOwned bool) error {
	root, err := resolvedOutputPath(".")
	if err != nil {
		return err
	}
	if pathWithin(deploy, root) {
		return fmt.Errorf("markdown deploy directory %q overlaps the site root", gen.GetDeployPath())
	}
	for _, protected := range []string{Dir, ".git", gen.Config.GetZString("layout")} {
		path, err := resolvedOutputPath(protected)
		if err != nil {
			return err
		}
		if pathWithin(deploy, path) || (protected == ".git" && pathWithin(path, deploy)) {
			return fmt.Errorf("markdown deploy directory %q covers protected source/configuration %q", gen.GetDeployPath(), protected)
		}
	}
	// Reserve the conventional .zas/deploy destination only. Other existing
	// source/configuration directories, including .zas/partials, cannot be
	// repurposed as deploy and cleared in a full build.
	defaultDeploy := filepath.Join(root, Dir, "deploy")
	if pathWithin(root, deploy) && deploy != defaultDeploy && !previouslyOwned {
		entries, err := os.ReadDir(deploy)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if len(entries) > 0 {
			return fmt.Errorf("markdown deploy directory %q overlaps an existing source directory; use a new directory or .zas/deploy", gen.GetDeployPath())
		}
	}
	return nil
}

func markdownTwinPath(path string) string {
	return filepath.ToSlash(swapExtension(path, ".md", ".md"))
}

func markdownPageEligible(page map[interface{}]interface{}) bool {
	draft, _ := page["draft"].(bool)
	optOut, specified := page["publish_markdown"].(bool)
	return pagePublished(page) && !draft && (!specified || optOut)
}

func (gen *Generator) markdownOutputEnabled(input []byte) bool {
	return gen.markdownState != nil && gen.markdownState.enabled && markdownPageEligible(earlyPageConfig(input))
}

func (gen *Generator) markdownTwinMissing(path string) bool {
	if gen.markdownState == nil || !hasExtension(path, ".md") {
		return false
	}
	twin := markdownTwinPath(path)
	gen.markdownState.mu.Lock()
	wanted := gen.markdownState.wanted[twin]
	gen.markdownState.mu.Unlock()
	if !wanted {
		return false
	}
	_, err := os.Stat(gen.BuildDeployPath(filepath.FromSlash(twin)))
	return err != nil
}

func (gen *Generator) markdownReaperProtected(path string, info os.FileInfo) bool {
	if gen.markdownState == nil {
		return false
	}
	// Visit children individually, so a deleted source directory cannot
	// bypass hash-guarded ownership cleanup with RemoveAll.
	if info.IsDir() {
		return true
	}
	rel, err := filepath.Rel(gen.GetDeployPath(), path)
	if err != nil {
		return false
	}
	_, owned := gen.markdownState.previous[filepath.ToSlash(rel)]
	return owned || hasExtension(path, ".md")
}

func (gen *Generator) excludeMarkdownOutputs(path string) {
	if gen.markdownState == nil {
		return
	}
	gen.markdownState.mu.Lock()
	defer gen.markdownState.mu.Unlock()
	if hasExtension(path, ".md") {
		delete(gen.markdownState.wanted, markdownTwinPath(path))
	}
	delete(gen.markdownState.wanted, filepath.ToSlash(swapExtension(path, ".md", ".html")))
}

func markdownOutputHash(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("markdown output %q is not a regular file", path)
	}
	input, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(input)
	return hex.EncodeToString(digest[:]), nil
}

func (gen *Generator) writeMarkdownOutput(path string, data *ZasData) error {
	state := gen.markdownState
	if state == nil {
		return nil
	}
	output := filepath.ToSlash(swapExtension(path, ".md", ".html"))
	if !hasExtension(path, ".md") {
		if _, previouslyOwned := state.previous[output]; !previouslyOwned {
			return nil
		}
	}
	digest, err := markdownOutputHash(gen.BuildDeployPath(filepath.FromSlash(output)))
	if err != nil {
		return err
	}
	state.mu.Lock()
	state.current[output] = digest
	state.mu.Unlock()
	if !hasExtension(path, ".md") {
		return nil
	}
	twin := markdownTwinPath(path)
	if !state.enabled || data.markdownBody == nil || !markdownPageEligible(data.Page) {
		state.mu.Lock()
		delete(state.wanted, twin)
		state.mu.Unlock()
		return nil
	}
	if err := gen.atomicWriteFile(gen.BuildDeployPath(filepath.FromSlash(twin)), func(w io.Writer) error {
		_, err := w.Write(data.markdownBody)
		return err
	}); err != nil {
		return err
	}
	twinDigest := sha256.Sum256(data.markdownBody)
	digest = hex.EncodeToString(twinDigest[:])
	state.mu.Lock()
	state.current[twin] = digest
	state.mu.Unlock()
	return nil
}

func (gen *Generator) cleanupMarkdownOutputs() error {
	state := gen.markdownState
	if state == nil {
		return nil
	}
	for path, oldDigest := range state.previous {
		if state.wanted[path] {
			if _, rewritten := state.current[path]; !rewritten {
				state.current[path] = oldDigest
			}
			continue
		}
		output := gen.BuildDeployPath(filepath.FromSlash(path))
		digest, err := markdownOutputHash(output)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if digest == oldDigest {
			if err := os.Remove(output); err != nil {
				return err
			}
		}
	}
	return nil
}

func (gen *Generator) finishMarkdownOutputs() error {
	state := gen.markdownState
	if state == nil {
		return nil
	}
	manifestPath := filepath.Join(Dir, markdownManifestName)
	if !state.enabled {
		return os.Remove(manifestPath)
	}
	manifest := markdownManifest{Version: 1, Deploy: state.deploy, Files: state.current}
	return gen.atomicWriteFile(manifestPath, func(w io.Writer) error {
		return json.NewEncoder(w).Encode(manifest)
	})
}

// A failed render can still have successfully produced other pages' twins.
// Keep their ownership recoverable without deleting anything on failure.
func (gen *Generator) recoverMarkdownOwnership() error {
	state := gen.markdownState
	if state == nil || !state.enabled || len(state.current) == 0 {
		return nil
	}
	files := make(map[string]string, len(state.previous)+len(state.current))
	for path, digest := range state.previous {
		files[path] = digest
	}
	for path, digest := range state.current {
		files[path] = digest
	}
	manifest := markdownManifest{Version: 1, Deploy: state.deploy, Files: files}
	return gen.atomicWriteFile(filepath.Join(Dir, markdownManifestName), func(w io.Writer) error {
		return json.NewEncoder(w).Encode(manifest)
	})
}

func (gen *Generator) renderMarkdownTwin(path string, input []byte) error {
	data := NewZasData(path, gen)
	data.Directory, _, _ = gen.loadZasDirectoryConfig(path)
	data.Page = earlyPageConfig(input)
	data.pageConfigSource, _ = leadingConfigComment(input)
	var preview bytes.Buffer
	if err := markdownConverter.Convert(input, &preview); err != nil {
		return err
	}
	if title, ok := leadingH1Text(preview.Bytes()); ok {
		data.FirstTitle = title
	}
	processed, err := executeMarkdownTemplate(input, &data)
	if err != nil {
		return err
	}
	// Keep the rendered config for the existing final extraction/layout.
	comment, hasComment := leadingConfigComment(processed)
	if hasComment {
		var finalPage map[interface{}]interface{}
		if err := yaml.Unmarshal(comment, &finalPage); err != nil {
			return fmt.Errorf("rendered Markdown page configuration: %w", err)
		}
		data.Page = finalPage
		data.pageConfigSource = comment
	}
	// Embeds see the rendered heading/metadata, rather than a preview of a
	// main-page heading that was itself a template action.
	preview.Reset()
	if err := markdownConverter.Convert(processed, &preview); err != nil {
		return err
	}
	if title, ok := leadingH1Text(preview.Bytes()); ok {
		data.FirstTitle = title
	}
	body, err := gen.expandMarkdownEmbeds(stripMarkdownConfig(processed), &data, filepath.Dir(path), 0, true)
	if err != nil {
		return fmt.Errorf("markdown output: %w; set publish_markdown: false on this page to retain HTML-only rendering", err)
	}
	data.markdownBody = body
	var converted bytes.Buffer
	if hasComment {
		converted.WriteString("<!--")
		converted.Write(comment)
		converted.WriteString("-->\n")
	}
	if err := markdownConverter.Convert(body, &converted); err != nil {
		return err
	}
	return gen.renderPrepared(path, converted.Bytes(), &data)
}

func executeMarkdownTemplate(input []byte, data *ZasData) ([]byte, error) {
	if pageOptsOutOfTemplating(input) || !bytes.Contains(input, templateActionOpen) {
		return input, nil
	}
	parsed, err := template.New("current").Parse(string(input))
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	if err := parsed.Execute(&output, data); err != nil {
		return nil, err
	}
	if bytes.Contains(output.Bytes(), templateActionOpen) {
		return nil, errors.New("rendered Markdown contains unresolved Go template delimiters; use template: false for literal template documentation, or publish_markdown: false for HTML-only rendering")
	}
	return output.Bytes(), nil
}

func stripMarkdownConfig(input []byte) []byte {
	if _, present := leadingConfigComment(input); !present {
		return input
	}
	start := bytes.Index(input, []byte(commentOpen))
	end := bytes.Index(input[start+len(commentOpen):], commentClose)
	return append(append([]byte{}, input[:start]...), input[start+len(commentOpen)+end+len(commentClose):]...)
}

type markdownSpan struct{ start, end int }

type markdownReplacement struct {
	markdownSpan
	content []byte
}

// Only visit raw HTML nodes: fenced/indented code and inline code remain
// literal Markdown, including examples of embeds and script plugins.
func markdownHTMLSpans(input []byte) ([]markdownSpan, error) {
	var spans []markdownSpan
	root := markdownConverter.Parser().Parse(text.NewReader(input))
	err := ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := node.(type) {
		case *ast.HTMLBlock:
			if node.Lines().Len() > 0 {
				start := node.Lines().At(0).Start
				end := node.Lines().At(node.Lines().Len() - 1).Stop
				if node.HasClosure() {
					end = node.ClosureLine.Stop
				}
				spans = append(spans, markdownSpan{start, end})
			}
		case *ast.RawHTML:
			for i := range node.Segments.Len() {
				segment := node.Segments.At(i)
				spans = append(spans, markdownSpan{segment.Start, segment.Stop})
			}
		}
		return ast.WalkContinue, nil
	})
	return spans, err
}

func (gen *Generator) expandMarkdownEmbeds(input []byte, data *ZasData, base string, depth int, markdown bool) ([]byte, error) {
	if depth >= maxEmbedDepth {
		return nil, fmt.Errorf("embed nesting deeper than %d levels; check for a self- or mutually-embedding file", maxEmbedDepth)
	}
	spans := []markdownSpan{{0, len(input)}}
	if markdown {
		var err error
		spans, err = markdownHTMLSpans(input)
		if err != nil {
			return nil, err
		}
	}
	var replacements []markdownReplacement
	for _, span := range spans {
		z := html5.NewTokenizer(bytes.NewReader(input[span.start:span.end]))
		offset := span.start
		for {
			kind := z.Next()
			if kind == html5.ErrorToken {
				if !errors.Is(z.Err(), io.EOF) {
					return nil, z.Err()
				}
				break
			}
			raw := z.Raw()
			start, end := offset, offset+len(raw)
			offset = end
			if kind != html5.StartTagToken && kind != html5.SelfClosingTagToken {
				continue
			}
			token := z.Token()
			attrs := make(map[string]string)
			for _, attr := range token.Attr {
				if _, seen := attrs[attr.Key]; !seen {
					attrs[attr.Key] = attr.Val
				}
			}
			if token.DataAtom == atom.Script && strings.HasPrefix(strings.ToLower(strings.TrimSpace(attrs["type"])), "application/zas+") {
				return nil, errors.New("script plugins cannot be emitted faithfully as Markdown")
			}
			if token.DataAtom != atom.Embed {
				continue
			}
			src, typ := attrs["src"], attrs["type"]
			if src == "" || typ == "" {
				return nil, errors.New("markdown embed requires src and type attributes")
			}
			plugin := strings.ToLower(gen.resolveMIMETypePlugin(typ))
			if _, builtin := embedPlugin(plugin); !builtin {
				return nil, fmt.Errorf("external embed plugin for %q cannot be emitted faithfully as Markdown", typ)
			}
			if plugin == "markdown" && markdown && !standaloneMarkdownEmbed(input, start, end) {
				return nil, errors.New("markdown embeds must occupy a standalone line")
			}
			if plugin == "markdown" && markdown && markdownScopedDefinitions(input) {
				return nil, errors.New("markdown embeds alongside reference definitions or footnotes cannot preserve separate document scopes")
			}
			content, err := gen.markdownEmbed(src, plugin, data, base, depth+1)
			if err != nil {
				return nil, err
			}
			if plugin == "markdown" && !markdown {
				var converted bytes.Buffer
				if err := markdownConverter.Convert(content, &converted); err != nil {
					return nil, err
				}
				content = converted.Bytes()
			}
			replacements = append(replacements, markdownReplacement{markdownSpan{start, end}, content})
		}
	}
	sort.Slice(replacements, func(i, j int) bool { return replacements[i].start < replacements[j].start })
	var output bytes.Buffer
	previous := 0
	for _, replacement := range replacements {
		if replacement.start < previous {
			return nil, errors.New("overlapping markdown embed directives")
		}
		output.Write(input[previous:replacement.start])
		output.Write(replacement.content)
		previous = replacement.end
	}
	output.Write(input[previous:])
	return output.Bytes(), nil
}

func standaloneMarkdownEmbed(input []byte, start, end int) bool {
	lineStart := bytes.LastIndexByte(input[:start], '\n') + 1
	lineEnd := bytes.IndexByte(input[end:], '\n')
	if lineEnd < 0 {
		lineEnd = len(input) - end
	}
	return len(bytes.TrimSpace(input[lineStart:start])) == 0 && len(bytes.TrimSpace(input[end:end+lineEnd])) == 0
}

func (gen *Generator) markdownEmbed(src, plugin string, data *ZasData, base string, depth int) ([]byte, error) {
	resolved, err := gen.resolveEmbedSrc(base, src)
	if err != nil {
		return nil, err
	}
	resolved, err = filepath.EvalSymlinks(resolved)
	if err != nil {
		return nil, err
	}
	root, err := filepath.EvalSymlinks(".")
	if err != nil {
		return nil, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if !pathWithin(root, resolved) {
		return nil, fmt.Errorf("embed src %q escapes the site root through a symlink", src)
	}
	input, err := os.ReadFile(resolved)
	if err != nil {
		return nil, err
	}
	input, err = executeMarkdownTemplate(input, data)
	if err != nil {
		return nil, err
	}
	input = stripMarkdownConfig(input)
	if plugin == "markdown" && markdownScopedDefinitions(input) {
		return nil, errors.New("embedded markdown reference definitions or footnotes cannot preserve separate document scopes")
	}
	if plugin == "plain" {
		return []byte(escapePlainMarkdown(string(input))), nil
	}
	input, err = gen.expandMarkdownEmbeds(input, data, filepath.Dir(resolved), depth, plugin == "markdown")
	if err != nil || plugin == "markdown" {
		return input, err
	}
	// HTML partials remain raw HTML in the twin; like legacy HTML embeds,
	// only the body survives. Directives were resolved before parsing.
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(input))
	if err != nil {
		return nil, err
	}
	body, err := doc.Find(atom.Body.String()).Html()
	return []byte(body), err
}

// Entities prevent plain-text Markdown punctuation/newlines from turning
// into new formatting, without adding a wrapper to the rendered HTML.
func escapePlainMarkdown(input string) string {
	var output strings.Builder
	for _, char := range input {
		if strings.ContainsRune("\\`*_{}[]()#+-.!|~\n\r", char) {
			output.WriteString("&#")
			output.WriteString(strconv.FormatInt(int64(char), 10))
			output.WriteByte(';')
		} else {
			output.WriteString(html.EscapeString(string(char)))
		}
	}
	return output.String()
}

func markdownScopedDefinitions(input []byte) bool {
	context := parser.NewContext()
	root := markdownConverter.Parser().Parse(text.NewReader(input), parser.WithContext(context))
	if len(context.References()) > 0 {
		return true
	}
	hasFootnotes := false
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && node.Kind().String() == "Footnote" {
			hasFootnotes = true
		}
		return ast.WalkContinue, nil
	})
	return hasFootnotes
}

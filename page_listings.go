package zas

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	yaml "go.yaml.in/yaml/v3"
)

// PageSummary contains static source metadata, without rendering the page.
// URL is an escaped root-relative navigation path; Canonical includes baseurl.
type PageSummary struct {
	Title     string
	URL       string
	Canonical string
	Date      time.Time
	Language  string
	Summary   string
}

type listedPage struct {
	source  string
	summary PageSummary
}

// Built completely before rendering and read-only for the rest of the run.
type pageListingState struct {
	pages   []listedPage
	outputs map[string]string
	deploy  string
	layout  string
}

// Pages lists public static-source pages recursively beneath section. The
// section's own index is omitted; nested article indexes remain included.
// Each call returns an independent slice that templates and callers may edit.
func (zd *ZasData) Pages(section string) ([]PageSummary, error) {
	if zd.pageListings == nil {
		return nil, errors.New("Pages requires site.page_listings: true")
	}
	section, err := pageListingSection(section)
	if err != nil {
		return nil, err
	}
	pages := make([]PageSummary, 0)
	for _, page := range zd.pageListings.pages {
		if section != "." && !strings.HasPrefix(page.source, section+"/") {
			continue
		}
		if path.Dir(page.source) == section && strings.EqualFold(path.Base(swapExtension(page.source, ".md", ".html")), "index.html") {
			continue
		}
		pages = append(pages, page.summary)
	}
	return pages, nil
}

func pageListingSection(section string) (string, error) {
	if section == "" {
		return ".", nil
	}
	if strings.ContainsAny(section, "\\?#%:\x00\r\n") || path.IsAbs(section) {
		return "", fmt.Errorf("Pages: invalid relative section %q", section)
	}
	section = strings.TrimRight(section, "/")
	for _, part := range strings.Split(section, "/") {
		if part == ".." {
			return "", fmt.Errorf("Pages: invalid relative section %q: traversal is not allowed", section)
		}
	}
	if section == "" || path.Clean(section) != section {
		return "", fmt.Errorf("Pages: invalid relative section %q", section)
	}
	return section, nil
}

// The default-off path reads the option and returns without scanning sources.
func (gen *Generator) preparePageListings() error {
	gen.pageListings = nil
	site := gen.Config.GetSection("site")
	enabled, exists := site["page_listings"]
	if exists {
		if _, ok := enabled.(bool); !ok {
			return errors.New("site.page_listings must be a boolean")
		}
	}
	if enabled == true {
		gen.pageListingsUsed = true
	}
	if !gen.pageListingsUsed {
		return nil
	}
	// A reused enabled generator must not retain deleted sources, inherited
	// language settings, output claims or errors from its previous run.
	gen.cachedZasDirectoryConfigs = nil
	gen.claimedOutputs = nil
	gen.reapedDirs = nil
	gen.dirEntriesFoldCache = nil
	gen.errs = nil
	gen.layoutModTime, gen.configModTime, gen.i18nModTime = time.Time{}, time.Time{}, time.Time{}
	if enabled != true {
		return nil
	}
	deploy, err := resolvedOutputPath(gen.GetDeployPath())
	if err != nil {
		return err
	}
	root, err := resolvedOutputPath(".")
	if err != nil {
		return err
	}
	if pathWithin(deploy, root) {
		return fmt.Errorf("page listings: deploy directory %q overlaps the site root", gen.GetDeployPath())
	}
	layout, err := resolvedOutputPath(gen.Config.GetZString("layout"))
	if err != nil {
		return err
	}
	if pathWithin(deploy, filepath.Join(root, Dir)) || pathWithin(filepath.Join(root, ".git"), deploy) || pathWithin(deploy, layout) {
		return fmt.Errorf("page listings: deploy directory %q overlaps protected configuration or layout", gen.GetDeployPath())
	}
	state := &pageListingState{deploy: deploy, layout: layout, outputs: make(map[string]string)}
	directories := make(map[string]bool)
	err = filepath.Walk(".", func(source string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		absolute, err := filepath.Abs(source)
		if err != nil {
			return err
		}
		if source != "." && (gen.sourceExcluded(source, info) || absolute == layout || pathWithin(deploy, absolute)) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if info.IsDir() {
			directories[filepath.ToSlash(source)] = true
			return nil
		}
		output := filepath.ToSlash(swapExtension(source, ".md", ".html"))
		if other, exists := state.outputs[output]; exists {
			return fmt.Errorf("%s: output path %q already claimed by %s", source, output, other)
		}
		state.outputs[output] = source
		if !hasExtension(source, ".md") && !hasExtension(source, ".html") {
			return nil
		}
		input, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		page, selected, err := gen.staticPageSummary(source, input)
		if err != nil {
			return fmt.Errorf("%s: page listings: %w", source, err)
		}
		if selected {
			state.pages = append(state.pages, listedPage{source: filepath.ToSlash(source), summary: page})
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Claims include assets and unpublished pages, so cleanup can never
	// delete another source's output or a directory containing source files.
	for output, source := range state.outputs {
		resolved, err := resolvedOutputPath(gen.BuildDeployPath(filepath.FromSlash(output)))
		if err != nil {
			return err
		}
		if !pathWithin(deploy, resolved) {
			return fmt.Errorf("page listings: output path %q escapes deploy through a symlink", output)
		}
		if resolved != filepath.Join(deploy, filepath.FromSlash(output)) {
			return fmt.Errorf("page listings: output path %q uses a symlink inside deploy", output)
		}
		if directories[output] {
			return fmt.Errorf("page listings: output path %q collides with a source directory", output)
		}
		for parent := path.Dir(output); parent != "."; parent = path.Dir(parent) {
			if other, exists := state.outputs[parent]; exists {
				return fmt.Errorf("%s: output path %q is inside file output %q claimed by %s", source, output, parent, other)
			}
		}
	}
	sort.Slice(state.pages, func(i, j int) bool {
		a, b := state.pages[i].summary, state.pages[j].summary
		if a.Date.IsZero() != b.Date.IsZero() {
			return b.Date.IsZero()
		}
		if a.Date.Equal(b.Date) {
			return a.URL < b.URL
		}
		return a.Date.After(b.Date)
	})
	gen.pageListings = state
	return nil
}

// sourceExcluded is shared with the render walk, preserving the legacy
// source selection when listings are disabled.
func (gen *Generator) sourceExcluded(source string, info os.FileInfo) bool {
	dotPath := strings.HasPrefix(filepath.Base(source), ".") && !gen.allowedDotDir(source, info)
	if dotPath || pathHasComponent(source, Dir) || source == gen.GetDeployPath() || source == gen.Config.GetZString("layout") {
		return true
	}
	if gen.pageListings != nil {
		absolute, err := filepath.Abs(source)
		return err == nil && (absolute == gen.pageListings.layout || pathWithin(gen.pageListings.deploy, absolute))
	}
	return false
}

func (gen *Generator) staticPageSummary(source string, input []byte) (PageSummary, bool, error) {
	var summary PageSummary
	sourceInput := input
	templateInput := input
	var comment []byte
	if hasExtension(source, ".md") {
		twin := gen.Config.GetSection("site").GetBool("publish_markdown") && markdownPageEligible(earlyPageConfig(sourceInput))
		markdownInput := input
		if twin {
			// Twins remove the raw config before conversion and reinsert
			// it as a real comment, including an indented raw comment.
			comment, _ = leadingConfigComment(sourceInput)
			markdownInput = stripMarkdownConfig(sourceInput)
		}
		var converted bytes.Buffer
		if err := markdownConverter.Convert(markdownInput, &converted); err != nil {
			return summary, false, err
		}
		input = converted.Bytes()
		// Legacy pages template the converted HTML; Markdown twins
		// template the original Markdown. Mirror that opt-out decision.
		if !twin {
			templateInput = input
		}
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(input))
	if err != nil {
		return summary, false, err
	}
	// Use the normal HTML5 metadata extraction so indented/fenced
	// Markdown comments remain code examples rather than configuration.
	if comment == nil {
		comment = pageConfigComment(doc)
	}
	var page map[interface{}]interface{}
	if err := yaml.Unmarshal(comment, &page); err != nil {
		return summary, false, fmt.Errorf("requires a static leading YAML map: %w", err)
	}
	templated := !pageOptsOutOfTemplating(templateInput)
	if templated {
		// Even an action inside a YAML comment can emit a newline and
		// introduce publish/draft keys. Require the whole config comment
		// to be static, not merely the fields visible to this YAML parse.
		if bytes.Contains(comment, templateActionOpen) {
			for _, key := range []string{"title", "date", "summary", "language", "publish", "draft"} {
				if value, ok := page[key].(string); ok && strings.Contains(value, "{{") {
					return summary, false, fmt.Errorf("config value %q must be static source metadata, without template actions", key)
				}
			}
			return summary, false, errors.New("the leading YAML metadata comment must be static, without template actions")
		}
		i := skipConfigSpace(sourceInput, 0)
		if hasFoldPrefix(sourceInput[i:], doctypePrefix) {
			if end := bytes.IndexByte(sourceInput[i:], '>'); end >= 0 {
				i = skipConfigSpace(sourceInput, i+end+1)
			}
		}
		if bytes.HasPrefix(sourceInput[i:], templateActionOpen) {
			return summary, false, errors.New("a leading template action may generate publication metadata; use a static leading YAML comment or heading")
		}
	}
	for _, key := range []string{"publish", "draft"} {
		if value, exists := page[key]; exists {
			if _, ok := value.(bool); !ok {
				return summary, false, fmt.Errorf("config value %q must be a static boolean", key)
			}
		}
	}
	draft, _ := page["draft"].(bool)
	if !pagePublished(page) || draft {
		return summary, false, nil
	}
	pagePath := (&url.URL{Path: "/" + filepath.ToSlash(swapExtension(source, ".md", ".html"))}).EscapedPath()
	data := ZasData{sourcePath: source, config: gen.Config, Path: pagePath, Page: page, pageConfigSource: comment}
	data.Site.BaseURL = gen.Config.GetSection("site").GetString("baseurl")
	data.Directory, _, err = gen.loadZasDirectoryConfig(source)
	if err != nil && !os.IsNotExist(err) {
		return summary, false, err
	}
	if summary.Date, err = data.Date(); err != nil {
		return summary, false, err
	}
	if summary.Summary, err = data.Summary(); err != nil {
		return summary, false, err
	}
	if summary.Language, err = data.Language(); err != nil {
		return summary, false, err
	}
	if templated && strings.Contains(summary.Language, "{{") {
		return summary, false, errors.New("resolved language must be static source metadata, without template actions")
	}
	if title, exists := page["title"]; exists {
		var ok bool
		if summary.Title, ok = title.(string); !ok {
			return summary, false, fmt.Errorf("config value %q must be a string, got %T", "title", title)
		}
	} else {
		summary.Title = gen.getTitle(doc)
		if templated && strings.Contains(summary.Title, "{{") {
			summary.Title = ""
		}
	}
	summary.Canonical = data.Canonical()
	navigation, err := url.Parse(summary.Canonical)
	if err != nil {
		return summary, false, fmt.Errorf("site.baseurl cannot form a listing URL: %w", err)
	}
	summary.URL = "/" + strings.TrimLeft(navigation.EscapedPath(), "/")
	return summary, true, nil
}

func (gen *Generator) pageListingNeedsRender(source string) bool {
	return gen.pageListings != nil && (hasExtension(source, ".md") || hasExtension(source, ".html"))
}

// Templates may hide generated metadata behind HTML parser preambles. Fail
// the build if final publication flags would expose a private source in an
// otherwise successful static listing, rather than guessing more preambles.
func (gen *Generator) checkPageListingPublication(source string, page map[interface{}]interface{}) error {
	draft, _ := page["draft"].(bool)
	if gen.pageListings == nil || (pagePublished(page) && !draft) {
		return nil
	}
	for _, listed := range gen.pageListings.pages {
		if listed.source == filepath.ToSlash(source) {
			return errors.New("page listings: template-generated publication metadata disagrees with the static source snapshot; use literal publish/draft metadata")
		}
	}
	return nil
}

func (gen *Generator) removeUnpublishedListingOutput(source string) error {
	if gen.pageListings == nil {
		return nil
	}
	output := filepath.ToSlash(swapExtension(source, ".md", ".html"))
	if gen.pageListings.outputs[output] != source {
		return fmt.Errorf("page listings: unpublished output %q has no unique source claim", output)
	}
	destination := gen.BuildDeployPath(filepath.FromSlash(output))
	info, err := os.Lstat(destination)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("page listings: unpublished output %q is not a regular file", output)
	}
	return os.Remove(destination)
}

package zas

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

type feedConfig struct {
	enabled, perLanguage, strict bool
	title, description, language string
	baseURL                      string
	sections                     []string
	limit                        int
}

// Feed language codes also become directory names. Accept conventional
// language tags without separators that could escape the deployment tree.
var feedLanguagePattern = regexp.MustCompile(`^[A-Za-z]{2,8}(?:-[A-Za-z0-9]{1,8})*$`)

func readFeedConfig(config ConfigSection) (feedConfig, error) {
	site := config.GetSection("site")
	options := feedConfig{strict: true, limit: 50}
	var err error
	if options.enabled, err = feedBool(site, "feed", false); err != nil || !options.enabled {
		return options, err
	}
	if options.strict, err = feedBool(site, "feed_strict", true); err != nil {
		return options, err
	}
	if options.perLanguage, err = feedBool(site, "feed_per_language", false); err != nil {
		return options, err
	}
	if options.title, err = feedString(site, "feed_title", site.GetString("title")); err != nil {
		return options, err
	}
	if strings.TrimSpace(options.title) == "" {
		return options, errors.New("feed: site.feed_title or site.title must be nonempty")
	}
	if options.description, err = feedString(site, "feed_description", site.GetString("description")); err != nil {
		return options, err
	}
	if options.description == "" {
		options.description = options.title
	}
	options.language = site.GetString("language")
	if !feedLanguagePattern.MatchString(options.language) {
		return options, errors.New("feed: site.language must be a language tag")
	}
	options.baseURL = strings.TrimRight(site.GetString("baseurl"), "/")
	base, err := url.Parse(options.baseURL)
	if err != nil || base == nil || (base.Scheme != "https" && base.Scheme != "http") || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return options, errors.New("feed: site.baseurl must be an absolute HTTP(S) URL without credentials, query or fragment")
	}
	if value, exists := site["feed_limit"]; exists {
		limit, ok := value.(int)
		if !ok || limit <= 0 {
			return options, errors.New("feed: site.feed_limit must be a positive integer")
		}
		options.limit = limit
	}
	if value, exists := site["feed_sections"]; exists {
		var sections []string
		switch value := value.(type) {
		case []string:
			sections = value
		case []interface{}:
			for _, item := range value {
				section, ok := item.(string)
				if !ok {
					return options, errors.New("feed: site.feed_sections must be a list of relative directory paths")
				}
				sections = append(sections, section)
			}
		default:
			return options, errors.New("feed: site.feed_sections must be a list of relative directory paths")
		}
		for _, section := range sections {
			if section == "" || strings.Contains(section, "\\") || path.IsAbs(section) || path.Clean(section) != section || section == ".." || strings.HasPrefix(section, "../") {
				return options, fmt.Errorf("feed: invalid feed section %q", section)
			}
			options.sections = append(options.sections, section)
		}
	}
	return options, nil
}

func feedBool(section ConfigSection, key string, fallback bool) (bool, error) {
	value, exists := section[key]
	if !exists {
		return fallback, nil
	}
	flag, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("feed: site.%s must be a boolean", key)
	}
	return flag, nil
}

func feedString(section ConfigSection, key, fallback string) (string, error) {
	value, exists := section[key]
	if !exists {
		return fallback, nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("feed: site.%s must be a string", key)
	}
	return text, nil
}

func (options feedConfig) contains(sourcePath string) bool {
	sourcePath = filepath.ToSlash(sourcePath)
	for _, section := range options.sections {
		if section == "." || strings.HasPrefix(sourcePath, section+"/") {
			return true
		}
	}
	return len(options.sections) == 0
}

func feedExcluded(page map[interface{}]interface{}) bool {
	publish, _ := page["publish"].(bool)
	_, hasPublish := page["publish"].(bool)
	draft, _ := page["draft"].(bool)
	feed, _ := page["feed"].(bool)
	_, hasFeed := page["feed"].(bool)
	return (hasPublish && !publish) || draft || (hasFeed && !feed)
}

func (options feedConfig) selected(sourcePath string, page map[interface{}]interface{}) bool {
	if feedExcluded(page) {
		return false
	}
	if article, exists := page["article"]; exists {
		if flag, ok := article.(bool); ok {
			return flag
		}
		// A malformed opt-in must reach strict validation rather than
		// silently behaving like an intentional article: false opt-out.
		return true
	}
	// Only section-root indexes are archives. A dated slug/index page is
	// an article without needing an additional article flag.
	if strings.EqualFold(filepath.Base(swapExtension(sourcePath, ".md", ".html")), "index.html") {
		directory := filepath.ToSlash(filepath.Dir(sourcePath))
		if len(options.sections) == 0 && directory == "." {
			return false
		}
		for _, section := range options.sections {
			if directory == section {
				return false
			}
		}
	}
	_, dated := page["date"]
	return dated || len(options.sections) > 0
}

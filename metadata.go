package zas

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Date returns the explicit page date, or zero time when absent. Timestamps
// must include a timezone in RFC3339 format; no publication date is inferred.
func (zd *ZasData) Date() (time.Time, error) {
	return zd.pageDate("date")
}

// Updated returns the explicit page updated timestamp, otherwise the most
// recent git commit date for the currently tracked source. Missing history
// returns zero time; file mtimes and the build time are never used.
func (zd *ZasData) Updated() (time.Time, error) {
	if _, ok := zd.Page["updated"]; ok {
		return zd.pageDate("updated")
	}
	if zd.sourcePath == "" {
		return time.Time{}, nil
	}
	// Require a current index entry: git log alone can find history for a
	// source that has since been removed from the index. Literal pathspecs
	// also keep filenames containing glob characters from matching others.
	tracked := exec.Command("git", "--literal-pathspecs", "ls-files", "--error-unmatch", "--", zd.sourcePath).Run() == nil
	if !tracked {
		return time.Time{}, nil
	}
	date, _ := gitCommitDate(":(literal)" + zd.sourcePath)
	return date, nil
}

func (zd *ZasData) pageDate(key string) (time.Time, error) {
	value, ok := zd.Page[key]
	if !ok {
		return time.Time{}, nil
	}
	switch value := value.(type) {
	case string:
		return parsePageDate(key, value)
	case time.Time:
		// Decode scalar text only when a date helper is actually called.
		// Decoding to Nodes also preserves YAML aliases and merged keys;
		// follow an alias to its scalar before validating the spelling.
		if zd.pageConfigSource != nil {
			var fields map[interface{}]yaml.Node
			if err := yaml.Unmarshal(zd.pageConfigSource, &fields); err != nil {
				return time.Time{}, fmt.Errorf("config value %q: %w", key, err)
			}
			node := fields[key]
			for node.Kind == yaml.AliasNode && node.Alias != nil {
				node = *node.Alias
			}
			if node.ShortTag() == "!!timestamp" {
				return parsePageDate(key, node.Value)
			}
		}
		return value, nil
	default:
		return time.Time{}, fmt.Errorf("config value %q must be an RFC3339 timestamp, got %T", key, value)
	}
}

// Go's RFC3339 parser also accepts a one-digit hour, comma fractions and
// out-of-range timezone offsets. Check the spelling and offset ranges first;
// time.Parse then validates the calendar date and clock components.
var pageDatePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-](?:[01]\d|2[0-3]):[0-5]\d)$`)

func parsePageDate(key, value string) (time.Time, error) {
	if !pageDatePattern.MatchString(value) {
		return time.Time{}, fmt.Errorf("config value %q must be an RFC3339 timestamp with a timezone", key)
	}
	date, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("config value %q must be an RFC3339 timestamp with a timezone: %w", key, err)
	}
	return date, nil
}

// Summary returns the explicit page summary, or an empty string when absent.
func (zd *ZasData) Summary() (string, error) {
	value, ok := zd.Page["summary"]
	if !ok {
		return "", nil
	}
	summary, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("config value %q must be a string, got %T", "summary", value)
	}
	return summary, nil
}

// Tags returns the page's string list in its original order, or an empty
// list when absent. A scalar or a list containing other types is an error.
func (zd *ZasData) Tags() ([]string, error) {
	value, ok := zd.Page["tags"]
	if !ok {
		return nil, nil
	}
	switch tags := value.(type) {
	case []string:
		return append([]string(nil), tags...), nil
	case []interface{}:
		result := make([]string, len(tags))
		for i, tag := range tags {
			text, ok := tag.(string)
			if !ok {
				return nil, fmt.Errorf("config value %q item %d must be a string, got %T", "tags", i, tag)
			}
			result[i] = text
		}
		return result, nil
	default:
		return nil, fmt.Errorf("config value %q must be a list of strings, got %T", "tags", value)
	}
}

// Canonical returns the full URL, replacing a final /index.html with /.
// Standalone .html URLs, Path, URL and sitemap generation are unchanged.
func (zd *ZasData) Canonical() string {
	pagePath := zd.Path
	if strings.HasSuffix(pagePath, "/index.html") {
		pagePath = strings.TrimSuffix(pagePath, "index.html")
	}
	return strings.TrimRight(zd.Site.BaseURL, "/") + pagePath
}

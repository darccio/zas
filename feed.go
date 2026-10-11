package zas

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	feedAtomNamespace    = "http://www.w3.org/2005/Atom"
	feedContentNamespace = "http://purl.org/rss/1.0/modules/content/"
	feedDCNamespace      = "http://purl.org/dc/elements/1.1/"
)

type feedState struct {
	config     feedConfig
	manifest   feedManifest
	deploy     string
	candidates map[string]bool
	mu         sync.Mutex
	entries    []feedEntry
}

type feedEntry struct {
	title, canonical, summary, body, creator, language string
	date, updated                                      time.Time
	tags                                               []string
}

// collectFeed runs after the page's own template, embeds and plugins, but
// before layout execution. It never executes content a second time.
func (gen *Generator) collectFeed(data *ZasData) error {
	state := gen.feed
	if state == nil || !state.config.enabled || !state.config.contains(data.sourcePath) || !state.config.selected(data.sourcePath, data.Page) {
		return nil
	}
	entry, err := state.entry(data)
	if err != nil {
		if state.config.strict {
			return fmt.Errorf("feed: %w", err)
		}
		gen.printLine(data.sourcePath, "=> feed: skipping:", err)
		return nil
	}
	state.mu.Lock()
	state.entries = append(state.entries, entry)
	state.mu.Unlock()
	return nil
}

func (state *feedState) entry(data *ZasData) (feedEntry, error) {
	entry := feedEntry{title: data.Title(), canonical: data.Canonical()}
	for _, key := range []string{"publish", "draft", "feed", "article"} {
		if value, present := data.Page[key]; present {
			if _, ok := value.(bool); !ok {
				return entry, fmt.Errorf("config value %q must be a boolean", key)
			}
		}
	}
	var err error
	if entry.date, err = data.Date(); err != nil {
		return entry, err
	}
	if entry.date.IsZero() {
		return entry, errors.New("selected article requires an explicit RFC3339 date")
	}
	if strings.TrimSpace(entry.title) == "" {
		return entry, errors.New("selected article requires a title or H1")
	}
	if entry.summary, err = data.Summary(); err != nil {
		return entry, err
	}
	if entry.tags, err = data.Tags(); err != nil {
		return entry, err
	}
	if entry.language, err = data.Language(); err != nil {
		return entry, err
	}
	if !feedLanguagePattern.MatchString(entry.language) {
		return entry, errors.New("selected article requires a valid language tag")
	}
	if entry.creator, err = data.Resolve("author"); err != nil {
		return entry, err
	}
	if entry.creator == "" {
		entry.creator = state.config.title
	}
	if entry.updated, err = data.Updated(); err != nil {
		return entry, err
	}
	if entry.body, err = feedHTML(string(data.Body), entry.canonical); err != nil {
		return entry, err
	}
	return entry, nil
}

type rssDocument struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Atom    string     `xml:"xmlns:atom,attr"`
	Content string     `xml:"xmlns:content,attr"`
	DC      string     `xml:"xmlns:dc,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	Language      string    `xml:"language"`
	Self          rssLink   `xml:"atom:link"`
	LastBuildDate string    `xml:"lastBuildDate,omitempty"`
	Items         []rssItem `xml:"item"`
}

type rssLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

type rssItem struct {
	Language    string     `xml:"xml:lang,attr"`
	Title       string     `xml:"title"`
	Link        string     `xml:"link"`
	GUID        rssGUID    `xml:"guid"`
	PubDate     string     `xml:"pubDate"`
	Description string     `xml:"description"`
	Content     rssContent `xml:"content:encoded"`
	Creator     string     `xml:"dc:creator"`
	DCLanguage  string     `xml:"dc:language"`
	Categories  []string   `xml:"category"`
}

type rssGUID struct {
	Permalink bool   `xml:"isPermaLink,attr"`
	Value     string `xml:",chardata"`
}

type rssContent struct {
	HTML string `xml:",cdata"`
}

func (state *feedState) documents() (map[string][]byte, error) {
	entries := state.entries
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].date.Equal(entries[j].date) {
			return entries[i].canonical < entries[j].canonical
		}
		return entries[i].date.After(entries[j].date)
	})
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if seen[entry.canonical] {
			return nil, fmt.Errorf("feed: duplicate canonical GUID %q", entry.canonical)
		}
		seen[entry.canonical] = true
	}
	sets := map[string][]feedEntry{"": entries}
	if state.config.perLanguage {
		for _, entry := range entries {
			if entry.language != state.config.language {
				sets[entry.language] = append(sets[entry.language], entry)
			}
		}
	}
	documents := make(map[string][]byte, len(sets))
	for language, subset := range sets {
		name := "feed.xml"
		if language != "" {
			name = language + "/feed.xml"
		}
		if len(subset) > state.config.limit {
			subset = subset[:state.config.limit]
		}
		content, err := state.document(name, language, subset)
		if err != nil {
			return nil, err
		}
		documents[name] = content
	}
	return documents, nil
}

func (state *feedState) document(name, language string, entries []feedEntry) ([]byte, error) {
	channel := rssChannel{
		Title:       state.config.title,
		Link:        joinSiteURL(state.config.baseURL, ""),
		Description: state.config.description,
		Language:    state.config.language,
		Self: rssLink{
			Href: joinSiteURL(state.config.baseURL, name),
			Rel:  "self",
			Type: "application/rss+xml",
		},
	}
	if language != "" {
		label := language
		switch language {
		case "en":
			label = "English"
		case "es":
			label = "Español"
		}
		channel.Title += " (" + label + ")"
		channel.Language = language
	}
	var latest time.Time
	for _, entry := range entries {
		channel.Items = append(channel.Items, rssItem{
			Language: entry.language,
			Title:    entry.title,
			Link:     entry.canonical,
			GUID:     rssGUID{Permalink: true, Value: entry.canonical},
			PubDate:  entry.date.Format(time.RFC1123Z),
			// RSS readers interpret item descriptions as HTML. First escape
			// the plain summary for that layer; encoding/xml escapes it again
			// for transport, so literal tags/entities survive in reader text.
			Description: html.EscapeString(entry.summary),
			Content:     rssContent{HTML: entry.body},
			Creator:     entry.creator,
			DCLanguage:  entry.language,
			Categories:  entry.tags,
		})
		if entry.updated.After(latest) {
			latest = entry.updated
		}
	}
	if !latest.IsZero() {
		channel.LastBuildDate = latest.Format(time.RFC1123Z)
	}
	document := rssDocument{
		Version: "2.0", Atom: feedAtomNamespace, Content: feedContentNamespace,
		DC: feedDCNamespace, Channel: channel,
	}
	output, err := xml.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("feed: encoding %s: %w", name, err)
	}
	output = append([]byte(xml.Header), output...)
	output = append(output, '\n')
	// CDATA is written verbatim by encoding/xml, including otherwise
	// forbidden XML control characters. Validate before publishing any feed.
	decoder := xml.NewDecoder(bytes.NewReader(output))
	for {
		if _, err := decoder.Token(); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("feed: invalid XML in %s: %w", name, err)
		}
	}
	return output, nil
}

func (gen *Generator) feedNeedsRender(sourcePath string) bool {
	if gen.feed == nil {
		return false
	}
	if gen.feed.config.enabled && gen.feed.candidates[sourcePath] {
		return true
	}
	// Hand a former feed path to a real source even if its mtime is older
	// than the generated feed. Cleanup then sees the copied source bytes.
	_, owned := gen.feed.manifest.Files[filepath.ToSlash(sourcePath)]
	return owned
}

func readFeedPreview(sourcePath string) (map[interface{}]interface{}, bool, error) {
	input, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, false, err
	}
	page := earlyPageConfig(input)
	return page, bytes.Contains(input, templateActionOpen), nil
}

package zas

import (
	"errors"
	"net/url"
	"strings"

	html5 "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func feedHTML(body, canonical string) (string, error) {
	base, err := url.Parse(canonical)
	if err != nil {
		return "", err
	}
	context := &html5.Node{Type: html5.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html5.ParseFragment(strings.NewReader(body), context)
	if err != nil {
		return "", err
	}
	var visit func(*html5.Node) error
	visit = func(node *html5.Node) error {
		if node.Type == html5.ElementNode {
			if node.DataAtom == atom.Embed {
				return errors.New("article body contains a residual embed directive; use feed: false for content requiring a second layout pass")
			}
			if node.DataAtom == atom.Script {
				for _, attribute := range node.Attr {
					if attribute.Key == "type" && strings.HasPrefix(strings.ToLower(strings.TrimSpace(attribute.Val)), scriptPluginType) {
						return errors.New("article body contains a residual Zas script directive; use feed: false for content requiring a second layout pass")
					}
				}
			}
		}
		for i := range node.Attr {
			attribute := &node.Attr[i]
			switch attribute.Key {
			case "href", "src":
				attribute.Val = feedReference(base, attribute.Val)
			case "srcset":
				attribute.Val = feedSrcset(base, attribute.Val)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if err := visit(child); err != nil {
				return err
			}
		}
		return nil
	}
	var output strings.Builder
	for _, node := range nodes {
		if err := visit(node); err != nil {
			return "", err
		}
		if err := html5.Render(&output, node); err != nil {
			return "", err
		}
	}
	return output.String(), nil
}

func feedReference(base *url.URL, reference string) string {
	parsed, err := url.Parse(strings.TrimSpace(reference))
	if err != nil || (parsed.Scheme != "" && parsed.Scheme != "http" && parsed.Scheme != "https") {
		return reference
	}
	return base.ResolveReference(parsed).String()
}

// Identify URL spans using the HTML srcset parsing algorithm, then rewrite
// only those spans. Commas inside a URL (notably data URLs) are not candidate
// separators; descriptors, whitespace and separators are preserved verbatim.
func feedSrcset(base *url.URL, value string) string {
	var output strings.Builder
	last := 0
	for position := 0; position < len(value); {
		for position < len(value) && (srcsetSpace(value[position]) || value[position] == ',') {
			position++
		}
		start := position
		for position < len(value) && !srcsetSpace(value[position]) {
			position++
		}
		end := position
		for end > start && value[end-1] == ',' {
			end--
		}
		if start == end {
			continue
		}
		output.WriteString(value[last:start])
		output.WriteString(feedReference(base, value[start:end]))
		last = end
		if end != position {
			continue
		}
		parentheses := 0
		for position < len(value) {
			character := value[position]
			position++
			switch character {
			case '(':
				parentheses++
			case ')':
				if parentheses > 0 {
					parentheses--
				}
			case ',':
				if parentheses == 0 {
					goto nextCandidate
				}
			}
		}
	nextCandidate:
	}
	output.WriteString(value[last:])
	return output.String()
}

func srcsetSpace(character byte) bool {
	return character == ' ' || character == '\t' || character == '\n' || character == '\r' || character == '\f'
}

package links

import (
	"regexp"
	"strings"
)

// Link is a link found in a run's text: what it says and where it goes.
type Link struct{ Label, URL string }

var (
	markdownLink = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^)\s]+)\)`)
	bareURL      = regexp.MustCompile(`https?://\S+`)
	// fileLink is a markdown link to a file beside the text, such as a
	// question's brief: [brief](questions/hidden-ready.md).
	fileLink = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s:]+\.md)\)`)
)

// InText splits s into its words and the links it carries: markdown links
// [label](url), then bare URLs, which are labelled by their host. The words
// come back without the links, so a long URL -- a chat with its message
// already written, say -- does not crowd out what the line says. Links to
// files beside the text (Files) are left out of both.
func InText(s string) (string, []Link) {
	var found []Link
	s = fileLink.ReplaceAllString(s, "")
	s = markdownLink.ReplaceAllStringFunc(s, func(m string) string {
		p := markdownLink.FindStringSubmatch(m)
		found = append(found, Link{Label: p[1], URL: p[2]})
		return ""
	})
	s = bareURL.ReplaceAllStringFunc(s, func(m string) string {
		url := strings.TrimRight(m, ".,;:)")
		found = append(found, Link{Label: Host(url), URL: url})
		return m[len(url):]
	})
	return strings.Join(strings.Fields(s), " "), found
}

// Host is a URL's host, a bare URL's label.
func Host(url string) string {
	h := url[strings.Index(url, "//")+2:]
	if i := strings.IndexAny(h, "/?#"); i >= 0 {
		h = h[:i]
	}
	return h
}

// Describe is what a question asking to open l says about it: its label, and
// where it goes when the label does not say.
func (l Link) Describe() string {
	if h := Host(l.URL); h != l.Label {
		return l.Label + " (" + h + ")"
	}
	return l.Label
}

// Files are the markdown links in s to files beside it, such as a
// question's brief, [brief](questions/hidden-ready.md): relative paths to
// .md files. InText leaves them out of its words and its links.
func Files(s string) []Link {
	var out []Link
	for _, m := range fileLink.FindAllStringSubmatch(s, -1) {
		out = append(out, Link{Label: m[1], URL: m[2]})
	}
	return out
}

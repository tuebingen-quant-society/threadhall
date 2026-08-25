package message

import (
	"bytes"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

var markdownParser = goldmark.New(goldmark.WithExtensions(extension.GFM))

var markdownPolicy = func() *bluemonday.Policy {
	policy := bluemonday.StrictPolicy()
	policy.AllowElements(
		"a", "blockquote", "br", "code", "del", "em", "h1", "h2", "h3", "h4", "h5", "h6",
		"hr", "li", "ol", "p", "pre", "strong", "table", "tbody", "td", "tfoot", "th", "thead", "tr", "ul",
	)
	policy.AllowAttrs("href", "title").OnElements("a")
	policy.AllowStandardURLs()
	policy.RequireNoFollowOnLinks(true)
	policy.RequireNoReferrerOnLinks(true)
	policy.AddTargetBlankToFullyQualifiedLinks(true)
	return policy
}()

// RenderMarkdown converts a validated raw body into server-owned HTML.
func RenderMarkdown(body string) (string, error) {
	var rendered bytes.Buffer
	if err := markdownParser.Convert([]byte(body), &rendered); err != nil {
		return "", err
	}
	return markdownPolicy.Sanitize(rendered.String()), nil
}

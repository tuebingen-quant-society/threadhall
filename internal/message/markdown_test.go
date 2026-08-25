package message

import (
	"strings"
	"testing"
)

func TestRenderMarkdownProducesSafeServerHTML(t *testing.T) {
	raw := "**safe** [docs](https://example.com/path)\n\n" +
		"<script>alert('x')</script><img src=x onerror=alert(1)>\n\n" +
		"[script](javascript:alert(1)) [data](data:text/html,pwned)\n\n" +
		"![tracker](https://evil.example/tracker.png)\n\n" +
		"<a href=\"https://evil.example\" onclick=\"steal()\">raw link</a>"

	rendered, err := RenderMarkdown(raw)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if !strings.Contains(rendered, "<strong>safe</strong>") ||
		!strings.Contains(rendered, `href="https://example.com/path"`) {
		t.Fatalf("safe Markdown was not rendered: %q", rendered)
	}
	for _, dangerous := range []string{"<script", "alert('x')", "onerror", "onclick", "javascript:", "data:text/html", "<img", "<a href=\"https://evil.example\""} {
		if strings.Contains(strings.ToLower(rendered), strings.ToLower(dangerous)) {
			t.Errorf("rendered HTML contains %q: %q", dangerous, rendered)
		}
	}
	if !strings.Contains(rendered, "nofollow") || !strings.Contains(rendered, "noreferrer") {
		t.Fatalf("rendered link lacks safe rel attributes: %q", rendered)
	}
}

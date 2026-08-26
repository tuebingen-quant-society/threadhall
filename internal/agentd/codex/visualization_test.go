package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
)

func TestExtractVisualizationsReadsBoundedTaskOwnedFragment(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "agent-flow.html")
	fragment := `<div id="agent-flow"><button type="button">Run</button></div>`
	if err := os.WriteFile(path, []byte(fragment), 0o600); err != nil {
		t.Fatal(err)
	}

	output := "Coordinator summary.\n\n" + visualizeReference(path, "Agent flow")
	cleaned, apps, err := extractVisualizations(output, root)
	if err != nil {
		t.Fatalf("extractVisualizations: %v", err)
	}
	if cleaned != "Coordinator summary.\n\n[agent-flow.html](#attachment-"+filepath.Base(apps[0].ResourceURI)+")" || len(apps) != 1 {
		t.Fatalf("cleaned = %q, apps = %#v", cleaned, apps)
	}
	if apps[0].Server != "visualize" || apps[0].Tool != "render" || apps[0].HTML != fragment ||
		!strings.HasPrefix(apps[0].ResourceURI, "ui://visualize/") {
		t.Fatalf("visualization app = %#v", apps[0])
	}
}

func TestExtractVisualizationsUsesTitleAsTextFallback(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "agent-flow.html")
	if err := os.WriteFile(path, []byte("<svg></svg>"), 0o600); err != nil {
		t.Fatal(err)
	}

	cleaned, apps, err := extractVisualizations(visualizeReference(path, "Agent flow"), root)
	if err != nil || cleaned != "[agent-flow.html](#attachment-"+filepath.Base(apps[0].ResourceURI)+")" || len(apps) != 1 {
		t.Fatalf("cleaned = %q, apps = %#v, err = %v", cleaned, apps, err)
	}
}

func TestExtractVisualizationsConvertsTaskOwnedMarkdownLink(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "agent-flow.html")
	if err := os.WriteFile(path, []byte("<svg></svg>"), 0o600); err != nil {
		t.Fatal(err)
	}

	output := "Created [Agent flow](" + path + ").\n\nReady."
	cleaned, apps, err := extractVisualizations(output, root)
	if err != nil {
		t.Fatalf("extractVisualizations: %v", err)
	}
	if cleaned != "Created [agent-flow.html](#attachment-"+filepath.Base(apps[0].ResourceURI)+").\n\nReady." || len(apps) != 1 || strings.Contains(cleaned, root) {
		t.Fatalf("cleaned = %q, apps = %#v", cleaned, apps)
	}
}

func TestExtractVisualizationsCapturesSanitizedMarkdownFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "notes.md")
	if err := os.WriteFile(path, []byte("# Notes\n\n**Safe**\n\n<script>alert(1)</script>"), 0o600); err != nil {
		t.Fatal(err)
	}

	cleaned, apps, err := extractVisualizations("Read [notes]("+path+")", root)
	if err != nil || len(apps) != 1 {
		t.Fatalf("cleaned = %q, apps = %#v, err = %v", cleaned, apps, err)
	}
	if !strings.Contains(cleaned, "[notes.md](#attachment-") || apps[0].Server != "threadhall-files" ||
		apps[0].Tool != "preview" || strings.Contains(apps[0].HTML, "script") || !strings.Contains(apps[0].HTML, "<strong>Safe</strong>") {
		t.Fatalf("cleaned = %q, app = %#v", cleaned, apps[0])
	}
}

func TestExtractVisualizationsRejectsUncapturedTaskPath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	output := "Saved a file at " + filepath.Join(root, "notes.txt")

	_, _, err := extractVisualizations(output, root)
	if err == nil || !strings.Contains(err.Error(), "temporary task path") {
		t.Fatalf("error = %v", err)
	}
}

func TestExtractVisualizationsRejectsPathsOutsideTaskRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.html")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := extractVisualizations(visualizeReference(outside, "Secret"), root)
	if err == nil || !strings.Contains(err.Error(), "task directory") {
		t.Fatalf("error = %v", err)
	}
}

func TestExtractVisualizationsRejectsOversizedFragment(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "large.html")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", agenttask.MaxInlineAppBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := extractVisualizations(visualizeReference(path, "Large"), root)
	if err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("error = %v", err)
	}
}

func visualizeReference(path, title string) string {
	return "visualize{\"path\":\"" + path + "\",\"title\":\"" + title + "\"}"
}

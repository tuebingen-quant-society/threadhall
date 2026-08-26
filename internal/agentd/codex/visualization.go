package codex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
	"github.com/tuebingen-quant-society/threadhall/internal/message"
)

const (
	visualizePrefix = "\uE200visualize\uE202"
	visualizeSuffix = "\uE201"
)

type visualizationReference struct {
	Path  string `json:"path"`
	Mode  string `json:"mode"`
	Title string `json:"title"`
}

var markdownArtifact = regexp.MustCompile(`\[([^\]\r\n]{1,120})\]\((/[^)\r\n]{1,1000}\.(?:html|md|markdown))\)`)

func extractVisualizations(output, root string) (string, []agenttask.InlineApp, error) {
	var cleaned strings.Builder
	apps := make([]agenttask.InlineApp, 0, 1)
	fallback := ""
	remaining := output
	for {
		start := strings.Index(remaining, visualizePrefix)
		if start < 0 {
			cleaned.WriteString(remaining)
			break
		}
		cleaned.WriteString(remaining[:start])
		payloadStart := start + len(visualizePrefix)
		end := strings.Index(remaining[payloadStart:], visualizeSuffix)
		if end < 0 {
			return "", nil, errors.New("visualization reference is incomplete")
		}
		var reference visualizationReference
		if err := json.Unmarshal([]byte(remaining[payloadStart:payloadStart+end]), &reference); err != nil {
			return "", nil, errors.New("visualization reference is invalid")
		}
		app, err := loadVisualization(root, reference)
		if err != nil {
			return "", nil, err
		}
		apps = append(apps, app)
		cleaned.WriteString(artifactLink(app))
		if len(apps) > agenttask.MaxInlineApps {
			return "", nil, errors.New("too many inline visualizations")
		}
		if fallback == "" {
			fallback = artifactFilename(app)
		}
		remaining = remaining[payloadStart+end+len(visualizeSuffix):]
	}
	text, linkedApps, linkedFallback, err := extractLinkedVisualizations(cleaned.String(), root)
	if err != nil {
		return "", nil, err
	}
	apps = append(apps, linkedApps...)
	if len(apps) > agenttask.MaxInlineApps {
		return "", nil, errors.New("too many inline visualizations")
	}
	if fallback == "" {
		fallback = linkedFallback
	}
	text = strings.TrimSpace(text)
	if strings.Contains(text, filepath.Clean(root)) {
		return "", nil, errors.New("Codex exposed a temporary task path")
	}
	if text == "" && len(apps) > 0 {
		text = fallback
		if text == "" {
			text = "Interactive visualization"
		}
	}
	return text, apps, nil
}

func extractLinkedVisualizations(output, root string) (string, []agenttask.InlineApp, string, error) {
	matches := markdownArtifact.FindAllStringSubmatchIndex(output, -1)
	if len(matches) == 0 {
		return output, nil, "", nil
	}
	var cleaned strings.Builder
	apps := make([]agenttask.InlineApp, 0, len(matches))
	fallback, cursor := "", 0
	for _, match := range matches {
		cleaned.WriteString(output[cursor:match[0]])
		title, path := output[match[2]:match[3]], output[match[4]:match[5]]
		app, err := loadArtifact(root, visualizationReference{Path: path, Title: title})
		if err != nil {
			return "", nil, "", err
		}
		apps = append(apps, app)
		cleaned.WriteString(artifactLink(app))
		if fallback == "" {
			fallback = title
		}
		cursor = match[1]
	}
	cleaned.WriteString(output[cursor:])
	return cleaned.String(), apps, fallback, nil
}

func loadVisualization(root string, reference visualizationReference) (agenttask.InlineApp, error) {
	return loadArtifact(root, reference)
}

func loadArtifact(root string, reference visualizationReference) (agenttask.InlineApp, error) {
	extension := strings.ToLower(filepath.Ext(reference.Path))
	if !filepath.IsAbs(reference.Path) || (extension != ".html" && extension != ".md" && extension != ".markdown") {
		return agenttask.InlineApp{}, errors.New("artifact must be an absolute HTML or Markdown path")
	}
	rootPath, err := filepath.EvalSymlinks(root)
	if err != nil {
		return agenttask.InlineApp{}, fmt.Errorf("resolve visualization task directory: %w", err)
	}
	path, err := filepath.EvalSymlinks(reference.Path)
	if err != nil {
		return agenttask.InlineApp{}, fmt.Errorf("resolve visualization: %w", err)
	}
	relative, err := filepath.Rel(rootPath, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return agenttask.InlineApp{}, errors.New("visualization path is outside the task directory")
	}
	file, err := os.Open(path)
	if err != nil {
		return agenttask.InlineApp{}, fmt.Errorf("open visualization: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return agenttask.InlineApp{}, errors.New("visualization must be a regular file")
	}
	if info.Size() <= 0 || info.Size() > agenttask.MaxInlineAppBytes {
		return agenttask.InlineApp{}, errors.New("visualization exceeds the inline size limit")
	}
	fragment, err := io.ReadAll(io.LimitReader(file, agenttask.MaxInlineAppBytes+1))
	if err != nil || len(fragment) > agenttask.MaxInlineAppBytes {
		return agenttask.InlineApp{}, errors.New("visualization exceeds the inline size limit")
	}
	if reference.Mode != "" && reference.Mode != "wide" {
		return agenttask.InlineApp{}, errors.New("visualization mode is invalid")
	}
	if len(reference.Title) > 120 {
		return agenttask.InlineApp{}, errors.New("visualization title is too long")
	}
	contentType := "text/html"
	server, tool, body := "visualize", "render", string(fragment)
	if extension != ".html" {
		contentType, server, tool = "text/markdown", "threadhall-files", "preview"
		body, err = message.RenderMarkdown(string(fragment))
		if err != nil {
			return agenttask.InlineApp{}, fmt.Errorf("render markdown artifact: %w", err)
		}
	}
	metadata, _ := json.Marshal(map[string]string{
		"mode": reference.Mode, "title": reference.Title,
		"filename": filepath.Base(path), "content_type": contentType,
	})
	digest := sha256.Sum256(fragment)
	resourceKind := "visualize"
	if server == "threadhall-files" {
		resourceKind = "threadhall-file"
	}
	return agenttask.InlineApp{
		Server: server, Tool: tool, ResourceURI: "ui://" + resourceKind + "/" + hex.EncodeToString(digest[:8]),
		HTML: body, Arguments: metadata, Result: json.RawMessage(`{}`),
	}, nil
}

func artifactLink(app agenttask.InlineApp) string {
	return "[" + strings.ReplaceAll(artifactFilename(app), "]", "\\]") + "](#attachment-" + filepath.Base(app.ResourceURI) + ")"
}

func artifactFilename(app agenttask.InlineApp) string {
	var metadata map[string]string
	_ = json.Unmarshal(app.Arguments, &metadata)
	if filename := filepath.Base(metadata["filename"]); filename != "." && filename != "" {
		return filename
	}
	return "generated-file"
}

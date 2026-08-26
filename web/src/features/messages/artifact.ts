import type { InlineApp } from "../../api/types";

export interface ArtifactMetadata {
	filename: string;
	contentType: "text/html" | "text/markdown";
	title: string;
}

export function attachmentID(app: InlineApp) {
	return app.resource_uri.split("/").pop() ?? "";
}

export function artifactMetadata(app: InlineApp): ArtifactMetadata | null {
	if (!((app.server === "visualize" && app.tool === "render") ||
		(app.server === "threadhall-files" && app.tool === "preview"))) return null;
	const input = typeof app.arguments === "object" && app.arguments !== null ? app.arguments as Record<string, unknown> : {};
	const title = typeof input.title === "string" ? input.title : "Generated file";
	const fallback = app.server === "visualize" ? `${title.replace(/[^a-z0-9]+/gi, "-").replace(/^-|-$/g, "").toLowerCase() || "visualization"}.html` : "document.md";
	return {
		filename: typeof input.filename === "string" && input.filename !== "" ? input.filename : fallback,
		contentType: input.content_type === "text/markdown" ? "text/markdown" : "text/html",
		title,
	};
}

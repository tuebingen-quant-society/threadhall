import type { InlineApp } from "../../api/types";
import { artifactMetadata } from "./artifact";
import { McpApp } from "./mcp-app";

export function FilePreview({ app, onClose }: { app: InlineApp; onClose: () => void }) {
	const metadata = artifactMetadata(app);
	if (!metadata) return null;
	return <section class="file-preview" aria-label={`Preview ${metadata.filename}`}>
		<header><div><h2>{metadata.filename}</h2><p>{metadata.contentType === "text/markdown" ? "Markdown" : "HTML"}</p></div><button type="button" onClick={onClose}>Back</button></header>
		{metadata.contentType === "text/markdown"
			? <div class="file-preview-markdown message-body" dangerouslySetInnerHTML={{ __html: app.html }} />
			: <McpApp app={app} />}
	</section>;
}

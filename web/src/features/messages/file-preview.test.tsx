import { render, screen } from "@testing-library/preact";
import { describe, expect, it } from "vitest";

import type { InlineApp } from "../../api/types";
import { FilePreview } from "./file-preview";

describe("FilePreview", () => {
	it("renders stored Markdown HTML without an executable frame", () => {
		const app: InlineApp = {
			server: "threadhall-files", tool: "preview", resource_uri: "ui://threadhall-file/notes", html: "<h1>Notes</h1><p>Safe</p>",
			arguments: { filename: "notes.md", content_type: "text/markdown" }, result: {},
		};
		render(<FilePreview app={app} />);
		expect(screen.getByRole("heading", { name: "notes.md" })).toBeTruthy();
		expect(screen.getByRole("heading", { name: "Notes" })).toBeTruthy();
		expect(screen.queryByTitle(/Interactive UI/)).toBeNull();
	});
});

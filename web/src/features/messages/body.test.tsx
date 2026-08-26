import { fireEvent, render, screen } from "@testing-library/preact";
import { describe, expect, it, vi } from "vitest";

import type { InlineApp } from "../../api/types";
import { MessageBody } from "./body";

const app: InlineApp = {
	server: "visualize", tool: "render", resource_uri: "ui://visualize/deadbeef", html: "<p>Preview</p>",
	arguments: { filename: "plan.html", content_type: "text/html" }, result: {},
};

describe("MessageBody attachments", () => {
	it("opens a matching durable artifact without navigating the page", () => {
		const open = vi.fn();
		render(<MessageBody html={'<p>Created <a href="#attachment-deadbeef">plan.html</a>.</p>'} memberNames={new Map()} attachments={[app]} onOpenAttachment={open} />);
		fireEvent.click(screen.getByRole("link", { name: "plan.html" }));
		expect(open).toHaveBeenCalledWith(app);
		expect(location.hash).toBe("");
	});
});

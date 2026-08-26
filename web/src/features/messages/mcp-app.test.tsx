import { render, screen } from "@testing-library/preact";
import { describe, expect, it } from "vitest";

import type { InlineApp } from "../../api/types";
import { McpApp, mcpAppBridgeMessages } from "./mcp-app";

const app: InlineApp = {
	server: "forms", tool: "ask", resource_uri: "ui://forms/ask",
	html: "<form><button>Choose</button></form>",
	arguments: { question: "Choose" }, result: { content: [], structuredContent: { choice: "A" } },
};

describe("McpApp", () => {
	it("isolates plugin HTML in an opaque-origin, no-network sandbox", () => {
		render(<McpApp app={app} />);
		const frame = screen.getByTitle("Interactive UI from forms") as HTMLIFrameElement;
		expect(frame.getAttribute("sandbox")).toBe("allow-scripts");
		expect(frame.getAttribute("sandbox")).not.toContain("allow-same-origin");
		expect(frame.srcdoc).toContain("default-src 'none'");
		expect(frame.srcdoc).toContain(app.html);
	});

	it("answers the MCP Apps handshake and supplies bounded tool input and result", () => {
		const initialized = mcpAppBridgeMessages({ jsonrpc: "2.0", id: 7, method: "ui/initialize", params: { protocolVersion: "2026-01-26" } }, app);
		expect(initialized).toEqual([expect.objectContaining({ jsonrpc: "2.0", id: 7, result: expect.objectContaining({
			protocolVersion: "2026-01-26", hostContext: expect.objectContaining({ displayMode: "inline" }),
		}) })]);
		const data = mcpAppBridgeMessages({ jsonrpc: "2.0", method: "ui/notifications/initialized" }, app);
		expect(data[0]).toMatchObject({ method: "ui/notifications/tool-input", params: { arguments: app.arguments } });
		expect(data[1]).toMatchObject({ method: "ui/notifications/tool-result", params: app.result });
	});

	it("rejects plugin-initiated tool calls instead of granting ambient authority", () => {
		const messages = mcpAppBridgeMessages({ jsonrpc: "2.0", id: "call-1", method: "tools/call" }, app);
		expect(messages).toEqual([{ jsonrpc: "2.0", id: "call-1", error: { code: -32601, message: "Interactive tool calls are not enabled in Threadhall" } }]);
	});
});

import { useEffect, useMemo, useRef, useState } from "preact/hooks";

import type { InlineApp } from "../../api/types";
import { visualizationDocument, visualizationMetadata } from "./visualization-host";

interface RPCRequest {
	jsonrpc?: string;
	id?: string | number;
	method?: string;
	params?: { protocolVersion?: string };
}

const latestProtocolVersion = "2026-01-26";
const supportedProtocolVersions = new Set([latestProtocolVersion, "2025-06-18"]);
const csp = "default-src 'none'; img-src data: blob:; style-src 'unsafe-inline'; script-src 'unsafe-inline' blob:; font-src data:; connect-src 'none'; form-action 'none'; base-uri 'none'";

function sandboxDocument(html: string) {
	const policy = `<meta http-equiv="Content-Security-Policy" content="${csp}">`;
	const head = html.match(/<head(?:\s[^>]*)?>/i);
	if (!head || head.index === undefined) return `<!doctype html><html><head>${policy}</head><body>${html}</body></html>`;
	const end = head.index + head[0].length;
	return html.slice(0, end) + policy + html.slice(end);
}

export function mcpAppBridgeMessages(request: RPCRequest, app: InlineApp): unknown[] {
	if (request.jsonrpc !== "2.0" || typeof request.method !== "string") return [];
	if (request.method === "ui/notifications/initialized" && request.id === undefined) {
		return [
			{ jsonrpc: "2.0", method: "ui/notifications/tool-input", params: { arguments: app.arguments } },
			{ jsonrpc: "2.0", method: "ui/notifications/tool-result", params: app.result },
		];
	}
	if (request.id === undefined) return [];
	if (request.method !== "ui/initialize") {
		return [{ jsonrpc: "2.0", id: request.id, error: {
			code: -32601, message: "Interactive tool calls are not enabled in Threadhall",
		} }];
	}
	const requested = request.params?.protocolVersion;
	const protocolVersion = requested && supportedProtocolVersions.has(requested) ? requested : latestProtocolVersion;
	return [
		{ jsonrpc: "2.0", id: request.id, result: {
			protocolVersion, hostInfo: { name: "threadhall", version: "0.1" }, hostCapabilities: {}, hostContext: {
				theme: "light", displayMode: "inline", availableDisplayModes: ["inline"], platform: "web",
				containerDimensions: { maxWidth: 672, maxHeight: 288 }, userAgent: "threadhall/0.1",
			},
		} },
	];
}

export function McpApp({ app }: { app: InlineApp }) {
	const frame = useRef<HTMLIFrameElement>(null);
	const visualization = app.server === "visualize" && app.tool === "render";
	const metadata = useMemo(() => visualizationMetadata(app.arguments), [app.arguments]);
	const source = useMemo(() => sandboxDocument(visualization ? visualizationDocument(app.html) : app.html), [app.html, visualization]);
	const [height, setHeight] = useState(288);
	useEffect(() => {
		function receive(event: MessageEvent) {
			if (event.source !== frame.current?.contentWindow || typeof event.data !== "object" || event.data === null) return;
			if (visualization && (event.data as { type?: string }).type === "threadhall/resize") {
				const requested = Number((event.data as { height?: unknown }).height);
				if (Number.isFinite(requested)) setHeight(Math.max(120, Math.min(720, Math.ceil(requested))));
				return;
			}
			for (const message of mcpAppBridgeMessages(event.data as RPCRequest, app)) {
				frame.current?.contentWindow?.postMessage(message, "*");
			}
		}
		window.addEventListener("message", receive);
		return () => window.removeEventListener("message", receive);
	}, [app, visualization]);
	const className = `mcp-app-frame${visualization ? ` visualization-frame${metadata.mode === "wide" ? " wide" : ""}` : ""}`;
	return <iframe ref={frame} class={className} title={visualization ? metadata.title : `Interactive UI from ${app.server}`}
		style={visualization ? { height: `${height}px` } : undefined}
		sandbox="allow-scripts" referrerPolicy="no-referrer" srcDoc={source} />;
}

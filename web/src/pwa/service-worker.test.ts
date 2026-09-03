import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { afterEach, describe, expect, it } from "vitest";

const temporaryRoots: string[] = [];

afterEach(async () => {
	await Promise.all(temporaryRoots.splice(0).map((root) => rm(root, { force: true, recursive: true })));
});

describe("service worker generation", () => {
	it("precaches only the generated shell assets", async () => {
		const outputRoot = await mkdtemp(join(tmpdir(), "threadhall-service-worker-"));
		temporaryRoots.push(outputRoot);
		await Promise.all([
			mkdir(join(outputRoot, "assets"), { recursive: true }),
			mkdir(join(outputRoot, "icons"), { recursive: true }),
		]);
		await Promise.all([
			writeFile(join(outputRoot, "index.html"), "<!doctype html>"),
			writeFile(join(outputRoot, "manifest.webmanifest"), "{}"),
			writeFile(join(outputRoot, "icons", "threadhall-192.png"), "192"),
			writeFile(join(outputRoot, "icons", "threadhall-512.png"), "512"),
			writeFile(join(outputRoot, "icons", "threadhall.svg"), "svg"),
			writeFile(join(outputRoot, "assets", "index-abc123.js"), "js"),
			writeFile(join(outputRoot, "assets", "index-def456.css"), "css"),
		]);

		const generator = await import(pathToFileURL(resolve(process.cwd(), "scripts/write-service-worker.mjs")).href);
		const result = await generator.writeServiceWorker({ outputRoot });
		const worker = await readFile(join(outputRoot, "sw.js"), "utf8");

		expect(result.shellAssets).toEqual([
			"/",
			"/index.html",
			"/manifest.webmanifest",
			"/icons/threadhall-192.png",
			"/icons/threadhall-512.png",
			"/icons/threadhall.svg",
			"/assets/index-abc123.js",
			"/assets/index-def456.css",
		]);
		expect(new Set(result.shellAssets).size).toBe(result.shellAssets.length);
		expect(worker).toContain(JSON.stringify(result.shellAssets));
		expect(result.cacheName).toMatch(/^threadhall-shell-[a-f0-9]{12}$/);
		expect(worker).toContain(`const CACHE_NAME = ${JSON.stringify(result.cacheName)};`);
		expect(worker).toContain('request.method !== "GET"');
		expect(worker).toContain("url.origin !== self.location.origin");
		expect(worker).toContain("SHELL_ASSET_SET.has(url.pathname)");
		expect(worker).toContain("event.respondWith(fetch(request));");
		expect(worker).toContain('Object.keys(data).length === 1 && data.type === "SKIP_WAITING"');

		for (const unsafePath of ["/api/v1/session", "/api/v1/realtime", "/healthz", "/unknown-navigation"]) {
			expect(result.shellAssets).not.toContain(unsafePath);
		}
		for (const method of ["POST", "PATCH", "DELETE"]) {
			expect(result.shellAssets).not.toContain(method);
		}
	});
});

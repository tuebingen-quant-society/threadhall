import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const manifestPath = resolve(process.cwd(), "public/manifest.webmanifest");

describe("Threadhall install manifest", () => {
	it("declares the standalone app and install icons", async () => {
		const manifest = JSON.parse(await readFile(manifestPath, "utf8"));

		expect(manifest).toMatchObject({
			name: "Threadhall",
			short_name: "Threadhall",
			start_url: "/",
			scope: "/",
			display: "standalone",
			theme_color: "#171717",
			background_color: "#ffffff",
		});
		expect(manifest.icons).toEqual(expect.arrayContaining([
			{ src: "/icons/threadhall-192.png", sizes: "192x192", type: "image/png" },
			{ src: "/icons/threadhall-512.png", sizes: "512x512", type: "image/png", purpose: "any maskable" },
		]));
	});
});

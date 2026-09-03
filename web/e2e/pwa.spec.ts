import { expect, test } from "@playwright/test";

test("installs the built shell and keeps offline state authoritative", async ({ context, page }) => {
	await page.goto("/");

	const manifestLink = page.locator('link[rel="manifest"]');
	await expect(manifestLink).toHaveAttribute("href", "/manifest.webmanifest");
	const manifest = await page.evaluate(async () => {
		const link = document.querySelector<HTMLLinkElement>('link[rel="manifest"]');
		if (link === null) throw new Error("manifest link is missing");
		return fetch(link.href).then((response) => response.json());
	});
	expect(manifest).toMatchObject({ name: "Threadhall", display: "standalone", start_url: "/", scope: "/" });

	await page.waitForFunction(async () => (await navigator.serviceWorker.ready).active !== null);
	await page.reload();
	await expect.poll(() => page.evaluate(() => navigator.serviceWorker.controller?.scriptURL ?? ""))
		.toMatch(/\/sw\.js$/);

	await page.getByLabel("Username").fill("pwa-admin");
	await page.getByLabel("Password").fill("threadhall-e2e-password");
	await page.getByRole("button", { name: "Sign in", exact: true }).click();
	await expect(page.locator(".connection-status")).toHaveText("Live");
	expect(await page.evaluate(() => ({ width: innerWidth, scrollWidth: document.documentElement.scrollWidth })))
		.toEqual({ width: 390, scrollWidth: 390 });

	const updateURL = `/sw.js?e2e-update=${Date.now()}`;
	await page.evaluate((url) => navigator.serviceWorker.register(url, { scope: "/" }), updateURL);
	await page.waitForFunction(async () => {
		const registration = await navigator.serviceWorker.getRegistration("/");
		return registration?.waiting?.scriptURL.includes("e2e-update=") === true;
	});
	await expect(page.getByText("A new version of Threadhall is ready.")).toBeVisible();
	await expect(page.getByRole("button", { name: "Update", exact: true })).toBeVisible();
	await page.getByRole("button", { name: "Dismiss" }).click();
	await expect(page.getByText("A new version of Threadhall is ready.")).toBeHidden();

	await context.setOffline(true);
	await expect(page.locator(".connection-status")).toHaveText("Offline");
	const offlineResponse = await page.reload({ waitUntil: "domcontentloaded" });
	expect(offlineResponse?.fromServiceWorker()).toBe(true);
	await expect(page.getByRole("heading", { name: "Threadhall is out of reach." })).toBeVisible();
	await expect(page.getByRole("alert")).toHaveText("Threadhall could not reach the server.");
	await expect(page.getByText("Offline support could not be enabled.")).toHaveCount(0);
	await expect(page.locator(".connection-status")).toHaveCount(0);
});

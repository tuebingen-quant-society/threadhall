const CACHE_NAME = "threadhall-shell-bfea5d982e2f";
const SHELL_ASSETS = ["/","/index.html","/manifest.webmanifest","/icons/threadhall-192.png","/icons/threadhall-512.png","/icons/threadhall.svg","/assets/index-BmXJAqlM.css","/assets/index-LOwB4f3L.js"];
const SHELL_ASSET_SET = new Set(SHELL_ASSETS);

self.addEventListener("install", (event) => {
	event.waitUntil(caches.open(CACHE_NAME).then((cache) => cache.addAll(SHELL_ASSETS)));
});

self.addEventListener("activate", (event) => {
	event.waitUntil(caches.keys().then((names) => Promise.all(
		names
			.filter((name) => name.startsWith("threadhall-shell-") && name !== CACHE_NAME)
			.map((name) => caches.delete(name)),
	)));
});

self.addEventListener("message", (event) => {
	const data = event.data;
	if (data !== null && typeof data === "object" && !Array.isArray(data) && Object.keys(data).length === 1 && data.type === "SKIP_WAITING") {
		self.skipWaiting();
	}
});

self.addEventListener("fetch", (event) => {
	const request = event.request;
	const url = new URL(request.url);
	if (request.method !== "GET" || url.origin !== self.location.origin || !SHELL_ASSET_SET.has(url.pathname)) {
		event.respondWith(fetch(request));
		return;
	}

	event.respondWith(caches.open(CACHE_NAME).then(async (cache) => (await cache.match(request)) ?? fetch(request)));
});

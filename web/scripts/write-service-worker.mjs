import { createHash } from "node:crypto";
import { readdir, readFile, writeFile } from "node:fs/promises";
import { dirname, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const scriptDirectory = dirname(fileURLToPath(import.meta.url));
const defaultOutputRoot = resolve(scriptDirectory, "../../internal/webassets/dist");
const templatePath = resolve(scriptDirectory, "sw-template.js");

async function listFiles(root, directory) {
	const entries = await readdir(directory, { withFileTypes: true });
	const files = await Promise.all(entries.map(async (entry) => {
		const path = resolve(directory, entry.name);
		if (entry.isDirectory()) {
			return listFiles(root, path);
		}
		return entry.isFile() ? ["/" + relative(root, path).replaceAll("\\", "/")] : [];
	}));
	return files.flat().sort();
}

async function filesIn(outputRoot, directory) {
	return listFiles(outputRoot, resolve(outputRoot, directory));
}

export async function writeServiceWorker({ outputRoot = defaultOutputRoot } = {}) {
	const [icons, assets, template] = await Promise.all([
		filesIn(outputRoot, "icons"),
		filesIn(outputRoot, "assets"),
		readFile(templatePath, "utf8"),
	]);
	const shellAssets = ["/", "/index.html", "/manifest.webmanifest", ...icons, ...assets];
	const cacheName = `threadhall-shell-${createHash("sha256").update(JSON.stringify(shellAssets)).digest("hex").slice(0, 12)}`;
	const worker = template
		.replace("__CACHE_NAME__", JSON.stringify(cacheName))
		.replace("__SHELL_ASSETS__", JSON.stringify(shellAssets));

	await writeFile(resolve(outputRoot, "sw.js"), worker);
	return { cacheName, outputPath: resolve(outputRoot, "sw.js"), shellAssets };
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
	await writeServiceWorker();
}

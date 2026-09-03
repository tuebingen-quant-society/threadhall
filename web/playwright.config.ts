import { defineConfig, devices } from "@playwright/test";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("..", import.meta.url));
const e2ePrefix = `/tmp/threadhall-pwa-e2e-${process.pid}`;
const statePath = `${e2ePrefix}.db`;
const serverPath = `${e2ePrefix}-server`;

export default defineConfig({
	testDir: "./e2e",
	fullyParallel: false,
	workers: 1,
	use: {
		baseURL: "http://127.0.0.1:4173",
	},
	projects: [{
		name: "chromium",
		use: {
			...devices["Desktop Chrome"],
			viewport: { width: 390, height: 844 },
		},
	}],
	webServer: {
		cwd: repositoryRoot,
		command: [
			`rm -f ${statePath} ${statePath}-shm ${statePath}-wal ${serverPath}`,
			"npm --prefix web run build",
			`go build -tags sqlite_fts5 -o ${serverPath} ./cmd/threadhall`,
			`printf 'threadhall-e2e-password\\n' | ${serverPath} bootstrap-admin --state-path ${statePath} --username pwa-admin`,
			`exec ${serverPath} serve --addr 127.0.0.1:4173 --state-path ${statePath} --public-url http://127.0.0.1:4173 --writer-queue 32 --read-connections 4`,
		].join(" && "),
		port: 4173,
		reuseExistingServer: false,
		timeout: 120_000,
	},
});

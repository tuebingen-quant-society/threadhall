import preact from "@preact/preset-vite";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [preact()],
  build: {
    outDir: fileURLToPath(new URL("../internal/webassets/dist", import.meta.url)),
    emptyOutDir: true,
  },
  test: {
    environment: "jsdom",
		include: ["src/**/*.test.ts", "src/**/*.test.tsx"],
		setupFiles: ["./src/test/setup.ts"],
  },
});

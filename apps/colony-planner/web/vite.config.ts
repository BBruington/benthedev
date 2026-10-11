import { fileURLToPath, URL } from "node:url";
import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// The Go server (mise run dev builds it with devauth). The proxy keeps the browser's Host header,
// so cookies, CrossOriginProtection and SSE all see one origin. COLONY_DEV_GO_URL points a second
// dev server (`bunx vite --port 5199`) at another Go server without touching the usual pair.
const goServer = process.env.COLONY_DEV_GO_URL ?? "http://127.0.0.1:8080";
const goPrefixes = ["/api", "/auth", "/img", "/calendar", "/healthz"];

export default defineConfig({
	build: {
		emptyOutDir: true,
		// A subfolder, so the tracked dist/.gitkeep survives builds and //go:embed always compiles.
		outDir: "dist/spa",
	},
	plugins: [
		tanstackRouter({ autoCodeSplitting: true, quoteStyle: "double", target: "react" }),
		react(),
		tailwindcss(),
	],
	resolve: {
		alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) },
	},
	server: {
		port: 5173,
		proxy: Object.fromEntries(
			goPrefixes.map((prefix) => [prefix, { changeOrigin: false, target: goServer }]),
		),
		strictPort: true,
	},
});

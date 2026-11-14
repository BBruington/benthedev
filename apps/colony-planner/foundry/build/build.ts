import { mkdir } from "node:fs/promises";
import { join } from "node:path";
import packageJson from "../package.json";
import { createZip } from "./zip";

/**
 * Builds the module into dist/ for Go to embed (foundry/embed.go): dist/module.json, and
 * dist/colony-bridge.zip holding that manifest and the bundled script in the layout Foundry installs
 * as Data/modules/colony-bridge/. The version is package.json's; the server fills manifest and download
 * with its PUBLIC_URL when it serves them (design §12.4), so here they are placeholders.
 */

const foundryRoot = join(import.meta.dir, "..");
const distDirectory = join(foundryRoot, "dist");
const scriptPath = "scripts/colony-bridge.js";

const manifest = {
	compatibility: { minimum: "13", verified: "14" },
	description:
		"Connects this world to a Colony Planner campaign: the colony's summary and new days show up here, and Foundry's clock moves reach the DM on Colony Planner.",
	download: "",
	esmodules: [scriptPath],
	id: "colony-bridge",
	manifest: "",
	title: "Colony Bridge",
	version: packageJson.version,
};

const result = await Bun.build({
	entrypoints: [join(foundryRoot, "src/main.ts")],
	format: "esm",
	target: "browser",
});
const [script] = result.outputs;
if (!(result.success && script)) {
	throw new AggregateError(result.logs, "The module's build failed.");
}

const encoder = new TextEncoder();
const manifestBytes = encoder.encode(`${JSON.stringify(manifest, null, "\t")}\n`);
await mkdir(distDirectory, { recursive: true });
await Bun.write(join(distDirectory, "module.json"), manifestBytes);
await Bun.write(
	join(distDirectory, "colony-bridge.zip"),
	createZip([
		{ data: manifestBytes, name: "module.json" },
		{ data: new Uint8Array(await script.arrayBuffer()), name: scriptPath },
	]),
);

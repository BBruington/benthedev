import type { FoundryModule } from "./foundry-types";

const localHosts = new Set(["localhost", "127.0.0.1", "[::1]"]);

/**
 * Colony Planner's origin, from the manifest URL Foundry installed the module from (design §12.4), so
 * nothing is baked in at build time. Plain http is allowed only for a local server, since the bridge
 * token travels on every request.
 */
export function colonyOrigin(installedModule: FoundryModule | undefined): string | undefined {
	if (!installedModule?.manifest) {
		return undefined;
	}
	let manifestUrl: URL;
	try {
		manifestUrl = new URL(installedModule.manifest);
	} catch {
		return undefined;
	}
	if (manifestUrl.protocol === "https:") {
		return manifestUrl.origin;
	}
	if (manifestUrl.protocol === "http:" && localHosts.has(manifestUrl.hostname)) {
		return manifestUrl.origin;
	}
	return undefined;
}

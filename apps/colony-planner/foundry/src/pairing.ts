import { BridgeHttpError, type Fetch, pair } from "./bridge-client";

export type PairingResult = { campaignName: string; ok: true } | { message: string; ok: false };

const badCodeMessage = "That code didn't work. Make a new one on Colony Planner's Settings.";

/**
 * Trades a pairing code for a bridge token (design §3.1) and stores the link. Every failure becomes a
 * sentence the GM can act on; nothing is stored unless Colony Planner accepted the code.
 */
export async function connectWithCode(options: {
	code: string;
	colonyOrigin: string | undefined;
	fetchFunction?: Fetch;
	saveLink: (link: { campaignName: string; token: string }) => Promise<void>;
}): Promise<PairingResult> {
	const code = options.code.trim().toUpperCase();
	if (code === "") {
		return { message: "Type the code from Colony Planner's Settings.", ok: false };
	}
	if (!options.colonyOrigin) {
		return {
			message:
				"Colony Bridge doesn't know Colony Planner's address. Install the module with the manifest link on Colony Planner's Settings.",
			ok: false,
		};
	}
	try {
		const link = await pair(options.colonyOrigin, code, options.fetchFunction);
		await options.saveLink(link);
		return { campaignName: link.campaignName, ok: true };
	} catch (error) {
		return { message: pairingFailureMessage(error, options.colonyOrigin), ok: false };
	}
}

function pairingFailureMessage(error: unknown, colonyOrigin: string): string {
	if (error instanceof BridgeHttpError) {
		switch (error.status) {
			case 400:
				return error.detail ?? badCodeMessage;
			case 403:
				return "Colony Planner accepts a connection only from a Foundry world on The Forge.";
			case 429:
				return "Too many tries. Wait a minute, then try again.";
			default:
				return `Colony Planner couldn't connect this world (error ${error.status}). Try again in a moment.`;
		}
	}
	// fetch rejects with a TypeError when the request never got an answer: offline, DNS, or CORS.
	if (error instanceof TypeError) {
		return `Couldn't reach Colony Planner at ${colonyOrigin}. Check the connection and try again.`;
	}
	return error instanceof Error ? error.message : String(error);
}

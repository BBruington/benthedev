import { describe, expect, test } from "bun:test";
import type { Fetch } from "./bridge-client";
import { connectWithCode } from "./pairing";
import { jsonResponse } from "./testing/fakes";

const origin = "https://colony.benthedev.com";

function attempt(code: string, fetchFunction: Fetch, colonyOrigin: string | undefined = origin) {
	const saved: { campaignName: string; token: string }[] = [];
	const result = connectWithCode({
		code,
		colonyOrigin,
		fetchFunction,
		saveLink: (link) => {
			saved.push(link);
			return Promise.resolve();
		},
	});
	return { result, saved };
}

const answer =
	(status: number, body: unknown): Fetch =>
	() =>
		Promise.resolve(jsonResponse(status, body));

describe("connectWithCode", () => {
	test("posts the code as JSON and stores the link", async () => {
		const requests: { init: RequestInit; url: string }[] = [];
		const fetchFunction: Fetch = (url, init) => {
			requests.push({ init, url });
			return Promise.resolve(jsonResponse(200, { campaignName: "Port Eldari", token: "secret" }));
		};
		const { result, saved } = attempt("  k7q-4xm ", fetchFunction);
		expect(await result).toEqual({ campaignName: "Port Eldari", ok: true });
		expect(saved).toEqual([{ campaignName: "Port Eldari", token: "secret" }]);
		expect(requests).toHaveLength(1);
		expect(requests[0]?.url).toBe(`${origin}/api/bridge/pair`);
		expect(requests[0]?.init.method).toBe("POST");
		expect(requests[0]?.init.body).toBe(JSON.stringify({ code: "K7Q-4XM" }));
	});

	test("an empty code or a missing address never reaches the server", async () => {
		const refuse: Fetch = () => Promise.reject(new Error("should not be called"));
		expect(await attempt("   ", refuse).result).toEqual({
			message: "Type the code from Colony Planner's Settings.",
			ok: false,
		});
		const { result } = attempt("K7Q-4XM", refuse, undefined);
		expect((await result).ok).toBe(false);
	});

	test("a bad code shows the server's sentence, or the standard one", async () => {
		const serverSentence = "That code didn't work. Make a new one on Colony Planner's Settings.";
		const withDetail = attempt("K7Q-4XM", answer(400, { detail: serverSentence, status: 400 }));
		expect(await withDetail.result).toEqual({ message: serverSentence, ok: false });
		expect(withDetail.saved).toEqual([]);
		const withoutBody: Fetch = () => Promise.resolve(new Response("", { status: 400 }));
		expect(await attempt("K7Q-4XM", withoutBody).result).toEqual({
			message: serverSentence,
			ok: false,
		});
	});

	test("a refused origin, a rate limit, an outage and no network each say what to do", async () => {
		const messageFor = async (fetchFunction: Fetch) => {
			const outcome = await attempt("K7Q-4XM", fetchFunction).result;
			return outcome.ok ? "" : outcome.message;
		};
		expect(await messageFor(answer(403, { title: "Forbidden" }))).toBe(
			"Colony Planner accepts a connection only from a Foundry world on The Forge.",
		);
		expect(await messageFor(answer(429, {}))).toBe(
			"Too many tries. Wait a minute, then try again.",
		);
		expect(await messageFor(answer(503, {}))).toBe(
			"Colony Planner couldn't connect this world (error 503). Try again in a moment.",
		);
		expect(await messageFor(() => Promise.reject(new TypeError("Failed to fetch")))).toBe(
			`Couldn't reach Colony Planner at ${origin}. Check the connection and try again.`,
		);
	});

	test("an answer without a token is an error, and nothing is stored", async () => {
		const { result, saved } = attempt("K7Q-4XM", answer(200, { campaignName: "Port Eldari" }));
		expect(await result).toEqual({
			message: "Colony Planner's answer to the pairing code wasn't understood.",
			ok: false,
		});
		expect(saved).toEqual([]);
	});
});

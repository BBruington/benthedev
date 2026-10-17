// The only web code that knows the transport. Screens use these clients and the generated types,
// so a switch of API toolchain changes this folder and not the screens (spec §8.6).
import createFetchClient, { type Middleware } from "openapi-fetch";
import createQueryClient from "openapi-react-query";
import { problemResponse } from "./problem";
import type { components, paths } from "./schema";

// Every route answers errors with problem+json, but a proxy in between (Vite in development, a
// host's edge later) or a dropped connection may not. Rewriting those into the same shape means
// every failed query and mutation rejects with a problem the screens can read.
const problemMiddleware: Middleware = {
	onError({ error }) {
		if (error instanceof DOMException && error.name === "AbortError") {
			return error;
		}
		return problemResponse(503, "Can't reach the colony. Check your connection and try again.");
	},
	onResponse({ response }) {
		if (response.ok) {
			return undefined;
		}
		const contentType = response.headers.get("Content-Type") ?? "";
		if (contentType.includes("application/problem+json")) {
			return undefined;
		}
		return problemResponse(response.status, fallbackDetail(response.status));
	},
};

function fallbackDetail(status: number): string {
	if (status === 502 || status === 503 || status === 504) {
		return "The colony server isn't answering right now. Try again in a moment.";
	}
	return "Something went wrong. Try again.";
}

// Same origin everywhere: Vite proxies /api in development and the Go binary serves both in a build.
export const fetchClient = createFetchClient<paths>({ credentials: "same-origin" });
fetchClient.use(problemMiddleware);

export const api = createQueryClient(fetchClient);

export type Schemas = components["schemas"];

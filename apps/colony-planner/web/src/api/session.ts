import { ProblemError, problemResponse } from "./problem";

// Sign-in and sign-out are plain routes outside the OpenAPI document (spec §8.5): sign-in is a
// full-page navigation that ends with a session cookie, and sign-out is a body-less POST.

export const discordSignInPath = "/auth/discord/login";

/**
 * Dev sign-in, devauth builds only. as is owner, dm, player, pending or an account id; "new" with a name signs
 * in a brand-new person the way a first Discord sign-in would.
 */
export function devSignInPath(as: string, name?: string): string {
	const search = new URLSearchParams({ as });
	if (name) {
		search.set("name", name);
	}
	return `/auth/dev?${search.toString()}`;
}

export type SignOutScope = "this-device" | "everywhere";

/** Ends the session (or every session); an already-ended session counts as signed out. */
export async function signOut(scope: SignOutScope): Promise<void> {
	const path = scope === "everywhere" ? "/auth/logout-everywhere" : "/auth/logout";
	let response: Response;
	try {
		response = await fetch(path, { credentials: "same-origin", method: "POST" });
	} catch {
		response = problemResponse(503, "Can't reach the colony. Check your connection and try again.");
	}
	if (response.ok || response.status === 401) {
		return;
	}
	throw new ProblemError(await readProblem(response));
}

async function readProblem(response: Response) {
	const fallback = {
		detail: "Signing out didn't work. Try again.",
		status: response.status,
		title: "Sign-out failed",
		type: "about:blank",
	};
	try {
		const body: unknown = await response.json();
		if (typeof body === "object" && body !== null && "detail" in body) {
			return { ...fallback, detail: String(body.detail) };
		}
	} catch {
		return fallback;
	}
	return fallback;
}

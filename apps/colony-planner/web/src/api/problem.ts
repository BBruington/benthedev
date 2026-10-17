import type { components } from "./schema";

export type Problem = components["schemas"]["ErrorModel"];

const fieldPathSeparator = /[.[]/;

/** A problem as a throwable Error, for transport calls made outside openapi-fetch. */
export class ProblemError extends Error {
	readonly problem: Problem;

	constructor(problem: Problem) {
		super(problem.detail ?? problem.title ?? "Request failed");
		this.name = "ProblemError";
		this.problem = problem;
	}
}

export function isProblem(value: unknown): value is Problem {
	return (
		typeof value === "object" &&
		value !== null &&
		"status" in value &&
		typeof value.status === "number"
	);
}

export function problemResponse(status: number, detail: string): Response {
	const body: Problem = { detail, status, title: statusTitle(status), type: "about:blank" };
	return new Response(JSON.stringify(body), {
		headers: { "Content-Type": "application/problem+json" },
		status,
	});
}

function statusTitle(status: number): string {
	if (status === 503) {
		return "Service Unavailable";
	}
	if (status >= 500) {
		return "Server Error";
	}
	return "Request Failed";
}

function toProblem(error: unknown): Problem | undefined {
	if (error instanceof ProblemError) {
		return error.problem;
	}
	if (isProblem(error)) {
		return error;
	}
	return undefined;
}

export function problemStatus(error: unknown): number | undefined {
	return toProblem(error)?.status;
}

export function isUnauthorized(error: unknown): boolean {
	return problemStatus(error) === 401;
}

/** The sentence to show a person for a failed call: the server's detail when it gave one. */
export function problemMessage(error: unknown, fallback: string): string {
	const problem = toProblem(error);
	if (!problem) {
		return fallback;
	}
	if (problem.detail) {
		return problem.detail;
	}
	return problem.title ?? fallback;
}

/**
 * Field errors from a 422, keyed by the top-level body field ("body.timezone" → "timezone"), so a
 * form can show each message under its input. The server is the only validator (spec §8.3).
 */
export function fieldErrors(error: unknown): Record<string, string> {
	const messages: Record<string, string> = {};
	const problem = toProblem(error);
	if (problem?.status !== 422) {
		return messages;
	}
	for (const detail of problem.errors ?? []) {
		const field = fieldFromLocation(detail.location);
		if (field && detail.message && !(field in messages)) {
			messages[field] = sentenceCase(detail.message);
		}
	}
	return messages;
}

function fieldFromLocation(location: string | undefined): string | undefined {
	if (!location?.startsWith("body.")) {
		return undefined;
	}
	const path = location.slice("body.".length);
	const [field] = path.split(fieldPathSeparator, 1);
	return field;
}

function sentenceCase(message: string): string {
	return message.charAt(0).toUpperCase() + message.slice(1);
}

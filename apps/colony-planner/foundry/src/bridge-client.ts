import {
	type Acknowledgement,
	type InboundEvent,
	isOutboxPage,
	isPairResponse,
	type OutboxPage,
	type PairResponse,
	readField,
} from "./wire";

/** A bridge request answered with a status other than success. */
export class BridgeHttpError extends Error {
	readonly detail: string | undefined;
	readonly status: number;

	constructor(status: number, detail: string | undefined) {
		super(detail ?? `Colony Planner answered ${status}.`);
		this.name = "BridgeHttpError";
		this.status = status;
		this.detail = detail;
	}
}

/** The token was refused (§12.2's 401): the link is gone, and only a new pairing code brings it back. */
export class UnauthorizedError extends Error {
	constructor() {
		super("Colony Planner no longer accepts this world's bridge token.");
		this.name = "UnauthorizedError";
	}
}

/** The one place the module talks to Colony Planner (design §12.2). */
export interface BridgeClient {
	acknowledge(acknowledgement: Acknowledgement): Promise<void>;
	outbox(after: number): Promise<OutboxPage>;
	sendEvents(events: InboundEvent[]): Promise<void>;
	stream(signal: AbortSignal): Promise<ReadableStream<Uint8Array>>;
}

export type Fetch = (input: string, init: RequestInit) => Promise<Response>;

// Every request but the stream gets this long, its answer's body included, so one stalled across a
// laptop's sleep ends and its caller can try again.
const defaultRequestTimeoutInMilliseconds = 30_000;

export async function pair(
	origin: string,
	code: string,
	fetchFunction: Fetch = fetch,
): Promise<PairResponse> {
	const response = await fetchFunction(`${origin}/api/bridge/pair`, {
		body: JSON.stringify({ code }),
		headers: { "Content-Type": "application/json" },
		method: "POST",
	});
	if (!response.ok) {
		throw new BridgeHttpError(response.status, await readProblemDetail(response));
	}
	const body: unknown = await response.json();
	if (!isPairResponse(body)) {
		throw new Error("Colony Planner's answer to the pairing code wasn't understood.");
	}
	return body;
}

export function createBridgeClient(options: {
	fetchFunction?: Fetch;
	onUnauthorized: () => void;
	origin: string;
	requestTimeoutInMilliseconds?: number;
	/** Aborted when the bridge session ends: requests then stop, and a late 401 changes nothing. */
	signal: AbortSignal;
	token: string;
}): BridgeClient {
	const fetchFunction = options.fetchFunction ?? fetch;
	const requestTimeout =
		options.requestTimeoutInMilliseconds ?? defaultRequestTimeoutInMilliseconds;

	/** The stream passes its own signal, since it lives far longer than any other request. */
	async function request(path: string, init: RequestInit): Promise<Response> {
		const headers = new Headers(init.headers);
		headers.set("Authorization", `Bearer ${options.token}`);
		const response = await fetchFunction(`${options.origin}${path}`, {
			...init,
			headers,
			signal: init.signal ?? AbortSignal.any([options.signal, AbortSignal.timeout(requestTimeout)]),
		});
		if (response.status === 401) {
			if (!options.signal.aborted) {
				options.onUnauthorized();
			}
			throw new UnauthorizedError();
		}
		if (!response.ok) {
			throw new BridgeHttpError(response.status, await readProblemDetail(response));
		}
		return response;
	}

	function postJson(path: string, body: unknown): Promise<Response> {
		return request(path, {
			body: JSON.stringify(body),
			headers: { "Content-Type": "application/json" },
			method: "POST",
		});
	}

	return {
		async acknowledge(acknowledgement) {
			await postJson("/api/bridge/acks", acknowledgement);
		},
		async outbox(after) {
			const response = await request(`/api/bridge/outbox?after=${after}`, { method: "GET" });
			const body: unknown = await response.json();
			if (!isOutboxPage(body)) {
				throw new Error("Colony Planner's outbox page wasn't understood.");
			}
			return body;
		},
		async sendEvents(events) {
			await postJson("/api/bridge/events", { events });
		},
		async stream(signal) {
			const response = await request("/api/bridge/stream", {
				headers: { Accept: "text/event-stream" },
				method: "GET",
				signal,
			});
			if (!response.body) {
				throw new Error("Colony Planner's stream had no body.");
			}
			return response.body;
		},
	};
}

/** huma's problem body carries the person-readable message in `detail`. */
async function readProblemDetail(response: Response): Promise<string | undefined> {
	try {
		const detail = readField(await response.json(), "detail");
		return typeof detail === "string" && detail !== "" ? detail : undefined;
	} catch {
		return undefined;
	}
}

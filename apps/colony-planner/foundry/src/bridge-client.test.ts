import { describe, expect, test } from "bun:test";
import { createBridgeClient, type Fetch, UnauthorizedError } from "./bridge-client";
import { createEventQueue } from "./event-queue";
import { createFakeSchedule, jsonResponse } from "./testing/fakes";
import type { InboundEvent } from "./wire";

const origin = "https://colony.benthedev.com";
const requestTimeoutInMilliseconds = 20;

/** Never answers, as a request stalled across a laptop's sleep, until its signal aborts it. */
function stalled(signal: AbortSignal | null | undefined): Promise<Response> {
	return new Promise((_resolve, reject) => {
		signal?.addEventListener("abort", () => reject(signal.reason));
	});
}

function clientWith(fetchFunction: Fetch, session = new AbortController()) {
	const unauthorized: number[] = [];
	const client = createBridgeClient({
		fetchFunction,
		onUnauthorized: () => unauthorized.push(1),
		origin,
		requestTimeoutInMilliseconds,
		signal: session.signal,
		token: "secret-token",
	});
	return { client, session, unauthorized };
}

describe("createBridgeClient", () => {
	test("a stalled request ends at its deadline, and the event queue can send again", async () => {
		const sent: InboundEvent[][] = [];
		let requests = 0;
		const fetchFunction: Fetch = (_url, init) => {
			requests += 1;
			if (requests === 1) {
				return stalled(init.signal);
			}
			sent.push(JSON.parse(String(init.body)).events);
			return Promise.resolve(jsonResponse(200, { accepted: 1, duplicates: 0 }));
		};
		const { client } = clientWith(fetchFunction);
		const storage = { events: [] as InboundEvent[] };
		const fakeSchedule = createFakeSchedule();
		const queue = createEventQueue({
			onDropped: () => undefined,
			onError: (error) => {
				throw error;
			},
			schedule: fakeSchedule.schedule,
			send: client.sendEvents,
			storage: {
				load: () => storage.events,
				save: (events) => {
					storage.events = events;
					return Promise.resolve();
				},
			},
		});
		queue.enqueue("TIME_ADVANCED", { days: 2 });
		await Bun.sleep(requestTimeoutInMilliseconds * 3);
		expect(fakeSchedule.waitingDelays()).toHaveLength(1);
		expect(storage.events).toHaveLength(1);
		queue.retryNow();
		await Bun.sleep(5);
		expect(requests).toBe(2);
		expect(sent.map((batch) => batch.map((event) => event.payload))).toEqual([[{ days: 2 }]]);
		expect(storage.events).toEqual([]);
	});

	test("the deadline covers reading the answer's body", async () => {
		const fetchFunction: Fetch = (_url, init) => {
			const body = new ReadableStream<Uint8Array>({
				start(controller) {
					init.signal?.addEventListener("abort", () => controller.error(init.signal?.reason));
				},
			});
			return Promise.resolve(new Response(body, { status: 200 }));
		};
		const { client } = clientWith(fetchFunction);
		await expect(client.outbox(0)).rejects.toThrow();
	});

	test("the stream keeps its own signal and no deadline", async () => {
		let streamSignal: AbortSignal | null | undefined;
		const fetchFunction: Fetch = (_url, init) => {
			streamSignal = init.signal;
			return Promise.resolve(new Response(new ReadableStream(), { status: 200 }));
		};
		const { client } = clientWith(fetchFunction);
		const connection = new AbortController();
		await client.stream(connection.signal);
		await Bun.sleep(requestTimeoutInMilliseconds * 3);
		expect(streamSignal).toBe(connection.signal);
		expect(streamSignal?.aborted).toBe(false);
	});

	test("a 401 from any request tells the session its link is gone, unless the session has ended", async () => {
		const refuse: Fetch = () => Promise.resolve(jsonResponse(401, { title: "Unauthorized" }));
		const { client, session, unauthorized } = clientWith(refuse);
		const calls = [
			() => client.acknowledge({ failed: [], through: 1 }),
			() => client.outbox(0),
			() => client.sendEvents([]),
			() => client.stream(new AbortController().signal),
		];
		await Promise.all(
			calls.map((call) => expect(call()).rejects.toBeInstanceOf(UnauthorizedError)),
		);
		expect(unauthorized).toHaveLength(calls.length);
		session.abort();
		await expect(client.outbox(0)).rejects.toBeInstanceOf(UnauthorizedError);
		expect(unauthorized).toHaveLength(calls.length);
	});
});

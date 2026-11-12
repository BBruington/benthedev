import { describe, expect, test } from "bun:test";
import { UnauthorizedError } from "./bridge-client";
import { type ConnectionState, startStreamConnection } from "./stream-connection";
import { createFakeSchedule, settle } from "./testing/fakes";

const idleTimeoutInMilliseconds = 60_000;

interface FakeStream {
	close(): void;
	send(text: string): void;
}

function connectionWith(openings: ((signal: AbortSignal) => Promise<void>)[]) {
	const fakeSchedule = createFakeSchedule();
	const states: ConnectionState[] = [];
	const streams: FakeStream[] = [];
	let connects = 0;
	let outboxSignals = 0;
	let clock = 0;
	const encoder = new TextEncoder();

	function open(signal: AbortSignal): Promise<ReadableStream<Uint8Array>> {
		const nextOpening = openings.shift();
		if (!nextOpening) {
			return Promise.reject(new Error("no more openings"));
		}
		return nextOpening(signal).then(() => {
			let streamController: ReadableStreamDefaultController<Uint8Array> | undefined;
			const body = new ReadableStream<Uint8Array>({
				start(controller) {
					streamController = controller;
				},
			});
			signal.addEventListener("abort", () => streamController?.error(signal.reason));
			streams.push({
				close: () => streamController?.close(),
				send: (text) => streamController?.enqueue(encoder.encode(text)),
			});
			return body;
		});
	}

	const connection = startStreamConnection({
		idleTimeoutInMilliseconds,
		now: () => clock,
		onConnected: () => {
			connects += 1;
		},
		onOutbox: () => {
			outboxSignals += 1;
		},
		onStateChange: (state) => states.push(state),
		open,
		schedule: fakeSchedule.schedule,
	});
	return {
		advanceClock: (milliseconds: number) => {
			clock += milliseconds;
		},
		connection,
		counts: () => ({ connects, outboxSignals }),
		fakeSchedule,
		states,
		streams,
	};
}

const opens = () => Promise.resolve();
const fails = () => Promise.reject(new TypeError("Failed to fetch"));

describe("startStreamConnection", () => {
	test("goes live, signals each outbox event and ignores pings", async () => {
		const { counts, states, streams } = connectionWith([opens]);
		await settle();
		streams[0]?.send(": ping\n\n");
		streams[0]?.send('event: outbox\ndata: {"seq": 4}\n\n');
		await settle();
		expect(states).toEqual(["connecting", "live"]);
		expect(counts()).toEqual({ connects: 1, outboxSignals: 1 });
	});

	test("backs off 1, 2 and 4 seconds while the server can't be reached, then resets", async () => {
		const { advanceClock, counts, fakeSchedule, streams } = connectionWith([
			fails,
			fails,
			fails,
			opens,
			opens,
		]);
		await settle();
		expect(fakeSchedule.runNext()).toBe(1000);
		await settle();
		expect(fakeSchedule.runNext()).toBe(2000);
		await settle();
		expect(fakeSchedule.runNext()).toBe(4000);
		await settle();
		expect(counts().connects).toBe(1);
		// The server rotates the stream: a clean end reconnects after the first delay again.
		advanceClock(10 * 60_000);
		streams[0]?.close();
		await settle();
		expect(fakeSchedule.waitingDelays()).toEqual([1000]);
		fakeSchedule.runNext();
		await settle();
		expect(counts().connects).toBe(2);
	});

	test("a server that answers and hangs up at once is retried with backoff", async () => {
		const { fakeSchedule, streams } = connectionWith([opens, opens, opens]);
		await settle();
		streams[0]?.close();
		await settle();
		expect(fakeSchedule.runNext()).toBe(1000);
		await settle();
		streams[1]?.close();
		await settle();
		expect(fakeSchedule.waitingDelays()).toEqual([2000]);
	});

	test("an answer that never comes is given up after the idle timeout", async () => {
		const { counts, fakeSchedule, states } = connectionWith([
			(signal) =>
				new Promise<void>((_resolve, reject) => {
					signal.addEventListener("abort", () => reject(signal.reason));
				}),
		]);
		await settle();
		expect(fakeSchedule.runNext()).toBe(idleTimeoutInMilliseconds);
		await settle();
		expect(counts().connects).toBe(0);
		expect(states.at(-1)).toBe("reconnecting");
	});

	test("never waits longer than 60 seconds", async () => {
		const { fakeSchedule } = connectionWith(Array.from({ length: 10 }, () => fails));
		await settle();
		const delays = await Array.from({ length: 9 }).reduce<Promise<number[]>>(async (previous) => {
			const runDelays = await previous;
			runDelays.push(fakeSchedule.runNext());
			await settle();
			return runDelays;
		}, Promise.resolve([]));
		expect(delays).toEqual([1000, 2000, 4000, 8000, 16_000, 32_000, 60_000, 60_000, 60_000]);
	});

	test("a refused token stops it for good", async () => {
		const { fakeSchedule, states } = connectionWith([
			() => Promise.reject(new UnauthorizedError()),
		]);
		await settle();
		expect(states).toEqual(["connecting", "stopped"]);
		expect(fakeSchedule.waitingDelays()).toEqual([]);
	});

	test("a stream silent past the idle timeout is dropped and reconnected", async () => {
		const { counts, fakeSchedule, states } = connectionWith([opens, opens]);
		await settle();
		expect(fakeSchedule.runNext()).toBe(idleTimeoutInMilliseconds);
		await settle();
		expect(states.at(-1)).toBe("reconnecting");
		fakeSchedule.runNext();
		await settle();
		expect(counts().connects).toBe(2);
	});

	test("stop closes the stream and schedules nothing", async () => {
		const { connection, fakeSchedule, states } = connectionWith([opens]);
		await settle();
		connection.stop();
		await settle();
		expect(states.at(-1)).toBe("stopped");
		expect(fakeSchedule.waitingDelays()).toEqual([]);
	});
});

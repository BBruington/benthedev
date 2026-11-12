import { describe, expect, test } from "bun:test";
import { BridgeHttpError, UnauthorizedError } from "./bridge-client";
import { createEventQueue } from "./event-queue";
import { createFakeSchedule, settle } from "./testing/fakes";
import type { InboundEvent } from "./wire";

function queueWith(send: (events: InboundEvent[]) => Promise<void>, stored: InboundEvent[] = []) {
	const fakeSchedule = createFakeSchedule();
	const storage = { events: stored };
	const dropped: string[] = [];
	let nextId = 0;
	const queue = createEventQueue({
		createId: () => {
			nextId += 1;
			return `event-${nextId}`;
		},
		now: () => new Date("2026-11-12T10:00:00Z"),
		onDropped: (reason) => dropped.push(reason),
		onError: (error) => {
			throw error;
		},
		schedule: fakeSchedule.schedule,
		send,
		storage: {
			load: () => storage.events,
			save: (events) => {
				storage.events = events;
				return Promise.resolve();
			},
		},
	});
	return { dropped, fakeSchedule, queue, storage };
}

describe("createEventQueue", () => {
	test("sends an event with its own id and time, and forgets it once accepted", async () => {
		const sent: InboundEvent[][] = [];
		const { queue, storage } = queueWith((events) => {
			sent.push(events);
			return Promise.resolve();
		});
		queue.enqueue("TIME_ADVANCED", { days: 3 });
		await settle();
		expect(sent).toEqual([
			[
				{
					id: "event-1",
					kind: "TIME_ADVANCED",
					occurredAt: "2026-11-12T10:00:00.000Z",
					payload: { days: 3 },
				},
			],
		]);
		expect(storage.events).toEqual([]);
	});

	test("keeps events through failures and retries them with backoff until accepted", async () => {
		let failuresLeft = 2;
		const sent: string[][] = [];
		const { fakeSchedule, queue, storage } = queueWith((events) => {
			sent.push(events.map((event) => event.id));
			if (failuresLeft > 0) {
				failuresLeft -= 1;
				return Promise.reject(new BridgeHttpError(503, undefined));
			}
			return Promise.resolve();
		});
		queue.enqueue("TIME_ADVANCED", { days: 1 });
		await settle();
		expect(storage.events).toHaveLength(1);
		expect(fakeSchedule.runNext()).toBe(1000);
		await settle();
		expect(fakeSchedule.runNext()).toBe(2000);
		await settle();
		expect(sent).toEqual([["event-1"], ["event-1"], ["event-1"]]);
		expect(storage.events).toEqual([]);
		expect(fakeSchedule.waitingDelays()).toEqual([]);
	});

	test("sends events stored before a reload, at most 100 at a time", async () => {
		const stored: InboundEvent[] = Array.from({ length: 150 }, (_, index) => ({
			id: `stored-${index}`,
			kind: "TIME_ADVANCED",
			occurredAt: "2026-11-12T09:00:00.000Z",
			payload: { days: 1 },
		}));
		const batchSizes: number[] = [];
		const { queue, storage } = queueWith((events) => {
			batchSizes.push(events.length);
			return Promise.resolve();
		}, stored);
		queue.flush();
		await settle();
		expect(batchSizes).toEqual([100, 50]);
		expect(storage.events).toEqual([]);
	});

	test("a batch refused for good is dropped and reported, not retried", async () => {
		const { dropped, fakeSchedule, queue, storage } = queueWith(() =>
			Promise.reject(new BridgeHttpError(422, "days must be at most 3650")),
		);
		queue.enqueue("TIME_ADVANCED", { days: 9999 });
		await settle();
		expect(dropped).toEqual(["days must be at most 3650"]);
		expect(storage.events).toEqual([]);
		expect(fakeSchedule.waitingDelays()).toEqual([]);
	});

	test("a refused token stops sending, and the queue stores nothing more", async () => {
		let attempts = 0;
		const { fakeSchedule, queue, storage } = queueWith(() => {
			attempts += 1;
			return Promise.reject(new UnauthorizedError());
		});
		queue.enqueue("TIME_ADVANCED", { days: 1 });
		await settle();
		queue.enqueue("TIME_ADVANCED", { days: 2 });
		await settle();
		expect(attempts).toBe(1);
		expect(storage.events).toHaveLength(1);
		expect(fakeSchedule.waitingDelays()).toEqual([]);
	});

	test("retryNow sends at once when the connection comes back", async () => {
		let failuresLeft = 1;
		const { fakeSchedule, queue, storage } = queueWith(() => {
			if (failuresLeft > 0) {
				failuresLeft -= 1;
				return Promise.reject(new TypeError("offline"));
			}
			return Promise.resolve();
		});
		queue.enqueue("TIME_ADVANCED", { days: 1 });
		await settle();
		expect(fakeSchedule.waitingDelays()).toEqual([1000]);
		queue.retryNow();
		await settle();
		expect(storage.events).toEqual([]);
		expect(fakeSchedule.waitingDelays()).toEqual([]);
	});

	test("stop cancels a waiting retry", async () => {
		const { fakeSchedule, queue } = queueWith(() => Promise.reject(new TypeError("offline")));
		queue.enqueue("TIME_ADVANCED", { days: 1 });
		await settle();
		expect(fakeSchedule.waitingDelays()).toEqual([1000]);
		queue.stop();
		expect(fakeSchedule.waitingDelays()).toEqual([]);
	});
});

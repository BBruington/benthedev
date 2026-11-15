import { describe, expect, test } from "bun:test";
import { BridgeHttpError, UnauthorizedError } from "./bridge-client";
import { createEventQueue } from "./event-queue";
import { createFakeSchedule, settle } from "./testing/fakes";
import type { InboundEvent } from "./wire";

function queueWith(
	send: (events: InboundEvent[]) => Promise<void>,
	stored: InboundEvent[] = [],
	maySend?: (event: InboundEvent) => boolean,
) {
	const fakeSchedule = createFakeSchedule();
	const storage = { events: stored };
	const dropped: string[] = [];
	const droppedEvents: InboundEvent[][] = [];
	const sentBatches: string[][] = [];
	let nextId = 0;
	const queue = createEventQueue({
		createId: () => {
			nextId += 1;
			return `event-${nextId}`;
		},
		maySend,
		now: () => new Date("2026-11-12T10:00:00Z"),
		onDropped: (reason, events) => {
			dropped.push(reason);
			droppedEvents.push(events);
		},
		onError: (error) => {
			throw error;
		},
		onSent: (events) => sentBatches.push(events.map((event) => event.id)),
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
	return { dropped, droppedEvents, fakeSchedule, queue, sentBatches, storage };
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

	function storedEvent(id: string, kind: InboundEvent["kind"]): InboundEvent {
		const payloads: Record<InboundEvent["kind"], InboundEvent["payload"]> = {
			COIN_DEPOSIT: { actorId: "abcdefghij123456", actorName: "The Wayfarers", gold: 40 },
			ITEM_IMPORT: {
				compendiumUuid: null,
				description: "",
				foundryUuid: "Item.abcdefghij123456",
				name: "Rope",
				priceGold: 1,
			},
			NPC_IMPORT: {
				appearance: "",
				characterClass: "",
				dmNotes: "",
				foundryUuid: "Actor.abcdefghij123456",
				level: 3,
				name: "Brother Aldo",
				race: "",
			},
			STATUS: {
				foundryVersion: "13.351",
				moduleVersion: "0.3.0",
				systemId: "pf2e",
				systemVersion: "7.12.2",
				worldTitle: "World",
			},
			TIME_ADVANCED: { days: 1 },
		};
		return { id, kind, occurredAt: "2026-11-14T09:00:00.000Z", payload: payloads[kind] };
	}

	test("a deposit or an import goes in a batch of its own, and other events batch around it", async () => {
		const stored = [
			storedEvent("time-1", "TIME_ADVANCED"),
			storedEvent("status-1", "STATUS"),
			storedEvent("deposit-1", "COIN_DEPOSIT"),
			storedEvent("npc-1", "NPC_IMPORT"),
			storedEvent("time-2", "TIME_ADVANCED"),
			storedEvent("item-1", "ITEM_IMPORT"),
		];
		const { queue, sentBatches } = queueWith(() => Promise.resolve(), stored);
		queue.flush();
		await settle();
		expect(sentBatches).toEqual([
			["time-1", "status-1"],
			["deposit-1"],
			["npc-1"],
			["time-2"],
			["item-1"],
		]);
	});

	test("a refused deposit is dropped alone and handed back with its reason", async () => {
		const stored = [
			storedEvent("deposit-1", "COIN_DEPOSIT"),
			storedEvent("time-1", "TIME_ADVANCED"),
		];
		const { dropped, droppedEvents, queue, sentBatches } = queueWith(
			(events) =>
				events.some((event) => event.kind === "COIN_DEPOSIT")
					? Promise.reject(new BridgeHttpError(422, "actorName is too long"))
					: Promise.resolve(),
			stored,
		);
		queue.flush();
		await settle();
		expect(dropped).toEqual(["actorName is too long"]);
		expect(droppedEvents.map((events) => events.map((event) => event.id))).toEqual([["deposit-1"]]);
		expect(sentBatches).toEqual([["time-1"]]);
	});

	test("an event that may no longer leave is dropped unsent, and the rest still go", async () => {
		const stored = [
			storedEvent("deposit-1", "COIN_DEPOSIT"),
			storedEvent("time-1", "TIME_ADVANCED"),
		];
		const { dropped, queue, sentBatches, storage } = queueWith(
			() => Promise.resolve(),
			stored,
			(event) => event.kind !== "COIN_DEPOSIT",
		);
		queue.flush();
		await settle();
		expect(sentBatches).toEqual([["time-1"]]);
		expect(dropped).toEqual([]);
		expect(storage.events).toEqual([]);
	});

	test("add queues an event made elsewhere under its own ID, once", async () => {
		const sent: string[] = [];
		let answer: (() => void) | undefined;
		const { queue, storage } = queueWith((events) => {
			sent.push(...events.map((event) => event.id));
			return new Promise((resolve) => {
				answer = resolve;
			});
		});
		const deposit = storedEvent("deposit-1", "COIN_DEPOSIT");
		queue.add(deposit);
		queue.add(deposit);
		expect(queue.holds("deposit-1")).toBe(true);
		expect(storage.events).toEqual([deposit]);
		answer?.();
		await settle();
		expect(queue.holds("deposit-1")).toBe(false);
		expect(sent).toEqual(["deposit-1"]);
	});
});

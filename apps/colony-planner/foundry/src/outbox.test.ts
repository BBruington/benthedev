import { describe, expect, test } from "bun:test";
import { type RowApplier, syncOutbox } from "./outbox";
import type { Acknowledgement, OutboxPage, OutboxRow } from "./wire";

const liveSignal = new AbortController().signal;

function row(seq: number, kind = "COLONY_SUMMARY"): OutboxRow {
	return { kind, payload: {}, seq, targetKey: `target:${seq}` };
}

function fakeOutbox(pages: OutboxPage[]) {
	const requestedAfter: number[] = [];
	const acknowledgements: Acknowledgement[] = [];
	let cursor = 0;
	return {
		acknowledgements,
		client: {
			acknowledge(acknowledgement: Acknowledgement) {
				acknowledgements.push(acknowledgement);
				return Promise.resolve();
			},
			outbox(after: number) {
				requestedAfter.push(after);
				return Promise.resolve(pages.shift() ?? { more: false, rows: [] });
			},
		},
		cursor: {
			load: () => cursor,
			save(seq: number) {
				cursor = seq;
				return Promise.resolve();
			},
		},
		cursorValue: () => cursor,
		requestedAfter,
	};
}

describe("syncOutbox", () => {
	test("reads pages until more is false, applies rows in order and acknowledges each page", async () => {
		const outbox = fakeOutbox([
			{ more: true, rows: [row(1), row(2)] },
			{ more: false, rows: [row(3)] },
		]);
		const applied: number[] = [];
		const applier: RowApplier = async (outboxRow) => {
			await Promise.resolve();
			applied.push(outboxRow.seq);
		};

		await syncOutbox({
			appliers: { COLONY_SUMMARY: applier },
			client: outbox.client,
			cursor: outbox.cursor,
			signal: liveSignal,
		});

		expect(applied).toEqual([1, 2, 3]);
		expect(outbox.requestedAfter).toEqual([0, 2]);
		expect(outbox.acknowledgements).toEqual([
			{ failed: [], through: 2 },
			{ failed: [], through: 3 },
		]);
		expect(outbox.cursorValue()).toBe(3);
	});

	test("a failing row is acknowledged as failed with its reason, and the rest still apply", async () => {
		const outbox = fakeOutbox([{ more: false, rows: [row(4), row(5), row(6, "NPC_UPSERT")] }]);
		const applied: number[] = [];
		const applier: RowApplier = (outboxRow) => {
			if (outboxRow.seq === 4) {
				return Promise.reject(new Error("x".repeat(400)));
			}
			applied.push(outboxRow.seq);
			return Promise.resolve();
		};

		await syncOutbox({
			appliers: { COLONY_SUMMARY: applier },
			client: outbox.client,
			cursor: outbox.cursor,
			signal: liveSignal,
		});

		expect(applied).toEqual([5]);
		expect(outbox.acknowledgements).toEqual([
			{
				failed: [
					{ reason: "x".repeat(300), seq: 4 },
					{
						reason:
							"This version of Colony Bridge doesn't know NPC_UPSERT rows. Update the module.",
						seq: 6,
					},
				],
				through: 6,
			},
		]);
		expect(outbox.cursorValue()).toBe(6);
	});

	test("failed rows are never fetched again: the next sync starts after them", async () => {
		const outbox = fakeOutbox([{ more: false, rows: [row(7)] }]);
		const failing: RowApplier = () => Promise.reject(new Error("no journal"));
		await syncOutbox({
			appliers: { COLONY_SUMMARY: failing },
			client: outbox.client,
			cursor: outbox.cursor,
			signal: liveSignal,
		});
		await syncOutbox({
			appliers: { COLONY_SUMMARY: failing },
			client: outbox.client,
			cursor: outbox.cursor,
			signal: liveSignal,
		});
		expect(outbox.requestedAfter).toEqual([0, 7]);
		expect(outbox.acknowledgements).toHaveLength(1);
	});

	test("a stopped session applies, acknowledges and saves nothing more", async () => {
		const outbox = fakeOutbox([{ more: false, rows: [row(9), row(10)] }]);
		const session = new AbortController();
		const applied: number[] = [];
		const applier: RowApplier = (outboxRow) => {
			applied.push(outboxRow.seq);
			session.abort();
			return Promise.resolve();
		};
		await expect(
			syncOutbox({
				appliers: { COLONY_SUMMARY: applier },
				client: outbox.client,
				cursor: outbox.cursor,
				signal: session.signal,
			}),
		).rejects.toThrow();
		expect(applied).toEqual([9]);
		expect(outbox.acknowledgements).toEqual([]);
		expect(outbox.cursorValue()).toBe(0);
	});

	test("an empty outbox acknowledges nothing", async () => {
		const outbox = fakeOutbox([]);
		await syncOutbox({
			appliers: {},
			client: outbox.client,
			cursor: outbox.cursor,
			signal: liveSignal,
		});
		expect(outbox.acknowledgements).toEqual([]);
	});

	test("when the acknowledgement fails the cursor stays, so the page is read again next time", async () => {
		const outbox = fakeOutbox([{ more: false, rows: [row(8)] }]);
		const client = {
			...outbox.client,
			acknowledge: () => Promise.reject(new TypeError("Failed to fetch")),
		};
		const applier: RowApplier = () => Promise.resolve();
		await expect(
			syncOutbox({
				appliers: { COLONY_SUMMARY: applier },
				client,
				cursor: outbox.cursor,
				signal: liveSignal,
			}),
		).rejects.toThrow("Failed to fetch");
		expect(outbox.cursorValue()).toBe(0);
	});
});

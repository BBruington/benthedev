import { describe, expect, test } from "bun:test";
import { hexesWithinRadius } from "./hex-math";
import {
	batchStrokeQueue,
	chunk,
	emptyStrokeQueue,
	maximumCellsPerCall,
	queueCells,
} from "./stroke-queue";

describe("stroke batching", () => {
	test("a later stroke over the same hex replaces the earlier one", () => {
		let queue = queueCells(emptyStrokeQueue, [{ q: 0, r: 0 }], "FOREST");
		queue = queueCells(queue, [{ q: 0, r: 0 }], "WATER");
		queue = queueCells(queue, [{ q: 1, r: 0 }], "WATER");
		queue = queueCells(queue, [{ q: 1, r: 0 }], null);
		expect(batchStrokeQueue(queue)).toEqual({
			erase: [[{ q: 1, r: 0 }]],
			paint: [[{ q: 0, r: 0, terrain: "WATER" }]],
		});
	});

	test("queueing leaves the earlier queue untouched", () => {
		const first = queueCells(emptyStrokeQueue, [{ q: 0, r: 0 }], "FOREST");
		queueCells(first, [{ q: 5, r: 5 }], "DESERT");
		expect(first.size).toBe(1);
		expect(emptyStrokeQueue.size).toBe(0);
	});

	test("long strokes are split into calls of at most 200 cells", () => {
		const cells = hexesWithinRadius({ q: 0, r: 0 }, 9);
		expect(cells).toHaveLength(271);
		const calls = batchStrokeQueue(queueCells(emptyStrokeQueue, cells, "PLAINS"));
		expect(calls.erase).toEqual([]);
		expect(calls.paint.map((call) => call.length)).toEqual([maximumCellsPerCall, 71]);
		expect(calls.paint.flat()).toHaveLength(271);
	});

	test("an empty queue makes no calls", () => {
		expect(batchStrokeQueue(emptyStrokeQueue)).toEqual({ erase: [], paint: [] });
	});

	test("chunk", () => {
		expect(chunk([1, 2, 3, 4, 5], 2)).toEqual([[1, 2], [3, 4], [5]]);
		expect(chunk([], 2)).toEqual([]);
	});
});

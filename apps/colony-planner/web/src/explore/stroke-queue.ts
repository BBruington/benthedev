import type { Schemas } from "@/api/client";
import { type HexCoordinate, hexKey } from "./hex-math";

export type Terrain = Schemas["Terrain"];
/** What a stroke lays down: a terrain, or null to unchart. */
export type StrokeTerrain = Terrain | null;
export type QueuedCell = { q: number; r: number; terrain: StrokeTerrain };
/** Cells waiting to be sent, one per hex; a later stroke over the same hex replaces the earlier. */
export type StrokeQueue = ReadonlyMap<string, QueuedCell>;
export type StrokeCalls = {
	paint: Schemas["PaintedHex"][][];
	erase: Schemas["HexCoordinate"][][];
};

/** The server takes at most this many cells per paint or erase call (spec §5.10). */
export const maximumCellsPerCall = 200;
export const strokeSendIntervalInMilliseconds = 250;

export const emptyStrokeQueue: StrokeQueue = new Map();

export function queueCells(
	queue: StrokeQueue,
	cells: readonly HexCoordinate[],
	terrain: StrokeTerrain,
): StrokeQueue {
	const next = new Map(queue);
	for (const cell of cells) {
		next.set(hexKey(cell.q, cell.r), { q: cell.q, r: cell.r, terrain });
	}
	return next;
}

/** Splits the queue into paint and erase calls of at most 200 cells each. */
export function batchStrokeQueue(queue: StrokeQueue): StrokeCalls {
	const painted: Schemas["PaintedHex"][] = [];
	const erased: Schemas["HexCoordinate"][] = [];
	for (const cell of queue.values()) {
		if (cell.terrain === null) {
			erased.push({ q: cell.q, r: cell.r });
		} else {
			painted.push({ q: cell.q, r: cell.r, terrain: cell.terrain });
		}
	}
	return {
		erase: chunk(erased, maximumCellsPerCall),
		paint: chunk(painted, maximumCellsPerCall),
	};
}

export function chunk<Item>(items: readonly Item[], size: number): Item[][] {
	const chunks: Item[][] = [];
	for (let start = 0; start < items.length; start += size) {
		chunks.push(items.slice(start, start + size));
	}
	return chunks;
}

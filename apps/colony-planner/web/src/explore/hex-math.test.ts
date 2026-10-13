import { describe, expect, test } from "bun:test";
import {
	axialRound,
	frontierCells,
	hexDistance,
	hexesWithinRadius,
	hexFromNumericKey,
	hexKey,
	hexLine,
	hexSize,
	hexToWorld,
	hexWorldBounds,
	numericHexKey,
	unitHexCorners,
	worldToHex,
} from "./hex-math";

describe("hex centres", () => {
	test("pointy-top axial layout", () => {
		expect(hexToWorld({ q: 0, r: 0 })).toEqual({ x: 0, y: 0 });
		expect(hexToWorld({ q: 1, r: 0 }).x).toBeCloseTo(Math.sqrt(3) * hexSize);
		expect(hexToWorld({ q: 1, r: 0 }).y).toBe(0);
		expect(hexToWorld({ q: 0, r: 1 }).x).toBeCloseTo((Math.sqrt(3) / 2) * hexSize);
		expect(hexToWorld({ q: 0, r: 1 }).y).toBe(1.5 * hexSize);
	});

	test("the first corner is the upper right and corners sit on the circumradius", () => {
		const [first] = unitHexCorners;
		expect(first?.x).toBeCloseTo(Math.sqrt(3) / 2);
		expect(first?.y).toBeCloseTo(-0.5);
		for (const corner of unitHexCorners) {
			expect(Math.hypot(corner.x, corner.y)).toBeCloseTo(1);
		}
	});
});

describe("hit testing by axial rounding", () => {
	test("every centre maps back to its own hex", () => {
		for (const cell of hexesWithinRadius({ q: 3, r: -7 }, 4)) {
			expect(worldToHex(hexToWorld(cell))).toEqual(cell);
		}
	});

	test("points inside a hex, near its corners, still hit it", () => {
		const cell = { q: -2, r: 5 };
		const center = hexToWorld(cell);
		for (const corner of unitHexCorners) {
			const inside = {
				x: center.x + corner.x * hexSize * 0.9,
				y: center.y + corner.y * hexSize * 0.9,
			};
			expect(worldToHex(inside)).toEqual(cell);
		}
	});

	test("a point just past an edge hits the neighbour", () => {
		const center = hexToWorld({ q: 0, r: 0 });
		const inradius = (Math.sqrt(3) / 2) * hexSize;
		expect(worldToHex({ x: center.x + inradius + 0.5, y: center.y })).toEqual({ q: 1, r: 0 });
		expect(worldToHex({ x: center.x - inradius - 0.5, y: center.y })).toEqual({ q: -1, r: 0 });
	});

	test("rounding never returns negative zero", () => {
		const rounded = axialRound(-0.2, 0.1);
		expect(Object.is(rounded.q, -0)).toBe(false);
		expect(Object.is(rounded.r, -0)).toBe(false);
		expect(rounded).toEqual({ q: 0, r: 0 });
	});

	test("cube rounding fixes the component with the largest error", () => {
		expect(axialRound(0.6, 0.6)).toEqual({ q: 1, r: 0 });
		expect(axialRound(0.4, 0.4)).toEqual({ q: 0, r: 1 });
	});
});

describe("neighbourhoods", () => {
	test("distance", () => {
		expect(hexDistance({ q: 0, r: 0 }, { q: 0, r: 0 })).toBe(0);
		expect(hexDistance({ q: 0, r: 0 }, { q: 3, r: -1 })).toBe(3);
		expect(hexDistance({ q: -2, r: 4 }, { q: 1, r: 1 })).toBe(3);
	});

	test("a radius-5 area holds 91 hexes", () => {
		const cells = hexesWithinRadius({ q: 0, r: 0 }, 5);
		expect(cells).toHaveLength(91);
		expect(new Set(cells.map((cell) => hexKey(cell.q, cell.r))).size).toBe(91);
		for (const cell of cells) {
			expect(hexDistance(cell, { q: 0, r: 0 })).toBeLessThanOrEqual(5);
		}
	});

	test("the frontier is the uncharted ring two hexes deep", () => {
		expect(frontierCells([{ q: 0, r: 0 }])).toHaveLength(18);
		const charted = [
			{ q: 0, r: 0 },
			{ q: 1, r: 0 },
		];
		const frontier = frontierCells(charted);
		const keys = new Set(frontier.map((cell) => hexKey(cell.q, cell.r)));
		expect(keys.size).toBe(frontier.length);
		expect(keys.has("0,0")).toBe(false);
		expect(keys.has("1,0")).toBe(false);
		expect(keys.has("3,0")).toBe(true);
		expect(keys.has("4,0")).toBe(false);
		for (const cell of frontier) {
			const nearest = Math.min(...charted.map((chartedCell) => hexDistance(cell, chartedCell)));
			expect(nearest === 1 || nearest === 2).toBe(true);
		}
	});

	test("numeric keys are distinct at the server's coordinate bounds", () => {
		const keys = [
			numericHexKey(-100_000, -100_000),
			numericHexKey(-100_000, 100_000),
			numericHexKey(100_000, -100_000),
			numericHexKey(100_000, 100_000),
			numericHexKey(0, 0),
			numericHexKey(0, 1),
			numericHexKey(1, 0),
		];
		expect(new Set(keys).size).toBe(keys.length);
		for (const key of keys) {
			expect(Number.isSafeInteger(key)).toBe(true);
		}
		for (const cell of [
			{ q: -100_000, r: 100_000 },
			{ q: 0, r: 0 },
			{ q: 37, r: -12 },
		]) {
			expect(hexFromNumericKey(numericHexKey(cell.q, cell.r))).toEqual(cell);
		}
	});
});

describe("strokes", () => {
	test("a line covers every hex between its ends without gaps", () => {
		const line = hexLine({ q: 0, r: 0 }, { q: 4, r: -2 });
		expect(line).toHaveLength(5);
		expect(line[0]).toEqual({ q: 0, r: 0 });
		expect(line.at(-1)).toEqual({ q: 4, r: -2 });
		for (let index = 1; index < line.length; index += 1) {
			const previous = line[index - 1];
			const current = line[index];
			if (previous && current) {
				expect(hexDistance(previous, current)).toBe(1);
			}
		}
	});

	test("a line to the same hex is that hex", () => {
		expect(hexLine({ q: 2, r: 3 }, { q: 2, r: 3 })).toEqual([{ q: 2, r: 3 }]);
	});
});

test("world bounds hold every hex whole", () => {
	expect(hexWorldBounds([])).toBeUndefined();
	const bounds = hexWorldBounds([
		{ q: 0, r: 0 },
		{ q: 2, r: 1 },
	]);
	const far = hexToWorld({ q: 2, r: 1 });
	expect(bounds?.minX).toBeCloseTo(-(Math.sqrt(3) / 2) * hexSize);
	expect(bounds?.minY).toBe(-hexSize);
	expect(bounds?.maxX).toBeCloseTo(far.x + (Math.sqrt(3) / 2) * hexSize);
	expect(bounds?.maxY).toBe(far.y + hexSize);
});

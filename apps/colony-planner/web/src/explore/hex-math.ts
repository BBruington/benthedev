// Pointy-top axial hexes (spec §5.10). World points are map units: x grows east, y grows south, and
// CRS.Simple draws one unit as one pixel at zoom 0.

export type HexCoordinate = { q: number; r: number };
export type WorldPoint = { x: number; y: number };
export type WorldBounds = { minX: number; minY: number; maxX: number; maxY: number };

/** A hex's circumradius in map units. */
export const hexSize = 16;
export const starterRadius = 5;
export const frontierDepth = 2;

const squareRootOfThree = Math.sqrt(3);

/** Corner offsets of a hex of circumradius 1, starting at the upper right and going clockwise. */
export const unitHexCorners: readonly WorldPoint[] = Array.from({ length: 6 }, (_, index) => {
	const angle = (Math.PI / 180) * (60 * index - 30);
	return { x: Math.cos(angle), y: Math.sin(angle) };
});

export function hexKey(q: number, r: number): string {
	return `${q},${r}`;
}

// The server keeps q and r within ±100,000, so this packs both into one safe integer: number keys
// make the frontier's set lookups cheap enough to rerun on every stroke at 5,000 hexes.
const coordinateLimit = 100_000;
const coordinateSpan = 2 * coordinateLimit + 1;

export function numericHexKey(q: number, r: number): number {
	return (q + coordinateLimit) * coordinateSpan + (r + coordinateLimit);
}

export function hexFromNumericKey(key: number): HexCoordinate {
	return {
		q: Math.floor(key / coordinateSpan) - coordinateLimit,
		r: (key % coordinateSpan) - coordinateLimit,
	};
}

export function hexToWorld({ q, r }: HexCoordinate, size = hexSize): WorldPoint {
	return { x: size * squareRootOfThree * (q + r / 2), y: size * 1.5 * r };
}

/** The hex under a world point: the hit test for taps and strokes. */
export function worldToHex({ x, y }: WorldPoint, size = hexSize): HexCoordinate {
	const fractionalQ = ((squareRootOfThree / 3) * x - y / 3) / size;
	const fractionalR = ((2 / 3) * y) / size;
	return axialRound(fractionalQ, fractionalR);
}

/** Rounds fractional axial coordinates to the hex that contains them, by way of cube rounding. */
export function axialRound(fractionalQ: number, fractionalR: number): HexCoordinate {
	const fractionalS = -fractionalQ - fractionalR;
	let q = Math.round(fractionalQ);
	let r = Math.round(fractionalR);
	const s = Math.round(fractionalS);
	const qDifference = Math.abs(q - fractionalQ);
	const rDifference = Math.abs(r - fractionalR);
	const sDifference = Math.abs(s - fractionalS);
	if (qDifference > rDifference && qDifference > sDifference) {
		q = -r - s;
	} else if (rDifference > sDifference) {
		r = -q - s;
	}
	// Adding zero turns a rounded -0 into 0, so keys and comparisons never see two zeros.
	return { q: q + 0, r: r + 0 };
}

export function hexDistance(first: HexCoordinate, second: HexCoordinate): number {
	const qDifference = first.q - second.q;
	const rDifference = first.r - second.r;
	return (Math.abs(qDifference) + Math.abs(qDifference + rDifference) + Math.abs(rDifference)) / 2;
}

export function hexesWithinRadius(center: HexCoordinate, radius: number): HexCoordinate[] {
	const cells: HexCoordinate[] = [];
	for (let q = -radius; q <= radius; q += 1) {
		const lowestR = Math.max(-radius, -q - radius);
		const highestR = Math.min(radius, -q + radius);
		for (let r = lowestR; r <= highestR; r += 1) {
			cells.push({ q: center.q + q, r: center.r + r });
		}
	}
	return cells;
}

const frontierOffsets = hexesWithinRadius({ q: 0, r: 0 }, frontierDepth).filter(
	(offset) => offset.q !== 0 || offset.r !== 0,
);

/** Uncharted hexes up to two steps from a charted one: the dashed ring the map grows into. */
export function frontierCells(charted: readonly HexCoordinate[]): HexCoordinate[] {
	const chartedKeys = new Set(charted.map((cell) => numericHexKey(cell.q, cell.r)));
	const seen = new Set<number>();
	const frontier: HexCoordinate[] = [];
	for (const cell of charted) {
		for (const offset of frontierOffsets) {
			const q = cell.q + offset.q;
			const r = cell.r + offset.r;
			const key = numericHexKey(q, r);
			if (chartedKeys.has(key) || seen.has(key)) {
				continue;
			}
			seen.add(key);
			frontier.push({ q, r });
		}
	}
	return frontier;
}

/** Every hex a straight stroke crosses from one hex to another, both ends included, without gaps. */
export function hexLine(from: HexCoordinate, to: HexCoordinate): HexCoordinate[] {
	const steps = hexDistance(from, to);
	if (steps === 0) {
		return [{ q: from.q, r: from.r }];
	}
	// A tiny nudge keeps points that land exactly on an edge from rounding back and forth.
	const nudge = 1e-6;
	const cells: HexCoordinate[] = [];
	for (let step = 0; step <= steps; step += 1) {
		const fraction = step / steps;
		cells.push(
			axialRound(
				from.q + nudge + (to.q - from.q) * fraction,
				from.r + nudge + (to.r - from.r) * fraction,
			),
		);
	}
	return cells;
}

/** The world rectangle that holds every given hex whole, or undefined for none. */
export function hexWorldBounds(
	cells: readonly HexCoordinate[],
	size = hexSize,
): WorldBounds | undefined {
	if (cells.length === 0) {
		return undefined;
	}
	let minX = Number.POSITIVE_INFINITY;
	let minY = Number.POSITIVE_INFINITY;
	let maxX = Number.NEGATIVE_INFINITY;
	let maxY = Number.NEGATIVE_INFINITY;
	for (const cell of cells) {
		const center = hexToWorld(cell, size);
		minX = Math.min(minX, center.x);
		minY = Math.min(minY, center.y);
		maxX = Math.max(maxX, center.x);
		maxY = Math.max(maxY, center.y);
	}
	const halfWidth = (squareRootOfThree / 2) * size;
	return { maxX: maxX + halfWidth, maxY: maxY + size, minX: minX - halfWidth, minY: minY - size };
}

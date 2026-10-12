import { describe, expect, test } from "bun:test";
import { findHexesByLabel, truncateLabel } from "./labels";

// Every character is 10 units wide, the ellipsis included.
const measure = (text: string) => Array.from(text).length * 10;

describe("truncateLabel", () => {
	test("a label that fits is kept whole", () => {
		expect(truncateLabel("Ford", 40, measure)).toBe("Ford");
	});

	test("a long label is cut to fit, with an ellipsis", () => {
		expect(truncateLabel("Old Watchtower", 60, measure)).toBe("Old W…");
		expect(measure(truncateLabel("Old Watchtower", 60, measure))).toBeLessThanOrEqual(60);
	});

	test("trailing spaces before the ellipsis are dropped", () => {
		expect(truncateLabel("Old Watchtower", 50, measure)).toBe("Old…");
	});

	test("too narrow for anything but the ellipsis, or not even that", () => {
		expect(truncateLabel("Watchtower", 10, measure)).toBe("…");
		expect(truncateLabel("Watchtower", 5, measure)).toBe("");
	});
});

describe("findHexesByLabel", () => {
	const hexes = [
		{ label: "", q: 0, r: 0 },
		{ label: "Old Ford", q: 1, r: 0 },
		{ label: "Fordham", q: 2, r: 0 },
		{ label: "Ash Ford", q: 3, r: 0 },
		{ label: "Camp", q: 4, r: 0 },
	];

	test("labels starting with the search come first, then the rest by label", () => {
		expect(findHexesByLabel(hexes, " ford ", 10).map((hex) => hex.label)).toEqual([
			"Fordham",
			"Ash Ford",
			"Old Ford",
		]);
	});

	test("a blank search finds nothing, and results stop at the limit", () => {
		expect(findHexesByLabel(hexes, "  ", 10)).toEqual([]);
		expect(findHexesByLabel(hexes, "o", 2)).toHaveLength(2);
	});
});

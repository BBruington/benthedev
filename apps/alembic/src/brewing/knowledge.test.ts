import { describe, expect, test } from "bun:test";
import { knownTotals, maskTotals } from "./knowledge";
import type { IngredientView, KnownValues } from "./types";

function view(id: string, values: KnownValues): IngredientView {
	return {
		id,
		name: id,
		rarity: "common",
		type: "botanical",
		form: "leaf",
		biome: "forest",
		values,
		deconstruct: {
			dc: 10,
			odds: { criticalSuccess: 0, success: 0, failure: 0, criticalFailure: 0 },
		},
	};
}

describe("knownTotals", () => {
	test("sums learned contributions and flags axes with unlearned parts", () => {
		expect(
			knownTotals({ a: 2, b: 1 }, [
				view("a", [1, null, 3, null]),
				view("b", [5, null, null, 8]),
			]),
		).toEqual({
			known: [7, null, 6, 8],
			hidden: [false, true, true, true],
		});
	});
});

describe("maskTotals", () => {
	const mix = { a: 2, b: 1 };

	test("a fully known mix keeps its stored totals", () => {
		expect(
			maskTotals([7, 8, 9, 10], mix, [
				view("a", [1, 2, 3, 4]),
				view("b", [5, 6, 7, 8]),
			]),
		).toEqual([7, 8, 9, 10]);
	});

	test("one partly known ingredient masks exactly its unknown axes", () => {
		expect(
			maskTotals([7, 8, 9, 10], mix, [
				view("a", [1, null, 3, null]),
				view("b", [5, 6, 7, 8]),
			]),
		).toEqual([7, null, 9, null]);
	});

	test("an ingredient missing from the views masks every axis", () => {
		expect(maskTotals([7, 8, 9, 10], mix, [view("a", [1, 2, 3, 4])])).toEqual([
			null,
			null,
			null,
			null,
		]);
	});
});

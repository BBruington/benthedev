import { describe, expect, test } from "bun:test";
import { WREN } from "#/brewing/catalog";
import {
	brewAttemptSchema,
	crafterEditSchema,
	deconstructSchema,
	inventoryAdjustSchema,
} from "./schema";

const crafter = {
	level: WREN.level,
	intMod: WREN.intMod,
	craftingRank: WREN.craftingRank,
	alchemicalCrafting: WREN.alchemicalCrafting,
	tools: WREN.tools,
};

describe("brew attempt input", () => {
	test.each([
		[{ heartmoss: 1 }],
		[{ heartmoss: 4 }],
		[{ heartmoss: 2, glowcap: 1, "ember-salt": 1 }],
	])("accepts 1 to 4 total units: %j", (mix) => {
		expect(brewAttemptSchema.safeParse({ mix }).success).toBe(true);
	});

	test.each([
		[{}],
		[{ heartmoss: 0 }],
		[{ heartmoss: 5 }],
		[{ heartmoss: 2, glowcap: 3 }],
		[{ heartmoss: 1.5 }],
	])("rejects empty, zero, fractional or over-4 mixes: %j", (mix) => {
		expect(brewAttemptSchema.safeParse({ mix }).success).toBe(false);
	});

	test("table roll is optional and limited to a d20 face", () => {
		const mix = { heartmoss: 1 };
		for (const d20 of [undefined, 1, 20])
			expect(brewAttemptSchema.safeParse({ mix, d20 }).success).toBe(true);
		for (const d20 of [0, 21, 7.5])
			expect(brewAttemptSchema.safeParse({ mix, d20 }).success).toBe(false);
	});
});

describe("crafter edit input", () => {
	test("accepts the default crafter and PF2e level and Int bounds", () => {
		for (const edit of [
			crafter,
			{ ...crafter, level: 1, intMod: -1 },
			{ ...crafter, level: 20, intMod: 7 },
		])
			expect(crafterEditSchema.safeParse(edit).success).toBe(true);
	});

	test.each([
		{ level: 0 },
		{ level: 21 },
		{ intMod: -2 },
		{ intMod: 8 },
		{ craftingRank: "grandmaster" },
		{ tools: "deluxe" },
	])("rejects out-of-range fields: %j", (change) => {
		expect(crafterEditSchema.safeParse({ ...crafter, ...change }).success).toBe(
			false,
		);
	});

	test("Alchemical Crafting requires trained Crafting", () => {
		const untrained = { ...crafter, craftingRank: "untrained" };
		const refused = crafterEditSchema.safeParse({
			...untrained,
			alchemicalCrafting: true,
		});
		expect(refused.success).toBe(false);
		expect(refused.error?.issues[0]?.path).toEqual(["alchemicalCrafting"]);
		expect(
			crafterEditSchema.safeParse({ ...untrained, alchemicalCrafting: false })
				.success,
		).toBe(true);
	});
});

test("inventory adjustments move one unit at a time", () => {
	for (const kind of ["ingredient", "potion"]) {
		for (const delta of [1, -1])
			expect(
				inventoryAdjustSchema.safeParse({ kind, itemId: "heartmoss", delta })
					.success,
			).toBe(true);
		for (const delta of [0, 2, -2])
			expect(
				inventoryAdjustSchema.safeParse({ kind, itemId: "heartmoss", delta })
					.success,
			).toBe(false);
	}
});

test("deconstruction takes a known item kind and a real d20", () => {
	const heartmoss = { kind: "ingredient", itemId: "heartmoss" };
	expect(deconstructSchema.safeParse({ ...heartmoss, d20: 20 }).success).toBe(
		true,
	);
	expect(deconstructSchema.safeParse({ ...heartmoss, d20: 21 }).success).toBe(
		false,
	);
	expect(
		deconstructSchema.safeParse({ ...heartmoss, kind: "spell" }).success,
	).toBe(false);
});

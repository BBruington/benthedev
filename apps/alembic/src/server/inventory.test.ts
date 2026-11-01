import { describe, expect, test } from "bun:test";
import { ORPCError } from "@orpc/server";
import { INGREDIENTS, MENTOR_MIX } from "#/brewing/catalog";
import type { Inventory, Mix } from "#/brewing/types";
import { assertEnoughIngredients } from "./inventory";

const freshCampaign: Inventory = Object.fromEntries(
	INGREDIENTS.map((ingredient) => [ingredient.id, 1]),
);

function refusal(mix: Mix, inventory: Inventory): ORPCError<string, unknown> {
	try {
		assertEnoughIngredients(mix, inventory);
	} catch (error) {
		if (error instanceof ORPCError) return error;
		throw error;
	}
	throw new Error("Expected the brew to be refused");
}

describe("brew bench ingredient stock", () => {
	test("a fresh campaign has enough stock for the mentor's mix", () => {
		expect(() =>
			assertEnoughIngredients(MENTOR_MIX, freshCampaign),
		).not.toThrow();
	});

	test("refuses a brew using an ingredient that is out of stock", () => {
		const error = refusal(MENTOR_MIX, { ...freshCampaign, heartmoss: 0 });
		expect(error.code).toBe("BAD_REQUEST");
		expect(error.message).toBe(
			"Not enough ingredients: Heartmoss (need 1, have 0)",
		);
	});

	test("refuses a brew using an ingredient with no stock record", () => {
		const { glowcap: _, ...withoutGlowcap } = freshCampaign;
		const error = refusal({ glowcap: 1 }, withoutGlowcap);
		expect(error.message).toBe(
			"Not enough ingredients: Glowcap (need 1, have 0)",
		);
	});

	test("refuses repeats of one ingredient beyond its stock", () => {
		const error = refusal({ heartmoss: 2, glowcap: 1 }, freshCampaign);
		expect(error.code).toBe("BAD_REQUEST");
		expect(error.message).toBe(
			"Not enough ingredients: Heartmoss (need 2, have 1)",
		);
		expect(() =>
			assertEnoughIngredients(
				{ heartmoss: 2, glowcap: 1 },
				{ ...freshCampaign, heartmoss: 2 },
			),
		).not.toThrow();
	});

	test("names every short ingredient", () => {
		const error = refusal(
			{ heartmoss: 3, glowcap: 1 },
			{
				...freshCampaign,
				heartmoss: 2,
				glowcap: 0,
			},
		);
		expect(error.message).toBe(
			"Not enough ingredients: Heartmoss (need 3, have 2), Glowcap (need 1, have 0)",
		);
	});
});

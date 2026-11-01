import { describe, expect, test } from "bun:test";
import {
	MENTOR_MIX,
	POTIONS,
	STARTER_POTION_ID,
	WREN,
} from "#/brewing/catalog";
import { toWorkshopView } from "./projection";

describe("player workshop projection", () => {
	test("all known potions expose metadata without answer-key fields or values", () => {
		const view = toWorkshopView({
			viewingAs: "player",
			isGm: true,
			crafter: { ...WREN, downtimeDays: 0 },
			entries: POTIONS.map((potion) => ({
				potionId: potion.id,
				mentor: false,
				formed: false,
				revealed: true,
				formulaRevealed: false,
				perfected: false,
				bestOffBy: null,
				bestMix: null,
				note: null,
			})),
			log: [],
			inventory: {},
		});
		expect(view.codex).toHaveLength(11);
		expect(view.undiscoveredCount).toBe(0);
		const serialized = JSON.stringify(view);
		for (const key of [
			"target",
			"primaryRoom",
			"secondaryBudget",
			"requiredIngredient",
			"perfectRecipe",
		]) {
			expect(serialized).not.toContain(`"${key}"`);
		}
		for (const potion of POTIONS) {
			expect(serialized).not.toContain(potion.target.join("/"));
			expect(
				view.codex.find((entry) => entry.potionId === potion.id)?.solvedMix,
			).toBeNull();
			expect(serialized).not.toContain(JSON.stringify(potion.perfectRecipe));
			expect(serialized).not.toContain(JSON.stringify(potion.target));
		}
	});

	test("only mentor, formed, or revealed entries are known", () => {
		const entries = POTIONS.map((potion, index) => ({
			potionId: potion.id,
			mentor: index === 0,
			formed: index === 1,
			revealed: index === 2,
			formulaRevealed: false,
			perfected: index === 1,
			bestOffBy: index === 1 ? 0 : null,
			bestMix: index === 1 ? potion.perfectRecipe : null,
			note: index === 0 ? "Mentor note" : null,
		}));
		const view = toWorkshopView({
			viewingAs: "gm",
			isGm: true,
			crafter: { ...WREN, downtimeDays: 3 },
			entries,
			log: [],
			inventory: {},
		});
		expect(view.codex.map((entry) => entry.source)).toEqual([
			"mentor",
			"brewed",
			"revealed",
		]);
		expect(view.undiscoveredCount).toBe(8);
		expect(view.crafter.downtimeDays).toBe(3);
		expect(view.crafter.modifier).toBe(14);
		expect(view.codex[0].solvedMix).toEqual(MENTOR_MIX);
		expect(view.codex[1].solvedMix).toEqual(POTIONS[1].perfectRecipe);
		expect(view.codex[2].solvedMix).toBeNull();
		for (const entry of view.codex) {
			for (const potion of POTIONS) {
				if (entry.potionId === potion.id && entry.source === "brewed") continue;
				expect(entry.solvedMix).not.toEqual(potion.perfectRecipe);
			}
		}
	});

	test.each([
		{ bestMix: { ...MENTOR_MIX, glowcap: 2 }, bestOffBy: 14, perfected: false },
		{ bestMix: { ...MENTOR_MIX, glowcap: 1 }, bestOffBy: 0, perfected: true },
	])("a mentor's own formed mix takes precedence over the note: %j", ({
		bestMix,
		bestOffBy,
		perfected,
	}) => {
		const view = toWorkshopView({
			viewingAs: "player",
			isGm: true,
			crafter: { ...WREN, downtimeDays: 1 },
			entries: [
				{
					potionId: STARTER_POTION_ID,
					mentor: true,
					formed: true,
					revealed: false,
					formulaRevealed: false,
					perfected,
					bestOffBy,
					bestMix,
					note: null,
				},
			],
			log: [],
			inventory: {},
		});
		expect(view.codex[0].solvedMix).toEqual(bestMix);
		expect(view.codex[0].bestOffBy).toBe(bestOffBy);
	});

	test("every revealed formula shares only its combination, never hidden matching rules", () => {
		const view = toWorkshopView({
			viewingAs: "player",
			isGm: true,
			crafter: { ...WREN, downtimeDays: 0 },
			entries: POTIONS.map((potion) => ({
				potionId: potion.id,
				mentor: false,
				formed: false,
				revealed: true,
				formulaRevealed: true,
				perfected: false,
				bestOffBy: null,
				bestMix: null,
				note: null,
			})),
			log: [],
			inventory: {},
		});
		expect(view.codex).toHaveLength(POTIONS.length);
		const serialized = JSON.stringify(view);
		for (const key of [
			"target",
			"primaryRoom",
			"secondaryBudget",
			"requiredIngredient",
			"perfectRecipe",
		]) {
			expect(serialized).not.toContain(`"${key}"`);
		}
		for (const potion of POTIONS) {
			const entry = view.codex.find((row) => row.potionId === potion.id);
			expect(entry?.solvedMix).toEqual(potion.perfectRecipe);
			expect(entry?.formulaRevealed).toBe(true);
			expect(entry?.source).toBe("revealed");
			expect(entry?.perfected).toBe(false);
			expect(serialized).not.toContain(JSON.stringify(potion.target));
		}
	});

	test.each([
		false,
		true,
	])("GM formula wins over own best mix and mentor note (formed: %j)", (formed) => {
		const bestMix = { ...MENTOR_MIX, glowcap: 2 };
		const view = toWorkshopView({
			viewingAs: "player",
			isGm: true,
			crafter: { ...WREN, downtimeDays: 1 },
			entries: [
				{
					potionId: STARTER_POTION_ID,
					mentor: true,
					formed,
					revealed: true,
					formulaRevealed: true,
					perfected: false,
					bestOffBy: formed ? 9 : null,
					bestMix: formed ? bestMix : null,
					note: "Mentor note",
				},
			],
			log: [],
			inventory: {},
		});
		expect(view.codex[0].solvedMix).toEqual(POTIONS[0].perfectRecipe);
		expect(view.codex[0].note).toBe("Mentor note");
	});
});

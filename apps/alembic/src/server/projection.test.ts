import { describe, expect, test } from "bun:test";
import {
	INGREDIENTS,
	MENTOR_MIX,
	POTIONS,
	STARTER_POTION_ID,
	WREN,
} from "#/brewing/catalog";
import { resolveBrew } from "#/brewing/engine";
import type { BrewRecord, Mix, ViewingAs } from "#/brewing/types";
import { knowledgeFrom, toWorkshopView } from "./projection";

const none = knowledgeFrom([]);

function viewWith(
	viewingAs: ViewingAs,
	knowledge = none,
	log: { mix: Mix; result: BrewRecord }[] = [],
) {
	return toWorkshopView({
		viewingAs,
		isGm: true,
		crafter: { ...WREN, downtimeDays: 0 },
		entries: [
			{
				potionId: STARTER_POTION_ID,
				mentor: true,
				formed: false,
				revealed: false,
				formulaRevealed: false,
				perfected: false,
				bestOffBy: null,
				bestMix: null,
				note: null,
			},
		],
		log: log.map((entry, index) => ({
			id: String(index),
			createdAt: new Date(0).toISOString(),
			...entry,
		})),
		inventory: {},
		potionStock: {},
		knowledge,
	});
}

function brewed(mix: Mix): { mix: Mix; result: BrewRecord } {
	const result = resolveBrew({
		mix,
		crafter: WREN,
		d20: 10,
		tableRoll: true,
		rollD3: () => 1,
	});
	return { mix, result: { ...result, potionName: null } };
}

describe("tradition knowledge", () => {
	test("with nothing learned, no ingredient value reaches the player", () => {
		const view = viewWith("player");
		const serialized = JSON.stringify(view);
		for (const ingredient of INGREDIENTS)
			expect(serialized).not.toContain(JSON.stringify(ingredient.values));
		for (const ingredient of view.ingredients)
			expect(ingredient.values).toEqual([null, null, null, null]);
		expect(view.codex[0].traditions).toEqual([null, null, null, null]);
	});

	test("a learned tradition shows only that axis, and mixed totals stay hidden", () => {
		const view = viewWith(
			"player",
			knowledgeFrom([
				{ itemKind: "ingredient", itemId: "heartmoss", tradition: "primal" },
			]),
			[brewed({ heartmoss: 1, "marsh-toad-gland": 1 })],
		);
		expect(
			view.ingredients.find((ingredient) => ingredient.id === "heartmoss")
				?.values,
		).toEqual([null, 14, null, null]);
		expect(view.log[0].result.totals).toEqual([null, null, null, null]);
	});

	test("a potion with arcane learned exposes only its arcane target", () => {
		const view = viewWith(
			"player",
			knowledgeFrom([
				{ itemKind: "potion", itemId: STARTER_POTION_ID, tradition: "arcane" },
			]),
		);
		const potion = POTIONS.find((p) => p.id === STARTER_POTION_ID);
		if (!potion) throw new Error("starter potion missing");
		expect(view.codex[0].traditions).toEqual([
			potion.target[0],
			null,
			null,
			null,
		]);
		const serialized = JSON.stringify(view);
		expect(serialized).not.toContain('"target"');
		expect(serialized).not.toContain(JSON.stringify(potion.target));
	});

	test("the GM view shows every value unmasked", () => {
		const mix = { heartmoss: 1, "marsh-toad-gland": 1 };
		const entry = brewed(mix);
		const view = viewWith("gm", none, [entry]);
		expect(view.ingredients.map((ingredient) => ingredient.values)).toEqual(
			INGREDIENTS.map((ingredient) => ingredient.values),
		);
		expect(view.log[0].result.totals).toEqual(entry.result.totals);
		expect(view.codex[0].traditions).toEqual(POTIONS[0].target);
	});
});

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
			potionStock: {},
			knowledge: none,
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
			potionStock: {},
			knowledge: none,
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
			potionStock: {},
			knowledge: none,
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
			potionStock: {},
			knowledge: none,
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
			potionStock: {},
			knowledge: none,
		});
		expect(view.codex[0].solvedMix).toEqual(POTIONS[0].perfectRecipe);
		expect(view.codex[0].note).toBe("Mentor note");
	});
});

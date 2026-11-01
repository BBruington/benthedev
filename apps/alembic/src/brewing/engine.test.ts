import { describe, expect, mock, test } from "bun:test";
import { POTIONS, WREN } from "./catalog";
import {
	crafterGates,
	crafterModifier,
	degreeOf,
	odds,
	potionDc,
	primaryOf,
	resolveBrew,
	totalsOf,
} from "./engine";
import type { BrewResult, Crafter, Degree, Mix } from "./types";

const note: Mix = { heartmoss: 1, "marsh-toad-gland": 1 };
const perfectHealing: Mix = { ...note, glowcap: 1 };
const veteran: Crafter = { ...WREN, level: 20, craftingRank: "master" };

function brew(
	mix: Mix,
	d20 = 6,
	crafter: Crafter = WREN,
	rollD3 = () => 2,
): BrewResult {
	return resolveBrew({ mix, crafter, d20, tableRoll: true, rollD3 });
}

function formed(result: BrewResult) {
	if (result.kind !== "formed")
		throw new Error(`Expected formed, got ${result.kind}`);
	return result;
}

const lowHealingArrows = {
	arcane: "on",
	primal: "on",
	occult: "low",
	divine: "low",
} as const;
const perfectArrows = {
	arcane: "on",
	primal: "on",
	occult: "on",
	divine: "on",
} as const;

describe("§12 worked examples", () => {
	test("1: mentor recipe totals, matching, arrows and successful quantity", () => {
		expect(brew(note)).toEqual({
			kind: "formed",
			potionId: "healing-potion-minor",
			totals: [7, 26, 1, 8],
			offBy: 14,
			perfect: false,
			arrows: lowHealingArrows,
			check: {
				d20: 6,
				tableRoll: true,
				modifier: 14,
				total: 20,
				dc: 15,
				degree: "success",
			},
			bonusRoll: null,
			quantity: 1,
			worstAxis: null,
		});
	});

	test("2: Glowcap perfects the recipe and grants a d3", () => {
		expect(brew(perfectHealing)).toEqual({
			kind: "formed",
			potionId: "healing-potion-minor",
			totals: [7, 26, 10, 13],
			offBy: 0,
			perfect: true,
			arrows: perfectArrows,
			check: {
				d20: 6,
				tableRoll: true,
				modifier: 14,
				total: 20,
				dc: 15,
				degree: "success",
			},
			bonusRoll: 2,
			quantity: 3,
			worstAxis: null,
		});
	});

	test("3: imperfect critical success reveals the worst tradition target", () => {
		const result = formed(brew(note, 11));
		expect(result.totals).toEqual([7, 26, 1, 8]);
		expect(result.offBy).toBe(14);
		expect(result.arrows).toEqual(lowHealingArrows);
		expect(result.check).toEqual({
			d20: 11,
			tableRoll: true,
			modifier: 14,
			total: 25,
			dc: 15,
			degree: "critical-success",
		});
		expect(result.quantity).toBe(2);
		expect(result.bonusRoll).toBeNull();
		expect(result.worstAxis).toEqual({ tradition: "occult", target: 10 });
	});

	test("4: natural one curdles imperfect and salvages perfect", () => {
		for (const [mix, quantity, offBy, arrows] of [
			[note, 0, 14, lowHealingArrows],
			[perfectHealing, 1, 0, perfectArrows],
		] as const) {
			const result = formed(brew(mix, 1));
			expect(result.check).toEqual({
				d20: 1,
				tableRoll: true,
				modifier: 14,
				total: 15,
				dc: 15,
				degree: "failure",
			});
			expect(result.quantity).toBe(quantity);
			expect(result.offBy).toBe(offBy);
			expect(result.arrows).toEqual(arrows);
			expect(result.perfect).toBe(offBy === 0);
			expect(result.bonusRoll).toBeNull();
			expect(result.worstAxis).toBeNull();
		}
	});

	test("5: Truth is blocked only by Wren's level", () => {
		expect(
			brew({ silverleaf: 1, "ghost-orchid": 1, "moonwell-water": 1 }),
		).toEqual({
			kind: "beyond",
			totals: [6, 6, 20, 25],
			potionId: "truth-potion",
			gates: [{ kind: "level", need: 6, have: 5 }],
		});
	});

	test("6: Fire Resistance DC, critical failure and critical success", () => {
		const mix = { "ember-salt": 1, "wyrm-scale-dust": 1, sunpetal: 1 };
		for (const [d20, degree, quantity, bonusRoll] of [
			[1, "critical-failure", 0, null],
			[18, "critical-success", 4, 2],
		] as const) {
			const result = formed(brew(mix, d20));
			expect(result.potionId).toBe("potion-of-fire-resistance");
			expect(result.totals).toEqual([24, 14, 2, 13]);
			expect(result.offBy).toBe(0);
			expect(result.perfect).toBe(true);
			expect(result.arrows).toEqual(perfectArrows);
			expect(result.check).toEqual({
				d20,
				tableRoll: true,
				modifier: 14,
				total: d20 + 14,
				dc: 22,
				degree,
			});
			expect(result.quantity).toBe(quantity);
			expect(result.bonusRoll).toBe(bonusRoll);
			expect(result.worstAxis).toBeNull();
		}
	});

	test("7: Ember Salt is inert with exactly one anonymous hint", () => {
		expect(brew({ "ember-salt": 1 })).toEqual({
			kind: "inert",
			totals: [10, 3, 0, 1],
			hint: { tradition: "arcane", direction: "more" },
		});
	});

	test("8: Wren's modifier and exact d20 odds", () => {
		expect(crafterModifier(WREN)).toBe(14);
		expect(odds(14, 15)).toEqual({
			criticalSuccess: 0.5,
			success: 0.45,
			failure: 0.05,
			criticalFailure: 0,
		});
	});
});

describe("catalog consistency", () => {
	for (const potion of POTIONS) {
		test(`${potion.name}: its perfect recipe forms itself`, () => {
			expect(totalsOf(potion.perfectRecipe)).toEqual(potion.target);
			const result = formed(brew(potion.perfectRecipe, 10, veteran));
			expect(result.potionId).toBe(potion.id);
			expect(result.offBy).toBe(0);
			expect(result.perfect).toBe(true);
			expect(result.arrows).toEqual(perfectArrows);
		});
	}
});

describe("§7 every outcome cell", () => {
	const novice: Crafter = {
		...WREN,
		level: 1,
		craftingRank: "trained",
		intMod: 0,
		tools: "basic",
	};
	const rows: readonly [number, Degree, number, number][] = [
		[20, "critical-success", 2, 4],
		[12, "success", 1, 3],
		[5, "failure", 0, 1],
		[1, "critical-failure", 0, 0],
	];
	for (const [d20, degree, imperfectQuantity, perfectQuantity] of rows) {
		for (const perfect of [false, true]) {
			test(`${degree}, ${perfect ? "perfect" : "imperfect"}`, () => {
				const rollD3 = mock(() => 2);
				const result = formed(
					brew(perfect ? perfectHealing : note, d20, novice, rollD3),
				);
				const getsBonus =
					perfect && (degree === "success" || degree === "critical-success");
				expect(result.check.degree).toBe(degree);
				expect(result.check.total).toBe(d20 + 3);
				expect(result.quantity).toBe(
					perfect ? perfectQuantity : imperfectQuantity,
				);
				expect(result.perfect).toBe(perfect);
				expect(result.offBy).toBe(perfect ? 0 : 14);
				expect(result.bonusRoll).toBe(getsBonus ? 2 : null);
				expect(rollD3).toHaveBeenCalledTimes(getsBonus ? 1 : 0);
				expect(result.worstAxis).toEqual(
					degree === "critical-success" && !perfect
						? { tradition: "occult", target: 10 }
						: null,
				);
			});
		}
	}
	test("the full d3 range is added on perfect successes", () => {
		for (const bonus of [1, 2, 3]) {
			expect(formed(brew(perfectHealing, 6, WREN, () => bonus)).quantity).toBe(
				1 + bonus,
			);
			expect(formed(brew(perfectHealing, 11, WREN, () => bonus)).quantity).toBe(
				2 + bonus,
			);
		}
	});
});

describe("PF2e Crafting", () => {
	test("all proficiency ranks and tool modifiers", () => {
		for (const [craftingRank, expected] of [
			["untrained", 5],
			["trained", 12],
			["expert", 14],
			["master", 16],
			["legendary", 18],
		] as const) {
			expect(crafterModifier({ ...WREN, craftingRank })).toBe(expected);
		}
		expect(crafterModifier({ ...WREN, tools: "basic" })).toBe(13);
		expect(crafterModifier({ ...WREN, tools: "none" })).toBe(11);
	});

	test("level DCs through 20 and rarity adjustments", () => {
		const dcs = [
			14, 15, 16, 18, 19, 20, 22, 23, 24, 26, 27, 28, 30, 31, 32, 34, 35, 36,
			38, 39, 40,
		];
		for (const [level, dc] of dcs.entries()) {
			for (const [rarity, adjustment] of [
				["common", 0],
				["uncommon", 2],
				["rare", 5],
			] as const) {
				expect(potionDc({ ...POTIONS[0], level, rarity })).toBe(
					dc + adjustment,
				);
			}
		}
		expect(() => potionDc({ ...POTIONS[0], level: 21 })).toThrow(
			"Unsupported potion level",
		);
	});

	test("degree thresholds and natural shifts are clamped", () => {
		for (const [total, expected] of [
			[25, "critical-success"],
			[15, "success"],
			[14, "failure"],
			[6, "failure"],
			[5, "critical-failure"],
		] as const) {
			expect(degreeOf(10, total, 15)).toBe(expected);
		}
		for (const [total, twenty, one] of [
			[30, "critical-success", "success"],
			[20, "critical-success", "failure"],
			[10, "success", "critical-failure"],
			[0, "failure", "critical-failure"],
		] as const) {
			expect(degreeOf(20, total, 15)).toBe(twenty);
			expect(degreeOf(1, total, 15)).toBe(one);
		}
	});

	test("odds include natural shifts at extreme modifiers", () => {
		expect(odds(100, 15)).toEqual({
			criticalSuccess: 0.95,
			success: 0.05,
			failure: 0,
			criticalFailure: 0,
		});
		expect(odds(-100, 15)).toEqual({
			criticalSuccess: 0,
			success: 0,
			failure: 0.05,
			criticalFailure: 0.95,
		});
	});
});

describe("resolution gates and ordering", () => {
	test("the closest eligible candidate wins instead of catalog order", () => {
		const mix = { "marsh-toad-gland": 1, silverleaf: 1, "sea-glass-pearl": 1 };
		const result = formed(brew(mix));
		expect(result.totals).toEqual([10, 22, 10, 17]);
		expect(result.potionId).toBe("potion-of-swimming");
		expect(result.offBy).toBe(8);
	});

	test("a blocked closest candidate does not prevent another eligible potion forming", () => {
		const mix = { "marsh-toad-gland": 1, silverleaf: 1, "sea-glass-pearl": 1 };
		const result = formed(brew(mix, 6, { ...WREN, level: 1 }));
		expect(result.potionId).toBe("healing-potion-minor");
		expect(result.offBy).toBe(11);
	});

	test("Beyond names the closest blocked candidate rather than catalog order", () => {
		const mix = { "marsh-toad-gland": 1, silverleaf: 1, "sea-glass-pearl": 1 };
		expect(
			brew(mix, 6, {
				...WREN,
				alchemicalCrafting: false,
				craftingRank: "untrained",
			}),
		).toEqual({
			kind: "beyond",
			totals: [10, 22, 10, 17],
			potionId: "potion-of-swimming",
			gates: [
				{ kind: "feat", feat: "Alchemical Crafting" },
				{ kind: "rank", need: "trained", have: "untrained" },
			],
		});
	});

	test("primary room includes its exact boundary", () => {
		const result = formed(brew({ heartmoss: 2, "moonwell-water": 1 }));
		expect(result.totals).toEqual([6, 30, 6, 22]);
		expect(result.potionId).toBe("healing-potion-minor");
		expect(result.offBy).toBe(18);
	});

	test("inert hint includes distance exactly twenty", () => {
		expect(brew({ heartmoss: 2, "marsh-toad-gland": 1 })).toEqual({
			kind: "inert",
			totals: [9, 40, 1, 14],
			hint: { tradition: "arcane", direction: "less" },
		});
	});

	test("feat and trained rank are required even on perfect recipes", () => {
		expect(
			brew(perfectHealing, 20, {
				...WREN,
				alchemicalCrafting: false,
				craftingRank: "untrained",
			}),
		).toEqual({
			kind: "beyond",
			totals: [7, 26, 10, 13],
			potionId: "healing-potion-minor",
			gates: [
				{ kind: "feat", feat: "Alchemical Crafting" },
				{ kind: "rank", need: "trained", have: "untrained" },
			],
		});
	});

	test("Beyond includes rarity, required ingredient, feat, level AND rank", () => {
		expect(
			brew({ "quicksilver-bead": 2, "ember-salt": 1, sunpetal: 1 }, 20, {
				...WREN,
				level: 1,
				alchemicalCrafting: false,
				craftingRank: "untrained",
			}),
		).toEqual({
			kind: "beyond",
			totals: [40, 8, 6, 13],
			potionId: "potion-of-flying",
			gates: [
				{ kind: "rarity", need: "rare" },
				{ kind: "ingredient", ingredientId: "wyrm-scale-dust" },
				{ kind: "feat", feat: "Alchemical Crafting" },
				{ kind: "level", need: 8, have: 1 },
				{ kind: "rank", need: "master", have: "untrained" },
			],
		});
	});

	test("an uncommon potion requires uncommon-or-rarer material", () => {
		expect(brew({ "ember-salt": 2, sunpetal: 1 })).toEqual({
			kind: "beyond",
			totals: [22, 11, 0, 14],
			potionId: "potion-of-fire-resistance",
			gates: [{ kind: "rarity", need: "uncommon" }],
		});
		expect(
			formed(brew({ "phoenix-ash": 1, "ember-salt": 1, "stormglass-shard": 1 }))
				.potionId,
		).toBe("potion-of-fire-resistance");
	});

	test("required ingredient is independent from rarity", () => {
		expect(brew({ "ghost-orchid": 2, "quicksilver-bead": 1 })).toEqual({
			kind: "beyond",
			totals: [20, 0, 27, 8],
			potionId: "invisibility-potion",
			gates: [{ kind: "ingredient", ingredientId: "shadeleaf" }],
		});
	});

	test("rare potions require master; expert cannot brew even with rare material", () => {
		const mindshield = POTIONS.find((p) => p.id === "mindshield-draught");
		if (!mindshield) throw new Error("Missing Mindshield");
		expect(crafterGates(mindshield, { ...WREN, level: 20 })).toEqual([
			{ kind: "rank", need: "master", have: "expert" },
		]);
		expect(crafterGates(mindshield, veteran)).toEqual([]);
		expect(
			brew(mindshield.perfectRecipe, 20, { ...WREN, level: 20 }).kind,
		).toBe("beyond");
	});

	test("secondary budget is checked before ingredient and crafter gates", () => {
		expect(
			brew({ "ember-salt": 4 }, 20, { ...WREN, alchemicalCrafting: false }),
		).toEqual({
			kind: "inert",
			totals: [40, 12, 0, 4],
			hint: null,
		});
	});

	test("inert can suggest reducing a tradition without naming a potion", () => {
		expect(brew({ "ember-salt": 2, "quicksilver-bead": 1 })).toEqual({
			kind: "inert",
			totals: [34, 6, 3, 2],
			hint: { tradition: "arcane", direction: "less" },
		});
	});

	test("primary uses the largest target with catalog tradition order for ties", () => {
		expect(primaryOf(POTIONS[0])).toBe("primal");
		expect(primaryOf({ ...POTIONS[0], target: [5, 5, 1, 0] })).toBe("arcane");
	});

	test("totals count repeated units and do not mutate the mix", () => {
		const mix = Object.freeze({ "ember-salt": 2 });
		expect(totalsOf(mix)).toEqual([20, 6, 0, 2]);
		expect(totalsOf({})).toEqual([0, 0, 0, 0]);
		expect(() => totalsOf({ unknown: 1 })).toThrow("Unknown ingredient");
	});

	test("the engine preserves server-generated roll provenance", () => {
		const result = formed(
			resolveBrew({
				mix: note,
				crafter: WREN,
				d20: 6,
				tableRoll: false,
				rollD3: () => 3,
			}),
		);
		expect(result.check.tableRoll).toBe(false);
	});
});

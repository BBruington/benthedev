import { INGREDIENTS, POTIONS } from "./catalog";
import {
	type Arrow,
	type BrewInput,
	type BrewResult,
	CRAFTING_RANKS,
	type Crafter,
	type Degree,
	type Gate,
	type Mix,
	type Odds,
	type Potion,
	TRADITIONS,
	type Tradition,
	type Values,
} from "./types";

const LEVEL_DCS = [
	14, 15, 16, 18, 19, 20, 22, 23, 24, 26, 27, 28, 30, 31, 32, 34, 35, 36, 38,
	39, 40,
];
const DEGREES: readonly Degree[] = [
	"critical-failure",
	"failure",
	"success",
	"critical-success",
];
const INGREDIENT_BY_ID = new Map(
	INGREDIENTS.map((ingredient) => [ingredient.id, ingredient]),
);

export function totalsOf(mix: Mix): Values {
	const totals: [number, number, number, number] = [0, 0, 0, 0];
	for (const [id, units] of Object.entries(mix)) {
		const ingredient = INGREDIENT_BY_ID.get(id);
		if (!ingredient) throw new Error(`Unknown ingredient: ${id}`);
		for (let axis = 0; axis < 4; axis++)
			totals[axis] += ingredient.values[axis] * units;
	}
	return totals;
}

export function crafterModifier(c: Crafter): number {
	const rank = CRAFTING_RANKS.indexOf(c.craftingRank);
	const proficiency = rank === 0 ? 0 : c.level + rank * 2;
	return (
		proficiency +
		c.intMod +
		(c.tools === "none" ? -2 : c.tools === "expanded" ? 1 : 0)
	);
}

export function potionDc(p: Potion): number {
	const levelDc = LEVEL_DCS[p.level];
	if (levelDc === undefined)
		throw new Error(`Unsupported potion level: ${p.level}`);
	return levelDc + (p.rarity === "rare" ? 5 : p.rarity === "uncommon" ? 2 : 0);
}

export function primaryOf(p: Potion): Tradition {
	return TRADITIONS[primaryIndex(p)];
}

function primaryIndex(p: Potion): number {
	let primary = 0;
	for (let axis = 1; axis < 4; axis++) {
		if (p.target[axis] > p.target[primary]) primary = axis;
	}
	return primary;
}

export function crafterGates(p: Potion, c: Crafter): Gate[] {
	const gates: Gate[] = [];
	if (!c.alchemicalCrafting)
		gates.push({ kind: "feat", feat: "Alchemical Crafting" });
	if (p.level > c.level)
		gates.push({ kind: "level", need: p.level, have: c.level });
	const need = p.rarity === "rare" ? "master" : "trained";
	if (CRAFTING_RANKS.indexOf(c.craftingRank) < CRAFTING_RANKS.indexOf(need)) {
		gates.push({ kind: "rank", need, have: c.craftingRank });
	}
	return gates;
}

export function degreeOf(d20: number, total: number, dc: number): Degree {
	let degree =
		total >= dc + 10 ? 3 : total >= dc ? 2 : total <= dc - 10 ? 0 : 1;
	if (d20 === 20) degree = Math.min(3, degree + 1);
	if (d20 === 1) degree = Math.max(0, degree - 1);
	return DEGREES[degree];
}

export function odds(modifier: number, dc: number): Odds {
	const counts: Odds = {
		criticalSuccess: 0,
		success: 0,
		failure: 0,
		criticalFailure: 0,
	};
	for (let d20 = 1; d20 <= 20; d20++) {
		const degree = degreeOf(d20, d20 + modifier, dc);
		if (degree === "critical-success") counts.criticalSuccess++;
		else if (degree === "critical-failure") counts.criticalFailure++;
		else counts[degree]++;
	}
	return {
		criticalSuccess: counts.criticalSuccess / 20,
		success: counts.success / 20,
		failure: counts.failure / 20,
		criticalFailure: counts.criticalFailure / 20,
	};
}

function distance(totals: Values, p: Potion): number {
	return p.target.reduce(
		(sum, target, axis) => sum + Math.abs(totals[axis] - target),
		0,
	);
}

function worstIndex(totals: Values, p: Potion): number {
	let worst = 0;
	for (let axis = 1; axis < 4; axis++) {
		if (
			Math.abs(totals[axis] - p.target[axis]) >
			Math.abs(totals[worst] - p.target[worst])
		)
			worst = axis;
	}
	return worst;
}

function ingredientGates(p: Potion, mix: Mix): Gate[] {
	const gates: Gate[] = [];
	if (p.rarity !== "common") {
		const passes = INGREDIENTS.some(
			(ingredient) =>
				(mix[ingredient.id] ?? 0) > 0 &&
				(p.rarity === "rare"
					? ingredient.rarity === "rare"
					: ingredient.rarity !== "common"),
		);
		if (!passes) gates.push({ kind: "rarity", need: p.rarity });
	}
	if (p.requiredIngredient && !(mix[p.requiredIngredient] > 0)) {
		gates.push({ kind: "ingredient", ingredientId: p.requiredIngredient });
	}
	return gates;
}

export function resolveBrew(input: BrewInput): BrewResult {
	const { mix, crafter, d20, tableRoll, rollD3 } = input;
	const totals = totalsOf(mix);
	const candidates = POTIONS.filter((p) => {
		const primary = primaryIndex(p);
		return (
			Math.abs(totals[primary] - p.target[primary]) <= p.primaryRoom &&
			p.target.reduce(
				(sum, target, axis) =>
					sum + (axis === primary ? 0 : Math.abs(totals[axis] - target)),
				0,
			) <= p.secondaryBudget
		);
	}).sort(
		(a, b) => distance(totals, a) - distance(totals, b) || b.level - a.level,
	);
	if (candidates.length === 0) {
		let closest = POTIONS[0];
		for (const p of POTIONS) {
			if (distance(totals, p) < distance(totals, closest)) closest = p;
		}
		const worst = worstIndex(totals, closest);
		return {
			kind: "inert",
			totals,
			hint:
				distance(totals, closest) <= 20
					? {
							tradition: TRADITIONS[worst],
							direction:
								totals[worst] < closest.target[worst] ? "more" : "less",
						}
					: null,
		};
	}
	for (const p of candidates) {
		const gates = [...ingredientGates(p, mix), ...crafterGates(p, crafter)];
		if (gates.length > 0) continue;
		const offBy = distance(totals, p);
		const perfect = offBy === 0;
		const modifier = crafterModifier(crafter);
		const dc = potionDc(p);
		const total = d20 + modifier;
		const degree = degreeOf(d20, total, dc);
		const successful = degree === "success" || degree === "critical-success";
		const bonusRoll = perfect && successful ? rollD3() : null;
		const quantity =
			degree === "critical-success"
				? 2 + (bonusRoll ?? 0)
				: degree === "success"
					? 1 + (bonusRoll ?? 0)
					: degree === "failure" && perfect
						? 1
						: 0;
		const arrows = Object.fromEntries(
			TRADITIONS.map((tradition, axis) => [
				tradition,
				totals[axis] === p.target[axis]
					? "on"
					: totals[axis] < p.target[axis]
						? "low"
						: "high",
			]),
		) as Record<Tradition, Arrow>;
		const worst = worstIndex(totals, p);
		return {
			kind: "formed",
			potionId: p.id,
			totals,
			offBy,
			perfect,
			arrows,
			check: { d20, tableRoll, modifier, total, dc, degree },
			bonusRoll,
			quantity,
			worstAxis:
				degree === "critical-success" && !perfect
					? { tradition: TRADITIONS[worst], target: p.target[worst] }
					: null,
		};
	}
	const blocked = candidates[0];
	return {
		kind: "beyond",
		totals,
		potionId: blocked.id,
		gates: [
			...ingredientGates(blocked, mix),
			...crafterGates(blocked, crafter),
		],
	};
}

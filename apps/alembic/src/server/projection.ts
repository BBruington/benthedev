import {
	INGREDIENTS,
	MENTOR_MIX,
	POTIONS,
	STARTER_POTION_ID,
} from "#/brewing/catalog";
import {
	crafterGates,
	crafterModifier,
	deconstructDc,
	odds,
	potionDc,
} from "#/brewing/engine";
import { maskTotals, maskValues } from "#/brewing/knowledge";
import type {
	BrewLogEntry,
	BrewRecord,
	Crafter,
	CrafterView,
	IngredientView,
	Inventory,
	ItemKind,
	Mix,
	Tradition,
	ViewingAs,
	WorkshopView,
} from "#/brewing/types";

export type Knowledge = Record<
	ItemKind,
	ReadonlyMap<string, ReadonlySet<Tradition>>
>;

export interface CodexRecord {
	potionId: string;
	mentor: boolean;
	formed: boolean;
	revealed: boolean;
	formulaRevealed: boolean;
	perfected: boolean;
	bestOffBy: number | null;
	bestMix: Mix | null;
	note: string | null;
}

export function toCrafterView(
	crafter: Crafter & { downtimeDays: number },
): CrafterView {
	return {
		name: crafter.name,
		className: crafter.className,
		level: crafter.level,
		intMod: crafter.intMod,
		craftingRank: crafter.craftingRank,
		alchemicalCrafting: crafter.alchemicalCrafting,
		tools: crafter.tools,
		downtimeDays: crafter.downtimeDays,
		modifier: crafterModifier(crafter),
	};
}

export function knowledgeFrom(
	rows: readonly { itemKind: ItemKind; itemId: string; tradition: Tradition }[],
): Knowledge {
	const knowledge = {
		ingredient: new Map<string, Set<Tradition>>(),
		potion: new Map<string, Set<Tradition>>(),
	};
	for (const row of rows) {
		const known = knowledge[row.itemKind].get(row.itemId) ?? new Set();
		known.add(row.tradition);
		knowledge[row.itemKind].set(row.itemId, known);
	}
	return knowledge;
}

export function toIngredientViews(input: {
	viewingAs: ViewingAs;
	knowledge: Knowledge;
	modifier: number;
}): IngredientView[] {
	return INGREDIENTS.map(({ values, ...ingredient }) => {
		const dc = deconstructDc(ingredient);
		return {
			...ingredient,
			values: maskValues(
				values,
				input.viewingAs === "gm"
					? "all"
					: (input.knowledge.ingredient.get(ingredient.id) ?? new Set()),
			),
			deconstruct: { dc, odds: odds(input.modifier, dc) },
		};
	});
}

export function toWorkshopView(input: {
	viewingAs: ViewingAs;
	isGm: boolean;
	crafter: Crafter & { downtimeDays: number };
	entries: readonly CodexRecord[];
	log: (Omit<BrewLogEntry, "result"> & { result: BrewRecord })[];
	inventory: Inventory;
	potionStock: Inventory;
	knowledge: Knowledge;
}): WorkshopView {
	const crafter = toCrafterView(input.crafter);
	const ingredients = toIngredientViews({
		viewingAs: input.viewingAs,
		knowledge: input.knowledge,
		modifier: crafter.modifier,
	});
	const codex: WorkshopView["codex"] = [];
	for (const potion of POTIONS) {
		const entry = input.entries.find((row) => row.potionId === potion.id);
		if (!entry || !(entry.mentor || entry.formed || entry.revealed)) continue;
		const dc = potionDc(potion);
		const deconstruct = deconstructDc(potion);
		codex.push({
			potionId: potion.id,
			name: potion.name,
			rarity: potion.rarity,
			level: potion.level,
			dc,
			odds: odds(crafter.modifier, dc),
			crafterGates: crafterGates(potion, crafter),
			bestOffBy: entry.bestOffBy,
			solvedMix:
				(entry.formulaRevealed ? potion.perfectRecipe : null) ??
				(entry.formed ? entry.bestMix : null) ??
				(entry.mentor && potion.id === STARTER_POTION_ID ? MENTOR_MIX : null),
			perfected: entry.perfected,
			formulaRevealed: entry.formulaRevealed,
			source: entry.mentor ? "mentor" : entry.formed ? "brewed" : "revealed",
			note: entry.note,
			traditions: maskValues(
				potion.target,
				input.viewingAs === "gm"
					? "all"
					: (input.knowledge.potion.get(potion.id) ?? new Set()),
			),
			stock: input.potionStock[potion.id] ?? 0,
			deconstruct: {
				dc: deconstruct,
				odds: odds(crafter.modifier, deconstruct),
			},
		});
	}
	return {
		viewingAs: input.viewingAs,
		isGm: input.isGm,
		crafter,
		ingredients,
		inventory: input.inventory,
		codex,
		undiscoveredCount: POTIONS.length - codex.length,
		log: input.log.map((entry) => ({
			...entry,
			result: {
				...entry.result,
				totals: maskTotals(entry.result.totals, entry.mix, ingredients),
			},
		})),
	};
}

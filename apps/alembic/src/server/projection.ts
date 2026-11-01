import {
	INGREDIENTS,
	MENTOR_MIX,
	POTIONS,
	STARTER_POTION_ID,
} from "#/brewing/catalog";
import {
	crafterGates,
	crafterModifier,
	odds,
	potionDc,
} from "#/brewing/engine";
import type {
	BrewLogEntry,
	Crafter,
	CrafterView,
	Inventory,
	Mix,
	ViewingAs,
	WorkshopView,
} from "#/brewing/types";

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

export function toWorkshopView(input: {
	viewingAs: ViewingAs;
	isGm: boolean;
	crafter: Crafter & { downtimeDays: number };
	entries: readonly CodexRecord[];
	log: BrewLogEntry[];
	inventory: Inventory;
}): WorkshopView {
	const crafter = toCrafterView(input.crafter);
	const codex: WorkshopView["codex"] = [];
	for (const potion of POTIONS) {
		const entry = input.entries.find((row) => row.potionId === potion.id);
		if (!entry || !(entry.mentor || entry.formed || entry.revealed)) continue;
		const dc = potionDc(potion);
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
		});
	}
	return {
		viewingAs: input.viewingAs,
		isGm: input.isGm,
		crafter,
		ingredients: INGREDIENTS,
		inventory: input.inventory,
		codex,
		undiscoveredCount: POTIONS.length - codex.length,
		log: input.log,
	};
}

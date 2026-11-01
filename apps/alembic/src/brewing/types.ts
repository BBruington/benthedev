export const TRADITIONS = ["arcane", "primal", "occult", "divine"] as const;
export type Tradition = (typeof TRADITIONS)[number];
/** Always A/P/O/D order. */
export type Values = readonly [number, number, number, number];

export type Rarity = "common" | "uncommon" | "rare";
export const CRAFTING_RANKS = [
	"untrained",
	"trained",
	"expert",
	"master",
	"legendary",
] as const;
export type CraftingRank = (typeof CRAFTING_RANKS)[number];
export const TOOLS = ["none", "basic", "expanded"] as const;
export type Tools = (typeof TOOLS)[number];
export type Degree =
	| "critical-success"
	| "success"
	| "failure"
	| "critical-failure";
export type Arrow = "on" | "low" | "high";

export interface Ingredient {
	id: string;
	name: string;
	rarity: Rarity;
	type: string;
	form: string;
	biome: string;
	values: Values;
}

export interface Potion {
	id: string;
	name: string;
	level: number;
	rarity: Rarity;
	target: Values;
	primaryRoom: number;
	secondaryBudget: number;
	requiredIngredient?: string;
	perfectRecipe: Readonly<Record<string, number>>;
}

export interface Crafter {
	name: string;
	className: string;
	level: number;
	intMod: number;
	craftingRank: CraftingRank;
	alchemicalCrafting: boolean;
	tools: Tools;
}

/** ingredientId -> units; 1-4 units total. */
export type Mix = Readonly<Record<string, number>>;

/** ingredientId -> units in stock. */
export type Inventory = Readonly<Record<string, number>>;

export type Gate =
	| { kind: "feat"; feat: "Alchemical Crafting" }
	| { kind: "level"; need: number; have: number }
	| { kind: "rank"; need: CraftingRank; have: CraftingRank }
	| { kind: "rarity"; need: "uncommon" | "rare" }
	| { kind: "ingredient"; ingredientId: string };

export interface Check {
	d20: number;
	tableRoll: boolean;
	modifier: number;
	total: number;
	dc: number;
	degree: Degree;
}

export type BrewResult =
	| {
			kind: "formed";
			potionId: string;
			totals: Values;
			offBy: number;
			perfect: boolean;
			arrows: Record<Tradition, Arrow>;
			check: Check;
			/** the 1d3 rolled on a perfect success/critical success */
			bonusRoll: number | null;
			quantity: number;
			/** critical success only */
			worstAxis: { tradition: Tradition; target: number } | null;
	  }
	| {
			kind: "inert";
			totals: Values;
			hint: { tradition: Tradition; direction: "more" | "less" } | null;
	  }
	| { kind: "beyond"; totals: Values; potionId: string; gates: Gate[] };

export interface BrewInput {
	mix: Mix;
	crafter: Crafter;
	/** 1-20 */
	d20: number;
	tableRoll: boolean;
	/** returns 1-3; only called when needed */
	rollD3: () => number;
}

export interface Odds {
	criticalSuccess: number;
	success: number;
	failure: number;
	criticalFailure: number;
}

/* ---- API views (player-safe unless noted) ---- */

export type ViewingAs = "gm" | "player";

export interface CrafterView extends Crafter {
	modifier: number;
	downtimeDays: number;
}

export interface CodexEntryView {
	potionId: string;
	name: string;
	rarity: Rarity;
	level: number;
	dc: number;
	odds: Odds;
	/** gates the crafter currently fails (excluding mix-dependent ones) */
	crafterGates: Gate[];
	bestOffBy: number | null;
	solvedMix: Mix | null;
	formulaRevealed: boolean;
	perfected: boolean;
	source: "mentor" | "brewed" | "revealed";
	note: string | null;
}

export interface BrewLogEntry {
	id: string;
	createdAt: string;
	mix: Mix;
	result: BrewView;
}

/** BrewResult as sent to the player: potionName added, nothing else hidden needed beyond engine output. */
export type BrewView = BrewResult & { potionName: string | null };

export interface WorkshopView {
	viewingAs: ViewingAs;
	isGm: boolean;
	crafter: CrafterView;
	ingredients: readonly Ingredient[];
	inventory: Inventory;
	codex: CodexEntryView[];
	undiscoveredCount: number;
	log: BrewLogEntry[];
}

/** GM only. */
export interface AnswerKeyEntry {
	potion: Potion;
	primary: Tradition;
	dc: number;
	known: boolean;
	revealed: boolean;
	formulaRevealed: boolean;
	perfected: boolean;
}

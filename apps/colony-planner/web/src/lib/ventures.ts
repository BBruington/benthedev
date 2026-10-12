import type { Schemas } from "@/api/client";
import { formatNumber, parseWholeNumber } from "./colony";

type Effects = Schemas["Effects"];
type QuestStatus = Schemas["QuestStatus"];
type QuestKind = Schemas["QuestKind"];
type RequestStatus = Schemas["RequestStatus"];

export const questStatusLabels: Record<QuestStatus, string> = {
	ACTIVE: "Active",
	COMPLETED: "Done",
	FAILED: "Failed",
};

export const questKindLabels: Record<QuestKind, string> = {
	COLONY: "Colony",
	MAINLAND: "Mainland",
};

export const requestStatusLabels: Record<RequestStatus, string> = {
	APPROVED: "Granted",
	DECLINED: "Declined",
	PENDING: "Waiting",
	WITHDRAWN: "Withdrawn",
};

/** The quests page's three shelves (spec §6.2), each in the order the server sent. */
export function groupQuestsByStatus<Quest extends { status: QuestStatus }>(
	quests: readonly Quest[],
): Record<QuestStatus, Quest[]> {
	const groups: Record<QuestStatus, Quest[]> = { ACTIVE: [], COMPLETED: [], FAILED: [] };
	for (const quest of quests) {
		groups[quest.status].push(quest);
	}
	return groups;
}

/** What an effects editor holds: the typed text of each resource's change and of new colonists. */
export type EffectsDraft = { resources: Record<string, string>; colonists: string };

export const emptyEffectsDraft: EffectsDraft = { colonists: "", resources: {} };

export function effectsToDraft(effects: Effects): EffectsDraft {
	const resources: Record<string, string> = {};
	for (const [key, amount] of Object.entries(effects.resources ?? {})) {
		if (amount !== 0) {
			resources[key] = String(amount);
		}
	}
	const colonists = effects.colonists ?? 0;
	return { colonists: colonists === 0 ? "" : String(colonists), resources };
}

/**
 * The Effects a draft describes, or the problems with it keyed by resource key (and "colonists").
 * Blank and zero changes are left out; colonists only ever arrive (spec §5.4).
 */
export function draftToEffects(draft: EffectsDraft): {
	effects: Effects;
	problems: Record<string, string>;
} {
	const resources: Record<string, number> = {};
	const problems: Record<string, string> = {};
	for (const [key, text] of Object.entries(draft.resources)) {
		const amount = parseWholeNumber(text);
		if (amount === "invalid") {
			problems[key] = "Enter a whole number, like 5 or -3.";
		} else if (amount !== null && amount !== 0) {
			resources[key] = amount;
		}
	}
	const colonists = parseWholeNumber(draft.colonists);
	let colonistCount = 0;
	if (colonists === "invalid" || (typeof colonists === "number" && colonists < 0)) {
		problems.colonists = "Enter 0 or more new colonists.";
	} else if (typeof colonists === "number") {
		colonistCount = colonists;
	}
	return { effects: { colonists: colonistCount, resources }, problems };
}

/**
 * The resource keys a draft names that aren't on the board (archived or deleted since it was saved).
 * The server refuses them, so the editor shows each as a row the DM can clear.
 */
export function inactiveEffectKeys(
	draft: EffectsDraft,
	activeResources: readonly { key: string }[],
): string[] {
	const activeKeys = new Set(activeResources.map((resource) => resource.key));
	return Object.keys(draft.resources)
		.filter((key) => !activeKeys.has(key))
		.sort();
}

export function withoutEffectResource(draft: EffectsDraft, key: string): EffectsDraft {
	const resources = Object.fromEntries(
		Object.entries(draft.resources).filter(([resourceKey]) => resourceKey !== key),
	);
	return { ...draft, resources };
}

export function isEmptyEffects(effects: Effects): boolean {
	const hasResource = Object.values(effects.resources ?? {}).some((amount) => amount !== 0);
	return !hasResource && (effects.colonists ?? 0) === 0;
}

/** A favor amount typed into a form: a whole number, negative only when `allowNegative`. */
export function parseFavor(
	text: string,
	allowNegative: boolean,
): { value: number; problem?: undefined } | { value?: undefined; problem: string } {
	const parsed = parseWholeNumber(text);
	if (parsed === null) {
		return { value: 0 };
	}
	if (parsed === "invalid" || (!allowNegative && parsed < 0)) {
		return { problem: allowNegative ? "Enter a whole number." : "Enter 0 or more." };
	}
	return { value: parsed };
}

type StockedResource = { key: string; label: string; amount: number; minAmount: number };

/**
 * What a STRICT spend would fall short on (spec §5.4), worded as the server words it: "Not enough
 * favor: Lady Vex has 2, needs 3", then each resource. Null when it is affordable. The server
 * re-checks; this only keeps the UI from offering a request it would refuse.
 */
export function spendShortfall(
	patron: { name: string; favor: number },
	favorCost: number,
	effects: Effects,
	resources: readonly StockedResource[],
): string | null {
	const messages: string[] = [];
	// A free action is affordable at any favor, even below 0 (game.Spend).
	if (favorCost > 0 && patron.favor < favorCost) {
		messages.push(`Not enough favor: ${patron.name} has ${patron.favor}, needs ${favorCost}`);
	}
	const amounts = effects.resources ?? {};
	for (const resource of [...resources].sort((first, second) =>
		first.key.localeCompare(second.key),
	)) {
		const delta = amounts[resource.key];
		if (delta === undefined || delta >= 0) {
			continue;
		}
		if (delta < resource.minAmount - resource.amount) {
			messages.push(
				`Not enough ${resource.label.toLowerCase()}: the colony has ${resource.amount}, needs ${resource.minAmount - delta}`,
			);
		}
	}
	return messages.length === 0 ? null : messages.join(". ");
}

/** "Favor 12 → 9": a patron's favor before and after, for previews. */
export function favorChangeLine(change: { patronName: string; before: number; after: number }) {
	return `${change.patronName} favor ${formatNumber(change.before)} → ${formatNumber(change.after)}`;
}

/** A loot row as typed: its quantity is text until it's saved, and rowKey keys it while it's edited. */
export type LootDraft = { name: string; quantityText: string; description: string; rowKey: string };

// Not crypto.randomUUID: browsers offer it only on secure origins, and dev:phone serves plain http.
let lootRowCount = 0;

export function newLootDraft(): LootDraft {
	lootRowCount += 1;
	return { description: "", name: "", quantityText: "", rowKey: `new-${lootRowCount}` };
}

export function lootToDrafts(loot: readonly Schemas["LootItem"][]): LootDraft[] {
	return loot.map((item, position) => ({
		description: item.description,
		name: item.name,
		quantityText: String(item.quantity),
		rowKey: `saved-${position}`,
	}));
}

export type LootProblem = { field: "name" | "quantity"; message: string };

/** The loot list to save, skipping blank rows; a quantity left empty is 1. */
export function draftsToLoot(drafts: readonly LootDraft[]): {
	loot: Schemas["LootItem"][];
	problems: Record<number, LootProblem>;
} {
	const loot: Schemas["LootItem"][] = [];
	const problems: Record<number, LootProblem> = {};
	drafts.forEach((draft, position) => {
		const name = draft.name.trim();
		if (name === "" && draft.description.trim() === "" && draft.quantityText.trim() === "") {
			return;
		}
		const quantity = parseWholeNumber(draft.quantityText);
		if (name === "") {
			problems[position] = { field: "name", message: "Name the item." };
		} else if (quantity === "invalid" || (typeof quantity === "number" && quantity < 1)) {
			problems[position] = { field: "quantity", message: "Enter 1 or more." };
		} else {
			loot.push({ description: draft.description.trim(), name, quantity: quantity ?? 1 });
		}
	});
	return { loot, problems };
}

/** A list key per saved loot item: its name and description, counted so a repeated item stays distinct. */
export function lootItemKeys(loot: readonly Schemas["LootItem"][]): string[] {
	const seen = new Map<string, number>();
	return loot.map((item) => {
		const itemKey = JSON.stringify([item.name, item.description]);
		const occurrence = (seen.get(itemKey) ?? 0) + 1;
		seen.set(itemKey, occurrence);
		return `${itemKey}#${occurrence}`;
	});
}

/**
 * What the DM can do about a refused grant, or null when it didn't fall short ("already decided").
 * The server names favor first ("Not enough favor: Lady Vex has 2, needs 3"), then each resource
 * ("Not enough food: the colony has 4, needs 10"). A tweak can't lower the favor cost, and adjusting
 * favor can't cover a resource.
 */
export function shortfallAdvice(message: string): string | null {
	if (!message.startsWith("Not enough ")) {
		return null;
	}
	const favorShort =
		message.startsWith("Not enough favor: ") &&
		!message.startsWith("Not enough favor: the colony has ");
	const resourceShort = message.includes(": the colony has ");
	if (favorShort && resourceShort) {
		return "Adjust favor and tweak the grant, then try again.";
	}
	if (favorShort) {
		return "Adjust favor, then try again.";
	}
	return "Tweak the grant, then try again.";
}

import type { Schemas } from "@/api/client";
import type { NpcListFilter } from "@/api/queries";
import { parseWholeNumber } from "./colony";

type NpcStatus = Schemas["NpcStatus"];
type NpcHealth = Schemas["NpcHealth"];
type NpcRelation = Schemas["NpcRelation"];

export const npcStatusLabels: Record<NpcStatus, string> = {
	ALIVE: "Alive",
	DEAD: "Dead",
	MISSING: "Missing",
	UNKNOWN: "Unknown",
};

export const npcHealthLabels: Record<NpcHealth, string> = {
	DEAD: "Dead",
	HEALTHY: "Healthy",
	INJURED: "Injured",
	SICK: "Sick",
	UNKNOWN: "Unknown",
};

export const npcRelationLabels: Record<NpcRelation, string> = {
	ALLY: "Ally",
	ENEMY: "Enemy",
	NEUTRAL: "Neutral",
	SUSPICIOUS: "Suspicious",
	UNKNOWN: "Unknown",
};

/** The enum values in a stable order, for pickers and filters. */
export function enumValues<Value extends string>(labels: Record<Value, string>): Value[] {
	return Object.keys(labels).filter((value): value is Value => Object.hasOwn(labels, value));
}

/**
 * Comma-separated text as a clean list, for aliases and labels: trimmed, blanks dropped, and a
 * repeat (ignoring case) kept only once, in the order typed.
 */
export function parseList(text: string): string[] {
	const seen = new Set<string>();
	const items: string[] = [];
	for (const part of text.split(",")) {
		const item = part.trim();
		const folded = item.toLocaleLowerCase();
		if (item === "" || seen.has(folded)) {
			continue;
		}
		seen.add(folded);
		items.push(item);
	}
	return items;
}

export type ColonistFilter = "all" | "colonists" | "others";
export type HiddenFilter = "all" | "hidden" | "visible";

export type RosterFilters = {
	search: string;
	status: NpcStatus | "";
	relation: NpcRelation | "";
	colonists: ColonistFilter;
	/** The DM's own filter; players only ever receive revealed NPCs. */
	hidden: HiddenFilter;
};

export const noRosterFilters: RosterFilters = {
	colonists: "all",
	hidden: "all",
	relation: "",
	search: "",
	status: "",
};

/**
 * The roster's filters as the server's query. "Not colonists" has no server filter, so the page
 * narrows that one itself (see keepsNpc).
 */
export function rosterQuery(filters: RosterFilters, isDm: boolean): NpcListFilter {
	const query: NpcListFilter = {};
	const search = filters.search.trim();
	if (search !== "") {
		query.search = search;
	}
	if (filters.status !== "") {
		query.status = filters.status;
	}
	if (filters.relation !== "") {
		query.relation = filters.relation;
	}
	if (filters.colonists === "colonists") {
		query.colonistsOnly = true;
	}
	if (isDm && filters.hidden === "hidden") {
		query.revealed = "false";
	} else if (isDm && filters.hidden === "visible") {
		query.revealed = "true";
	}
	return query;
}

export function keepsNpc(filters: RosterFilters, npc: { colonyRole: unknown }): boolean {
	return filters.colonists !== "others" || npc.colonyRole === null;
}

export function hasRosterFilters(filters: RosterFilters): boolean {
	return (
		filters.search.trim() !== "" ||
		filters.status !== "" ||
		filters.relation !== "" ||
		filters.colonists !== "all" ||
		filters.hidden !== "all"
	);
}

/** A role's headcount must be 0 before it can be archived; Unassigned never can be (spec §4.2). */
export function canArchiveRole(role: { isSystem: boolean; headcount: number }): boolean {
	return !role.isSystem && role.headcount === 0;
}

/**
 * The problem with a number of unnamed colonists to move out of a role, or undefined when it can
 * move. Blank text is no problem yet: the form simply can't be sent.
 */
export function transferCountProblem(
	text: string,
	from: { label: string; unnamedCount: number } | undefined,
): string | undefined {
	const count = parseWholeNumber(text);
	if (count === null) {
		return undefined;
	}
	if (count === "invalid" || count < 1) {
		return "Enter a whole number, 1 or more.";
	}
	if (from && count > from.unnamedCount) {
		const available =
			from.unnamedCount === 1 ? "1 unnamed colonist" : `${from.unnamedCount} unnamed colonists`;
		return `${from.label} has only ${available}, so ${count} can't move.`;
	}
	return undefined;
}

/** The problem with a role's new unnamed count, or undefined when it's fine or still blank. */
export function roleCountProblem(text: string): string | undefined {
	const count = parseWholeNumber(text);
	if (count === null) {
		return undefined;
	}
	if (count === "invalid" || count < 0) {
		return "Enter a whole number, 0 or more.";
	}
	return undefined;
}

/** The true name, for the DM, when the party knows the NPC by another one; otherwise null. */
export function hiddenTrueName(npc: { displayName: string; dm?: { name: string } }): string | null {
	if (npc.dm && npc.dm.name !== npc.displayName) {
		return npc.dm.name;
	}
	return null;
}

// Pure helpers for the Buildings page (docs/design/construction.md): text, grouping and the parsing of
// its sheets. No React, no I/O, so each one has a unit test in construction.test.ts.
import type { Schemas } from "@/api/client";
import { dayName, formatNumber, labelFromResourceKey, parseWholeNumber } from "./colony";

type Building = Schemas["Building"];
type ConstructionRole = Schemas["ConstructionRole"];
type Labelled = { key: string; label: string };
type StockedResource = Labelled & { amount: number; minAmount: number };

/** "The Granary is finished on Day 52.", as the Advance preview names each finish. */
export function finishLine(finish: { name: string; day: number }): string {
	return `The ${finish.name} is finished on ${dayName(finish.day)}.`;
}

/** "Builders": a role's colonists, as the server's own texts name them. */
export function pluralRole(label: string): string {
	return `${label}s`;
}

/** "1 Builder", "3 Builders". */
export function roleCount(count: number, label: string): string {
	return `${formatNumber(count)} ${count === 1 ? label : pluralRole(label)}`;
}

/** The label of a role the page lists, or its key read back as a word ("MAGE" → "Mage"). */
export function roleLabelOf(roleKey: string, roles: readonly { roleKey: string; label: string }[]) {
	const role = roles.find((candidate) => candidate.roleKey === roleKey);
	if (role) {
		return role.label;
	}
	return labelFromResourceKey(roleKey.toLowerCase());
}

/** "45 / 60 Builder-days". */
export function progressText(line: { progress: number; effort: number }, label: string): string {
	return `${formatNumber(line.progress)} / ${formatNumber(line.effort)} ${label}-days`;
}

/** How far a line is, 0 to 100, for its progress bar. */
export function progressPercent(line: { progress: number; effort: number }): number {
	if (line.effort <= 0) {
		return 100;
	}
	return Math.min(100, Math.round((line.progress / line.effort) * 100));
}

export function isLineDone(line: { progress: number; effort: number }): boolean {
	return line.progress >= line.effort;
}

/** "Finishes on Day 52", or what keeps it from finishing: "Needs Mages on the crew". */
export function projectedFinishText(building: Building, roleLabel: (roleKey: string) => string) {
	if (building.projectedFinishDay !== null) {
		return `Finishes on ${dayName(building.projectedFinishDay)}`;
	}
	const missing = building.labor
		.filter((line) => !isLineDone(line) && line.crew === 0)
		.map((line) => pluralRole(roleLabel(line.roleKey)));
	if (missing.length === 0) {
		return "No finish day yet";
	}
	return `Needs ${missing.join(" and ")} on the crew`;
}

/** "40 Builder-days, up to 4 at once". */
export function laborText(line: { effort: number; maxCrew: number | null }, label: string): string {
	const work = `${formatNumber(line.effort)} ${label}-days`;
	if (line.maxCrew === null) {
		return work;
	}
	return `${work}, up to ${formatNumber(line.maxCrew)} at once`;
}

/**
 * Colonists of a role working somewhere this viewer can't see (a hidden blueprint's project): the
 * headcount less the crews listed here and the idle ones. Always 0 for the DM.
 */
export function otherWorkCount(role: ConstructionRole): number {
	return Math.max(0, role.headcount - role.crewed - role.idle);
}

/** "Materials 30 · Gold 20", in board order; a resource the board no longer lists is named from its key. */
export function amountsText(amounts: Record<string, number>, resources: readonly Labelled[]) {
	const parts: string[] = [];
	const known = new Set<string>();
	for (const resource of resources) {
		known.add(resource.key);
		const amount = amounts[resource.key];
		if (amount !== undefined && amount !== 0) {
			parts.push(`${resource.label} ${formatNumber(amount)}`);
		}
	}
	for (const key of Object.keys(amounts).sort()) {
		const amount = amounts[key];
		if (!known.has(key) && amount !== undefined && amount !== 0) {
			parts.push(`${labelFromResourceKey(key)} ${formatNumber(amount)}`);
		}
	}
	return parts.join(" · ");
}

export type BuildingGroups = {
	constructing: Building[];
	standing: Building[];
	past: Building[];
};

/** Under construction, standing (finished) and past (cancelled or demolished), each in the server's order. */
export function groupBuildings(buildings: readonly Building[]): BuildingGroups {
	const groups: BuildingGroups = { constructing: [], past: [], standing: [] };
	for (const building of buildings) {
		if (building.status === "CONSTRUCTING") {
			groups.constructing.push(building);
		} else if (building.status === "FINISHED") {
			groups.standing.push(building);
		} else {
			groups.past.push(building);
		}
	}
	return groups;
}

/**
 * Why Start would be refused for the stockpile, worded as the server words it ("Not enough stone:
 * the colony has 40, needs 60"), or null when the colony can pay. Start is STRICT, so a cost may
 * take a resource down to its minimum and no further. The server re-checks.
 */
export function costShortfall(
	cost: Record<string, number>,
	resources: readonly StockedResource[],
): string | null {
	const messages: string[] = [];
	for (const key of Object.keys(cost).sort()) {
		const amount = cost[key] ?? 0;
		const resource = resources.find((candidate) => candidate.key === key);
		if (!resource) {
			messages.push(
				`${labelFromResourceKey(key)} is archived, so the DM has to edit the blueprint`,
			);
		} else if (resource.amount - amount < resource.minAmount) {
			messages.push(
				`Not enough ${resource.label.toLowerCase()}: the colony has ${formatNumber(resource.amount)}, needs ${formatNumber(resource.minAmount + amount)}`,
			);
		}
	}
	return messages.length === 0 ? null : `${messages.join(". ")}.`;
}

/** Typed text per key, for the crew, cost and refund editors. */
export type NumberDraft = Record<string, string>;

export function amountsToDraft(amounts: Record<string, number>): NumberDraft {
	return Object.fromEntries(
		Object.entries(amounts)
			.filter(([, amount]) => amount !== 0)
			.map(([key, amount]) => [key, String(amount)]),
	);
}

/**
 * Amounts of 0 or more (a cost or a refund), or the problem with each, keyed like the draft. Blank and
 * 0 are left out: the server takes {} for nothing.
 */
export function parseAmounts(draft: NumberDraft): {
	amounts: Record<string, number>;
	problems: Record<string, string>;
} {
	const amounts: Record<string, number> = {};
	const problems: Record<string, string> = {};
	for (const [key, text] of Object.entries(draft)) {
		const amount = parseWholeNumber(text);
		if (amount === "invalid" || (typeof amount === "number" && amount < 0)) {
			problems[key] = "Enter a whole number, 0 or more.";
		} else if (typeof amount === "number" && amount > 0) {
			amounts[key] = amount;
		}
	}
	return { amounts, problems };
}

/**
 * The Cancel sheet's prefill: the full cost Start took, less any resource archived since, which the
 * server refuses in a refund and the sheet names instead.
 */
export function refundPrefill(
	cost: Record<string, number>,
	resources: readonly Labelled[],
): { draft: NumberDraft; leftOut: string[] } {
	const draft: NumberDraft = {};
	const leftOut: string[] = [];
	for (const key of Object.keys(cost).sort()) {
		const amount = cost[key] ?? 0;
		if (resources.some((resource) => resource.key === key)) {
			draft[key] = String(amount);
		} else {
			leftOut.push(labelFromResourceKey(key));
		}
	}
	return { draft, leftOut };
}

/** One labor line a crew sheet sets: the role, its crew cap, and the crew it has now (0 at Start). */
export type CrewLine = { roleKey: string; maxCrew: number | null; crew: number };

/**
 * The crews to send, or the problem with each line. A line takes at most its cap and the role's free
 * colonists: the idle ones plus those already on this line. The server re-checks under the lock.
 */
export function parseCrews(
	draft: NumberDraft,
	lines: readonly CrewLine[],
	roles: readonly ConstructionRole[],
): { crews: Record<string, number>; problems: Record<string, string> } {
	const crews: Record<string, number> = {};
	const problems: Record<string, string> = {};
	for (const line of lines) {
		const crew = parseWholeNumber(draft[line.roleKey] ?? "");
		const role = roles.find((candidate) => candidate.roleKey === line.roleKey);
		const label = role?.label ?? labelFromResourceKey(line.roleKey.toLowerCase());
		const free = freeColonists(line, roles);
		if (crew === "invalid" || (typeof crew === "number" && crew < 0)) {
			problems[line.roleKey] = "Enter a whole number, 0 or more.";
		} else if (crew !== null && line.maxCrew !== null && crew > line.maxCrew) {
			problems[line.roleKey] = `At most ${roleCount(line.maxCrew, label)} can work on this.`;
		} else if (crew !== null && crew > line.crew && crew > free) {
			problems[line.roleKey] = `${freeText(free, label)}.`;
		} else {
			crews[line.roleKey] = crew ?? 0;
		}
	}
	return { crews, problems };
}

/** "Only 2 Mages are free", "Only 1 Mage is free" or "No Mages are free", as the server says it. */
export function freeText(free: number, label: string): string {
	if (free <= 0) {
		return `No ${pluralRole(label)} are free`;
	}
	return `Only ${roleCount(free, label)} ${free === 1 ? "is" : "are"} free`;
}

/** A line's role's idle colonists plus those already on this line. */
export function freeColonists(line: CrewLine, roles: readonly ConstructionRole[]): number {
	const idle = roles.find((role) => role.roleKey === line.roleKey)?.idle ?? 0;
	return idle + line.crew;
}

/** A labor line as typed in the blueprint sheet; rowKey keys it while it's edited. */
export type LaborDraft = {
	roleKey: string;
	effortText: string;
	maxCrewText: string;
	rowKey: string;
};

// Not crypto.randomUUID: browsers offer it only on secure origins, and dev:phone serves plain http.
let laborRowCount = 0;

export function newLaborDraft(roleKey: string): LaborDraft {
	laborRowCount += 1;
	return { effortText: "", maxCrewText: "", roleKey, rowKey: `new-${laborRowCount}` };
}

export function laborToDrafts(labor: readonly Schemas["BlueprintLabor"][]): LaborDraft[] {
	return labor.map((line) => ({
		effortText: String(line.effort),
		maxCrewText: line.maxCrew === null ? "" : String(line.maxCrew),
		roleKey: line.roleKey,
		rowKey: `saved-${line.roleKey}`,
	}));
}

/** The labor map to save, or the problem with each row (by rowKey) and with the whole ("labor"). */
export function parseLabor(drafts: readonly LaborDraft[]): {
	labor: Record<string, Schemas["BlueprintLaborInput"]>;
	problems: Record<string, string>;
} {
	const labor: Record<string, Schemas["BlueprintLaborInput"]> = {};
	const problems: Record<string, string> = {};
	for (const draft of drafts) {
		const effort = parseWholeNumber(draft.effortText);
		const maxCrew = parseWholeNumber(draft.maxCrewText);
		if (draft.roleKey in labor) {
			problems[draft.rowKey] = "This role already has a line. Use one line per role.";
		} else if (effort === null || effort === "invalid" || effort < 1) {
			problems[draft.rowKey] = "Enter the work in role-days, 1 or more.";
		} else if (maxCrew === "invalid" || (typeof maxCrew === "number" && maxCrew < 1)) {
			problems[draft.rowKey] = "A crew cap is 1 or more. Leave it blank for none.";
		} else {
			labor[draft.roleKey] = { effort, maxCrew };
		}
	}
	if (drafts.length === 0) {
		problems.labor =
			"Add at least one role's work. A building that needs none is a stock adjustment.";
	}
	return { labor, problems };
}

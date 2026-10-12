// Pure helpers for the colony board and the timeline: paging, numbers and text. No React, no I/O,
// so each one has a unit test in colony.test.ts.

const numberFormat = new Intl.NumberFormat("en-US");
const wholeNumberPattern = /^[+-]?\d+$/;

/** 12,345 with grouping; the board and the timeline show every number this way. */
export function formatNumber(value: number): string {
	return numberFormat.format(value);
}

/** "+20" or "−39" (a true minus sign), as on the delta chips; 0 reads "0". */
export function formatDelta(amount: number): string {
	if (amount === 0) {
		return "0";
	}
	const magnitude = formatNumber(Math.abs(amount));
	return amount > 0 ? `+${magnitude}` : `−${magnitude}`;
}

/** "Day 44", or "Founding" for Day 0 (spec §5.8). */
export function dayName(day: number): string {
	if (day === 0) {
		return "Founding";
	}
	return `Day ${formatNumber(day)}`;
}

// Timeline pages hold seven days: Days 1–7, 8–14 … 36–42 (spec §5.8 labels a page "Days 36–42"). Day 0,
// the founding, joins the first page rather than standing alone on a page of one.
export const daysPerPage = 7;

export type DayPage = { fromDay: number; toDay: number };

/** The page that holds `day`. */
export function pageForDay(day: number): DayPage {
	const safeDay = Math.max(0, Math.trunc(day));
	if (safeDay <= daysPerPage) {
		return { fromDay: 0, toDay: daysPerPage };
	}
	const pageIndex = Math.floor((safeDay - 1) / daysPerPage);
	const fromDay = pageIndex * daysPerPage + 1;
	return { fromDay, toDay: fromDay + daysPerPage - 1 };
}

/** The page before this one, or null on the first page. */
export function previousPage(page: DayPage): DayPage | null {
	if (page.fromDay === 0) {
		return null;
	}
	return pageForDay(page.fromDay - 1);
}

/** The page after this one. */
export function nextPage(page: DayPage): DayPage {
	return pageForDay(page.toDay + 1);
}

/** "Days 36–42", or "Founding – Day 7" for the first page. */
export function pageLabel(page: DayPage): string {
	if (page.fromDay === 0) {
		return `Founding – Day ${formatNumber(page.toDay)}`;
	}
	return `Days ${formatNumber(page.fromDay)}–${formatNumber(page.toDay)}`;
}

/** Every day on a page, in order. */
export function daysOfPage(page: DayPage): number[] {
	const days: number[] = [];
	for (let day = page.fromDay; day <= page.toDay; day += 1) {
		days.push(day);
	}
	return days;
}

/** "in 1 day", "in 3 days". */
export function inDays(count: number): string {
	return count === 1 ? "in 1 day" : `in ${formatNumber(count)} days`;
}

export type ShipmentSummary = { intervalDays: number; nextDay: number; daysUntil: number } | null;

/** "Next ship Day 47, in 3 days", or a line saying none is scheduled. */
export function shipmentLine(shipments: ShipmentSummary): string {
	if (!shipments) {
		return "No ships scheduled";
	}
	return `Next ship Day ${formatNumber(shipments.nextDay)}, ${inDays(shipments.daysUntil)}`;
}

/** How far the current voyage has come, 0 to 1, for the day dial's arc. */
export function voyageProgress(shipments: ShipmentSummary): number {
	if (!shipments || shipments.intervalDays <= 0) {
		return 0;
	}
	const done = shipments.intervalDays - shipments.daysUntil;
	return Math.min(1, Math.max(0, done / shipments.intervalDays));
}

/** "62 colonists · 5 unassigned". */
export function colonistsLine(colonists: number, unassigned: number): string {
	const total = colonists === 1 ? "1 colonist" : `${formatNumber(colonists)} colonists`;
	return `${total} · ${formatNumber(unassigned)} unassigned`;
}

/**
 * A whole number typed into a field. Empty text is null (an optional field left blank sends null,
 * never NaN, spec §5.3); anything that isn't a whole number is "invalid", so the form can say so.
 */
export function parseWholeNumber(text: string): number | null | "invalid" {
	const trimmed = text.trim().replaceAll(",", "");
	if (trimmed === "") {
		return null;
	}
	if (!wholeNumberPattern.test(trimmed)) {
		return "invalid";
	}
	const value = Number(trimmed);
	if (!Number.isSafeInteger(value)) {
		return "invalid";
	}
	return value;
}

/** Advancing more than this many days asks the DM to type the number again (spec §5.2). */
export const retypeAboveDays = 365;

export function needsRetype(days: number): boolean {
	return days > retypeAboveDays;
}

type Labelled = { key: string; label: string };

/** "dragon_scales" → "Dragon scales": a resource's key read back as words, for one the board no longer lists. */
export function labelFromResourceKey(key: string): string {
	const words = key.replaceAll("_", " ");
	return words.charAt(0).toUpperCase() + words.slice(1);
}

/**
 * "Food −39 · Gold +10 · 4 colonists": an entry's applied deltas in board order. A resource the
 * board no longer lists (archived) has no label here, so it is named from its key.
 */
export function describeEffects(
	effects: { resources?: Record<string, number>; colonists?: number },
	resources: readonly Labelled[],
): string {
	const parts: string[] = [];
	const amounts = effects.resources ?? {};
	const known = new Set<string>();
	for (const resource of resources) {
		known.add(resource.key);
		const amount = amounts[resource.key];
		if (amount !== undefined && amount !== 0) {
			parts.push(`${resource.label} ${formatDelta(amount)}`);
		}
	}
	for (const [key, amount] of Object.entries(amounts).sort(([first], [second]) =>
		first.localeCompare(second),
	)) {
		if (!known.has(key) && amount !== 0) {
			parts.push(`${labelFromResourceKey(key)} ${formatDelta(amount)}`);
		}
	}
	const colonists = effects.colonists ?? 0;
	if (colonists > 0) {
		parts.push(colonists === 1 ? "1 colonist" : `${formatNumber(colonists)} colonists`);
	}
	return parts.join(" · ");
}

type ToastableEntry = {
	id: string;
	createdAt: string;
	dm?: { visibility: "PUBLIC" | "DM"; undoneAt: string | null };
};

const colonyChangePathPatterns = [
	/^\/api\/campaigns\/\{campaignId\}\/colony(\/|$)/,
	/^\/api\/campaigns\/\{campaignId\}\/people\/(transfer|set-count)$/,
	/^\/api\/campaigns\/\{campaignId\}\/npcs\/\{npcId\}\/(join-colony|change-role|leave-colony)$/,
	/^\/api\/campaigns\/\{campaignId\}\/quests\/\{questId\}\/(complete|fail|reopen)$/,
	/^\/api\/campaigns\/\{campaignId\}\/patrons\/\{patronId\}\/adjust-favor$/,
	/^\/api\/campaigns\/\{campaignId\}\/requests\/(use-action|\{requestId\}\/approve)$/,
];

/** A mutation path that goes through the colony write path (spec §5.1) and so writes timeline entries. */
export function isColonyChangePath(path: string): boolean {
	return colonyChangePathPatterns.some((pattern) => pattern.test(path));
}

/** An entry everyone can see: the player projection has no dm object, the DM's says PUBLIC. */
export function isPublicEntry(entry: ToastableEntry): boolean {
	if (!entry.dm) {
		return true;
	}
	return entry.dm.visibility === "PUBLIC" && entry.dm.undoneAt === null;
}

/** When the newest of these entries was written, in milliseconds, or null for none. */
export function newestWrittenAt(entries: readonly ToastableEntry[]): number | null {
	let newest: number | null = null;
	for (const entry of entries) {
		const writtenAt = Date.parse(entry.createdAt);
		if (newest === null || writtenAt > newest) {
			newest = writtenAt;
		}
	}
	return newest;
}

/**
 * The entries to toast after the recent timeline changed (spec §6.3): public ones this tab hasn't
 * seen, oldest first. `entries` is the server's newest-first list. An unseen entry written before
 * the newest one already seen is history, not news: it slid into the recent list because newer
 * entries were undone or hidden (an undo, or the DM switching to View as player). The server stamps
 * createdAt in commit order (db/queries/timeline.sql), so news is never older than what was seen.
 */
export function entriesToToast<Entry extends ToastableEntry>(
	entries: readonly Entry[],
	seenEntryIds: ReadonlySet<string>,
	newestSeenAt: number | null,
): Entry[] {
	return entries
		.filter(
			(entry) =>
				!seenEntryIds.has(entry.id) &&
				isPublicEntry(entry) &&
				(newestSeenAt === null || Date.parse(entry.createdAt) >= newestSeenAt),
		)
		.reverse();
}

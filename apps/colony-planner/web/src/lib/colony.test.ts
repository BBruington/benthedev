import { describe, expect, test } from "bun:test";
import {
	colonistsLine,
	dayName,
	daysOfPage,
	describeEffects,
	entriesToToast,
	formatDelta,
	needsRetype,
	newestWrittenAt,
	nextPage,
	pageForDay,
	pageLabel,
	parseWholeNumber,
	previousPage,
	shipmentLine,
	voyageProgress,
} from "./colony";

describe("timeline day pages", () => {
	test("Day 0 shares the first page with Days 1–7", () => {
		expect(pageForDay(0)).toEqual({ fromDay: 0, toDay: 7 });
		expect(pageForDay(7)).toEqual({ fromDay: 0, toDay: 7 });
		expect(pageLabel(pageForDay(3))).toBe("Founding – Day 7");
	});

	test("later pages run seven days, like the spec's Days 36–42", () => {
		expect(pageForDay(8)).toEqual({ fromDay: 8, toDay: 14 });
		expect(pageForDay(36)).toEqual({ fromDay: 36, toDay: 42 });
		expect(pageForDay(42)).toEqual({ fromDay: 36, toDay: 42 });
		expect(pageForDay(44)).toEqual({ fromDay: 43, toDay: 49 });
		expect(pageLabel(pageForDay(40))).toBe("Days 36–42");
	});

	test("huge days page and label with grouping", () => {
		expect(pageForDay(1_000_040)).toEqual({ fromDay: 1_000_035, toDay: 1_000_041 });
		expect(pageLabel(pageForDay(1_000_040))).toBe("Days 1,000,035–1,000,041");
	});

	test("previous and next walk page by page", () => {
		expect(previousPage({ fromDay: 0, toDay: 7 })).toBeNull();
		expect(previousPage({ fromDay: 8, toDay: 14 })).toEqual({ fromDay: 0, toDay: 7 });
		expect(previousPage({ fromDay: 36, toDay: 42 })).toEqual({ fromDay: 29, toDay: 35 });
		expect(nextPage({ fromDay: 0, toDay: 7 })).toEqual({ fromDay: 8, toDay: 14 });
		expect(nextPage({ fromDay: 36, toDay: 42 })).toEqual({ fromDay: 43, toDay: 49 });
	});

	test("lists every day of a page", () => {
		expect(daysOfPage({ fromDay: 36, toDay: 42 })).toEqual([36, 37, 38, 39, 40, 41, 42]);
		expect(daysOfPage(pageForDay(0))).toHaveLength(8);
	});

	test("negative or fractional days land on the first page", () => {
		expect(pageForDay(-5)).toEqual({ fromDay: 0, toDay: 7 });
		expect(pageForDay(8.9)).toEqual({ fromDay: 8, toDay: 14 });
	});
});

describe("numbers and labels", () => {
	test("delta chips use a sign and a true minus", () => {
		expect(formatDelta(20)).toBe("+20");
		expect(formatDelta(-39)).toBe("−39");
		expect(formatDelta(-1250)).toBe("−1,250");
		expect(formatDelta(0)).toBe("0");
	});

	test("Day 0 is the founding", () => {
		expect(dayName(0)).toBe("Founding");
		expect(dayName(44)).toBe("Day 44");
		expect(dayName(1_000_040)).toBe("Day 1,000,040");
	});

	test("the shipment line counts down", () => {
		expect(shipmentLine({ daysUntil: 3, intervalDays: 7, nextDay: 47 })).toBe(
			"Next ship Day 47, in 3 days",
		);
		expect(shipmentLine({ daysUntil: 1, intervalDays: 7, nextDay: 45 })).toBe(
			"Next ship Day 45, in 1 day",
		);
		expect(shipmentLine(null)).toBe("No ships scheduled");
	});

	test("the dial's arc fills as the ship nears", () => {
		expect(voyageProgress({ daysUntil: 7, intervalDays: 7, nextDay: 14 })).toBe(0);
		expect(voyageProgress({ daysUntil: 1, intervalDays: 4, nextDay: 9 })).toBe(0.75);
		// A hand-set next day further out than one interval doesn't run the arc backwards.
		expect(voyageProgress({ daysUntil: 20, intervalDays: 7, nextDay: 30 })).toBe(0);
		expect(voyageProgress(null)).toBe(0);
	});

	test("the colonists strip", () => {
		expect(colonistsLine(62, 5)).toBe("62 colonists · 5 unassigned");
		expect(colonistsLine(1, 0)).toBe("1 colonist · 0 unassigned");
	});

	test("the retype guard starts above a year", () => {
		expect(needsRetype(365)).toBe(false);
		expect(needsRetype(366)).toBe(true);
	});
});

describe("whole-number fields", () => {
	test("empty is null, never NaN", () => {
		expect(parseWholeNumber("")).toBeNull();
		expect(parseWholeNumber("   ")).toBeNull();
	});

	test("reads signed whole numbers, with or without grouping", () => {
		expect(parseWholeNumber("12")).toBe(12);
		expect(parseWholeNumber("-4")).toBe(-4);
		expect(parseWholeNumber("+30")).toBe(30);
		expect(parseWholeNumber("1,000,000")).toBe(1_000_000);
	});

	test("anything else is invalid", () => {
		expect(parseWholeNumber("1.5")).toBe("invalid");
		expect(parseWholeNumber("abc")).toBe("invalid");
		expect(parseWholeNumber("1e3")).toBe("invalid");
		expect(parseWholeNumber("99999999999999999999")).toBe("invalid");
	});
});

describe("effects text", () => {
	const resources = [
		{ key: "gold", label: "Gold" },
		{ key: "food", label: "Food" },
	];

	test("lists resources in board order, then colonists", () => {
		expect(describeEffects({ colonists: 4, resources: { food: 30, gold: -10 } }, resources)).toBe(
			"Gold −10 · Food +30 · 4 colonists",
		);
	});

	test("names a resource the board no longer lists by its key, and skips zeros", () => {
		expect(describeEffects({ resources: { food: 0, timber: 5 } }, resources)).toBe("timber +5");
		expect(describeEffects({}, resources)).toBe("");
	});
});

describe("live toasts", () => {
	const publicEntry = { createdAt: "2026-10-10T12:00:00.100Z", id: "a" };
	const dmOnlyEntry = {
		createdAt: "2026-10-10T12:00:00.200Z",
		dm: { undoneAt: null, visibility: "DM" as const },
		id: "b",
	};
	const undoneEntry = {
		createdAt: "2026-10-10T12:00:00.300Z",
		dm: { undoneAt: "2026-10-10T12:00:01Z", visibility: "PUBLIC" as const },
		id: "c",
	};
	const dmPublicEntry = {
		createdAt: "2026-10-10T12:00:00.400Z",
		dm: { undoneAt: null, visibility: "PUBLIC" as const },
		id: "d",
	};

	test("toasts only unseen public entries, oldest first", () => {
		const newestFirst = [dmPublicEntry, undoneEntry, dmOnlyEntry, publicEntry];
		expect(entriesToToast(newestFirst, new Set(), null).map((entry) => entry.id)).toEqual([
			"a",
			"d",
		]);
		expect(entriesToToast(newestFirst, new Set(["a"]), null).map((entry) => entry.id)).toEqual([
			"d",
		]);
	});

	test("an older entry that slides into the recent list is not news", () => {
		const newestFirst = [dmPublicEntry, publicEntry];
		const newestSeenAt = newestWrittenAt([dmOnlyEntry]);
		expect(newestSeenAt).toBe(Date.parse(dmOnlyEntry.createdAt));
		expect(
			entriesToToast(newestFirst, new Set(["b"]), newestSeenAt).map((entry) => entry.id),
		).toEqual(["d"]);
		expect(newestWrittenAt([])).toBeNull();
	});
});

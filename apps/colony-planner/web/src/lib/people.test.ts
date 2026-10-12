import { describe, expect, test } from "bun:test";
import { isColonyChangePath } from "./colony";
import {
	canArchiveRole,
	enumValues,
	hasRosterFilters,
	hiddenTrueName,
	keepsNpc,
	noRosterFilters,
	npcRelationLabels,
	parseList,
	roleCountProblem,
	rosterQuery,
	transferCountProblem,
} from "./people";

describe("aliases and labels", () => {
	test("comma-separated text becomes a trimmed list without blanks or repeats", () => {
		expect(parseList(" The Grey Hand, Old Brannoc ,, the grey hand ,")).toEqual([
			"The Grey Hand",
			"Old Brannoc",
		]);
		expect(parseList("")).toEqual([]);
	});
});

describe("the roster's filters", () => {
	test("no filter asks the server for everyone", () => {
		expect(rosterQuery(noRosterFilters, true)).toEqual({});
		expect(hasRosterFilters(noRosterFilters)).toBe(false);
	});

	test("filters become the server's query, and only the DM's Hidden filter is sent", () => {
		const filters = {
			colonists: "colonists",
			hidden: "hidden",
			relation: "ALLY",
			search: "  bran ",
			status: "ALIVE",
		} as const;
		expect(rosterQuery(filters, true)).toEqual({
			colonistsOnly: true,
			relation: "ALLY",
			revealed: "false",
			search: "bran",
			status: "ALIVE",
		});
		expect(rosterQuery(filters, false)).toEqual({
			colonistsOnly: true,
			relation: "ALLY",
			search: "bran",
			status: "ALIVE",
		});
		expect(rosterQuery({ ...noRosterFilters, hidden: "visible" }, true)).toEqual({
			revealed: "true",
		});
		expect(hasRosterFilters({ ...noRosterFilters, search: "x" })).toBe(true);
	});

	test("'not colonists' is narrowed on the page, since the server has no such filter", () => {
		const others = { ...noRosterFilters, colonists: "others" } as const;
		expect(rosterQuery(others, true)).toEqual({});
		expect(keepsNpc(others, { colonyRole: null })).toBe(true);
		expect(keepsNpc(others, { colonyRole: { key: "FARMER" } })).toBe(false);
		expect(keepsNpc(noRosterFilters, { colonyRole: { key: "FARMER" } })).toBe(true);
	});

	test("enum pickers list every value", () => {
		expect(enumValues(npcRelationLabels)).toEqual([
			"ALLY",
			"ENEMY",
			"NEUTRAL",
			"SUSPICIOUS",
			"UNKNOWN",
		]);
	});
});

describe("workforce forms", () => {
	const unassigned = { label: "Unassigned", unnamedCount: 2 };

	test("a transfer moves at least one colonist and no more than the role has", () => {
		expect(transferCountProblem("", unassigned)).toBeUndefined();
		expect(transferCountProblem("2", unassigned)).toBeUndefined();
		expect(transferCountProblem("0", unassigned)).toBe("Enter a whole number, 1 or more.");
		expect(transferCountProblem("two", unassigned)).toBe("Enter a whole number, 1 or more.");
		expect(transferCountProblem("99", unassigned)).toBe(
			"Unassigned has only 2 unnamed colonists, so 99 can't move.",
		);
		expect(transferCountProblem("2", { label: "Healer", unnamedCount: 1 })).toBe(
			"Healer has only 1 unnamed colonist, so 2 can't move.",
		);
	});

	test("a role's count may be set to 0 but not below", () => {
		expect(roleCountProblem("0")).toBeUndefined();
		expect(roleCountProblem("")).toBeUndefined();
		expect(roleCountProblem("-1")).toBe("Enter a whole number, 0 or more.");
	});

	test("only an empty, ordinary role can be archived", () => {
		expect(canArchiveRole({ headcount: 0, isSystem: false })).toBe(true);
		expect(canArchiveRole({ headcount: 1, isSystem: false })).toBe(false);
		expect(canArchiveRole({ headcount: 0, isSystem: true })).toBe(false);
	});
});

describe("NPC names", () => {
	test("the DM sees the true name beside the name the party uses", () => {
		expect(hiddenTrueName({ displayName: "The Ferryman", dm: { name: "Selwyn" } })).toBe("Selwyn");
		expect(hiddenTrueName({ displayName: "Brannoc", dm: { name: "Brannoc" } })).toBeNull();
		expect(hiddenTrueName({ displayName: "The Ferryman" })).toBeNull();
	});
});

describe("colony change paths", () => {
	test("workforce and colonist writes count as colony changes, other writes don't", () => {
		expect(isColonyChangePath("/api/campaigns/{campaignId}/colony/advance")).toBe(true);
		expect(isColonyChangePath("/api/campaigns/{campaignId}/colony")).toBe(true);
		expect(isColonyChangePath("/api/campaigns/{campaignId}/people/transfer")).toBe(true);
		expect(isColonyChangePath("/api/campaigns/{campaignId}/people/set-count")).toBe(true);
		expect(isColonyChangePath("/api/campaigns/{campaignId}/npcs/{npcId}/join-colony")).toBe(true);
		expect(isColonyChangePath("/api/campaigns/{campaignId}/npcs/{npcId}/change-role")).toBe(true);
		expect(isColonyChangePath("/api/campaigns/{campaignId}/npcs/{npcId}/leave-colony")).toBe(true);
		expect(isColonyChangePath("/api/campaigns/{campaignId}/npcs/{npcId}")).toBe(false);
		expect(isColonyChangePath("/api/campaigns/{campaignId}/colonyx")).toBe(false);
		expect(isColonyChangePath("/api/campaigns/{campaignId}/roles")).toBe(false);
	});
});

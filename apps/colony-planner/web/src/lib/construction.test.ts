import { describe, expect, test } from "bun:test";
import type { Schemas } from "@/api/client";
import {
	amountsText,
	amountsToDraft,
	costShortfall,
	finishLine,
	freeColonists,
	groupBuildings,
	laborText,
	laborToDrafts,
	newLaborDraft,
	otherWorkCount,
	parseAmounts,
	parseCrews,
	parseLabor,
	progressPercent,
	progressText,
	projectedFinishText,
	refundPrefill,
	roleCount,
	roleLabelOf,
} from "./construction";

type Building = Schemas["Building"];

function building(change: Partial<Building>): Building {
	return {
		blueprintId: "00000000-0000-4000-8000-000000000001",
		completionEffects: {},
		cost: { materials: 30 },
		endedDay: null,
		finishedDay: null,
		id: "00000000-0000-4000-8000-000000000002",
		labor: [{ crew: 4, effort: 60, maxCrew: 4, progress: 45, roleKey: "BUILDER" }],
		name: "Granary",
		projectedFinishDay: 52,
		refund: null,
		startedDay: 40,
		status: "CONSTRUCTING",
		...change,
	};
}

const builders = { crewed: 4, headcount: 6, idle: 2, label: "Builder", roleKey: "BUILDER" };
const mages = { crewed: 0, headcount: 1, idle: 0, label: "Mage", roleKey: "MAGE" };
const roles = [builders, mages];

const resources = [
	{ amount: 40, key: "materials", label: "Materials", minAmount: 0 },
	{ amount: 25, key: "gold", label: "Gold", minAmount: 0 },
];

const label = (roleKey: string) => roleLabelOf(roleKey, roles);

describe("finishes and progress", () => {
	test("the preview names each finish as the design words it", () => {
		expect(finishLine({ day: 52, name: "Granary" })).toBe("The Granary is finished on Day 52.");
	});

	test("a line reads in role-days and fills its bar", () => {
		const line = { effort: 60, progress: 45 };
		expect(progressText(line, "Builder")).toBe("45 / 60 Builder-days");
		expect(progressPercent(line)).toBe(75);
		expect(progressPercent({ effort: 10, progress: 10 })).toBe(100);
	});

	test("a project with crews names its finish day", () => {
		expect(projectedFinishText(building({}), label)).toBe("Finishes on Day 52");
	});

	test("a project without one names the roles a line with work left is missing", () => {
		const stalled = building({
			labor: [
				{ crew: 0, effort: 30, maxCrew: null, progress: 30, roleKey: "BUILDER" },
				{ crew: 0, effort: 10, maxCrew: null, progress: 0, roleKey: "MAGE" },
			],
			projectedFinishDay: null,
		});
		expect(projectedFinishText(stalled, label)).toBe("Needs Mages on the crew");
	});

	test("labor names its cap only when it has one", () => {
		expect(laborText({ effort: 40, maxCrew: 4 }, "Builder")).toBe(
			"40 Builder-days, up to 4 at once",
		);
		expect(laborText({ effort: 60, maxCrew: null }, "Builder")).toBe("60 Builder-days");
	});

	test("a role the page doesn't list is named from its key", () => {
		expect(roleLabelOf("STONEMASON", roles)).toBe("Stonemason");
		expect(roleCount(1, "Builder")).toBe("1 Builder");
		expect(roleCount(3, "Builder")).toBe("3 Builders");
	});
});

describe("colonists for construction", () => {
	test("colonists on a project the viewer can't see count as other work", () => {
		expect(
			otherWorkCount({ crewed: 2, headcount: 6, idle: 1, label: "Builder", roleKey: "B" }),
		).toBe(3);
		expect(otherWorkCount(builders)).toBe(0);
	});
});

describe("grouping", () => {
	test("buildings fall into under construction, standing and past", () => {
		const groups = groupBuildings([
			building({ id: "a" }),
			building({ finishedDay: 50, id: "b", status: "FINISHED" }),
			building({ endedDay: 45, id: "c", status: "CANCELLED" }),
			building({ endedDay: 60, finishedDay: 50, id: "d", status: "DEMOLISHED" }),
		]);
		expect(groups.constructing.map((item) => item.id)).toEqual(["a"]);
		expect(groups.standing.map((item) => item.id)).toEqual(["b"]);
		expect(groups.past.map((item) => item.id)).toEqual(["c", "d"]);
	});
});

describe("costs and refunds", () => {
	test("amounts read in board order, unknown keys last", () => {
		expect(amountsText({ dragon_scales: 2, gold: 20, materials: 30 }, resources)).toBe(
			"Materials 30 · Gold 20 · Dragon scales 2",
		);
		expect(amountsText({}, resources)).toBe("");
	});

	test("a cost the colony can pay has no shortfall", () => {
		expect(costShortfall({ gold: 20, materials: 30 }, resources)).toBeNull();
		expect(costShortfall({ materials: 40 }, resources)).toBeNull();
	});

	test("every shortfall is named, as the server names it", () => {
		expect(costShortfall({ gold: 60, materials: 80 }, resources)).toBe(
			"Not enough gold: the colony has 25, needs 60. Not enough materials: the colony has 40, needs 80.",
		);
		expect(
			costShortfall({ food: 5 }, [{ amount: 10, key: "food", label: "Food", minAmount: 8 }]),
		).toBe("Not enough food: the colony has 10, needs 13.");
		expect(costShortfall({ stone: 5 }, resources)).toBe(
			"Stone is archived, so the DM has to edit the blueprint.",
		);
	});

	test("amounts parse to 0 or more, leaving blanks and zeros out", () => {
		expect(parseAmounts({ gold: "0", materials: "30", stone: "" })).toEqual({
			amounts: { materials: 30 },
			problems: {},
		});
		expect(parseAmounts({ gold: "-1", materials: "lots" }).problems).toEqual({
			gold: "Enter a whole number, 0 or more.",
			materials: "Enter a whole number, 0 or more.",
		});
		expect(amountsToDraft({ gold: 0, materials: 30 })).toEqual({ materials: "30" });
	});

	test("a refund starts at the full cost, leaving out what was archived since", () => {
		expect(refundPrefill({ gold: 20, materials: 30, stone: 5 }, resources)).toEqual({
			draft: { gold: "20", materials: "30" },
			leftOut: ["Stone"],
		});
	});
});

describe("crews", () => {
	const builderLine = { crew: 1, maxCrew: 4, roleKey: "BUILDER" };
	const lines = [builderLine, { crew: 0, maxCrew: null, roleKey: "MAGE" }];

	test("a line's free colonists are the idle ones plus its own crew", () => {
		expect(freeColonists(builderLine, roles)).toBe(3);
	});

	test("crews within the cap and the free colonists are sent, blanks as 0", () => {
		expect(parseCrews({ BUILDER: "3", MAGE: "" }, lines, roles)).toEqual({
			crews: { BUILDER: 3, MAGE: 0 },
			problems: {},
		});
	});

	test("too many for the cap, too few free, or not a number is refused per line", () => {
		expect(parseCrews({ BUILDER: "5", MAGE: "1" }, lines, roles).problems).toEqual({
			BUILDER: "At most 4 Builders can work on this.",
			MAGE: "No Mages are free.",
		});
		expect(parseCrews({ BUILDER: "-2" }, lines, roles).problems.BUILDER).toBe(
			"Enter a whole number, 0 or more.",
		);
		expect(
			parseCrews({ BUILDER: "4" }, [{ crew: 0, maxCrew: null, roleKey: "BUILDER" }], roles).problems
				.BUILDER,
		).toBe("Only 2 Builders are free.");
		expect(
			parseCrews(
				{ BUILDER: "1" },
				[{ crew: 0, maxCrew: null, roleKey: "BUILDER" }],
				[{ ...builders, idle: 1 }],
			).problems,
		).toEqual({});
	});
});

describe("labor", () => {
	test("lines parse to a labor map, a blank cap as none", () => {
		const builder = { ...newLaborDraft("BUILDER"), effortText: "40", maxCrewText: "4" };
		const mage = { ...newLaborDraft("MAGE"), effortText: "10" };
		expect(parseLabor([builder, mage])).toEqual({
			labor: { BUILDER: { effort: 40, maxCrew: 4 }, MAGE: { effort: 10, maxCrew: null } },
			problems: {},
		});
	});

	test("a line needs its work, a cap of 1 or more, and its own role", () => {
		const blank = newLaborDraft("BUILDER");
		const badCap = { ...newLaborDraft("MAGE"), effortText: "10", maxCrewText: "0" };
		const twice = { ...newLaborDraft("MAGE"), effortText: "5" };
		const good = { ...newLaborDraft("MAGE"), effortText: "10" };
		expect(parseLabor([blank, badCap]).problems).toEqual({
			[badCap.rowKey]: "A crew cap is 1 or more. Leave it blank for none.",
			[blank.rowKey]: "Enter the work in role-days, 1 or more.",
		});
		expect(parseLabor([good, twice]).problems).toEqual({
			[twice.rowKey]: "This role already has a line. Use one line per role.",
		});
		expect(parseLabor([]).problems.labor).toContain("at least one role");
	});

	test("saved labor reads back as text", () => {
		expect(laborToDrafts([{ effort: 40, maxCrew: null, roleKey: "BUILDER" }])).toEqual([
			{ effortText: "40", maxCrewText: "", roleKey: "BUILDER", rowKey: "saved-BUILDER" },
		]);
	});
});

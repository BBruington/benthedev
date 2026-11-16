import { describe, expect, test } from "bun:test";
import type { Schemas } from "@/api/client";
import {
	amountsText,
	amountsToDraft,
	costShortfall,
	dailyOutputText,
	finishLine,
	freeColonists,
	groupBuildings,
	laborText,
	laborToDrafts,
	newLaborDraft,
	newOutputDraft,
	otherWorkCount,
	outputToDrafts,
	parseAmounts,
	parseCrews,
	parseLabor,
	parseOutput,
	pastHistory,
	progressPercent,
	progressText,
	projectedFinishText,
	refundPrefill,
	roleCount,
	roleLabelOf,
	standingSince,
	storageBonusText,
} from "./construction";

type Building = Schemas["Building"];

function building(change: Partial<Building>): Building {
	return {
		active: true,
		added: false,
		blueprintId: "00000000-0000-4000-8000-000000000001",
		completionEffects: {},
		cost: { materials: 30 },
		endedDay: null,
		finishedDay: null,
		id: "00000000-0000-4000-8000-000000000002",
		labor: [{ crew: 4, effort: 60, maxCrew: 4, progress: 45, roleKey: "BUILDER" }],
		name: "Granary",
		output: {},
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

describe("standing and past buildings", () => {
	test("an added building reads as added, a built one by its finish day", () => {
		const added = building({ added: true, finishedDay: 22, startedDay: 22, status: "FINISHED" });
		const built = building({ finishedDay: 47, startedDay: 40, status: "FINISHED" });
		expect(standingSince(added)).toBe("Added on Day 22");
		expect(standingSince(built)).toBe("Finished on Day 47");
	});

	test("a past building's history names how it came to stand", () => {
		expect(
			pastHistory(
				building({
					added: true,
					endedDay: 30,
					finishedDay: 22,
					startedDay: 22,
					status: "DEMOLISHED",
				}),
			),
		).toBe("Added on Day 22");
		expect(
			pastHistory(
				building({ endedDay: 50, finishedDay: 47, startedDay: 40, status: "DEMOLISHED" }),
			),
		).toBe("Started on Day 40, finished on Day 47");
		expect(pastHistory(building({ endedDay: 41, startedDay: 40, status: "CANCELLED" }))).toBe(
			"Started on Day 40",
		);
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

describe("output", () => {
	const food = { amount: 50, key: "food", label: "Food", minAmount: 0 };
	const stocked = [food, ...resources];

	test("the daily change reads in board order, with signs, leaving out storage-only lines", () => {
		const output = {
			food: { dailyDelta: 6, storageBonus: 0 },
			gold: { dailyDelta: 0, storageBonus: 50 },
			materials: { dailyDelta: -1, storageBonus: 0 },
			stone: { dailyDelta: 2, storageBonus: 0 },
		};
		expect(dailyOutputText(output, stocked)).toBe("Food +6, Materials −1, Stone +2");
		expect(dailyOutputText({}, stocked)).toBe("");
	});

	test("storage bonuses read as plain words, joined with and", () => {
		expect(storageBonusText({ food: { dailyDelta: 0, storageBonus: 200 } }, stocked)).toBe(
			"Stores 200 more Food",
		);
		expect(
			storageBonusText(
				{
					food: { dailyDelta: 6, storageBonus: 200 },
					gold: { dailyDelta: 0, storageBonus: 10 },
					materials: { dailyDelta: 0, storageBonus: 1500 },
				},
				stocked,
			),
		).toBe("Stores 200 more Food, 1,500 more Materials and 10 more Gold");
		expect(storageBonusText({ food: { dailyDelta: 6, storageBonus: 0 } }, stocked)).toBe("");
	});

	test("lines parse to an output map, a blank as 0", () => {
		const mill = { ...newOutputDraft("food"), dailyText: "+6" };
		const upkeep = { ...newOutputDraft("materials"), dailyText: "\u22121" };
		const granary = { ...newOutputDraft("gold"), bonusText: "200" };
		expect(parseOutput([mill, upkeep, granary])).toEqual({
			output: {
				food: { dailyDelta: 6, storageBonus: 0 },
				gold: { dailyDelta: 0, storageBonus: 200 },
				materials: { dailyDelta: -1, storageBonus: 0 },
			},
			problems: {},
		});
		expect(parseOutput([])).toEqual({ output: {}, problems: {} });
	});

	test("a line changes something, takes whole numbers, a bonus of 0 or more, and its own resource", () => {
		const blank = newOutputDraft("food");
		const zero = { ...newOutputDraft("gold"), bonusText: "0", dailyText: "0" };
		const badDaily = { ...newOutputDraft("materials"), dailyText: "lots" };
		const badBonus = { ...newOutputDraft("stone"), bonusText: "-5" };
		expect(parseOutput([blank, zero, badDaily, badBonus]).problems).toEqual({
			[badBonus.rowKey]: "A storage bonus is a whole number, 0 or more.",
			[badDaily.rowKey]: "Enter the daily change as a whole number, such as +6 or -1.",
			[blank.rowKey]: "Give it a daily change or a storage bonus, or remove the line.",
			[zero.rowKey]: "Give it a daily change or a storage bonus, or remove the line.",
		});
		const first = { ...newOutputDraft("food"), dailyText: "3" };
		const twice = { ...newOutputDraft("food"), dailyText: "4" };
		expect(parseOutput([first, twice]).problems).toEqual({
			[twice.rowKey]: "This resource already has a line. Use one line per resource.",
		});
	});

	test("saved output reads back as text, zeros as blanks", () => {
		expect(
			outputToDrafts({
				food: { dailyDelta: -2, storageBonus: 0 },
				gold: { dailyDelta: 0, storageBonus: 50 },
			}),
		).toEqual([
			{ bonusText: "", dailyText: "-2", resourceKey: "food", rowKey: "saved-food" },
			{ bonusText: "50", dailyText: "", resourceKey: "gold", rowKey: "saved-gold" },
		]);
	});
});

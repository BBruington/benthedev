import { describe, expect, test } from "bun:test";
import {
	draftsToLoot,
	draftToEffects,
	effectsToDraft,
	groupQuestsByStatus,
	inactiveEffectKeys,
	isEmptyEffects,
	lootItemKeys,
	newLootDraft,
	parseFavor,
	shortfallAdvice,
	spendShortfall,
	withoutEffectResource,
} from "./ventures";

const food = { amount: 4, key: "food", label: "Food", minAmount: 0 };
const gold = { amount: 50, key: "gold", label: "Gold", minAmount: 0 };

describe("spendShortfall", () => {
	test("names the favor shortfall the way the server does", () => {
		expect(spendShortfall({ favor: 2, name: "Lady Vex" }, 3, {}, [food])).toBe(
			"Not enough favor: Lady Vex has 2, needs 3",
		);
	});

	test("names a resource shortfall against its minimum", () => {
		expect(
			spendShortfall({ favor: 5, name: "Lady Vex" }, 3, { resources: { food: -10 } }, [food]),
		).toBe("Not enough food: the colony has 4, needs 10");
		const flooredFood = { ...food, amount: 12, minAmount: 5 };
		expect(
			spendShortfall({ favor: 1, name: "Lady Vex" }, 3, { resources: { food: -10, gold: -60 } }, [
				gold,
				flooredFood,
			]),
		).toBe(
			"Not enough favor: Lady Vex has 1, needs 3. Not enough food: the colony has 12, needs 15. Not enough gold: the colony has 50, needs 60",
		);
	});

	test("is null when the spend is affordable, gains included", () => {
		expect(
			spendShortfall({ favor: 3, name: "Lady Vex" }, 3, { resources: { food: -4, gold: 7 } }, [
				food,
				gold,
			]),
		).toBeNull();
	});

	test("is null for a free action when favor is below 0, as the server allows", () => {
		expect(
			spendShortfall({ favor: -2, name: "Lady Vex" }, 0, { resources: { food: 1 } }, [food]),
		).toBeNull();
	});
});

describe("effects drafts", () => {
	test("round-trip, dropping zero and blank changes", () => {
		const draft = effectsToDraft({ colonists: 3, resources: { food: 20, gold: 0 } });
		expect(draft).toEqual({ colonists: "3", resources: { food: "20" } });
		expect(
			draftToEffects({ colonists: "", resources: { food: "20", gold: " ", wood: "0" } }),
		).toEqual({ effects: { colonists: 0, resources: { food: 20 } }, problems: {} });
	});

	test("refuse text that isn't a whole number and colonists leaving", () => {
		const { problems } = draftToEffects({ colonists: "-2", resources: { food: "ten" } });
		expect(Object.keys(problems).sort()).toEqual(["colonists", "food"]);
	});

	test("isEmptyEffects ignores zero amounts", () => {
		expect(isEmptyEffects({ colonists: 0, resources: { food: 0 } })).toBe(true);
		expect(isEmptyEffects({ resources: { food: -1 } })).toBe(false);
		expect(isEmptyEffects({ colonists: 1 })).toBe(false);
	});
});

describe("parseFavor", () => {
	test("blank is 0, and only adjustments may go negative", () => {
		expect(parseFavor("", false)).toEqual({ value: 0 });
		expect(parseFavor("-2", true)).toEqual({ value: -2 });
		expect(parseFavor("-2", false).problem).toBe("Enter 0 or more.");
		expect(parseFavor("1.5", true).problem).toBe("Enter a whole number.");
	});
});

describe("draftsToLoot", () => {
	test("skips blank rows, defaults the quantity and flags bad rows by position", () => {
		const { loot, problems } = draftsToLoot([
			{ description: "", name: "Silver ring", quantityText: "", rowKey: "a" },
			{ description: "", name: "", quantityText: "", rowKey: "b" },
			{ description: "", name: "Rope", quantityText: "0", rowKey: "c" },
			{ description: "ornate", name: "", quantityText: "2", rowKey: "d" },
		]);
		expect(loot).toEqual([{ description: "", name: "Silver ring", quantity: 1 }]);
		expect(problems).toEqual({
			2: { field: "quantity", message: "Enter 1 or more." },
			3: { field: "name", message: "Name the item." },
		});
	});

	test("new rows get distinct keys without crypto.randomUUID", () => {
		const first = newLootDraft();
		const second = newLootDraft();
		expect(first.rowKey).not.toBe(second.rowKey);
		expect(first).toMatchObject({ description: "", name: "", quantityText: "" });
	});
});

describe("lootItemKeys", () => {
	test("keeps repeated items distinct and keys each by its content", () => {
		const ring = { description: "", name: "Silver ring", quantity: 1 };
		const keys = lootItemKeys([ring, { ...ring, quantity: 2 }, { ...ring, name: "Rope" }]);
		expect(new Set(keys).size).toBe(3);
		expect(lootItemKeys([ring])[0]).toBe(keys[0]);
	});
});

describe("effects naming archived resources", () => {
	const board = [{ key: "food" }, { key: "gold" }];

	test("lists the saved keys the board no longer has", () => {
		const draft = effectsToDraft({ resources: { ale: -1, food: 5, timber: 3 } });
		expect(inactiveEffectKeys(draft, board)).toEqual(["ale", "timber"]);
		expect(inactiveEffectKeys(effectsToDraft({ resources: { food: 5 } }), board)).toEqual([]);
	});

	test("clearing one leaves the rest to save", () => {
		const draft = effectsToDraft({ colonists: 2, resources: { food: 5, timber: 3 } });
		const cleared = withoutEffectResource(draft, "timber");
		expect(cleared).toEqual({ colonists: "2", resources: { food: "5" } });
		expect(inactiveEffectKeys(cleared, board)).toEqual([]);
		expect(draftToEffects(cleared).effects).toEqual({ colonists: 2, resources: { food: 5 } });
		expect(draft.resources.timber).toBe("3");
	});
});

describe("shortfallAdvice", () => {
	test("offers only what can cover the shortfall", () => {
		expect(shortfallAdvice("Not enough favor: Lady Vex has 2, needs 3.")).toBe(
			"Adjust favor, then try again.",
		);
		expect(shortfallAdvice("Not enough food: the colony has 4, needs 10.")).toBe(
			"Tweak the grant, then try again.",
		);
		expect(
			shortfallAdvice(
				"Not enough favor: Lady Vex has 1, needs 3. Not enough food: the colony has 12, needs 15.",
			),
		).toBe("Adjust favor and tweak the grant, then try again.");
	});

	test("reads a resource labelled Favor as a resource", () => {
		expect(shortfallAdvice("Not enough favor: the colony has 4, needs 10.")).toBe(
			"Tweak the grant, then try again.",
		);
	});

	test("has nothing to offer when the request was already decided", () => {
		expect(shortfallAdvice("That request was already decided.")).toBeNull();
	});
});

describe("groupQuestsByStatus", () => {
	test("shelves each quest under its status", () => {
		const groups = groupQuestsByStatus([
			{ id: "a", status: "ACTIVE" as const },
			{ id: "b", status: "FAILED" as const },
			{ id: "c", status: "ACTIVE" as const },
		]);
		expect(groups.ACTIVE.map((quest) => quest.id)).toEqual(["a", "c"]);
		expect(groups.COMPLETED).toEqual([]);
		expect(groups.FAILED.map((quest) => quest.id)).toEqual(["b"]);
	});
});

import { describe, expect, test } from "bun:test";
import {
	goodsShipLine,
	goodsShipProgress,
	itemKind,
	offerText,
	orderWaitText,
	parseUnits,
	shipLine,
	unitsText,
} from "./market";

const food = { amount: 2, name: "Food", priceGold: 1, resourceKey: "food", roleKey: null };
const farmer = {
	amount: null,
	name: "Farmer",
	priceGold: 30,
	resourceKey: null,
	roleKey: "FARMER",
};
const boots = {
	amount: null,
	name: "Boots of Striding",
	priceGold: 120,
	resourceKey: null,
	roleKey: null,
};

describe("item kinds", () => {
	test("offers sell a resource or a role; anything else is special", () => {
		expect(itemKind(food)).toBe("SUPPLY");
		expect(itemKind(farmer)).toBe("COLONIST");
		expect(itemKind(boots)).toBe("SPECIAL");
	});
});

describe("offer text", () => {
	test("a lot names its size, anything sold one at a time its price each", () => {
		expect(offerText(food)).toBe("2 food for 1 gp");
		expect(offerText({ ...food, amount: 1, name: "Materials", priceGold: 5 })).toBe("5 gp each");
		expect(offerText(farmer)).toBe("30 gp each");
		expect(offerText(boots)).toBe("120 gp each");
	});
});

describe("units text", () => {
	test("counts units, not purchases", () => {
		expect(unitsText(500, food)).toBe("500 food");
		expect(unitsText(12, farmer)).toBe("12 Farmers");
		expect(unitsText(1, farmer)).toBe("1 Farmer");
		expect(unitsText(1, boots)).toBe("Boots of Striding");
		expect(unitsText(2, boots)).toBe("Boots of Striding × 2");
	});
});

describe("typed units", () => {
	const roomy = { limitLots: 10000 };

	test("a whole number of lots becomes lots", () => {
		expect(parseUnits("500", food, roomy)).toEqual({ kind: "VALID", lots: 250, units: 500 });
		expect(parseUnits("2", food, roomy)).toEqual({ kind: "VALID", lots: 1, units: 2 });
		expect(parseUnits("12", farmer, roomy)).toEqual({ kind: "VALID", lots: 12, units: 12 });
	});

	test("a number between lots is refused, never rounded", () => {
		expect(parseUnits("501", food, roomy)).toEqual({
			kind: "INVALID",
			message: "Food comes in lots of 2.",
		});
		expect(parseUnits("1", food, roomy)).toEqual({
			kind: "INVALID",
			message: "Food comes in lots of 2.",
		});
	});

	test("blank is empty, and zero or text is refused", () => {
		expect(parseUnits(" ", food, roomy)).toEqual({ kind: "EMPTY" });
		expect(parseUnits("0", food, roomy).kind).toBe("INVALID");
		expect(parseUnits("lots", food, roomy).kind).toBe("INVALID");
	});

	test("the cart's room is counted in units", () => {
		expect(parseUnits("20000", food, roomy)).toEqual({ kind: "VALID", lots: 10000, units: 20000 });
		expect(parseUnits("20002", food, roomy)).toEqual({
			kind: "INVALID",
			message: "The cart holds at most 20,000 food.",
		});
		expect(parseUnits("4", food, { inCartLots: 9999, limitLots: 10000 })).toEqual({
			kind: "INVALID",
			message: "The cart has room for 2 food more.",
		});
		expect(parseUnits("2", food, { inCartLots: 10000, limitLots: 10000 })).toEqual({
			kind: "INVALID",
			message: "The cart already holds the most it can: 20,000 food.",
		});
	});
});

describe("ship line", () => {
	test("counts down at sea and names the day once the colony day reaches it", () => {
		expect(shipLine({ arrivesDay: 36 }, 22)).toBe("At sea · docks Day 36 (in 14 days)");
		expect(shipLine({ arrivesDay: 36 }, 35)).toBe("At sea · docks Day 36 (in 1 day)");
		expect(shipLine({ arrivesDay: 36 }, 36)).toBe("Docked since Day 36");
		expect(shipLine({ arrivesDay: 36 }, 40)).toBe("Docked since Day 36");
	});

	test("the dial's arc fills from the order day to the docking day, and stays full at the docks", () => {
		const order = { arrivesDay: 36, placedDay: 22 };
		expect(goodsShipProgress(order, 22)).toBe(0);
		expect(goodsShipProgress(order, 29)).toBe(0.5);
		expect(goodsShipProgress(order, 40)).toBe(1);
		// A day corrected back before the order doesn't run the arc backwards.
		expect(goodsShipProgress(order, 20)).toBe(0);
		// A zero-day voyage docks the day it's placed.
		expect(goodsShipProgress({ arrivesDay: 22, placedDay: 22 }, 22)).toBe(1);
	});

	test("the board names the soonest ship's day, then waits for payment once it docks", () => {
		expect(goodsShipLine({ arrivesDay: 36 }, 22)).toBe("Next shipment arrives at Day 36");
		expect(goodsShipLine({ arrivesDay: 36 }, 36)).toBe(
			"Shipment has arrived and is awaiting payment",
		);
	});
});

describe("order wait note", () => {
	test("appears only once every allowed ship is out, and names the next one to dock", () => {
		const orders = [{ arrivesDay: 36 }, { arrivesDay: 40 }];
		expect(orderWaitText([], 1, 22, false)).toBeNull();
		expect(orderWaitText(orders, 3, 22, false)).toBeNull();
		expect(orderWaitText(orders, 2, 22, false)).toBe(
			"All 2 ships are out. The next docks Day 36 (in 14 days). Pay for it there, or ask the DM to cancel one, to order again.",
		);
		expect(orderWaitText(orders.slice(0, 1), 1, 36, true)).toBe(
			"The ship is out. Pay for a docked ship, or cancel one above, to order again.",
		);
		// A limit the DM lowered below the ships already out still waits.
		expect(orderWaitText(orders, 1, 22, true)).not.toBeNull();
	});
});

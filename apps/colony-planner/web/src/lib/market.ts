import { formatNumber, inDays, parseWholeNumber } from "./colony";
import { countOf } from "./words";

/** What a catalog row or cart line needs to say what it sells. */
type Sold = {
	name: string;
	resourceKey: string | null;
	amount: number | null;
	roleKey: string | null;
};

export const unassignedRoleKey = "UNASSIGNED";

// The server sells neither: the market charges gold, and morale isn't goods.
export const unsellableResourceKeys: readonly string[] = ["gold", "morale"];

export type ItemKind = "SUPPLY" | "COLONIST" | "SPECIAL";

/** A supply offer sells a resource, a colonist offer a role's colonist, and anything else is special. */
export function itemKind(item: { resourceKey: string | null; roleKey: string | null }): ItemKind {
	if (item.resourceKey !== null) {
		return "SUPPLY";
	}
	if (item.roleKey !== null) {
		return "COLONIST";
	}
	return "SPECIAL";
}

/** How many units one purchase brings: a supply offer's lot, otherwise one. */
export function lotSize(item: { amount: number | null }): number {
	return item.amount ?? 1;
}

/** "500 food", "12 Farmers", "1 Farmer", "Boots of Striding", "Lantern × 2". */
export function unitsText(units: number, item: Sold): string {
	switch (itemKind(item)) {
		case "SUPPLY":
			return `${formatNumber(units)} ${item.name.toLowerCase()}`;
		case "COLONIST":
			return countOf(units, item.name);
		default:
			return units === 1 ? item.name : `${item.name} × ${formatNumber(units)}`;
	}
}

/** "2 food for 1 gp" for a supply sold in lots, "30 gp each" for anything sold one at a time. */
export function offerText(item: Sold & { priceGold: number }): string {
	const price = `${formatNumber(item.priceGold)} gp`;
	if (lotSize(item) > 1) {
		return `${unitsText(lotSize(item), item)} for ${price}`;
	}
	return `${price} each`;
}

export type UnitsEntry =
	| { kind: "EMPTY" }
	| { kind: "INVALID"; message: string }
	| { kind: "VALID"; lots: number; units: number };

/**
 * Players type units ("500" food), the cart counts purchases (250 lots of 2). A number that isn't a
 * whole number of lots is refused, never rounded, so the party never buys a different amount than it
 * typed. The typed lots join inCartLots, and together they may not pass limitLots.
 */
export function parseUnits(
	text: string,
	item: Sold,
	{ limitLots, inCartLots = 0 }: { limitLots: number; inCartLots?: number },
): UnitsEntry {
	const units = parseWholeNumber(text);
	if (units === null) {
		return { kind: "EMPTY" };
	}
	const size = lotSize(item);
	if (units === "invalid" || units < 1) {
		return { kind: "INVALID", message: `Enter a whole number, ${formatNumber(size)} or more.` };
	}
	if (units % size !== 0) {
		return {
			kind: "INVALID",
			message: `${item.name} comes in lots of ${formatNumber(size)}.`,
		};
	}
	const lots = units / size;
	const roomLots = limitLots - inCartLots;
	if (lots > roomLots) {
		let message = `The cart holds at most ${unitsText(limitLots * size, item)}.`;
		if (roomLots < 1) {
			message = `The cart already holds the most it can: ${unitsText(limitLots * size, item)}.`;
		} else if (inCartLots > 0) {
			message = `The cart has room for ${unitsText(roomLots * size, item)} more.`;
		}
		return { kind: "INVALID", message };
	}
	return { kind: "VALID", lots, units };
}

// The server's line limit when an item sets no "most per order".
const largestCartQuantity = 10000;

/** The most purchases of one item a cart line may hold. */
export function cartLimit(item: { maxPerOrder: number | null } | undefined): number {
	return item?.maxPerOrder ?? largestCartQuantity;
}

/**
 * Docked from the colony day rather than the order's own flag, since the board's day can move ahead of
 * a market page fetched before an advance.
 */
export function isDocked(order: { arrivesDay: number }, today: number): boolean {
	return today >= order.arrivesDay;
}

/** "At sea · docks Day 36 (in 14 days)", or "Docked since Day 36" once the colony day reaches it. */
export function shipLine(order: { arrivesDay: number }, today: number): string {
	if (isDocked(order, today)) {
		return `Docked since Day ${formatNumber(order.arrivesDay)}`;
	}
	return `At sea · docks Day ${formatNumber(order.arrivesDay)} (${inDays(order.arrivesDay - today)})`;
}

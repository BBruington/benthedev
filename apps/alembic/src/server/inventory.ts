import { ORPCError } from "@orpc/server";
import { INGREDIENTS } from "#/brewing/catalog";
import { describeShortages, shortagesOf } from "#/brewing/inventory";
import type { Inventory, Mix } from "#/brewing/types";

export function assertEnoughIngredients(mix: Mix, inventory: Inventory) {
	const shortages = shortagesOf(mix, inventory);
	if (shortages.length === 0) return;
	throw new ORPCError("BAD_REQUEST", {
		message: describeShortages(shortages, INGREDIENTS),
	});
}

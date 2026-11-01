import type { Ingredient, Inventory, Mix } from "./types";

export interface Shortage {
	ingredientId: string;
	needed: number;
	available: number;
}

export function shortagesOf(mix: Mix, inventory: Inventory): Shortage[] {
	return Object.entries(mix)
		.map(([ingredientId, needed]) => ({
			ingredientId,
			needed,
			available: inventory[ingredientId] ?? 0,
		}))
		.filter((entry) => entry.needed > entry.available);
}

export function describeShortages(
	shortages: readonly Shortage[],
	ingredients: readonly Ingredient[],
): string {
	const details = shortages.map(({ ingredientId, needed, available }) => {
		const name =
			ingredients.find((ingredient) => ingredient.id === ingredientId)?.name ??
			ingredientId;
		return `${name} (need ${needed}, have ${available})`;
	});
	return `Not enough ingredients: ${details.join(", ")}`;
}

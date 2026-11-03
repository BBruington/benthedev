import {
	type IngredientView,
	type KnownValues,
	type Mix,
	TRADITIONS,
	type Tradition,
	type Values,
} from "./types";

export function maskValues(
	values: Values,
	known: ReadonlySet<Tradition> | "all",
): KnownValues {
	const [arcane, primal, occult, divine] = TRADITIONS.map((tradition, index) =>
		known === "all" || known.has(tradition) ? values[index] : null,
	);
	return [arcane, primal, occult, divine];
}

/** Per axis: the learned contributions summed (null when none is learned) and whether any unit's value there is unlearned. */
export function knownTotals(
	mix: Mix,
	ingredients: readonly IngredientView[],
): { known: KnownValues; hidden: readonly boolean[] } {
	const sums = [0, 0, 0, 0];
	const learned = [false, false, false, false];
	const hidden = [false, false, false, false];
	for (const [id, units] of Object.entries(mix)) {
		const values = ingredients.find((ingredient) => ingredient.id === id)
			?.values ?? [null, null, null, null];
		for (let axis = 0; axis < 4; axis++) {
			const value = values[axis];
			if (value === null) hidden[axis] = true;
			else {
				sums[axis] += value * units;
				learned[axis] = true;
			}
		}
	}
	const [arcane, primal, occult, divine] = sums.map((sum, axis) =>
		hidden[axis] && !learned[axis] ? null : sum,
	);
	return { known: [arcane, primal, occult, divine], hidden };
}

export function maskTotals(
	totals: Values,
	mix: Mix,
	ingredients: readonly IngredientView[],
): KnownValues {
	const { hidden } = knownTotals(mix, ingredients);
	const [arcane, primal, occult, divine] = totals.map((total, axis) =>
		hidden[axis] ? null : total,
	);
	return [arcane, primal, occult, divine];
}

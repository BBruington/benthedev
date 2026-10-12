const ellipsis = "…";

/**
 * The longest start of `text` that fits `maximumWidth` with an ellipsis, or the whole text when it
 * fits (spec §6.4: labels are truncated on the map; the sheet shows the full text).
 */
export function truncateLabel(
	text: string,
	maximumWidth: number,
	measure: (text: string) => number,
): string {
	if (measure(text) <= maximumWidth) {
		return text;
	}
	const characters = Array.from(text);
	let fitting = 0;
	let tooLong = characters.length;
	while (tooLong - fitting > 1) {
		const middle = Math.floor((fitting + tooLong) / 2);
		if (measure(`${characters.slice(0, middle).join("").trimEnd()}${ellipsis}`) <= maximumWidth) {
			fitting = middle;
		} else {
			tooLong = middle;
		}
	}
	if (fitting === 0) {
		return measure(ellipsis) <= maximumWidth ? ellipsis : "";
	}
	return `${characters.slice(0, fitting).join("").trimEnd()}${ellipsis}`;
}

type LabelledHex = { q: number; r: number; label: string };

/** Labelled hexes whose label contains the search, those starting with it first, then by label. */
export function findHexesByLabel<Hex extends LabelledHex>(
	hexes: readonly Hex[],
	search: string,
	limit: number,
): Hex[] {
	const needle = search.trim().toLocaleLowerCase();
	if (needle === "") {
		return [];
	}
	const matches = hexes.filter((hex) => hex.label.toLocaleLowerCase().includes(needle));
	matches.sort((first, second) => {
		const firstStarts = first.label.toLocaleLowerCase().startsWith(needle);
		const secondStarts = second.label.toLocaleLowerCase().startsWith(needle);
		if (firstStarts !== secondStarts) {
			return firstStarts ? -1 : 1;
		}
		return first.label.localeCompare(second.label);
	});
	return matches.slice(0, limit);
}

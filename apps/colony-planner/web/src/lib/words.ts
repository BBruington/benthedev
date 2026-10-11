/** "1 player", "3 players". English plurals only; every noun here takes an s. */
export function countOf(count: number, singular: string): string {
	const formatted = new Intl.NumberFormat().format(count);
	if (count === 1) {
		return `${formatted} ${singular}`;
	}
	return `${formatted} ${singular}s`;
}

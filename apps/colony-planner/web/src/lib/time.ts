const relativeTimeFormat = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });

const relativeSteps: [Intl.RelativeTimeFormatUnit, number][] = [
	["second", 60],
	["minute", 60],
	["hour", 24],
	["day", 7],
	["week", 4.345],
	["month", 12],
	["year", Number.POSITIVE_INFINITY],
];

/** "3 hours ago", "yesterday", "last week": for when something happened, not exact times. */
export function formatRelativeTime(isoTimestamp: string, now: Date = new Date()): string {
	const timestamp = Date.parse(isoTimestamp);
	if (Number.isNaN(timestamp)) {
		return "";
	}
	let amount = (timestamp - now.getTime()) / 1000;
	for (const [unit, unitsInNext] of relativeSteps) {
		if (Math.abs(amount) < unitsInNext) {
			if (unit === "second" && Math.abs(amount) < 45) {
				return "just now";
			}
			return relativeTimeFormat.format(Math.round(amount), unit);
		}
		amount /= unitsInNext;
	}
	return relativeTimeFormat.format(Math.round(amount), "year");
}

const absoluteFormat = new Intl.DateTimeFormat(undefined, {
	dateStyle: "medium",
	timeStyle: "short",
});

export function formatAbsoluteTime(isoTimestamp: string): string {
	const timestamp = Date.parse(isoTimestamp);
	if (Number.isNaN(timestamp)) {
		return "";
	}
	return absoluteFormat.format(timestamp);
}

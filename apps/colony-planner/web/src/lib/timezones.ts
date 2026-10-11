export type TimezoneOption = {
	/** The IANA name the server stores, e.g. America/New_York. */
	name: string;
	/** "New York" */
	city: string;
	/** "America" */
	region: string;
	/** "GMT-4" right now; it changes with daylight saving time. */
	offset: string;
	/** Minutes east of UTC right now, for sorting. */
	offsetMinutes: number;
};

const offsetPattern = /^GMT(?:([+-])(\d{1,2})(?::(\d{2}))?)?$/;
const underscorePattern = /_/g;
const separatorPattern = /[\s/_·]+/;

let cachedOptions: TimezoneOption[] | undefined;

/** Every IANA zone this browser knows, grouped by region and sorted by offset. Built once, lazily. */
export function timezoneOptions(): TimezoneOption[] {
	if (cachedOptions) {
		return cachedOptions;
	}
	const names = new Set(Intl.supportedValuesOf("timeZone"));
	names.add("UTC");
	const now = new Date();
	cachedOptions = [...names]
		.map((name) => describeTimezone(name, now))
		.sort(
			(first, second) =>
				first.region.localeCompare(second.region) ||
				first.offsetMinutes - second.offsetMinutes ||
				first.city.localeCompare(second.city),
		);
	return cachedOptions;
}

export function describeTimezone(name: string, now: Date = new Date()): TimezoneOption {
	const separator = name.indexOf("/");
	const region = separator === -1 ? "Other" : name.slice(0, separator);
	const place = separator === -1 ? name : name.slice(separator + 1);
	const city = place.split("/").join(" · ").replace(underscorePattern, " ");
	const offset = currentOffset(name, now);
	return { city, name, offset, offsetMinutes: offsetToMinutes(offset), region };
}

function currentOffset(name: string, now: Date): string {
	try {
		const parts = new Intl.DateTimeFormat("en-US", {
			timeZone: name,
			timeZoneName: "shortOffset",
		}).formatToParts(now);
		return parts.find((part) => part.type === "timeZoneName")?.value ?? "GMT";
	} catch {
		return "GMT";
	}
}

function offsetToMinutes(offset: string): number {
	const match = offsetPattern.exec(offset);
	if (!match?.[1]) {
		return 0;
	}
	const sign = match[1] === "-" ? -1 : 1;
	const hours = Number.parseInt(match[2] ?? "0", 10);
	const minutes = Number.parseInt(match[3] ?? "0", 10);
	return sign * (hours * 60 + minutes);
}

/** The zone this device is set to, which is usually the right answer for the DM. */
export function deviceTimezone(): string | undefined {
	try {
		return Intl.DateTimeFormat().resolvedOptions().timeZone;
	} catch {
		return undefined;
	}
}

function searchWords(text: string): string[] {
	return text.toLowerCase().split(separatorPattern).filter(Boolean);
}

/**
 * Ranks a zone for a search: a city that starts with the search first, then any word that does,
 * then anything containing it. Every word of the search must appear somewhere. Zero hides the zone.
 */
export function timezoneSearchScore(
	value: string,
	search: string,
	keywords: string[] = [],
): number {
	const terms = searchWords(search);
	if (terms.length === 0) {
		return 1;
	}
	const haystack = [value, ...keywords].join(" ").toLowerCase();
	if (!terms.every((term) => haystack.includes(term))) {
		return 0;
	}
	const city = (keywords.at(0) ?? "").toLowerCase();
	const query = terms.join(" ");
	if (city.startsWith(query)) {
		return 1;
	}
	const words = searchWords(haystack);
	if (terms.every((term) => words.some((word) => word.startsWith(term)))) {
		return 0.75;
	}
	return 0.4;
}

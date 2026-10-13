import type { Schemas } from "@/api/client";
import { dayName, formatNumber } from "@/lib/colony";
import { describeTimezone } from "@/lib/timezones";

type Session = Schemas["Session"];
type SessionSeries = Schemas["SessionSeries"];
type SessionSeriesInput = Schemas["SessionSeriesInput"];

export type Cadence = "weekly" | "fortnightly" | "once";

export const cadenceLabels: Record<Cadence, string> = {
	fortnightly: "Every other week",
	once: "One-off",
	weekly: "Every week",
};

const datePattern = /^\d{4}-\d{2}-\d{2}$/;
const clockPattern = /^([01]\d|2[0-3]):([0-5]\d)$/;
export const minimumDurationMinutes = 15;
export const maximumDurationMinutes = 480;
export const seriesTitleMaxLength = 80;
export const seriesDescriptionMaxLength = 2000;

function dateParts(localDate: string): [number, number, number] | null {
	if (!datePattern.test(localDate)) {
		return null;
	}
	const [year = 0, month = 0, day = 0] = localDate.split("-").map(Number);
	const time = Date.UTC(year, month - 1, day);
	if (new Date(time).getUTCDate() !== day) {
		return null;
	}
	return [year, month, day];
}

function clockParts(clock: string): [number, number] | null {
	const match = clockPattern.exec(clock);
	if (!match) {
		return null;
	}
	return [Number(match[1]), Number(match[2])];
}

/** The weekday names from Sunday (0) to Saturday (6), the API's numbering. */
export function weekdayNames(style: "long" | "short", locale?: string): string[] {
	const format = new Intl.DateTimeFormat(locale, { timeZone: "UTC", weekday: style });
	// 4 January 2026 is a Sunday.
	return Array.from({ length: 7 }, (_, weekday) => format.format(Date.UTC(2026, 0, 4 + weekday)));
}

/** 0 = Sunday for a YYYY-MM-DD date; null when it isn't a real date. */
export function weekdayOf(localDate: string): number | null {
	const parts = dateParts(localDate);
	if (!parts) {
		return null;
	}
	return new Date(Date.UTC(parts[0], parts[1] - 1, parts[2])).getUTCDay();
}

export function cadenceOf(series: Pick<SessionSeries, "everyWeeks">): Cadence {
	if (series.everyWeeks === null) {
		return "once";
	}
	return series.everyWeeks === 2 ? "fortnightly" : "weekly";
}

/** "Every Friday", "Every other Tuesday and Friday", "One-off". */
export function cadenceSummary(
	series: Pick<SessionSeries, "everyWeeks" | "weekdays">,
	locale?: string,
): string {
	const cadence = cadenceOf(series);
	if (cadence === "once") {
		return cadenceLabels.once;
	}
	const names = weekdayNames("long", locale);
	const days = [...series.weekdays]
		.sort((first, second) => first - second)
		.map((weekday) => names[weekday] ?? "");
	const list = new Intl.ListFormat(locale, { style: "long", type: "conjunction" }).format(days);
	return cadence === "fortnightly" ? `Every other ${list}` : `Every ${list}`;
}

/** "Day 36", or "Days 36–41" once it ended on a later day; null before it was played. */
export function colonyDaysLabel(start: number | null, end: number | null): string | null {
	if (start === null) {
		return null;
	}
	if (end === null || end === start) {
		return dayName(start);
	}
	return `Days ${formatNumber(start)}–${formatNumber(end)}`;
}

/** "45 min", "3 h", "2 h 30 min". */
export function durationLabel(minutes: number): string {
	const hours = Math.floor(minutes / 60);
	const rest = minutes % 60;
	if (hours === 0) {
		return `${rest} min`;
	}
	return rest === 0 ? `${hours} h` : `${hours} h ${rest} min`;
}

/** "Friday, October 16, 2026" for a date in the table's calendar (no time zone shift). */
export function localDateLabel(localDate: string, locale?: string): string {
	const parts = dateParts(localDate);
	if (!parts) {
		return localDate;
	}
	return new Intl.DateTimeFormat(locale, {
		dateStyle: "full",
		timeZone: "UTC",
	}).format(Date.UTC(parts[0], parts[1] - 1, parts[2]));
}

/** "8:00 PM" for an HH:MM wall-clock time. */
export function clockLabel(clock: string, locale?: string): string {
	const parts = clockParts(clock);
	if (!parts) {
		return clock;
	}
	return new Intl.DateTimeFormat(locale, {
		hour: "numeric",
		minute: "2-digit",
		timeZone: "UTC",
	}).format(Date.UTC(2000, 0, 1, parts[0], parts[1]));
}

export type ZonedTime = {
	/** "Fri, Oct 16" */
	date: string;
	/** "8:00 PM" */
	time: string;
	/** "EDT" or "GMT+2": the zone's short name at that instant. */
	zone: string;
	/** Minutes east of UTC at that instant, to tell whether two zones agree. */
	offsetMinutes: number;
};

/** An instant as a wall clock in a zone; no zone means this device's. */
export function zonedTime(isoInstant: string, timeZone?: string, locale?: string): ZonedTime {
	const instant = Date.parse(isoInstant);
	const date = new Intl.DateTimeFormat(locale, {
		day: "numeric",
		month: "short",
		timeZone,
		weekday: "short",
	}).format(instant);
	const timeParts = new Intl.DateTimeFormat(locale, {
		hour: "numeric",
		minute: "2-digit",
		timeZone,
		timeZoneName: "short",
	}).formatToParts(instant);
	const time = timeParts
		.filter((part) => part.type !== "timeZoneName")
		.map((part) => part.value)
		.join("")
		.trim();
	const zone = timeParts.find((part) => part.type === "timeZoneName")?.value ?? "";
	const zoneName = timeZone ?? new Intl.DateTimeFormat().resolvedOptions().timeZone;
	return {
		date,
		offsetMinutes: describeTimezone(zoneName, new Date(instant)).offsetMinutes,
		time,
		zone,
	};
}

/** "UTC", "UTC−4", "UTC+5:30". */
export function utcOffsetLabel(offsetMinutes: number): string {
	if (offsetMinutes === 0) {
		return "UTC";
	}
	const sign = offsetMinutes < 0 ? "−" : "+";
	const magnitude = Math.abs(offsetMinutes);
	const hours = Math.floor(magnitude / 60);
	const minutes = magnitude % 60;
	return `UTC${sign}${hours}${minutes === 0 ? "" : `:${String(minutes).padStart(2, "0")}`}`;
}

/**
 * The table's UTC offset on a date, worked out from the wall clock the series names now and the
 * instant the server computed for it. Players can't read the campaign's zone name, so this labels
 * their "table time". A played session keeps the start it was played at while its series' start time
 * may have moved since, so it gets null, as does a pair that isn't one real offset.
 */
export function tableOffsetMinutes(
	session: Pick<Session, "localDate" | "startsAt" | "status">,
	startLocal: string,
): number | null {
	if (session.status === "PLAYED") {
		return null;
	}
	const date = dateParts(session.localDate);
	const clock = clockParts(startLocal);
	const instant = Date.parse(session.startsAt);
	if (!(date && clock) || Number.isNaN(instant)) {
		return null;
	}
	const wallClockAsUtc = Date.UTC(date[0], date[1] - 1, date[2], clock[0], clock[1]);
	const offset = (wallClockAsUtc - instant) / 60_000;
	if (Math.abs(offset) > 14 * 60 || offset % 15 !== 0) {
		return null;
	}
	return offset;
}

/** The city of an IANA zone, "New York" for America/New_York. */
export function timezoneCity(timeZone: string): string {
	return describeTimezone(timeZone).city;
}

/** Today's YYYY-MM-DD on this device, the default first date for a new series. */
export function todayLocalDate(now: Date = new Date()): string {
	const year = now.getFullYear();
	const month = String(now.getMonth() + 1).padStart(2, "0");
	const day = String(now.getDate()).padStart(2, "0");
	return `${year}-${month}-${day}`;
}

/** The series form's state: text inputs hold strings, and the cadence picks every_weeks. */
export type SeriesDraft = {
	title: string;
	description: string;
	cadence: Cadence;
	weekdays: number[];
	firstDate: string;
	lastDate: string;
	startLocal: string;
	durationMinutes: string;
	announce: boolean;
	remind: boolean;
};

export function seriesDraft(series: SessionSeries | null, today: string): SeriesDraft {
	if (series) {
		return {
			announce: series.announce,
			cadence: cadenceOf(series),
			description: series.description,
			durationMinutes: String(series.durationMinutes),
			firstDate: series.firstDate,
			lastDate: series.lastDate ?? "",
			remind: series.remind,
			startLocal: series.startLocal,
			title: series.title,
			weekdays: [...series.weekdays],
		};
	}
	const weekday = weekdayOf(today);
	return {
		announce: true,
		cadence: "weekly",
		description: "",
		durationMinutes: "240",
		firstDate: today,
		lastDate: "",
		remind: true,
		startLocal: "19:00",
		title: "",
		weekdays: weekday === null ? [] : [weekday],
	};
}

export type SeriesDraftErrors = Partial<Record<keyof SessionSeriesInput, string>>;

/**
 * What keeps the form from saving, keyed like the API's field errors. The server still decides; this
 * only catches what the person can see is unfinished before a round trip.
 */
export function seriesDraftErrors(draft: SeriesDraft): SeriesDraftErrors {
	const errors: SeriesDraftErrors = {};
	if (draft.title.trim() === "") {
		errors.title = "Give it a title.";
	}
	if (draft.cadence !== "once" && draft.weekdays.length === 0) {
		errors.weekdays = "Pick at least one day of the week.";
	}
	if (!dateParts(draft.firstDate)) {
		errors.firstDate =
			draft.cadence === "once" ? "Pick the date." : "Pick the date of the first session.";
	}
	if (draft.cadence !== "once" && draft.lastDate !== "") {
		if (!dateParts(draft.lastDate)) {
			errors.lastDate = "Pick a real date, or clear it.";
		} else if (draft.lastDate < draft.firstDate) {
			errors.lastDate = "The last date can't come before the first.";
		}
	}
	if (!clockParts(draft.startLocal)) {
		errors.startLocal = "Pick a start time.";
	}
	const duration = Number(draft.durationMinutes);
	if (
		!Number.isInteger(duration) ||
		duration < minimumDurationMinutes ||
		duration > maximumDurationMinutes
	) {
		errors.durationMinutes = "Between 15 minutes and 8 hours, in whole minutes.";
	}
	return errors;
}

export function seriesInput(draft: SeriesDraft): SessionSeriesInput {
	const isOnce = draft.cadence === "once";
	let everyWeeks: number | null = null;
	if (draft.cadence === "weekly") {
		everyWeeks = 1;
	} else if (draft.cadence === "fortnightly") {
		everyWeeks = 2;
	}
	return {
		announce: draft.announce,
		description: draft.description.trim(),
		durationMinutes: Number(draft.durationMinutes),
		everyWeeks,
		firstDate: draft.firstDate,
		lastDate: isOnce || draft.lastDate === "" ? null : draft.lastDate,
		remind: draft.remind,
		startLocal: draft.startLocal,
		title: draft.title.trim(),
		weekdays: isOnce ? [] : [...draft.weekdays].sort((first, second) => first - second),
	};
}

/** A stable key for an upcoming date, which has no ID until something is stored for it. */
export function sessionDateKey(session: { seriesId: string | null; localDate: string }): string {
	return `${session.seriesId ?? "none"}:${session.localDate}`;
}

type UpcomingDate = Pick<Session, "startsAt" | "status">;

export type UpcomingMonth<Item> = {
	/** "2026-11" on the viewer's calendar. */
	key: string;
	/** "November 2026" */
	label: string;
	sessions: Item[];
};

export type UpcomingPlan<Item> = {
	/** The next scheduled date; a cancelled or played date before it stays in the list below. */
	next: Item | null;
	/** Any cancelled or played dates before the next game, then the few after it. */
	comingUp: Item[];
	/** The rest of the window, by month on the viewer's calendar. */
	later: UpcomingMonth<Item>[];
};

export const comingUpCount = 3;

function calendarParts(isoInstant: string, timeZone?: string) {
	const parts = new Intl.DateTimeFormat("en-US", {
		day: "numeric",
		hour: "numeric",
		hourCycle: "h23",
		month: "numeric",
		timeZone,
		year: "numeric",
	}).formatToParts(Date.parse(isoInstant));
	const value = (type: Intl.DateTimeFormatPartTypes) =>
		Number(parts.find((part) => part.type === type)?.value);
	return { day: value("day"), hour: value("hour"), month: value("month"), year: value("year") };
}

/**
 * Splits the upcoming window into the next game, the few dates after it and the rest by month, so a
 * weekly series doesn't fill the page with three months of cards.
 */
export function planUpcoming<Item extends UpcomingDate>(
	sessions: readonly Item[],
	timeZone?: string,
	locale?: string,
): UpcomingPlan<Item> {
	const byStart = [...sessions].sort(
		(first, second) => Date.parse(first.startsAt) - Date.parse(second.startsAt),
	);
	const nextIndex = byStart.findIndex((session) => session.status === "SCHEDULED");
	// Dates before the card stay in view, so cancelling the card's date never hides it.
	const before = byStart.slice(0, Math.max(nextIndex, 0));
	const after = byStart.slice(nextIndex + 1);
	const monthFormat = new Intl.DateTimeFormat(locale, { month: "long", timeZone, year: "numeric" });
	const later: UpcomingMonth<Item>[] = [];
	for (const session of after.slice(comingUpCount)) {
		const { year, month } = calendarParts(session.startsAt, timeZone);
		const key = `${year}-${String(month).padStart(2, "0")}`;
		const current = later.at(-1);
		if (current?.key === key) {
			current.sessions.push(session);
		} else {
			later.push({
				key,
				label: monthFormat.format(Date.parse(session.startsAt)),
				sessions: [session],
			});
		}
	}
	return {
		comingUp: [...before, ...after.slice(0, comingUpCount)],
		later,
		next: byStart[nextIndex] ?? null,
	};
}

const eveningHour = 17;

/** "under way", "in 40 minutes", "tonight", "today", "tomorrow", "in 3 days" on the viewer's calendar. */
export function countdownLabel(
	startsAt: string,
	now: Date = new Date(),
	timeZone?: string,
	locale?: string,
): string {
	const minutesAway = (Date.parse(startsAt) - now.getTime()) / 60_000;
	if (minutesAway <= 0) {
		return "under way";
	}
	const relative = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
	if (minutesAway < 60) {
		return relative.format(Math.ceil(minutesAway), "minute");
	}
	const start = calendarParts(startsAt, timeZone);
	const today = calendarParts(now.toISOString(), timeZone);
	const daysAway = Math.round(
		(Date.UTC(start.year, start.month - 1, start.day) -
			Date.UTC(today.year, today.month - 1, today.day)) /
			(24 * 60 * 60 * 1000),
	);
	if (daysAway === 0) {
		return start.hour >= eveningHour ? "tonight" : "today";
	}
	return relative.format(daysAway, "day");
}

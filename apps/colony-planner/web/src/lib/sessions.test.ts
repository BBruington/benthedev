import { describe, expect, test } from "bun:test";
import {
	cadenceSummary,
	clockLabel,
	colonyDaysLabel,
	countdownLabel,
	durationLabel,
	localDateLabel,
	planUpcoming,
	type SeriesDraft,
	seriesDraft,
	seriesDraftErrors,
	seriesInput,
	tableOffsetMinutes,
	todayLocalDate,
	utcOffsetLabel,
	weekdayOf,
	zonedTime,
} from "./sessions";

const friday = "2026-10-16";

function scheduled(localDate: string, startsAt: string) {
	return { localDate, startsAt, status: "SCHEDULED" as const };
}

function draft(changes: Partial<SeriesDraft> = {}): SeriesDraft {
	return { ...seriesDraft(null, friday), title: "Friday game", ...changes };
}

describe("session times", () => {
	test("keeps 8 PM in New York across the November change while UTC moves", () => {
		const beforeChange = zonedTime("2026-10-31T00:00:00Z", "America/New_York", "en-US");
		const afterChange = zonedTime("2026-11-07T01:00:00Z", "America/New_York", "en-US");
		expect(beforeChange).toEqual({
			date: "Fri, Oct 30",
			offsetMinutes: -240,
			time: "8:00 PM",
			zone: "EDT",
		});
		expect(afterChange).toEqual({
			date: "Fri, Nov 6",
			offsetMinutes: -300,
			time: "8:00 PM",
			zone: "EST",
		});
	});

	test("shows the same instant on another viewer's clock", () => {
		const london = zonedTime("2026-10-17T00:00:00Z", "Europe/London", "en-US");
		expect(london.date).toBe("Sat, Oct 17");
		expect(london.time).toBe("1:00 AM");
		expect(london.offsetMinutes).toBe(60);
	});

	test("works out the table's offset from its wall clock and the instant", () => {
		expect(tableOffsetMinutes(scheduled("2026-10-30", "2026-10-31T00:00:00Z"), "20:00")).toBe(-240);
		expect(tableOffsetMinutes(scheduled("2026-11-06", "2026-11-07T01:00:00Z"), "20:00")).toBe(-300);
		expect(tableOffsetMinutes(scheduled("2026-10-16", "2026-10-16T13:30:00Z"), "19:00")).toBe(330);
		expect(tableOffsetMinutes(scheduled("2026-10-16", "2026-10-17T00:00:00Z"), "20:07")).toBeNull();
		expect(tableOffsetMinutes(scheduled("not a date", "2026-10-17T00:00:00Z"), "20:00")).toBeNull();
	});

	test("gives no table offset for a played session, whose series' start time may have moved since", () => {
		// Played at 8:00 PM EDT; the series now starts at 7:00 PM, which would misread it as UTC−5.
		const played = {
			localDate: "2026-10-09",
			startsAt: "2026-10-10T00:00:00Z",
			status: "PLAYED" as const,
		};
		expect(tableOffsetMinutes(played, "19:00")).toBeNull();
		expect(tableOffsetMinutes({ ...played, status: "CANCELLED" }, "20:00")).toBe(-240);
	});

	test("labels offsets", () => {
		expect(utcOffsetLabel(0)).toBe("UTC");
		expect(utcOffsetLabel(-240)).toBe("UTC−4");
		expect(utcOffsetLabel(330)).toBe("UTC+5:30");
	});

	test("formats table dates without shifting them through a zone", () => {
		expect(localDateLabel(friday, "en-US")).toBe("Friday, October 16, 2026");
		expect(clockLabel("20:00", "en-US")).toBe("8:00 PM");
		expect(clockLabel("07:05", "en-US")).toBe("7:05 AM");
		expect(weekdayOf(friday)).toBe(5);
		expect(weekdayOf("2026-02-30")).toBeNull();
		expect(todayLocalDate(new Date(2026, 0, 5, 23, 30))).toBe("2026-01-05");
	});

	test("labels colony days and durations", () => {
		expect(colonyDaysLabel(null, null)).toBeNull();
		expect(colonyDaysLabel(36, null)).toBe("Day 36");
		expect(colonyDaysLabel(36, 36)).toBe("Day 36");
		expect(colonyDaysLabel(36, 41)).toBe("Days 36–41");
		expect(colonyDaysLabel(0, null)).toBe("Founding");
		expect(durationLabel(45)).toBe("45 min");
		expect(durationLabel(240)).toBe("4 h");
		expect(durationLabel(150)).toBe("2 h 30 min");
	});

	test("summarizes a cadence", () => {
		expect(cadenceSummary({ everyWeeks: 1, weekdays: [5] }, "en-US")).toBe("Every Friday");
		expect(cadenceSummary({ everyWeeks: 2, weekdays: [5, 2] }, "en-US")).toBe(
			"Every other Tuesday and Friday",
		);
		expect(cadenceSummary({ everyWeeks: null, weekdays: [] }, "en-US")).toBe("One-off");
	});
});

describe("the series form", () => {
	test("starts a new series weekly on the first date's weekday", () => {
		const fresh = seriesDraft(null, friday);
		expect(fresh.cadence).toBe("weekly");
		expect(fresh.weekdays).toEqual([5]);
		expect(fresh.firstDate).toBe(friday);
		expect(fresh.announce && fresh.remind).toBe(true);
	});

	test("accepts a complete draft", () => {
		expect(seriesDraftErrors(draft())).toEqual({});
	});

	test("names each unfinished field", () => {
		const errors = seriesDraftErrors(
			draft({
				durationMinutes: "10",
				firstDate: "",
				startLocal: "",
				title: "  ",
				weekdays: [],
			}),
		);
		expect(Object.keys(errors).sort()).toEqual([
			"durationMinutes",
			"firstDate",
			"startLocal",
			"title",
			"weekdays",
		]);
	});

	test("refuses a last date before the first, and durations outside 15 minutes to 8 hours", () => {
		expect(seriesDraftErrors(draft({ lastDate: "2026-10-01" })).lastDate).toBeDefined();
		expect(seriesDraftErrors(draft({ durationMinutes: "481" })).durationMinutes).toBeDefined();
		expect(seriesDraftErrors(draft({ durationMinutes: "90.5" })).durationMinutes).toBeDefined();
		expect(seriesDraftErrors(draft({ durationMinutes: "480" }))).toEqual({});
	});

	test("a one-off needs no weekday and ignores a last date", () => {
		const once = draft({ cadence: "once", lastDate: "2026-10-01", weekdays: [] });
		expect(seriesDraftErrors(once)).toEqual({});
		expect(seriesInput(once)).toMatchObject({ everyWeeks: null, lastDate: null, weekdays: [] });
	});

	test("builds the API body", () => {
		const body = seriesInput(
			draft({
				cadence: "fortnightly",
				lastDate: "2027-01-29",
				title: " Friday game ",
				weekdays: [5, 2],
			}),
		);
		expect(body).toEqual({
			announce: true,
			description: "",
			durationMinutes: 240,
			everyWeeks: 2,
			firstDate: friday,
			lastDate: "2027-01-29",
			remind: true,
			startLocal: "19:00",
			title: "Friday game",
			weekdays: [2, 5],
		});
	});
});

type UpcomingStatus = "SCHEDULED" | "CANCELLED" | "PLAYED";

function upcoming(title: string, startsAt: string, status: UpcomingStatus = "SCHEDULED") {
	return { startsAt, status, title };
}

/** Fridays at 8 PM in New York from 16 October 2026, a dozen of them across the November change. */
function weeklyFridays(count: number) {
	return Array.from({ length: count }, (_, week) => {
		const wallClock = Date.UTC(2026, 9, 16 + 7 * week, 20);
		const offsetHours = wallClock < Date.UTC(2026, 10, 1, 2) ? 4 : 5;
		return upcoming(
			`Friday ${week + 1}`,
			new Date(wallClock + offsetHours * 60 * 60 * 1000).toISOString(),
		);
	});
}

function cancelledFirst(sessions: ReturnType<typeof upcoming>[], count: number) {
	return sessions.map((session, index) =>
		index < count ? { ...session, status: "CANCELLED" as const } : session,
	);
}

function titles(sessions: { title: string }[]): string[] {
	return sessions.map((session) => session.title);
}

describe("upcoming plan", () => {
	const newYork = "America/New_York";

	test("a weekly series shows one next game, three more and the rest by month", () => {
		const plan = planUpcoming(weeklyFridays(12), newYork, "en-US");
		expect(plan.next?.title).toBe("Friday 1");
		expect(titles(plan.comingUp)).toEqual(["Friday 2", "Friday 3", "Friday 4"]);
		expect(plan.later.map((month) => [month.label, titles(month.sessions)])).toEqual([
			["November 2026", ["Friday 5", "Friday 6", "Friday 7"]],
			["December 2026", ["Friday 8", "Friday 9", "Friday 10", "Friday 11"]],
			["January 2027", ["Friday 12"]],
		]);
	});

	test("interleaves every-other-week dates and one-offs by start time", () => {
		const plan = planUpcoming(
			[
				upcoming("Fortnight 1", "2026-10-17T00:00:00Z"),
				upcoming("Fortnight 2", "2026-10-31T00:00:00Z"),
				upcoming("Fortnight 3", "2026-11-14T01:00:00Z"),
				upcoming("Fortnight 4", "2026-11-28T01:00:00Z"),
				upcoming("Fortnight 5", "2026-12-12T01:00:00Z"),
				upcoming("Picnic", "2026-10-24T16:00:00Z"),
				upcoming("Solstice", "2026-12-21T23:00:00Z"),
			],
			newYork,
			"en-US",
		);
		expect(plan.next?.title).toBe("Fortnight 1");
		expect(titles(plan.comingUp)).toEqual(["Picnic", "Fortnight 2", "Fortnight 3"]);
		expect(plan.later.map((month) => [month.key, titles(month.sessions)])).toEqual([
			["2026-11", ["Fortnight 4"]],
			["2026-12", ["Fortnight 5", "Solstice"]],
		]);
	});

	test("a cancelled first date leaves the card to the next scheduled one and leads the list", () => {
		const [first, ...rest] = weeklyFridays(6);
		const plan = planUpcoming(
			[{ ...upcoming("Friday 1", first?.startsAt ?? ""), status: "CANCELLED" as const }, ...rest],
			newYork,
			"en-US",
		);
		expect(plan.next?.title).toBe("Friday 2");
		expect(titles(plan.comingUp)).toEqual(["Friday 1", "Friday 3", "Friday 4", "Friday 5"]);
		expect(plan.comingUp[0]?.status).toBe("CANCELLED");
		expect(plan.later.flatMap((month) => titles(month.sessions))).toEqual(["Friday 6"]);
	});

	test("a run of cancelled dates stays in view and still leaves three dates after the card", () => {
		const plan = planUpcoming(cancelledFirst(weeklyFridays(12), 3), newYork, "en-US");
		expect(plan.next?.title).toBe("Friday 4");
		expect(titles(plan.comingUp)).toEqual([
			"Friday 1",
			"Friday 2",
			"Friday 3",
			"Friday 5",
			"Friday 6",
			"Friday 7",
		]);
		expect(plan.later.flatMap((month) => titles(month.sessions))[0]).toBe("Friday 8");
	});

	test("cancelling or restoring a date in view never moves it behind the disclosure", () => {
		for (const cancelledCount of [0, 1, 3, 4]) {
			const sessions = cancelledFirst(weeklyFridays(12), cancelledCount);
			const plan = planUpcoming(sessions, newYork, "en-US");
			for (const acted of [plan.next, ...plan.comingUp]) {
				if (acted === null) {
					continue;
				}
				const flipped: UpcomingStatus = acted.status === "CANCELLED" ? "SCHEDULED" : "CANCELLED";
				const after = planUpcoming(
					sessions.map((session) =>
						session === acted ? { ...session, status: flipped } : session,
					),
					newYork,
					"en-US",
				);
				expect(
					titles([after.next, ...after.comingUp].filter((session) => session !== null)),
				).toContain(acted.title);
			}
		}
	});

	test("fewer than four dates need no later months", () => {
		const three = planUpcoming(weeklyFridays(3), newYork, "en-US");
		expect(three.next?.title).toBe("Friday 1");
		expect(titles(three.comingUp)).toEqual(["Friday 2", "Friday 3"]);
		expect(three.later).toEqual([]);
		const one = planUpcoming(weeklyFridays(1), newYork, "en-US");
		expect(one.next?.title).toBe("Friday 1");
		expect(one.comingUp).toEqual([]);
		expect(planUpcoming([], newYork, "en-US")).toEqual({ comingUp: [], later: [], next: null });
	});

	test("with nothing scheduled there is no card and the list holds every date", () => {
		const plan = planUpcoming(
			[upcoming("Played early", "2026-10-17T00:00:00Z", "PLAYED")].concat(
				weeklyFridays(2).map((session) => ({ ...session, status: "CANCELLED" as const })),
			),
			newYork,
			"en-US",
		);
		expect(plan.next).toBeNull();
		expect(plan.comingUp).toHaveLength(3);
	});

	test("groups months on the viewer's calendar", () => {
		const sessions = [
			...weeklyFridays(2),
			upcoming("Tuesday", "2026-10-21T00:00:00Z"),
			upcoming("Another Tuesday", "2026-10-28T00:00:00Z"),
			upcoming("Halloween night", "2026-11-01T00:30:00Z"),
			upcoming("November proper", "2026-11-10T00:30:00Z"),
		];
		const inNewYork = planUpcoming(sessions, newYork, "en-US");
		expect(inNewYork.later.map((month) => [month.label, titles(month.sessions)])).toEqual([
			["October 2026", ["Halloween night"]],
			["November 2026", ["November proper"]],
		]);
		const inLondon = planUpcoming(sessions, "Europe/London", "en-US");
		expect(inLondon.later.map((month) => [month.label, titles(month.sessions)])).toEqual([
			["November 2026", ["Halloween night", "November proper"]],
		]);
	});

	test("counts down in the viewer's days", () => {
		const fridayNight = "2026-10-17T00:00:00Z";
		const at = (instant: string) => new Date(instant);
		expect(countdownLabel(fridayNight, at("2026-10-16T14:00:00Z"), newYork, "en-US")).toBe(
			"tonight",
		);
		expect(countdownLabel(fridayNight, at("2026-10-16T23:20:00Z"), newYork, "en-US")).toBe(
			"in 40 minutes",
		);
		expect(countdownLabel(fridayNight, at("2026-10-17T00:30:00Z"), newYork, "en-US")).toBe(
			"under way",
		);
		expect(countdownLabel(fridayNight, at("2026-10-15T14:00:00Z"), newYork, "en-US")).toBe(
			"tomorrow",
		);
		expect(countdownLabel(fridayNight, at("2026-10-13T14:00:00Z"), newYork, "en-US")).toBe(
			"in 3 days",
		);
		expect(
			countdownLabel("2026-10-17T15:00:00Z", at("2026-10-17T05:00:00Z"), newYork, "en-US"),
		).toBe("today");
		expect(countdownLabel(fridayNight, at("2026-10-16T14:00:00Z"), "Europe/London", "en-US")).toBe(
			"tomorrow",
		);
	});
});

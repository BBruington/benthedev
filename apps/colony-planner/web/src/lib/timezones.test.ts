import { describe, expect, test } from "bun:test";
import { describeTimezone, timezoneSearchScore } from "./timezones";

describe("time zones", () => {
	test("describes a zone by city and region", () => {
		const newYork = describeTimezone("America/New_York", new Date("2026-01-15T12:00:00Z"));
		expect(newYork.city).toBe("New York");
		expect(newYork.region).toBe("America");
		expect(newYork.offset).toBe("GMT-5");
		expect(newYork.offsetMinutes).toBe(-300);
	});

	test("ranks a city that starts with the search above a loose match", () => {
		const berlin = timezoneSearchScore("Europe/Berlin", "berl", ["Berlin", "Europe", "GMT+2"]);
		const libreville = timezoneSearchScore("Africa/Libreville", "berl", ["Libreville", "Africa"]);
		expect(berlin).toBe(1);
		expect(libreville).toBe(0);
	});

	test("matches words in any order and hides zones missing a word", () => {
		expect(timezoneSearchScore("America/New_York", "york new", ["New York", "America"])).toBe(0.75);
		expect(timezoneSearchScore("America/New_York", "new jersey", ["New York", "America"])).toBe(0);
	});
});

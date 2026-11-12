import { expect, test } from "bun:test";
import { createTimeTracker, secondsPerDay } from "./time-tracker";

function trackerStartingAt(day: number | undefined) {
	const reported: number[] = [];
	const saved: number[] = [];
	const tracker = createTimeTracker({
		load: () => day,
		onError: (error) => {
			throw error;
		},
		report: (days) => reported.push(days),
		save: (savedDay) => {
			saved.push(savedDay);
			return Promise.resolve();
		},
	});
	return { reported, saved, tracker };
}

test("the first time it sees the world it only records the day", () => {
	const { reported, saved, tracker } = trackerStartingAt(undefined);
	tracker.observe(10 * secondsPerDay + 500, false);
	expect(reported).toEqual([]);
	expect(saved).toEqual([10]);
});

test("reports forward moves of whole days, and not moves within a day", () => {
	const { reported, saved, tracker } = trackerStartingAt(10);
	tracker.observe(10 * secondsPerDay + 3600, false);
	tracker.observe(13 * secondsPerDay + 10, false);
	tracker.observe(14 * secondsPerDay, false);
	expect(reported).toEqual([3, 1]);
	expect(saved).toEqual([13, 14]);
});

test("a reload reads the saved day, so it reports nothing twice", () => {
	const { reported, tracker } = trackerStartingAt(14);
	tracker.observe(14 * secondsPerDay + 7200, false);
	expect(reported).toEqual([]);
});

test("moving back and forward again reports only days past the furthest one", () => {
	const { reported, tracker } = trackerStartingAt(20);
	tracker.observe(15 * secondsPerDay, false);
	tracker.observe(19 * secondsPerDay, false);
	tracker.observe(22 * secondsPerDay, false);
	expect(reported).toEqual([2]);
});

test("the bridge's own clock moves are recorded, never reported", () => {
	const { reported, saved, tracker } = trackerStartingAt(5);
	tracker.observe(8 * secondsPerDay, true);
	tracker.observe(9 * secondsPerDay, false);
	expect(reported).toEqual([1]);
	expect(saved).toEqual([8, 9]);
});

test("a jump past the server's limit is reported as the limit", () => {
	const { reported, tracker } = trackerStartingAt(0);
	tracker.observe(5000 * secondsPerDay, false);
	expect(reported).toEqual([3650]);
});

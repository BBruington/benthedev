import { maximumReportedDays } from "./wire";

export const secondsPerDay = 86_400;

export interface TimeTracker {
	/** A world time change; `madeByBridge` when the bridge itself moved the clock (a colony advance). */
	observe(worldTime: number, madeByBridge: boolean): void;
}

/**
 * Reports Foundry's clock moving forward by whole days (design §5). It keeps the furthest whole day it
 * has reported or seen the bridge set, in a world setting so a reload or a hand-off to another GM
 * doesn't report the same days twice; moving the clock back and forward again reports nothing until it
 * passes that day. The first time it sees a world it only records the day.
 */
export function createTimeTracker(options: {
	load(): number | undefined;
	report(days: number): void;
	save(day: number): Promise<unknown>;
	onError(error: unknown): void;
}): TimeTracker {
	let furthestDay = options.load();

	function record(day: number) {
		furthestDay = day;
		options.save(day).catch(options.onError);
	}

	return {
		observe(worldTime, madeByBridge) {
			const day = Math.floor(worldTime / secondsPerDay);
			if (furthestDay === undefined) {
				record(day);
				return;
			}
			if (day <= furthestDay) {
				return;
			}
			const days = day - furthestDay;
			record(day);
			if (!madeByBridge) {
				options.report(Math.min(days, maximumReportedDays));
			}
		},
	};
}

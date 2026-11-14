import type { Schedule } from "../schedule";

export interface FakeSchedule {
	/** Runs the timer due soonest; its delay is returned so tests can check the backoff. */
	runNext(): number;
	schedule: Schedule;
	waitingDelays(): number[];
}

export function createFakeSchedule(): FakeSchedule {
	let timers: { callback: () => void; delay: number }[] = [];
	return {
		runNext() {
			const soonest = timers.reduce<(typeof timers)[number] | undefined>(
				(earliest, timer) => (earliest && earliest.delay <= timer.delay ? earliest : timer),
				undefined,
			);
			if (!soonest) {
				throw new Error("No timer is waiting.");
			}
			timers = timers.filter((timer) => timer !== soonest);
			soonest.callback();
			return soonest.delay;
		},
		schedule(callback, delay) {
			const timer = { callback, delay };
			timers.push(timer);
			return () => {
				timers = timers.filter((waiting) => waiting !== timer);
			};
		},
		waitingDelays: () => timers.map((timer) => timer.delay),
	};
}

/** Lets every promise callback already queued (and the ones they queue) run. */
export function settle(): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, 5));
}

export function jsonResponse(status: number, body: unknown): Response {
	return new Response(JSON.stringify(body), {
		headers: { "Content-Type": "application/json" },
		status,
	});
}

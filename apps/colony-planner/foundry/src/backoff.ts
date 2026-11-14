const firstDelayInMilliseconds = 1000;
const longestDelayInMilliseconds = 60_000;

/** The wait before retry number `attempt` (0 first): 1 s, doubling to at most 60 s (design §6). */
export function backoffDelay(attempt: number): number {
	return Math.min(longestDelayInMilliseconds, firstDelayInMilliseconds * 2 ** Math.max(0, attempt));
}

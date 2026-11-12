/** Runs `callback` after a delay and returns a function that cancels it; tests pass a fake. */
export type Schedule = (callback: () => void, delayInMilliseconds: number) => () => void;

export const scheduleWithTimeout: Schedule = (callback, delayInMilliseconds) => {
	const handle = setTimeout(callback, delayInMilliseconds);
	return () => clearTimeout(handle);
};

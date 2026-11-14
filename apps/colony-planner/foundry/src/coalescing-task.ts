/**
 * Runs `task` once per request, never two at a time: a request made while it runs makes it run once
 * more afterwards, however many requests arrived meanwhile.
 */
export function createCoalescingTask(
	task: () => Promise<void>,
	onError: (error: unknown) => void,
): () => void {
	let running = false;
	let requestedAgain = false;

	async function run() {
		running = true;
		requestedAgain = false;
		try {
			await task();
		} catch (error) {
			onError(error);
		}
		running = false;
		if (requestedAgain) {
			startRun();
		}
	}

	function startRun() {
		run().catch(onError);
	}

	return () => {
		if (running) {
			requestedAgain = true;
			return;
		}
		startRun();
	};
}

import { describe, expect, test } from "bun:test";
import { QueryClient, QueryObserver } from "@tanstack/react-query";
import { createLiveInvalidator } from "./live-invalidation";

const queryKey = ["get", "/api/campaigns/{campaignId}/hexes"] as const;

function settle() {
	return new Promise((resolve) => setTimeout(resolve, 20));
}

/** A query whose reads finish only when the test says so, watched as a screen would watch it. */
function watchedQuery() {
	const queryClient = new QueryClient();
	const reads: { signal: AbortSignal; finish: (value: string) => void }[] = [];
	const observer = new QueryObserver(queryClient, {
		queryFn: ({ signal }) =>
			new Promise<string>((resolve) => {
				reads.push({ finish: resolve, signal });
			}),
		queryKey,
	});
	const unsubscribe = observer.subscribe(() => undefined);
	const invalidator = createLiveInvalidator(queryClient);
	return { invalidator, observer, queryClient, reads, unsubscribe };
}

describe("live invalidation", () => {
	test("never aborts a read in flight, and reads once more after it", async () => {
		const watched = watchedQuery();
		await settle();
		expect(watched.reads.length).toBe(1);

		// Updates arriving faster than the read finishes leave it running.
		watched.invalidator.invalidate(queryKey);
		watched.invalidator.invalidate(queryKey);
		await settle();
		expect(watched.reads.length).toBe(1);
		expect(watched.reads[0]?.signal.aborted).toBe(false);

		watched.reads[0]?.finish("before the write");
		await settle();
		expect(watched.observer.getCurrentResult().data).toBe("before the write");
		expect(watched.reads.length).toBe(2);

		watched.reads[1]?.finish("after the write");
		await settle();
		expect(watched.observer.getCurrentResult().data).toBe("after the write");
		expect(watched.reads.length).toBe(2);
		watched.invalidator.stop();
		watched.unsubscribe();
	});

	test("an update while nothing is reading reads once", async () => {
		const watched = watchedQuery();
		await settle();
		watched.reads[0]?.finish("first");
		await settle();

		watched.invalidator.invalidate(queryKey);
		await settle();
		expect(watched.reads.length).toBe(2);
		watched.reads[1]?.finish("second");
		await settle();
		expect(watched.reads.length).toBe(2);
		expect(watched.observer.getCurrentResult().data).toBe("second");
		watched.invalidator.stop();
		watched.unsubscribe();
	});
});

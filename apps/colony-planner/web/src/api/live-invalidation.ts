import type { QueryClient, QueryKey } from "@tanstack/react-query";

/**
 * Marks reads stale for a live update without aborting a read already in flight. Aborting would let
 * updates that come faster than a read finishes (hexes while someone paints, on a slow connection)
 * starve every read; instead a read that was in flight runs once more when it settles, because the
 * write may have committed after it started.
 */
export function createLiveInvalidator(queryClient: QueryClient) {
	const followUpQueryHashes = new Set<string>();
	const unsubscribe = queryClient.getQueryCache().subscribe((event) => {
		const query = event.query;
		if (!followUpQueryHashes.has(query.queryHash) || query.state.fetchStatus !== "idle") {
			return;
		}
		followUpQueryHashes.delete(query.queryHash);
		queryClient.invalidateQueries(
			{ exact: true, queryKey: query.queryKey },
			{ cancelRefetch: false },
		);
	});
	return {
		invalidate(queryKey: QueryKey) {
			for (const query of queryClient.getQueryCache().findAll({ queryKey })) {
				if (query.state.fetchStatus !== "idle") {
					followUpQueryHashes.add(query.queryHash);
				}
			}
			queryClient.invalidateQueries({ queryKey }, { cancelRefetch: false });
		},
		stop() {
			unsubscribe();
			followUpQueryHashes.clear();
		},
	};
}

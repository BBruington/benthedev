import type { Query, QueryClient } from "@tanstack/react-query";
import { meQuery } from "@/api/queries";

/**
 * Brings the query cache over to the projection the DM just switched to with View as player (spec
 * §2.1). Every read but /api/me changes projection. Cached reads hold the old one, and screens draw a
 * DM-only entry from its data, so a page opened from the cache could show the DM's entries in the
 * player view: reads on screen are read again and the rest are dropped. /api/me flips the view last
 * when entering the player view and first when leaving it, so the player view never shows DM data.
 */
export async function switchProjection(
	queryClient: QueryClient,
	viewingAsPlayer: boolean,
): Promise<void> {
	// DM-only reads would only answer 403 while viewing as a player, so they wait for the DM view.
	const changesProjection = (query: Query) =>
		query.queryKey[1] !== meQuery.queryKey[1] && !(viewingAsPlayer && query.meta?.dmOnly === true);
	// The switch answers with no body: it ran under the old projection, so /api/me is read again.
	if (!viewingAsPlayer) {
		await queryClient.invalidateQueries({ queryKey: meQuery.queryKey });
	}
	await queryClient.refetchQueries({ predicate: changesProjection, type: "active" });
	queryClient.removeQueries({
		predicate: (query) => changesProjection(query) && query.getObserversCount() === 0,
	});
	queryClient.invalidateQueries({ predicate: changesProjection, refetchType: "none" });
	if (viewingAsPlayer) {
		await queryClient.invalidateQueries({ queryKey: meQuery.queryKey });
	}
}

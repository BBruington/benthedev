import type { QueryClient } from "@tanstack/react-query";
import type { Schemas } from "@/api/client";
import { isUnauthorized, problemStatus } from "@/api/problem";
import { campaignMeQuery, meQuery } from "@/api/queries";

/** The signed-in account, outside any campaign (docs/design/multi-campaign.md §1). */
export type AccountMe = Schemas["AccountMe"];
/** The caller's membership in the campaign on screen: role, status and the campaign's status. */
export type Me = Schemas["CampaignMe"];

/**
 * The signed-in account for a route guard, or null for a visitor. A 404 also means signed out: the
 * session's account is gone, which only a local database reset does. Anything else is a real failure
 * and is thrown to the route's error screen. "fresh-once" skips retries, for /login, which should
 * appear at once even when the server is struggling.
 */
export async function loadAccount(
	queryClient: QueryClient,
	freshness: "cached" | "fresh" | "fresh-once",
): Promise<AccountMe | null> {
	try {
		if (freshness === "fresh-once") {
			return await queryClient.fetchQuery({ ...meQuery, retry: false, staleTime: 0 });
		}
		if (freshness === "fresh") {
			return await queryClient.fetchQuery({ ...meQuery, staleTime: 0 });
		}
		return await queryClient.ensureQueryData(meQuery);
	} catch (error) {
		if (isUnauthorized(error) || problemStatus(error) === 404) {
			return null;
		}
		throw error;
	}
}

/** The caller's membership from the cache, without fetching: for loaders that run after the guard. */
export function cachedMe(queryClient: QueryClient, campaignId: string): Me | undefined {
	return queryClient.getQueryData<Me>(campaignMeQuery(campaignId).queryKey);
}

/** DM screens and writes: the DM, and not while view as player is on (the server agrees, §2.1). */
export function canUseDmTools(me: Me): boolean {
	return me.isDm && !me.viewAsPlayer;
}

/** The campaign's screens are open to this membership: let in, and the campaign isn't paused. */
export function isLetIn(me: Me): boolean {
	return me.status === "ACTIVE" && me.campaign.status === "ACTIVE";
}

/** How the table knows someone: their character's name when they have one. */
export function tableName(member: { characterName: string | null; displayName: string }): string {
	return member.characterName?.trim() || member.displayName;
}

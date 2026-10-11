import { useQuery } from "@tanstack/react-query";
import { membersQuery, settingsQuery } from "@/api/queries";
import { canUseDmTools, type Me } from "@/lib/access";

export const fallbackCampaignName = "Colony Planner";

/**
 * The campaign's name for the top bar. Only the DM can read settings in M0, so everyone else (and
 * the DM viewing as a player) sees the app's name until a member-level read carries it.
 */
export function useCampaignName(me: Me): string | undefined {
	const { data } = useQuery({ ...settingsQuery, enabled: canUseDmTools(me) });
	return data?.campaignName || undefined;
}

/** How many people are waiting at the door, for the DM's badges; 0 for everyone else. */
export function usePendingCount(me: Me): number {
	const { data } = useQuery({ ...membersQuery, enabled: canUseDmTools(me) });
	if (!canUseDmTools(me)) {
		return 0;
	}
	return data?.pending.length ?? 0;
}

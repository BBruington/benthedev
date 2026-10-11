import { useQuery } from "@tanstack/react-query";
import { colonyBoardQuery, membersQuery } from "@/api/queries";
import { canUseDmTools, type Me } from "@/lib/access";

export const fallbackCampaignName = "Colony Planner";

/**
 * The campaign's and the colony's names, from the member-level board read, so every player sees
 * them too (the DM edits both on /settings). Undefined until the board has loaded.
 */
export function useColonyNames(): { campaignName: string; colonyName: string } | undefined {
	const { data } = useQuery(colonyBoardQuery);
	if (!data) {
		return undefined;
	}
	return { campaignName: data.campaignName, colonyName: data.colonyName };
}

/** The campaign's name for page titles; undefined until it has loaded. */
export function useCampaignName(): string | undefined {
	return useColonyNames()?.campaignName || undefined;
}

/** How many people are waiting at the door, for the DM's badges; 0 for everyone else. */
export function usePendingCount(me: Me): number {
	const { data } = useQuery({ ...membersQuery, enabled: canUseDmTools(me) });
	if (!canUseDmTools(me)) {
		return 0;
	}
	return data?.pending.length ?? 0;
}

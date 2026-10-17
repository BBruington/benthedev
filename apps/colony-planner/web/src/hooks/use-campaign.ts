import { useQuery } from "@tanstack/react-query";
import { createContext, use } from "react";
import { colonyBoardQuery, membersQuery } from "@/api/queries";
import { canUseDmTools, type Me } from "@/lib/access";

export const fallbackCampaignName = "Colony Planner";

/** The campaign on screen, from its /c/{slug} address (docs/design/multi-campaign.md §3.1). */
export type CampaignAddress = { campaignId: string; slug: string };

/** Provided by the /c/$campaignSlug layout once the slug resolves to one of the caller's campaigns. */
export const CampaignContext = createContext<CampaignAddress | null>(null);

/** The campaign on screen. Only for screens under /c/$campaignSlug, whose layout provides it. */
export function useCampaign(): CampaignAddress {
	const campaign = use(CampaignContext);
	if (!campaign) {
		throw new Error("useCampaign is only for screens inside a campaign");
	}
	return campaign;
}

export function useCampaignId(): string {
	return useCampaign().campaignId;
}

/**
 * The campaign's and the colony's names, from the member-level board read, so every player sees
 * them too (the DM edits both on Settings). Undefined until the board has loaded.
 */
export function useColonyNames(): { campaignName: string; colonyName: string } | undefined {
	const { data } = useQuery(colonyBoardQuery(useCampaignId()));
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
	const { data } = useQuery({
		...membersQuery(useCampaignId()),
		enabled: canUseDmTools(me),
	});
	if (!canUseDmTools(me)) {
		return 0;
	}
	return data?.pending.length ?? 0;
}

import { useSuspenseQuery } from "@tanstack/react-query";
import { campaignMeQuery, meQuery } from "@/api/queries";
import type { AccountMe, Me } from "@/lib/access";
import { useCampaignId } from "./use-campaign";

/** The caller's membership in the campaign on screen; its layout has already loaded it. */
export function useMe(): Me {
	return useSuspenseQuery(campaignMeQuery(useCampaignId())).data;
}

/** The signed-in account. Only for screens behind the sign-in guard, which has already loaded it. */
export function useAccount(): AccountMe {
	return useSuspenseQuery(meQuery).data;
}

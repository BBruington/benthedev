import type { QueryClient } from "@tanstack/react-query";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Outlet } from "@tanstack/react-router";
import { useEffect, useMemo, useRef } from "react";
import { problemStatus } from "@/api/problem";
import { campaignMeQuery, myCampaignsQuery } from "@/api/queries";
import { useLiveUpdates } from "@/api/stream";
import { FullScreenError } from "@/components/error-screens";
import { NoSuchCampaign } from "@/components/no-such-campaign";
import { CampaignContext } from "@/hooks/use-campaign";
import { isLetIn } from "@/lib/access";
import { findCampaignBySlug } from "@/lib/campaigns";
import { focusFirstVisible } from "@/lib/focus";
import { rememberLastCampaign } from "@/lib/last-campaign";

/**
 * The campaign behind a /c/{slug} address, or null when the caller has no membership there: the
 * same "No such campaign" whether it exists or not (docs/design/multi-campaign.md §3.1). The slug is
 * resolved from the caller's own campaigns; a cached list that lacks it is read again once, since
 * the caller may have just joined or been approved.
 */
async function resolveCampaignId(queryClient: QueryClient, slug: string): Promise<string | null> {
	const cached = await queryClient.ensureQueryData(myCampaignsQuery);
	let membership = findCampaignBySlug(cached.campaigns, slug);
	if (!membership) {
		const fresh = await queryClient.fetchQuery({ ...myCampaignsQuery, staleTime: 0 });
		membership = findCampaignBySlug(fresh.campaigns, slug);
	}
	if (!membership) {
		return null;
	}
	try {
		await queryClient.ensureQueryData(campaignMeQuery(membership.campaign.id));
	} catch (error) {
		// Deleted since the list was read.
		if (problemStatus(error) === 404) {
			return null;
		}
		throw error;
	}
	return membership.campaign.id;
}

export const Route = createFileRoute("/_signedIn/c/$campaignSlug")({
	beforeLoad: async ({ context, params }) => ({
		campaignId: await resolveCampaignId(context.queryClient, params.campaignSlug),
	}),
	component: CampaignLayout,
	errorComponent: FullScreenError,
});

function CampaignLayout() {
	const { campaignId } = Route.useRouteContext();
	const { campaignSlug } = Route.useParams();
	const shownCampaignId = useRef(campaignId);
	useEffect(() => {
		if (shownCampaignId.current !== campaignId) {
			shownCampaignId.current = campaignId;
			// The switch remounts the shell, the switcher that had focus included.
			focusFirstVisible("main");
		}
	}, [campaignId]);
	if (!campaignId) {
		return <NoSuchCampaign />;
	}
	return <CampaignScope campaignId={campaignId} key={campaignId} slug={campaignSlug} />;
}

/**
 * Everything inside one campaign: its address for every screen, and its live stream, which stays
 * open from the waiting page into the shell and back (§7.1).
 */
function CampaignScope({ campaignId, slug }: { campaignId: string; slug: string }) {
	const campaignMe = useQuery(campaignMeQuery(campaignId));
	const address = useMemo(() => ({ campaignId, slug }), [campaignId, slug]);
	const isLetInHere = campaignMe.data ? isLetIn(campaignMe.data) : false;

	useEffect(() => {
		if (isLetInHere) {
			rememberLastCampaign(slug);
		}
	}, [isLetInHere, slug]);

	// Deleted while open: the stream's reconnect is refused and the me read answers 404 (§5.5).
	if (problemStatus(campaignMe.error) === 404) {
		return <NoSuchCampaign />;
	}
	return (
		<CampaignContext value={address}>
			<LiveCampaign campaignId={campaignId} />
			<Outlet />
		</CampaignContext>
	);
}

function LiveCampaign({ campaignId }: { campaignId: string }) {
	useLiveUpdates(campaignId);
	return null;
}

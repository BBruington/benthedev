import { createFileRoute } from "@tanstack/react-router";
import { hexesQuery } from "@/api/queries";
import { ExploreScreen } from "@/explore/explore-screen";
import { fallbackCampaignName, useCampaignId, useCampaignName } from "@/hooks/use-campaign";
import { useDocumentTitle } from "@/hooks/use-document-title";

export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell/explore")({
	component: ExplorePage,
	loader: ({ context: { campaignId, queryClient } }) => {
		if (campaignId) {
			queryClient.prefetchQuery(hexesQuery(campaignId));
		}
	},
});

/** /explore (spec §6.2): the hex map, with the same tools for the DM and the players. */
function ExplorePage() {
	useDocumentTitle("Explore", useCampaignName() ?? fallbackCampaignName);
	const campaignId = useCampaignId();
	// Keyed, so strokes still queued for one campaign never go out under the next one's ID.
	return <ExploreScreen campaignId={campaignId} key={campaignId} />;
}

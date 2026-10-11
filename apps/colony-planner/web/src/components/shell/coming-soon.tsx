import { Link } from "@tanstack/react-router";
import { ArrowLeftIcon } from "lucide-react";
import { StatusPanel } from "@/components/status-panel";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { fallbackCampaignName, useCampaign, useCampaignName } from "@/hooks/use-campaign";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { type Milestone, type NavigationItemId, navigationItems } from "./navigation";

const milestoneNames: Record<Milestone, string> = {
	M1: "Colony clock and books",
	M2: "People and secrecy",
	M3: "Quests, patrons and requests",
	M4: "World maps",
	M5: "Sessions and Beny Bot",
};

/** The page behind a navigation item whose milestone hasn't been built yet. */
export function ComingSoon({ itemId }: { itemId: NavigationItemId }) {
	const item = navigationItems[itemId];
	useDocumentTitle(item.label, useCampaignName() ?? fallbackCampaignName);
	const { slug } = useCampaign();
	return (
		<StatusPanel
			actions={
				<Button asChild variant="outline">
					<Link params={{ campaignSlug: slug }} to="/c/$campaignSlug">
						<ArrowLeftIcon />
						Back to the colony
					</Link>
				</Button>
			}
			description={item.summary}
			eyebrow={
				item.arrivesIn ? (
					<Badge variant="gilt">
						Coming in {item.arrivesIn} · {milestoneNames[item.arrivesIn]}
					</Badge>
				) : null
			}
			flavor="This stretch of the map is still blank. The cartographers are on their way."
			icon={item.icon}
			title={item.label}
		/>
	);
}

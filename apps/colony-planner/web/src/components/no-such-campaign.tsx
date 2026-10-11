import { Link } from "@tanstack/react-router";
import { CompassIcon, LayoutGridIcon } from "lucide-react";
import { EldariBackdrop } from "@/components/eldari-backdrop";
import { StatusPanel } from "@/components/status-panel";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useDocumentTitle } from "@/hooks/use-document-title";

/**
 * A /c/{slug} the caller has no membership in, or one that was deleted. It reads the same whether
 * the campaign exists or not, as the server's 404 does (docs/design/multi-campaign.md §3.1, §4.1).
 */
export function NoSuchCampaign() {
	useDocumentTitle("No such campaign", "Colony Planner");
	return (
		<EldariBackdrop>
			<main className="w-full max-w-lg rounded-2xl border border-border bg-surface/92 px-6 py-2 shadow-float backdrop-blur-md">
				<StatusPanel
					actions={
						<Button asChild>
							<Link to="/">
								<LayoutGridIcon />
								Your campaigns
							</Link>
						</Button>
					}
					className="py-8 md:py-10"
					description="You aren't in a campaign at this address. If someone sent you here, ask their DM for the invite link."
					eyebrow={<Badge variant="gilt">404</Badge>}
					flavor="No harbour by that name on any chart."
					icon={CompassIcon}
					title="No such campaign"
				/>
			</main>
		</EldariBackdrop>
	);
}

import { Link } from "@tanstack/react-router";
import { ArrowLeftIcon, EyeIcon, ShieldAlertIcon } from "lucide-react";
import { StatusPanel } from "@/components/status-panel";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { useViewAsPlayer } from "@/hooks/use-view-as-player";
import type { Me } from "@/lib/access";

/** The 403 page: what a player sees on a DM screen, and what the DM sees there as a player. */
export function Forbidden({ me }: { me: Me }) {
	useDocumentTitle("DM only", "Colony Planner");
	const { setViewAsPlayer, isSwitching } = useViewAsPlayer();
	const backToColony = (
		<Button asChild variant="outline">
			<Link to="/">
				<ArrowLeftIcon />
				Back to the colony
			</Link>
		</Button>
	);

	if (me.isDm && me.viewAsPlayer) {
		return (
			<StatusPanel
				actions={
					<>
						<Button
							disabled={isSwitching}
							onClick={() => {
								setViewAsPlayer(false);
							}}
						>
							<EyeIcon />
							Back to DM view
						</Button>
						{backToColony}
					</>
				}
				description="Players can't open this page, so neither can you while you're viewing as one."
				eyebrow={<Badge variant="verdigris">403 · DM only</Badge>}
				icon={EyeIcon}
				title="You're viewing as a player"
				tone="verdigris"
			/>
		);
	}

	return (
		<StatusPanel
			actions={backToColony}
			description="This page is for the DM. If something here needs changing, ask them at the table or on Discord."
			eyebrow={<Badge variant="ember">403 · DM only</Badge>}
			flavor="The harbourmaster's ledgers are not for every eye."
			icon={ShieldAlertIcon}
			title="Only the DM can open this page"
			tone="ember"
		/>
	);
}

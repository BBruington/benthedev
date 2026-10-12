import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Outlet, redirect, useNavigate } from "@tanstack/react-router";
import { useEffect, useRef } from "react";
import { campaignMeQuery } from "@/api/queries";
import { useTimelineToasts } from "@/colony/live-toasts";
import { FullScreenError } from "@/components/error-screens";
import { BottomTabs } from "@/components/shell/bottom-tabs";
import { isLeavingCampaign } from "@/components/shell/leave-campaign-dialog";
import { Sidebar } from "@/components/shell/sidebar";
import { TopBar } from "@/components/shell/top-bar";
import { useCampaign, usePendingCount } from "@/hooks/use-campaign";
import { useAccount, useMe } from "@/hooks/use-me";
import { useShellHeaderHeight } from "@/hooks/use-shell-header-height";
import { isLetIn } from "@/lib/access";

// A campaign's screens (docs/design/multi-campaign.md §5.1): only for a member the DM has let in, in
// a campaign that isn't paused. Everyone else goes to the campaign's waiting page, which says why.
export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell")({
	beforeLoad: async ({ context, params }) => {
		if (!context.campaignId) {
			return;
		}
		const me = await context.queryClient.ensureQueryData(campaignMeQuery(context.campaignId));
		if (!isLetIn(me)) {
			throw redirect({
				params: { campaignSlug: params.campaignSlug },
				replace: true,
				to: "/c/$campaignSlug/pending",
			});
		}
	},
	component: CampaignShell,
	errorComponent: FullScreenError,
});

function CampaignShell() {
	const account = useAccount();
	const me = useMe();
	const { campaignId, slug } = useCampaign();
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const headerRef = useRef<HTMLElement>(null);
	const pendingCount = usePendingCount(me);
	const letIn = isLetIn(me);
	useTimelineToasts(campaignId);
	useShellHeaderHeight(headerRef);

	// Removed, or the campaign paused, while the tab is open: the live "me" topic brings the news. A
	// leave from this tab goes to the picker instead, which the leave dialog navigates to itself.
	useEffect(() => {
		if (letIn || isLeavingCampaign(queryClient, campaignId)) {
			return;
		}
		navigate({ params: { campaignSlug: slug }, replace: true, to: "/c/$campaignSlug/pending" });
	}, [campaignId, letIn, navigate, queryClient, slug]);

	return (
		<div className="min-h-dvh">
			<a
				className="sr-only z-50 rounded-md bg-primary px-4 py-2 text-primary-foreground focus:not-sr-only focus:fixed focus:top-3 focus:left-3"
				href="#main"
			>
				Skip to content
			</a>
			<TopBar account={account} me={me} ref={headerRef} />
			<div className="md:flex">
				<Sidebar className="hidden md:block" me={me} pendingCount={pendingCount} />
				<main
					className="min-w-0 flex-1 px-4 pt-6 pb-[calc(6.5rem+env(safe-area-inset-bottom))] outline-none md:px-8 md:pt-10 md:pb-16 lg:px-12"
					id="main"
					tabIndex={-1}
				>
					<div className="mx-auto w-full max-w-5xl">
						<Outlet />
					</div>
				</main>
			</div>
			<BottomTabs className="md:hidden" me={me} pendingCount={pendingCount} />
		</div>
	);
}

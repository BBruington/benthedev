import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { InboxIcon } from "lucide-react";
import type { Ref } from "react";
import { myCampaignsQuery } from "@/api/queries";
import { Crest } from "@/components/crest";
import { LiveDot } from "@/components/live-dot";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { fallbackCampaignName, useCampaign, useColonyNames } from "@/hooks/use-campaign";
import { type AccountMe, canUseDmTools, type Me } from "@/lib/access";
import { CampaignSwitcher } from "./campaign-switcher";
import { ProfileMenu } from "./profile-menu";
import { ViewAsPlayerBanner, ViewAsPlayerToggle } from "./view-as-player";

export function TopBar({
	account,
	me,
	ref,
}: {
	account: AccountMe;
	me: Me;
	ref: Ref<HTMLElement>;
}) {
	// The colony's name heads every screen (spec §6.1); every member can read it.
	const colonyName = useColonyNames()?.colonyName || fallbackCampaignName;
	const { slug } = useCampaign();
	const campaigns = useQuery(myCampaignsQuery).data?.campaigns ?? [];
	return (
		<header
			className="sticky top-0 z-40 border-border border-b bg-bg/88 pt-[env(safe-area-inset-top)] backdrop-blur-md supports-[backdrop-filter]:bg-bg/72"
			ref={ref}
		>
			<div className="flex h-14 items-center gap-1 pr-1.5 pl-3 md:h-16 md:gap-2 md:pr-4 md:pl-5">
				{campaigns.length > 1 ? (
					<div className="-ml-1 flex min-w-0 items-center gap-1">
						<Link
							aria-label={`${colonyName}: the colony board`}
							className="shrink-0 rounded-md p-1 outline-none focus-visible:ring-[3px] focus-visible:ring-ring/45"
							params={{ campaignSlug: slug }}
							to="/c/$campaignSlug"
						>
							<Crest className="size-7 text-gilt sm:size-8 md:size-9" />
						</Link>
						<CampaignSwitcher campaigns={campaigns} currentSlug={slug} title={colonyName} />
					</div>
				) : (
					<Link
						className="-ml-1 flex min-w-0 items-center gap-2.5 rounded-md px-1 py-1 outline-none focus-visible:ring-[3px] focus-visible:ring-ring/45"
						params={{ campaignSlug: slug }}
						to="/c/$campaignSlug"
					>
						<Crest className="size-7 text-gilt sm:size-8 md:size-9" />
						<span className="truncate font-display font-semibold text-base text-gilt-ink tracking-wide sm:text-lg md:text-xl">
							{colonyName}
						</span>
					</Link>
				)}
				<LiveDot className="ml-1 shrink-0 px-1.5 sm:ml-2" labelClassName="sr-only sm:not-sr-only" />
				<div className="ml-auto flex shrink-0 items-center gap-0.5 md:gap-1">
					{canUseDmTools(me) ? <RequestsButton slug={slug} /> : null}
					{me.isDm ? <ViewAsPlayerToggle me={me} /> : null}
					<ProfileMenu account={account} me={me} />
				</div>
			</div>
			{me.viewAsPlayer ? <ViewAsPlayerBanner /> : null}
		</header>
	);
}

/** The DM's way into the request queue; the count arrives with requests in M3. */
function RequestsButton({ slug }: { slug: string }) {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Button asChild size="icon" variant="ghost">
					<Link
						aria-label="Requests"
						params={{ campaignSlug: slug }}
						to="/c/$campaignSlug/requests"
					>
						<InboxIcon />
					</Link>
				</Button>
			</TooltipTrigger>
			<TooltipContent>Requests: patron asks arrive here in M3</TooltipContent>
		</Tooltip>
	);
}

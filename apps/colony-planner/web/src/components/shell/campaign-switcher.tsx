import { Link } from "@tanstack/react-router";
import { CheckIcon, ChevronsUpDownIcon, LayoutGridIcon } from "lucide-react";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { campaignCardLine, type MyCampaign, sortCampaigns } from "@/lib/campaigns";

/**
 * The top bar's title when the caller has more than one campaign (docs/design/multi-campaign.md
 * §5.4): it names the colony on screen and opens a list of every campaign, each with its own stream
 * and caches, so switching is just following a link.
 */
export function CampaignSwitcher({
	title,
	currentSlug,
	campaigns,
}: {
	title: string;
	currentSlug: string;
	campaigns: readonly MyCampaign[];
}) {
	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<button
					aria-label={`${title}: switch campaign`}
					className="flex min-h-11 min-w-0 items-center gap-1.5 rounded-md px-1.5 outline-none transition-colors hover:bg-accent focus-visible:ring-[3px] focus-visible:ring-ring/45"
					type="button"
				>
					<span className="truncate font-display font-semibold text-base text-gilt-ink tracking-wide sm:text-lg md:text-xl">
						{title}
					</span>
					<ChevronsUpDownIcon
						aria-hidden="true"
						className="size-4 shrink-0 text-muted-foreground"
					/>
				</button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="start" className="w-[min(20rem,calc(100vw-2rem))]">
				<DropdownMenuLabel>Your campaigns</DropdownMenuLabel>
				{sortCampaigns(campaigns).map((membership) => {
					const isCurrent = membership.campaign.slug === currentSlug;
					return (
						<DropdownMenuItem asChild key={membership.campaign.id}>
							<Link
								aria-current={isCurrent ? "page" : undefined}
								className="min-h-12"
								params={{ campaignSlug: membership.campaign.slug }}
								to="/c/$campaignSlug"
							>
								<span className="min-w-0 flex-1">
									<span className="block truncate font-medium">{membership.campaign.name}</span>
									<span className="block truncate text-muted-foreground text-xs">
										{campaignCardLine(membership)}
									</span>
								</span>
								{isCurrent ? <CheckIcon aria-hidden="true" className="text-gilt-ink" /> : null}
							</Link>
						</DropdownMenuItem>
					);
				})}
				<DropdownMenuSeparator />
				<DropdownMenuItem asChild>
					<Link to="/">
						<LayoutGridIcon />
						All campaigns
					</Link>
				</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
	);
}

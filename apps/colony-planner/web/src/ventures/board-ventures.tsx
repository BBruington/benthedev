import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowRightIcon, CrownIcon, InboxIcon } from "lucide-react";
import type { Schemas } from "@/api/client";
import { requestsQuery } from "@/api/queries";
import { Button } from "@/components/ui/button";
import { useCampaign } from "@/hooks/use-campaign";
import { formatNumber } from "@/lib/colony";
import { pendingRequestsQuery } from "./use-request-alerts";
import { RequestCard } from "./venture-parts";

type ColonyResource = Schemas["ColonyResource"];

const boardRequestLimit = 5;

/** The board's pending requests for the DM (spec §6.2), each one tap from its approval sheet. */
export function PendingRequestsCard({ resources }: { resources: readonly ColonyResource[] }) {
	const { campaignId, slug } = useCampaign();
	const pending = useQuery(pendingRequestsQuery(campaignId));
	const requests = pending.data?.requests ?? [];
	if (requests.length === 0) {
		return null;
	}
	return (
		<section
			aria-labelledby="board-requests-heading"
			className="flex flex-col gap-4 rounded-xl border border-gilt/40 bg-card p-4 shadow-card sm:p-5"
		>
			<div className="flex items-center justify-between gap-3">
				<h2
					className="flex items-center gap-2 font-display font-semibold text-xl tracking-wide"
					id="board-requests-heading"
				>
					<InboxIcon aria-hidden="true" className="size-5 text-gilt-ink" />
					Requests waiting
					<span className="font-sans font-normal text-base text-muted-foreground">
						{formatNumber(requests.length)}
					</span>
				</h2>
				<Button asChild size="sm" variant="ghost">
					<Link params={{ campaignSlug: slug }} to="/c/$campaignSlug/requests">
						All requests
						<ArrowRightIcon aria-hidden="true" />
					</Link>
				</Button>
			</div>
			<ul className="grid gap-2">
				{requests.slice(0, boardRequestLimit).map((request) => (
					<RequestCard
						actions={
							<Button asChild>
								<Link
									params={{ campaignSlug: slug }}
									search={{ review: request.id }}
									to="/c/$campaignSlug/requests"
								>
									Review
								</Link>
							</Button>
						}
						key={request.id}
						request={request}
						resources={resources}
					/>
				))}
			</ul>
		</section>
	);
}

/** "Ask a patron" and the player's own latest requests (spec §6.2). */
export function MyRequestsCard({ resources }: { resources: readonly ColonyResource[] }) {
	const { campaignId, slug } = useCampaign();
	const mine = useQuery(requestsQuery(campaignId, { mine: true }));
	const requests = mine.data?.requests ?? [];
	return (
		<section
			aria-labelledby="board-my-requests-heading"
			className="flex flex-col gap-4 rounded-xl border border-border bg-card p-4 shadow-card sm:p-5"
		>
			<div className="flex flex-wrap items-center justify-between gap-3">
				<h2
					className="flex items-center gap-2 font-display font-semibold text-xl tracking-wide"
					id="board-my-requests-heading"
				>
					<CrownIcon aria-hidden="true" className="size-5 text-gilt-ink" />
					My requests
				</h2>
				<div className="flex flex-wrap gap-2">
					<Button asChild>
						<Link params={{ campaignSlug: slug }} to="/c/$campaignSlug/patrons">
							<CrownIcon />
							Ask a patron
						</Link>
					</Button>
					{requests.length > 0 ? (
						<Button asChild variant="ghost">
							<Link params={{ campaignSlug: slug }} to="/c/$campaignSlug/requests">
								All requests
								<ArrowRightIcon aria-hidden="true" />
							</Link>
						</Button>
					) : null}
				</div>
			</div>
			{requests.length === 0 ? (
				<p className="text-muted-foreground">
					Patrons trade favor for help. Ask one, and the DM decides.
				</p>
			) : (
				<ul className="grid gap-2">
					{requests.slice(0, 3).map((request) => (
						<RequestCard key={request.id} request={request} resources={resources} />
					))}
				</ul>
			)}
		</section>
	);
}

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { CrownIcon, PlusIcon } from "lucide-react";
import type { ReactNode } from "react";
import type { Schemas } from "@/api/client";
import { problemMessage } from "@/api/problem";
import { patronQuery, patronsQuery } from "@/api/queries";
import { PageHeader } from "@/components/page-header";
import { StatusPanel } from "@/components/status-panel";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
	fallbackCampaignName,
	useCampaign,
	useCampaignId,
	useCampaignName,
} from "@/hooks/use-campaign";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { useMe } from "@/hooks/use-me";
import { useSheetState } from "@/hooks/use-sheet-state";
import { canUseDmTools } from "@/lib/access";
import { focusFirstVisible } from "@/lib/focus";
import { cn } from "@/lib/utils";
import { HiddenBadge } from "@/people/people-sections";
import { patronHeadingId, patronsPageHeadingId } from "@/ventures/ids";
import { PatronSheet } from "@/ventures/patron-sheets";
import { ArchivedBadge, FavorCount } from "@/ventures/venture-parts";

type Patron = Schemas["Patron"];

export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell/patrons/")({
	component: PatronsPage,
	loader: ({ context: { campaignId, queryClient } }) => {
		if (campaignId) {
			queryClient.prefetchQuery(patronsQuery(campaignId));
		}
	},
});

/** /patrons (spec §6.2, §5.7): the colony's backers and their favor; players see visible ones. */
function PatronsPage() {
	useDocumentTitle("Patrons", useCampaignName() ?? fallbackCampaignName);
	const me = useMe();
	const isDm = canUseDmTools(me);
	const campaignId = useCampaignId();
	const { slug } = useCampaign();
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const patrons = useQuery(patronsQuery(campaignId));
	const adding = useSheetState<true>();

	let body: ReactNode;
	if (patrons.isPending) {
		body = (
			<div aria-busy="true" className="grid gap-2 md:grid-cols-2">
				<Skeleton className="h-24 rounded-xl" />
				<Skeleton className="h-24 rounded-xl" />
			</div>
		);
	} else if (patrons.isError) {
		body = (
			<StatusPanel
				description={problemMessage(patrons.error, "The patrons couldn't be loaded.")}
				icon={CrownIcon}
				title="The patrons couldn't be loaded"
				tone="ember"
			/>
		);
	} else {
		const current = patrons.data.patrons.filter((patron) => !patron.archived);
		const archived = patrons.data.patrons.filter((patron) => patron.archived);
		body = (
			<>
				{current.length === 0 ? (
					<p className="rounded-xl border border-border border-dashed px-4 py-6 text-center text-muted-foreground">
						{isDm ? "No patrons yet. Add the first one." : "The colony has no known patrons yet."}
					</p>
				) : (
					<ul aria-label="Patrons" className="grid gap-2 md:grid-cols-2">
						{current.map((patron) => (
							<PatronCard key={patron.id} patron={patron} />
						))}
					</ul>
				)}
				{archived.length > 0 ? (
					<section aria-labelledby="archived-patrons" className="flex flex-col gap-3">
						<h2
							className="font-display font-semibold text-muted-foreground text-xl tracking-wide"
							id="archived-patrons"
						>
							Archived
						</h2>
						<ul className="grid gap-2 md:grid-cols-2">
							{archived.map((patron) => (
								<PatronCard key={patron.id} patron={patron} />
							))}
						</ul>
					</section>
				) : null}
			</>
		);
	}

	return (
		<div className="flex flex-col gap-6">
			<PageHeader
				actions={
					isDm ? (
						<Button
							onClick={() => {
								adding.show(true);
							}}
						>
							<PlusIcon />
							New patron
						</Button>
					) : null
				}
				description={
					isDm
						? "Everyone the colony can call on. Players see visible patrons and their exact favor."
						: "Who backs the colony, how much favor the party has with them, and what they can be asked for."
				}
				headingId={patronsPageHeadingId}
				title="Patrons"
			/>
			{body}
			{isDm && adding.value ? (
				<PatronSheet
					key={adding.session}
					onCreated={(created) => {
						queryClient.setQueryData(patronQuery(campaignId, created.patron.id).queryKey, created);
						navigate({
							params: { campaignSlug: slug, patronId: created.patron.id },
							to: "/c/$campaignSlug/patrons/$patronId",
						}).then(() => {
							focusFirstVisible(patronHeadingId);
						});
					}}
					onOpenChange={adding.onOpenChange}
					open={adding.open}
					patron={null}
				/>
			) : null}
		</div>
	);
}

function PatronCard({ patron }: { patron: Patron }) {
	const { slug } = useCampaign();
	return (
		<li
			className={cn(
				"relative flex flex-col gap-1.5 rounded-xl border border-border bg-card px-4 py-3 shadow-card transition-colors focus-within:ring-[3px] focus-within:ring-ring/40 hover:border-gilt/40",
				patron.archived && "opacity-75",
			)}
		>
			<div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
				<h3 className="font-semibold text-base leading-snug [overflow-wrap:anywhere]">
					<Link
						className="outline-none after:absolute after:inset-0 after:rounded-xl"
						params={{ campaignSlug: slug, patronId: patron.id }}
						to="/c/$campaignSlug/patrons/$patronId"
					>
						{patron.name}
					</Link>
				</h3>
				<FavorCount favor={patron.favor} />
			</div>
			{patron.description ? (
				<p className="line-clamp-2 text-muted-foreground text-sm">{patron.description}</p>
			) : null}
			<p className="flex flex-wrap gap-1.5">
				<HiddenBadge item={patron} />
				<ArchivedBadge archived={patron.archived} />
			</p>
		</li>
	);
}

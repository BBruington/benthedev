import { keepPreviousData, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { MapPinIcon, PlusIcon, SearchIcon } from "lucide-react";
import { type ReactNode, useState } from "react";
import { problemMessage } from "@/api/problem";
import { placeQuery, placesQuery } from "@/api/queries";
import { PageHeader } from "@/components/page-header";
import { StatusPanel } from "@/components/status-panel";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import {
	fallbackCampaignName,
	useCampaign,
	useCampaignId,
	useCampaignName,
} from "@/hooks/use-campaign";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { useMe } from "@/hooks/use-me";
import { useSheetState } from "@/hooks/use-sheet-state";
import { canUseDmTools } from "@/lib/access";
import { focusFirstVisible } from "@/lib/focus";
import { HiddenBadge } from "@/people/people-sections";
import { placeHeadingId, placesPageHeadingId } from "@/places/place-ids";
import { PlaceSheet } from "@/places/place-sheet";

export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell/places/")({
	component: PlacesPage,
	loader: ({ context: { campaignId, queryClient } }) => {
		if (campaignId) {
			queryClient.prefetchQuery(placesQuery(campaignId));
		}
	},
});

const searchDelayInMilliseconds = 250;

/** /places (spec §6.2): settlements, ruins and landmarks; players get visible ones only. */
function PlacesPage() {
	useDocumentTitle("Places", useCampaignName() ?? fallbackCampaignName);
	const me = useMe();
	const isDm = canUseDmTools(me);
	const campaignId = useCampaignId();
	const { slug } = useCampaign();
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const [search, setSearch] = useState("");
	const settledSearch = useDebouncedValue(search.trim(), searchDelayInMilliseconds);
	const places = useQuery({
		...placesQuery(campaignId, settledSearch),
		placeholderData: keepPreviousData,
	});
	const adding = useSheetState<true>();

	let body: ReactNode;
	if (places.isPending) {
		body = (
			<div aria-busy="true" className="grid gap-2 md:grid-cols-2">
				<Skeleton className="h-24 rounded-xl" />
				<Skeleton className="h-24 rounded-xl" />
			</div>
		);
	} else if (places.isError) {
		body = (
			<StatusPanel
				description={problemMessage(places.error, "The places couldn't be loaded.")}
				icon={MapPinIcon}
				title="The places couldn't be loaded"
				tone="ember"
			/>
		);
	} else if (places.data.places.length === 0) {
		let emptyText = "The party hasn't found anywhere worth noting yet.";
		if (settledSearch !== "") {
			emptyText = "No place matches that name.";
		} else if (isDm) {
			emptyText = "No places yet. Add the first one.";
		}
		body = (
			<p className="rounded-xl border border-border border-dashed px-4 py-6 text-center text-muted-foreground">
				{emptyText}
			</p>
		);
	} else {
		body = (
			<ul aria-label="Places" className="grid gap-2 md:grid-cols-2">
				{places.data.places.map((place) => (
					<li
						className="relative flex items-start gap-3 rounded-xl border border-border bg-card px-4 py-3 shadow-card transition-colors focus-within:ring-[3px] focus-within:ring-ring/40 hover:border-gilt/40"
						key={place.id}
					>
						<span
							aria-hidden="true"
							className="inline-flex size-10 shrink-0 items-center justify-center rounded-lg bg-gilt/12 text-gilt-ink"
						>
							<MapPinIcon className="size-5" />
						</span>
						<div className="min-w-0 flex-1">
							<h2 className="flex flex-wrap items-center gap-2 font-semibold leading-snug [overflow-wrap:anywhere]">
								<Link
									className="outline-none after:absolute after:inset-0 after:rounded-xl"
									params={{ campaignSlug: slug, placeId: place.id }}
									to="/c/$campaignSlug/places/$placeId"
								>
									{place.name}
								</Link>
								<HiddenBadge item={place} />
							</h2>
							{place.description ? (
								<p className="line-clamp-2 text-muted-foreground text-sm">{place.description}</p>
							) : null}
						</div>
					</li>
				))}
			</ul>
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
							Add a place
						</Button>
					) : null
				}
				description={
					isDm
						? "Settlements, ruins and landmarks. Players see the visible ones, without your notes."
						: "Settlements, ruins and landmarks the party has found."
				}
				headingId={placesPageHeadingId}
				title="Places"
			/>
			<search className="relative">
				<Label className="sr-only" htmlFor="place-search">
					Search places
				</Label>
				<SearchIcon
					aria-hidden="true"
					className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground"
				/>
				<Input
					autoComplete="off"
					className="pl-9"
					id="place-search"
					maxLength={80}
					onChange={(event) => {
						setSearch(event.target.value);
					}}
					placeholder="Search by name"
					type="search"
					value={search}
				/>
			</search>
			{body}
			{isDm && adding.value ? (
				<PlaceSheet
					key={adding.session}
					onCreated={(created) => {
						// Seeded, so the new page's heading is there to take focus once the navigation settles.
						queryClient.setQueryData(placeQuery(campaignId, created.id).queryKey, created);
						navigate({
							params: { campaignSlug: slug, placeId: created.id },
							to: "/c/$campaignSlug/places/$placeId",
						}).then(() => {
							focusFirstVisible(placeHeadingId);
						});
					}}
					onOpenChange={adding.onOpenChange}
					open={adding.open}
					place={null}
				/>
			) : null}
		</div>
	);
}

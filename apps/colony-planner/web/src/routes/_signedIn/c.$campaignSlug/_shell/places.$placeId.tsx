import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { ArrowLeftIcon, MapPinIcon, PenLineIcon, Trash2Icon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { problemMessage, problemStatus } from "@/api/problem";
import {
	npcQueryKeyPrefix,
	npcsQueryKeyPrefix,
	placeQuery,
	placeQueryKeyPrefix,
	placesQueryKeyPrefix,
} from "@/api/queries";
import { SwitchRow } from "@/components/form-controls";
import { StatusPanel } from "@/components/status-panel";
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog";
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
import { ProseBlock } from "@/npcs/npc-profile";
import { HiddenBadge, NpcAvatar, NpcNameLink } from "@/people/people-sections";
import { placeHeadingId, placesPageHeadingId } from "@/places/place-ids";
import { PlaceSheet } from "@/places/place-sheet";

type Place = Schemas["Place"];

export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell/places/$placeId")({
	component: PlacePage,
	loader: ({ context: { campaignId, queryClient }, params: { placeId } }) => {
		if (campaignId) {
			queryClient.prefetchQuery(placeQuery(campaignId, placeId));
		}
	},
});

function BackToPlaces() {
	const { slug } = useCampaign();
	return (
		<Button asChild className="self-start" size="sm" variant="ghost">
			<Link params={{ campaignSlug: slug }} to="/c/$campaignSlug/places">
				<ArrowLeftIcon />
				All places
			</Link>
		</Button>
	);
}

/** /places/{placeId} (spec §6.2): one place and who is there; players see the redacted view. */
function PlacePage() {
	const { placeId } = Route.useParams();
	const campaignId = useCampaignId();
	const isDm = canUseDmTools(useMe());
	const place = useQuery(placeQuery(campaignId, placeId));
	useDocumentTitle(place.data?.name ?? "Place", useCampaignName() ?? fallbackCampaignName);

	if (place.isPending) {
		return (
			<div aria-busy="true" className="flex flex-col gap-6">
				<Skeleton className="h-16 w-2/3" />
				<Skeleton className="h-64 rounded-2xl" />
			</div>
		);
	}
	if (place.isError) {
		const isMissing = problemStatus(place.error) === 404;
		return (
			<div className="flex flex-col gap-4">
				<BackToPlaces />
				<StatusPanel
					description={
						isMissing
							? "There's no place at that link. It may have been removed."
							: problemMessage(place.error, "This place couldn't be loaded.")
					}
					icon={MapPinIcon}
					title={isMissing ? "No such place" : "This place couldn't be loaded"}
					tone="ember"
				/>
			</div>
		);
	}
	return <PlaceDetail isDm={isDm} place={place.data} />;
}

function usePlaceSettled() {
	const queryClient = useQueryClient();
	const campaignId = useCampaignId();
	return () =>
		Promise.all([
			queryClient.invalidateQueries({ queryKey: placesQueryKeyPrefix(campaignId) }),
			queryClient.invalidateQueries({ queryKey: placeQueryKeyPrefix(campaignId) }),
			queryClient.invalidateQueries({ queryKey: npcsQueryKeyPrefix(campaignId) }),
			queryClient.invalidateQueries({ queryKey: npcQueryKeyPrefix(campaignId) }),
		]);
}

function PlaceDetail({ place, isDm }: { place: Place; isDm: boolean }) {
	const editing = useSheetState<true>();
	return (
		<div className="flex flex-col gap-6">
			<BackToPlaces />
			<header className="flex flex-wrap items-start gap-4">
				<span
					aria-hidden="true"
					className="inline-flex size-14 shrink-0 items-center justify-center rounded-xl bg-gilt/12 text-gilt-ink"
				>
					<MapPinIcon className="size-7" />
				</span>
				<div className="min-w-0 flex-1 space-y-2">
					<h1
						className="font-display font-semibold text-3xl tracking-wide outline-none [overflow-wrap:anywhere] md:text-4xl"
						id={placeHeadingId}
						tabIndex={-1}
					>
						{place.name}
					</h1>
					<HiddenBadge item={place} />
				</div>
				{isDm ? (
					<div className="flex w-full flex-wrap gap-2 sm:w-auto">
						<Button
							onClick={() => {
								editing.show(true);
							}}
							variant="outline"
						>
							<PenLineIcon />
							Edit
						</Button>
						<DeletePlaceButton place={place} />
					</div>
				) : null}
			</header>
			{isDm && place.dm ? <RevealSwitch place={place} revealed={place.dm.revealed} /> : null}
			{place.description || place.playerNotes || place.dm?.dmNotes ? (
				<section className="grid gap-6 rounded-2xl border border-border bg-card p-5 shadow-card md:p-7">
					<ProseBlock text={place.description} title="Description" />
					<ProseBlock text={place.playerNotes} title="Player notes" />
					{place.dm ? <ProseBlock dmOnly text={place.dm.dmNotes} title="DM notes" /> : null}
				</section>
			) : null}
			<section aria-labelledby="npcs-here-heading" className="flex flex-col gap-3">
				<h2 className="font-display font-semibold text-2xl tracking-wide" id="npcs-here-heading">
					NPCs who are here
				</h2>
				{place.npcsHere.length === 0 ? (
					<p className="rounded-xl border border-border border-dashed px-4 py-6 text-center text-muted-foreground">
						{isDm ? "No NPCs are here." : "Nobody the party knows is here."}
					</p>
				) : (
					<ul className="grid gap-2 md:grid-cols-2">
						{place.npcsHere.map((npc) => (
							<li
								className="flex items-center gap-3 rounded-xl border border-border bg-card px-4 py-3 shadow-card"
								key={npc.id}
							>
								<NpcAvatar displayName={npc.displayName} />
								<div className="min-w-0 flex-1">
									<NpcNameLink npc={npc} />
								</div>
								<HiddenBadge item={npc} />
							</li>
						))}
					</ul>
				)}
			</section>
			{isDm && editing.value ? (
				<PlaceSheet
					key={editing.session}
					onOpenChange={editing.onOpenChange}
					open={editing.open}
					place={place}
				/>
			) : null}
		</div>
	);
}

function RevealSwitch({ place, revealed }: { place: Place; revealed: boolean }) {
	const campaignId = useCampaignId();
	const settle = usePlaceSettled();
	const mutation = api.useMutation("put", "/api/campaigns/{campaignId}/places/{placeId}/revealed", {
		onError: (error) => {
			toast.error(problemMessage(error, "Its visibility couldn't be changed."));
		},
		// Returned, so the mutation stays pending until the refetch lands and the switch never
		// snaps back to the old value in between.
		onSettled: settle,
		onSuccess: (saved) => {
			toast.success(
				saved.dm?.revealed
					? `Players can now see ${saved.name}.`
					: `${saved.name} is hidden from players.`,
			);
		},
	});
	const shownRevealed = mutation.isPending
		? (mutation.variables?.body?.revealed ?? revealed)
		: revealed;
	return (
		<SwitchRow
			checked={shownRevealed}
			description={
				shownRevealed
					? "Players see it, without your notes."
					: "Hidden: players can't find it, and NPCs here don't mention it."
			}
			disabled={mutation.isPending}
			id="place-visible"
			label="Visible to players"
			onCheckedChange={(checked) => {
				mutation.mutate({
					body: { revealed: checked },
					params: { path: { campaignId, placeId: place.id } },
				});
			}}
		/>
	);
}

function DeletePlaceButton({ place }: { place: Place }) {
	const campaignId = useCampaignId();
	const { slug } = useCampaign();
	const navigate = useNavigate();
	const settle = usePlaceSettled();
	const [open, setOpen] = useState(false);
	const [confirmed, setConfirmed] = useState(false);
	const mutation = api.useMutation("delete", "/api/campaigns/{campaignId}/places/{placeId}", {
		onError: (error) => {
			toast.error(problemMessage(error, `${place.name} couldn't be deleted.`));
			document.getElementById(placeHeadingId)?.focus();
		},
		onSettled: settle,
		onSuccess: () => {
			toast.success(`${place.name} was deleted.`);
			navigate({ params: { campaignSlug: slug }, to: "/c/$campaignSlug/places" }).then(() => {
				focusFirstVisible(placesPageHeadingId);
			});
		},
	});
	return (
		<AlertDialog onOpenChange={setOpen} open={open}>
			<Button
				onClick={() => {
					setConfirmed(false);
					setOpen(true);
				}}
				variant="ghost-destructive"
			>
				<Trash2Icon />
				Delete
			</Button>
			<AlertDialogContent
				onCloseAutoFocus={(event) => {
					if (confirmed) {
						event.preventDefault();
					}
				}}
			>
				<AlertDialogHeader>
					<AlertDialogTitle>Delete {place.name}?</AlertDialogTitle>
					<AlertDialogDescription>
						It's gone for good. NPCs who were here or came from here keep their records, with no
						place set.
					</AlertDialogDescription>
				</AlertDialogHeader>
				<AlertDialogFooter>
					<AlertDialogCancel>Keep it</AlertDialogCancel>
					<AlertDialogAction
						onClick={() => {
							setConfirmed(true);
							mutation.mutate({ params: { path: { campaignId, placeId: place.id } } });
						}}
						variant="destructive"
					>
						Delete
					</AlertDialogAction>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	);
}

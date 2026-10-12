import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import type { Map as LeafletMap } from "leaflet";
import {
	ArrowLeftIcon,
	CrosshairIcon,
	MapIcon,
	MapPinIcon,
	MapPinOffIcon,
	PenLineIcon,
	StarIcon,
} from "lucide-react";
import { useRef, useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { problemMessage, problemStatus } from "@/api/problem";
import {
	mapQuery,
	mapQueryKeyPrefix,
	mapsQuery,
	placeQueryKeyPrefix,
	placesQueryKeyPrefix,
} from "@/api/queries";
import { StatusPanel } from "@/components/status-panel";
import { Badge } from "@/components/ui/badge";
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
import { artSource } from "@/maps/art";
import { ImageMap } from "@/maps/image-map";
import { mapHeadingId } from "@/maps/map-ids";
import { RenameMapSheet, useMapsSettled } from "@/maps/map-sheets";
import { PinSheet, PlacePinSheet } from "@/maps/pin-sheets";
import { latLngToPin, type PinPoint } from "@/maps/pins";
import { UploadImageButton } from "@/maps/upload-image-button";
import { HiddenBadge } from "@/people/people-sections";

type WorldMap = Schemas["WorldMap"];
type MapPin = Schemas["MapPin"];

export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell/maps/$mapId")({
	component: MapPage,
	loader: ({ context: { campaignId, queryClient }, params: { mapId } }) => {
		if (campaignId) {
			queryClient.prefetchQuery(mapQuery(campaignId, mapId));
		}
	},
});

function BackToMaps() {
	const { slug } = useCampaign();
	return (
		<Button asChild className="self-start" size="sm" variant="ghost">
			<Link params={{ campaignSlug: slug }} to="/c/$campaignSlug/maps">
				<ArrowLeftIcon />
				All maps
			</Link>
		</Button>
	);
}

/** /maps/{mapId} (spec §6.2): the art with its pins; the DM places, drags and removes them. */
function MapPage() {
	const { mapId } = Route.useParams();
	const campaignId = useCampaignId();
	const isDm = canUseDmTools(useMe());
	const map = useQuery(mapQuery(campaignId, mapId));
	useDocumentTitle(map.data?.name ?? "Map", useCampaignName() ?? fallbackCampaignName);

	if (map.isPending) {
		return (
			<div aria-busy="true" className="flex flex-col gap-6">
				<Skeleton className="h-12 w-2/3" />
				<Skeleton className="h-[60dvh] rounded-2xl" />
			</div>
		);
	}
	if (map.isError) {
		const isMissing = problemStatus(map.error) === 404;
		return (
			<div className="flex flex-col gap-4">
				<BackToMaps />
				<StatusPanel
					description={
						isMissing
							? "There's no map at that link. It may have been removed."
							: problemMessage(map.error, "This map couldn't be loaded.")
					}
					icon={MapIcon}
					title={isMissing ? "No such map" : "This map couldn't be loaded"}
					tone="ember"
				/>
			</div>
		);
	}
	return <MapDetail isDm={isDm} map={map.data} />;
}

function usePinsSettled() {
	const queryClient = useQueryClient();
	const campaignId = useCampaignId();
	return () =>
		Promise.all([
			queryClient.invalidateQueries({ queryKey: mapsQuery(campaignId).queryKey }),
			queryClient.invalidateQueries({ queryKey: mapQueryKeyPrefix(campaignId) }),
			queryClient.invalidateQueries({ queryKey: placesQueryKeyPrefix(campaignId) }),
			queryClient.invalidateQueries({ queryKey: placeQueryKeyPrefix(campaignId) }),
		]);
}

function MapDetail({ map, isDm }: { map: WorldMap; isDm: boolean }) {
	const campaignId = useCampaignId();
	const queryClient = useQueryClient();
	const settlePins = usePinsSettled();
	const settleMaps = useMapsSettled();
	const leafletMap = useRef<LeafletMap>(null);
	const [editingPins, setEditingPins] = useState(false);
	// View as player turns the DM's tools off, edit mode included.
	const isEditing = isDm && editingPins;
	const tappedPin = useSheetState<MapPin>();
	const placing = useSheetState<PinPoint>();
	const renaming = useSheetState<true>();

	const setPin = api.useMutation("put", "/api/campaigns/{campaignId}/places/{placeId}/pin", {
		onSettled: settlePins,
	});
	// A drag saves through its own mutation, so the place-pin sheet never shows a drag's error or save.
	const movePinSave = api.useMutation("put", "/api/campaigns/{campaignId}/places/{placeId}/pin", {
		onSettled: settlePins,
	});
	const clearPin = api.useMutation("delete", "/api/campaigns/{campaignId}/places/{placeId}/pin", {
		onError: (error) => {
			toast.error(problemMessage(error, "The pin couldn't be removed."));
		},
		onSettled: settlePins,
	});
	const replaceArt = api.useMutation("put", "/api/campaigns/{campaignId}/maps/{mapId}/art", {
		onError: (error) => {
			toast.error(problemMessage(error, "The art couldn't be replaced."));
		},
		onSettled: settleMaps,
		onSuccess: (saved) => {
			toast.success(`${saved.name} has new art. Its pins stay where they were.`);
		},
	});

	function movePin(pin: MapPin, point: PinPoint) {
		// Moved in the cache first, so the dropped pin doesn't jump back while the save is on its way.
		queryClient.setQueryData<WorldMap>(mapQuery(campaignId, map.id).queryKey, (current) =>
			current
				? {
						...current,
						pins: current.pins.map((each) =>
							each.placeId === pin.placeId ? { ...each, ...point } : each,
						),
					}
				: current,
		);
		movePinSave.mutate(
			{
				body: { mapId: map.id, ...point },
				params: { path: { campaignId, placeId: pin.placeId } },
			},
			{
				onError: (error) => {
					toast.error(problemMessage(error, `${pin.placeName}'s pin couldn't be moved.`));
				},
			},
		);
	}

	// The pin's marker and row go away with it, so focus lands on the map's heading.
	function removePin(pin: MapPin, from: "sheet" | "list") {
		clearPin.mutate(
			{ params: { path: { campaignId, placeId: pin.placeId } } },
			{
				onSuccess: () => {
					toast.success(`${pin.placeName} is no longer pinned.`);
					if (from === "list") {
						focusFirstVisible(mapHeadingId);
						return;
					}
					tappedPin.onOpenChange(false);
					// A frame later the closing sheet has released its focus trap.
					requestAnimationFrame(() => {
						focusFirstVisible(mapHeadingId);
					});
				},
			},
		);
	}

	function pinAtCentre() {
		const view = leafletMap.current;
		if (view) {
			placing.show(latLngToPin(view.getCenter(), map));
		}
	}

	const sortedPins = [...map.pins].sort((first, second) =>
		first.placeName.localeCompare(second.placeName),
	);
	return (
		<div className="flex flex-col gap-5">
			<BackToMaps />
			<header className="flex flex-wrap items-start gap-4">
				<div className="min-w-0 flex-1 space-y-2">
					<h1
						className="font-display font-semibold text-3xl tracking-wide outline-none [overflow-wrap:anywhere] md:text-4xl"
						id={mapHeadingId}
						tabIndex={-1}
					>
						{map.name}
					</h1>
					<p className="flex flex-wrap gap-1.5 empty:hidden">
						{map.isMainland ? (
							<Badge variant="gilt">
								<StarIcon />
								Mainland
							</Badge>
						) : null}
						<HiddenBadge item={map} />
					</p>
				</div>
				{isDm ? (
					<div className="flex w-full flex-wrap items-start gap-2 sm:w-auto">
						<Button
							aria-pressed={editingPins}
							onClick={() => {
								setEditingPins((current) => !current);
							}}
							variant={editingPins ? "default" : "outline"}
						>
							<MapPinIcon />
							{editingPins ? "Done editing pins" : "Edit pins"}
						</Button>
						<UploadImageButton
							busy={replaceArt.isPending}
							label="Replace art"
							onUploaded={(image) => {
								replaceArt.mutate({
									body: { externalUrl: "", height: 0, imageId: image.id, width: 0 },
									params: { path: { campaignId, mapId: map.id } },
								});
							}}
						/>
						<Button
							onClick={() => {
								renaming.show(true);
							}}
							variant="outline"
						>
							<PenLineIcon />
							Rename
						</Button>
					</div>
				) : null}
			</header>
			{isEditing ? (
				<div className="flex flex-col gap-3 rounded-xl border border-gilt/40 bg-gilt/8 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
					<p className="text-sm">
						Tap the map to pin a place there. Drag a pin to move it, or tap it to remove it.
					</p>
					<Button className="shrink-0" onClick={pinAtCentre} size="sm" variant="outline">
						<CrosshairIcon />
						Pin a place at the centre
					</Button>
				</div>
			) : null}
			<div className="h-[62dvh] min-h-72 overflow-hidden rounded-2xl border border-border shadow-card">
				<ImageMap
					artUrl={artSource(campaignId, map)}
					editing={isEditing}
					height={map.height}
					label={`${map.name} map`}
					mapRef={leafletMap}
					onArtTap={(point) => {
						placing.show(point);
					}}
					onPinActivate={(pin) => {
						tappedPin.show(pin);
					}}
					onPinMoved={movePin}
					pins={map.pins}
					width={map.width}
				/>
			</div>
			<section aria-labelledby="map-places-heading" className="flex flex-col gap-3">
				<h2 className="font-display font-semibold text-2xl tracking-wide" id="map-places-heading">
					Places on this map
				</h2>
				{sortedPins.length === 0 ? (
					<p className="rounded-xl border border-border border-dashed px-4 py-6 text-center text-muted-foreground">
						{isDm
							? "No pins yet. Choose Edit pins, then tap the map where a place is."
							: "No places the party knows are pinned here yet."}
					</p>
				) : (
					<ul className="grid gap-2 md:grid-cols-2">
						{sortedPins.map((pin) => (
							<PinnedPlaceRow
								isEditing={isEditing}
								isRemoving={
									clearPin.isPending && clearPin.variables?.params.path.placeId === pin.placeId
								}
								key={pin.placeId}
								onRemove={() => {
									removePin(pin, "list");
								}}
								pin={pin}
							/>
						))}
					</ul>
				)}
			</section>
			{tappedPin.value ? (
				<PinSheet
					isRemoving={clearPin.isPending}
					key={tappedPin.session}
					onOpenChange={tappedPin.onOpenChange}
					onRemove={
						isEditing && tappedPin.value
							? () => {
									if (tappedPin.value) {
										removePin(tappedPin.value, "sheet");
									}
								}
							: undefined
					}
					open={tappedPin.open}
					pin={tappedPin.value}
				/>
			) : null}
			{isEditing && placing.value ? (
				<PlacePinSheet
					error={setPin.error ? problemMessage(setPin.error, "The pin couldn't be placed.") : null}
					isSaving={setPin.isPending}
					key={placing.session}
					mapId={map.id}
					onOpenChange={(open) => {
						placing.onOpenChange(open);
						setPin.reset();
					}}
					onPin={(placeId) => {
						const point = placing.value;
						if (!point) {
							return;
						}
						setPin.mutate(
							{
								body: { mapId: map.id, ...point },
								params: { path: { campaignId, placeId } },
							},
							{
								onSuccess: (place) => {
									toast.success(`${place.name} is pinned on ${map.name}.`);
									placing.onOpenChange(false);
								},
							},
						);
					}}
					open={placing.open}
				/>
			) : null}
			{isDm && renaming.value ? (
				<RenameMapSheet
					key={renaming.session}
					map={map}
					onOpenChange={renaming.onOpenChange}
					open={renaming.open}
				/>
			) : null}
		</div>
	);
}

function PinnedPlaceRow({
	pin,
	isEditing,
	isRemoving,
	onRemove,
}: {
	pin: MapPin;
	isEditing: boolean;
	isRemoving: boolean;
	onRemove: () => void;
}) {
	const { slug } = useCampaign();
	return (
		<li className="flex items-center gap-3 rounded-xl border border-border bg-card px-4 py-2 shadow-card">
			<MapPinIcon aria-hidden="true" className="size-5 shrink-0 text-gilt-ink" />
			<Link
				className="min-h-11 min-w-0 flex-1 content-center font-semibold [overflow-wrap:anywhere] hover:underline"
				params={{ campaignSlug: slug, placeId: pin.placeId }}
				to="/c/$campaignSlug/places/$placeId"
			>
				{pin.placeName}
			</Link>
			{isEditing ? (
				<Button
					aria-label={`Remove ${pin.placeName}'s pin`}
					disabled={isRemoving}
					onClick={onRemove}
					size="icon"
					variant="ghost-destructive"
				>
					<MapPinOffIcon />
				</Button>
			) : null}
		</li>
	);
}

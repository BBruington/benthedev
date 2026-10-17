import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { ArrowDownIcon, ArrowUpIcon, MapIcon, PlusIcon, Trash2Icon } from "lucide-react";
import { type ReactNode, useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { problemMessage, problemStatus } from "@/api/problem";
import { mapQuery, mapsQuery } from "@/api/queries";
import { SwitchRow } from "@/components/form-controls";
import { PageHeader } from "@/components/page-header";
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
import { artSource } from "@/maps/art";
import { MapCard } from "@/maps/map-card";
import { mapHeadingId, mapsPageHeadingId } from "@/maps/map-ids";
import { NewMapSheet, useMapsSettled } from "@/maps/map-sheets";

type WorldMap = Schemas["WorldMap"];

export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell/maps/")({
	component: MapsPage,
	loader: ({ context: { campaignId, queryClient } }) => {
		if (campaignId) {
			queryClient.prefetchQuery(mapsQuery(campaignId));
		}
	},
});

/** /maps (spec §6.2, §4.4): the Mainland and any other maps; players get the visible ones. */
function MapsPage() {
	useDocumentTitle("Maps", useCampaignName() ?? fallbackCampaignName);
	const isDm = canUseDmTools(useMe());
	const campaignId = useCampaignId();
	const { slug } = useCampaign();
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const maps = useQuery(mapsQuery(campaignId));
	const adding = useSheetState<true>();

	let body: ReactNode;
	if (maps.isPending) {
		body = (
			<div aria-busy="true" className="grid gap-4 md:grid-cols-2">
				<Skeleton className="aspect-video rounded-2xl" />
				<Skeleton className="aspect-video rounded-2xl" />
			</div>
		);
	} else if (maps.isError) {
		body = (
			<StatusPanel
				description={problemMessage(maps.error, "The maps couldn't be loaded.")}
				icon={MapIcon}
				title="The maps couldn't be loaded"
				tone="ember"
			/>
		);
	} else if (maps.data.maps.length === 0) {
		body = (
			<p className="rounded-xl border border-border border-dashed px-4 py-6 text-center text-muted-foreground">
				{isDm
					? "No maps yet. Add your Mainland by uploading its art."
					: "The DM hasn't shared a map yet."}
			</p>
		);
	} else {
		const allMaps = maps.data.maps;
		body = (
			<ul aria-label="Maps" className="grid gap-4 md:grid-cols-2">
				{allMaps.map((map, index) => (
					<MapCard artUrl={artSource(campaignId, map)} key={map.id} map={map}>
						{isDm && map.dm ? (
							<>
								<MainlandSwitch map={map} />
								<RevealSwitch map={map} revealed={map.dm.revealed} />
								<div className="flex flex-wrap items-center gap-2">
									<MoveButtons allMaps={allMaps} index={index} />
									<DeleteMapButton map={map} />
								</div>
							</>
						) : null}
					</MapCard>
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
							Add a map
						</Button>
					) : null
				}
				description={
					isDm
						? "Upload your own art for the Mainland and any other maps, then pin your places on them. Players see the visible ones."
						: "The Mainland the ships come from, and any other maps the DM has shared."
				}
				headingId={mapsPageHeadingId}
				title="Maps"
			/>
			{body}
			{isDm && adding.value ? (
				<NewMapSheet
					hasMainland={maps.data?.maps.some((map) => map.isMainland) ?? false}
					key={adding.session}
					onCreated={(created) => {
						// Seeded, so the new page's heading is there to take focus once the navigation settles.
						queryClient.setQueryData(mapQuery(campaignId, created.id).queryKey, created);
						navigate({
							params: { campaignSlug: slug, mapId: created.id },
							to: "/c/$campaignSlug/maps/$mapId",
						}).then(() => {
							focusFirstVisible(mapHeadingId);
						});
					}}
					onOpenChange={adding.onOpenChange}
					open={adding.open}
				/>
			) : null}
		</div>
	);
}

function MainlandSwitch({ map }: { map: WorldMap }) {
	const campaignId = useCampaignId();
	const settle = useMapsSettled();
	const mutation = api.useMutation("put", "/api/campaigns/{campaignId}/maps/{mapId}/mainland", {
		onError: (error) => {
			toast.error(problemMessage(error, "The Mainland couldn't be changed."));
		},
		// Returned, so the switch stays on its new value until the refetch lands.
		onSettled: settle,
		onSuccess: (saved) => {
			toast.success(
				saved.isMainland
					? `${saved.name} is now the Mainland.`
					: `${saved.name} isn't the Mainland.`,
			);
		},
	});
	const shownMainland = mutation.isPending
		? (mutation.variables?.body?.mainland ?? map.isMainland)
		: map.isMainland;
	return (
		<SwitchRow
			checked={shownMainland}
			description={
				shownMainland
					? "Where the ships come from."
					: "Turn on to make it the Mainland instead of any other map."
			}
			disabled={mutation.isPending}
			id={`map-mainland-${map.id}`}
			label="Mainland"
			onCheckedChange={(checked) => {
				mutation.mutate({
					body: { mainland: checked },
					params: { path: { campaignId, mapId: map.id } },
				});
			}}
		/>
	);
}

function RevealSwitch({ map, revealed }: { map: WorldMap; revealed: boolean }) {
	const campaignId = useCampaignId();
	const settle = useMapsSettled();
	const mutation = api.useMutation("put", "/api/campaigns/{campaignId}/maps/{mapId}/revealed", {
		onError: (error) => {
			toast.error(problemMessage(error, "Its visibility couldn't be changed."));
		},
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
					? "Players can open it and see pins of visible places."
					: "Hidden: players can't find it or its pins."
			}
			disabled={mutation.isPending}
			id={`map-visible-${map.id}`}
			label="Visible to players"
			onCheckedChange={(checked) => {
				mutation.mutate({
					body: { revealed: checked },
					params: { path: { campaignId, mapId: map.id } },
				});
			}}
		/>
	);
}

function MoveButtons({ allMaps, index }: { allMaps: WorldMap[]; index: number }) {
	const campaignId = useCampaignId();
	const settle = useMapsSettled();
	const map = allMaps[index];
	const mutation = api.useMutation("put", "/api/campaigns/{campaignId}/maps/order", {
		onError: (error) => {
			toast.error(
				problemStatus(error) === 409
					? "The maps changed in the meantime. Try again with the new order."
					: problemMessage(error, "The maps couldn't be reordered."),
			);
		},
		onSettled: settle,
	});
	if (!map) {
		return null;
	}
	const mapId = map.id;
	function move(offset: -1 | 1) {
		const mapIds = allMaps.map((each) => each.id);
		const target = index + offset;
		const swapped = mapIds[target];
		if (swapped === undefined) {
			return;
		}
		mapIds[target] = mapIds[index] ?? swapped;
		mapIds[index] = swapped;
		mutation.mutate(
			{ body: { mapIds }, params: { path: { campaignId } } },
			{
				onSuccess: () => {
					focusMoveButton(mapId, target, allMaps.length, offset);
				},
			},
		);
	}
	return (
		<div className="flex gap-1">
			<Button
				aria-label={`Move ${map.name} up`}
				disabled={index === 0 || mutation.isPending}
				id={`map-move-up-${map.id}`}
				onClick={() => {
					move(-1);
				}}
				size="icon"
				variant="outline"
			>
				<ArrowUpIcon />
			</Button>
			<Button
				aria-label={`Move ${map.name} down`}
				disabled={index === allMaps.length - 1 || mutation.isPending}
				id={`map-move-down-${map.id}`}
				onClick={() => {
					move(1);
				}}
				size="icon"
				variant="outline"
			>
				<ArrowDownIcon />
			</Button>
		</div>
	);
}

// Both buttons disable while a move saves, and the pressed one stays disabled once its map reaches the
// end of the list, so focus lands on whichever of the moved map's buttons still works.
function focusMoveButton(mapId: string, newIndex: number, mapCount: number, offset: -1 | 1) {
	let direction = offset === -1 ? "up" : "down";
	if (newIndex === 0) {
		direction = "down";
	} else if (newIndex === mapCount - 1) {
		direction = "up";
	}
	requestAnimationFrame(() => {
		document.getElementById(`map-move-${direction}-${mapId}`)?.focus();
	});
}

function DeleteMapButton({ map }: { map: WorldMap }) {
	const campaignId = useCampaignId();
	const settle = useMapsSettled();
	const [open, setOpen] = useState(false);
	const [confirmed, setConfirmed] = useState(false);
	const mutation = api.useMutation("delete", "/api/campaigns/{campaignId}/maps/{mapId}", {
		onError: (error) => {
			toast.error(problemMessage(error, `${map.name} couldn't be deleted.`));
			focusFirstVisible(mapsPageHeadingId);
		},
		onSettled: settle,
		onSuccess: () => {
			toast.success(`${map.name} was deleted.`);
			focusFirstVisible(mapsPageHeadingId);
		},
	});
	return (
		<AlertDialog onOpenChange={setOpen} open={open}>
			<Button
				className="ms-auto"
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
					<AlertDialogTitle>Delete {map.name}?</AlertDialogTitle>
					<AlertDialogDescription>
						The map is gone for good. Places pinned on it stay, without a pin.
					</AlertDialogDescription>
				</AlertDialogHeader>
				<AlertDialogFooter>
					<AlertDialogCancel>Keep it</AlertDialogCancel>
					<AlertDialogAction
						onClick={() => {
							setConfirmed(true);
							mutation.mutate({ params: { path: { campaignId, mapId: map.id } } });
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

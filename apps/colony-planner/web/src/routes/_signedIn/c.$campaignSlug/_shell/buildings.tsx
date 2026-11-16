import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { HammerIcon, PlusIcon } from "lucide-react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { problemMessage } from "@/api/problem";
import { colonyBoardQuery, constructionQuery, settingsQuery } from "@/api/queries";
import { AddBuildingDialog } from "@/buildings/add-building-dialog";
import { BlueprintCard, type BlueprintControls } from "@/buildings/blueprint-card";
import { BlueprintSheet } from "@/buildings/blueprint-sheet";
import {
	activeButtonId,
	PastBuildings,
	ProjectCard,
	StandingCard,
} from "@/buildings/building-cards";
import { CrewsSheet, StartSheet } from "@/buildings/crew-sheets";
import { type RefundKind, RefundSheet } from "@/buildings/refund-sheet";
import { BuildingsSection, CollapsibleGroup, RoleAvailability } from "@/buildings/sections";
import { useColonyChanges } from "@/colony/use-colony-changes";
import { PageHeader } from "@/components/page-header";
import { StatusPanel } from "@/components/status-panel";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { fallbackCampaignName, useCampaignId, useCampaignName } from "@/hooks/use-campaign";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { useMe } from "@/hooks/use-me";
import { useSheetState } from "@/hooks/use-sheet-state";
import { cachedMe, canUseDmTools } from "@/lib/access";
import { formatNumber } from "@/lib/colony";
import { groupBuildings, roleLabelOf } from "@/lib/construction";
import { focusFirstVisible } from "@/lib/focus";
import { focusAfterClose } from "@/people/colonist-actions";

type Building = Schemas["Building"];
type BuildingBlueprint = Schemas["BuildingBlueprint"];
type ConstructionPage = Schemas["ConstructionPage"];

export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell/buildings")({
	component: BuildingsScreen,
	loader: ({ context: { campaignId, queryClient } }) => {
		if (!campaignId) {
			return;
		}
		queryClient.prefetchQuery(constructionQuery(campaignId));
		queryClient.prefetchQuery(colonyBoardQuery(campaignId));
		const me = cachedMe(queryClient, campaignId);
		if (me && canUseDmTools(me)) {
			queryClient.prefetchQuery(settingsQuery(campaignId));
		}
	},
});

const pageHeadingId = "buildings-heading";
const constructingHeadingId = "buildings-constructing-heading";
const standingHeadingId = "buildings-standing-heading";
const blueprintsHeadingId = "buildings-blueprints-heading";

/**
 * /buildings: the colony's construction (docs/design/construction.md). Any member starts a building
 * from a blueprint and moves crews; the DM keeps the blueprints, cancels and demolishes.
 */
function BuildingsScreen() {
	useDocumentTitle("Buildings", useCampaignName() ?? fallbackCampaignName);
	const me = useMe();
	const isDm = canUseDmTools(me);
	const campaignId = useCampaignId();
	const queryClient = useQueryClient();
	const construction = useQuery(constructionQuery(campaignId));
	const board = useQuery(colonyBoardQuery(campaignId));
	const settings = useQuery({ ...settingsQuery(campaignId), enabled: isDm });
	const changes = useColonyChanges();
	const startSheet = useSheetState<string>();
	const crewsSheet = useSheetState<string>();
	const refundSheet = useSheetState<{ buildingId: string; kind: RefundKind }>();
	const blueprintSheet = useSheetState<BuildingBlueprint | "new">();
	const addConfirm = useSheetState<BuildingBlueprint>();

	function showBlueprint(saved: BuildingBlueprint) {
		queryClient.setQueryData(
			constructionQuery(campaignId).queryKey,
			(page: ConstructionPage | undefined) =>
				page && {
					...page,
					blueprints: page.blueprints.map((blueprint) =>
						blueprint.id === saved.id ? saved : blueprint,
					),
				},
		);
		return queryClient.invalidateQueries({ queryKey: constructionQuery(campaignId).queryKey });
	}

	const reveal = api.useMutation(
		"put",
		"/api/campaigns/{campaignId}/construction/blueprints/{blueprintId}/revealed",
		{
			onError: (error) => {
				toast.error(problemMessage(error, "Its visibility couldn't be changed."));
			},
			onSuccess: (saved) => {
				toast.success(
					saved.dm?.revealed
						? `${saved.name} is visible to players.`
						: `${saved.name} is hidden from players.`,
				);
				return showBlueprint(saved);
			},
		},
	);
	const archive = api.useMutation(
		"put",
		"/api/campaigns/{campaignId}/construction/blueprints/{blueprintId}/archived",
		{
			onError: (error) => {
				toast.error(problemMessage(error, "It couldn't be archived."));
			},
			onSuccess: async (saved) => {
				toast.success(saved.archived ? `${saved.name} is archived.` : `${saved.name} is back.`);
				await showBlueprint(saved);
				focusFirstVisible(blueprintsHeadingId);
			},
		},
	);

	const blueprintControls: BlueprintControls = {
		onAdd: addConfirm.show,
		onArchive: (blueprint, archived) => {
			archive.mutate({
				body: { archived },
				params: { path: { blueprintId: blueprint.id, campaignId } },
			});
		},
		onEdit: blueprintSheet.show,
		onReveal: (blueprint, revealed) => {
			reveal.mutate({
				body: { revealed },
				params: { path: { blueprintId: blueprint.id, campaignId } },
			});
		},
		pendingArchiveId: archive.isPending
			? (archive.variables?.params.path.blueprintId ?? null)
			: null,
		pendingReveal:
			reveal.isPending && reveal.variables
				? {
						blueprintId: reveal.variables.params.path.blueprintId,
						revealed: reveal.variables.body.revealed,
					}
				: null,
	};

	// One mutation serves every building, so all its buttons wait while any one saves.
	const isSettingActive = changes.idleBuilding.isPending || changes.activateBuilding.isPending;

	function setActive(building: Building, active: boolean, version: number) {
		const mutation = active ? changes.activateBuilding : changes.idleBuilding;
		mutation.mutate(
			{
				body: { expectedVersion: version },
				params: { path: { ...changes.inCampaign.params.path, buildingId: building.id } },
			},
			{
				onError: (error) => {
					toast.error(
						problemMessage(
							error,
							active
								? `The ${building.name} couldn't be set Active.`
								: `The ${building.name} couldn't be set Idle.`,
						),
					);
				},
				// The button is disabled while it saves, which drops focus; the frame waits for it to re-enable.
				onSettled: () => {
					requestAnimationFrame(() => {
						focusFirstVisible(activeButtonId(building), standingHeadingId);
					});
				},
			},
		);
	}

	function addBuilding(blueprint: BuildingBlueprint, version: number) {
		changes.addBuilding.mutate(
			{
				body: { expectedVersion: version },
				params: { path: { ...changes.inCampaign.params.path, blueprintId: blueprint.id } },
			},
			{
				onError: (error) => {
					toast.error(problemMessage(error, `The ${blueprint.name} couldn't be added.`));
				},
			},
		);
	}

	let body: React.ReactNode;
	if (construction.isPending || board.isPending) {
		body = (
			<div aria-busy="true" className="flex flex-col gap-6">
				<Skeleton className="h-28 rounded-xl" />
				<Skeleton className="h-48 rounded-xl" />
				<Skeleton className="h-64 rounded-xl" />
			</div>
		);
	} else if (construction.isError || board.isError) {
		body = (
			<StatusPanel
				description={problemMessage(
					construction.error ?? board.error,
					"Buildings couldn't be loaded.",
				)}
				icon={HammerIcon}
				title="Buildings couldn't be loaded"
				tone="ember"
			/>
		);
	} else {
		const page = construction.data;
		const colony = board.data;
		const roleLabels = [
			...page.roles,
			...colony.roles.map((role) => ({ label: role.label, roleKey: role.key })),
		];
		const labelOf = (roleKey: string) => roleLabelOf(roleKey, roleLabels);
		const groups = groupBuildings(page.buildings);
		const blueprintOf = (blueprintId: string) =>
			isDm ? page.blueprints.find((blueprint) => blueprint.id === blueprintId) : undefined;
		const openBlueprints = page.blueprints.filter((blueprint) => !blueprint.archived);
		const archivedBlueprints = isDm
			? page.blueprints.filter((blueprint) => blueprint.archived)
			: [];
		const startBlueprint = page.blueprints.find((blueprint) => blueprint.id === startSheet.value);
		const crewsBuilding = page.buildings.find((building) => building.id === crewsSheet.value);
		const refundBuilding = page.buildings.find(
			(building) => building.id === refundSheet.value?.buildingId,
		);
		const cardFor = (blueprint: BuildingBlueprint) => (
			<BlueprintCard
				blueprint={blueprint}
				dmControls={isDm ? blueprintControls : null}
				key={blueprint.id}
				labelOf={labelOf}
				onStart={(chosen) => {
					startSheet.show(chosen.id);
				}}
				resources={colony.resources}
			/>
		);

		body = (
			<div className="flex flex-col gap-10">
				<BuildingsSection
					description="Who can work on a building, and who is free to."
					headingId="buildings-roles-heading"
					title="Colonists for construction"
				>
					<RoleAvailability roles={page.roles} />
				</BuildingsSection>

				<BuildingsSection headingId={constructingHeadingId} title="Under construction">
					{groups.constructing.length === 0 ? (
						<p className="text-muted-foreground">
							Nothing is being built. Start a blueprint below.
						</p>
					) : (
						<ul className="grid gap-4 lg:grid-cols-2">
							{groups.constructing.map((building) => (
								<ProjectCard
									blueprint={blueprintOf(building.blueprintId)}
									building={building}
									isDm={isDm}
									key={building.id}
									labelOf={labelOf}
									onCancel={(chosen) => {
										refundSheet.show({ buildingId: chosen.id, kind: "cancel" });
									}}
									onSetCrews={(chosen) => {
										crewsSheet.show(chosen.id);
									}}
								/>
							))}
						</ul>
					)}
				</BuildingsSection>

				<BuildingsSection headingId={standingHeadingId} title="Standing">
					{groups.standing.length === 0 ? (
						<p className="text-muted-foreground">No building is standing.</p>
					) : (
						<ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
							{groups.standing.map((building) => (
								<StandingCard
									blueprint={blueprintOf(building.blueprintId)}
									building={building}
									isDm={isDm}
									isSettingActive={isSettingActive}
									key={building.id}
									onDemolish={(chosen) => {
										refundSheet.show({ buildingId: chosen.id, kind: "demolish" });
									}}
									onSetActive={(chosen, active) => {
										setActive(chosen, active, page.version);
									}}
									resources={colony.resources}
								/>
							))}
						</ul>
					)}
					{groups.past.length > 0 ? (
						<CollapsibleGroup label={`Past (${formatNumber(groups.past.length)})`}>
							<PastBuildings buildings={groups.past} resources={colony.resources} />
						</CollapsibleGroup>
					) : null}
				</BuildingsSection>

				<BuildingsSection
					actions={
						isDm ? (
							<Button
								disabled={settings.isPending}
								onClick={() => {
									blueprintSheet.show("new");
								}}
								variant="outline"
							>
								<PlusIcon />
								New blueprint
							</Button>
						) : null
					}
					description="What the colony can build, what each costs and the work it needs."
					headingId={blueprintsHeadingId}
					title="Blueprints"
				>
					{openBlueprints.length === 0 ? (
						<p className="text-muted-foreground">
							{isDm
								? "No blueprints yet. Add one, then make it visible when the party may build it."
								: "No blueprints yet. The DM adds them."}
						</p>
					) : (
						<ul className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
							{openBlueprints.map(cardFor)}
						</ul>
					)}
					{archivedBlueprints.length > 0 ? (
						<CollapsibleGroup label={`Archived (${formatNumber(archivedBlueprints.length)})`}>
							<ul className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
								{archivedBlueprints.map(cardFor)}
							</ul>
						</CollapsibleGroup>
					) : null}
				</BuildingsSection>

				{startBlueprint ? (
					<StartSheet
						blueprint={startBlueprint}
						changes={changes}
						key={`start-${startSheet.session}`}
						labelOf={labelOf}
						onOpenChange={startSheet.onOpenChange}
						onStarted={() => {
							focusAfterClose(constructingHeadingId);
						}}
						open={startSheet.open}
						resources={colony.resources}
						roles={page.roles}
						version={page.version}
					/>
				) : null}
				{crewsBuilding ? (
					<CrewsSheet
						building={crewsBuilding}
						changes={changes}
						key={`crews-${crewsSheet.session}`}
						labelOf={labelOf}
						onOpenChange={crewsSheet.onOpenChange}
						open={crewsSheet.open}
						roles={page.roles}
						version={page.version}
					/>
				) : null}
				{isDm && refundBuilding && refundSheet.value ? (
					<RefundSheet
						building={refundBuilding}
						changes={changes}
						key={`refund-${refundSheet.session}`}
						kind={refundSheet.value.kind}
						onDone={() => {
							focusAfterClose(
								refundSheet.value?.kind === "cancel" ? constructingHeadingId : standingHeadingId,
								pageHeadingId,
							);
						}}
						onOpenChange={refundSheet.onOpenChange}
						open={refundSheet.open}
						resources={colony.resources}
						version={page.version}
					/>
				) : null}
				{isDm && addConfirm.value ? (
					<AddBuildingDialog
						name={addConfirm.value.name}
						onConfirm={() => {
							if (addConfirm.value) {
								addBuilding(addConfirm.value, page.version);
							}
						}}
						onOpenChange={addConfirm.onOpenChange}
						open={addConfirm.open}
					/>
				) : null}
				{isDm && blueprintSheet.value ? (
					<BlueprintSheet
						blueprint={blueprintSheet.value === "new" ? null : blueprintSheet.value}
						constructionRoleKey={settings.data?.constructionRoleKey ?? null}
						key={`blueprint-${blueprintSheet.session}`}
						labelOf={labelOf}
						onOpenChange={blueprintSheet.onOpenChange}
						open={blueprintSheet.open}
						resources={colony.resources}
						roles={colony.roles}
					/>
				) : null}
			</div>
		);
	}

	return (
		<div className="flex flex-col gap-8">
			<PageHeader
				description="Raise buildings from the colony's blueprints with its materials and colonists."
				headingId={pageHeadingId}
				title="Buildings"
			/>
			{body}
		</div>
	);
}

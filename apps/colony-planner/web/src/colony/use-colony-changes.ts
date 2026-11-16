import { type QueryClient, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { problemMessage, problemStatus } from "@/api/problem";
import {
	adjustPreviewQueryKeyPrefix,
	advancePreviewQueryKeyPrefix,
	approveRequestPreviewQueryKeyPrefix,
	colonyBoardQuery,
	constructionQuery,
	marketQuery,
	npcQueryKeyPrefix,
	npcsQueryKeyPrefix,
	patronActionsQuery,
	patronQueryKeyPrefix,
	patronsQuery,
	payMarketOrderPreviewQueryKeyPrefix,
	questPreviewQueryKeyPrefixes,
	questQueryKeyPrefix,
	questsQueryKeyPrefix,
	recentTimelineQuery,
	requestQueryKeyPrefix,
	requestsQueryKeyPrefix,
	resourcesQuery,
	rolesQuery,
	timelineQueryKeyPrefix,
	workforceQuery,
} from "@/api/queries";
import { useCampaignId } from "@/hooks/use-campaign";
import { cachedMe, canUseDmTools } from "@/lib/access";
import { formatNumber, isPublicEntry } from "@/lib/colony";
import { markAnnounced } from "./live-toasts";

type ColonyChange = Schemas["ColonyChange"];

const undoToastDurationInMilliseconds = 12_000;
const goldResourceKey = "gold";

/**
 * Brings this tab up to date after the DM's own change, without waiting for the live stream: the
 * board comes back with the change, and everything it touched is read again. It settles once the
 * workforce, the roles and the Buildings page are fresh, the versions the next sheet starts from.
 */
function refreshAfter(
	queryClient: QueryClient,
	campaignId: string,
	change: ColonyChange | undefined,
): Promise<unknown> {
	if (change) {
		queryClient.setQueryData(colonyBoardQuery(campaignId).queryKey, change.board);
	} else {
		queryClient.invalidateQueries({ queryKey: colonyBoardQuery(campaignId).queryKey });
	}
	queryClient.invalidateQueries({ queryKey: advancePreviewQueryKeyPrefix(campaignId) });
	queryClient.invalidateQueries({ queryKey: adjustPreviewQueryKeyPrefix(campaignId) });
	queryClient.invalidateQueries({ queryKey: approveRequestPreviewQueryKeyPrefix(campaignId) });
	queryClient.invalidateQueries({ queryKey: payMarketOrderPreviewQueryKeyPrefix(campaignId) });
	queryClient.invalidateQueries({ queryKey: marketQuery(campaignId).queryKey });
	for (const queryKey of questPreviewQueryKeyPrefixes(campaignId)) {
		queryClient.invalidateQueries({ queryKey });
	}
	queryClient.invalidateQueries({ queryKey: timelineQueryKeyPrefix(campaignId) });
	queryClient.invalidateQueries({ queryKey: recentTimelineQuery(campaignId).queryKey });
	queryClient.invalidateQueries({ queryKey: resourcesQuery(campaignId).queryKey });
	queryClient.invalidateQueries({ queryKey: npcsQueryKeyPrefix(campaignId) });
	queryClient.invalidateQueries({ queryKey: npcQueryKeyPrefix(campaignId) });
	queryClient.invalidateQueries({ queryKey: questsQueryKeyPrefix(campaignId) });
	queryClient.invalidateQueries({ queryKey: questQueryKeyPrefix(campaignId) });
	queryClient.invalidateQueries({ queryKey: patronsQuery(campaignId).queryKey });
	queryClient.invalidateQueries({ queryKey: patronQueryKeyPrefix(campaignId) });
	queryClient.invalidateQueries({ queryKey: patronActionsQuery(campaignId).queryKey });
	queryClient.invalidateQueries({ queryKey: requestsQueryKeyPrefix(campaignId) });
	queryClient.invalidateQueries({ queryKey: requestQueryKeyPrefix(campaignId) });
	return Promise.all([
		queryClient.invalidateQueries({ queryKey: workforceQuery(campaignId).queryKey }),
		queryClient.invalidateQueries({ queryKey: constructionQuery(campaignId).queryKey }),
		queryClient.invalidateQueries({ queryKey: rolesQuery(campaignId).queryKey }),
	]);
}

/**
 * The toast for this tab's own change: its public text, and Undo for the DM (spec §6.3). A player's
 * change (paying for the goods ship) has no Undo, since only the DM may undo.
 */
function toastChange(
	change: ColonyChange,
	undo: ((groupId: string) => void) | null,
	title?: string,
	descriptionOverride?: string,
) {
	markAnnounced(change.entries.map((entry) => entry.id));
	const publicEntry = change.entries.find(isPublicEntry);
	let description = "Only you can see it on the timeline.";
	if (publicEntry) {
		description = publicEntry.body || "Everyone sees it on the timeline.";
	}
	description = descriptionOverride ?? description;
	toast.success(title ?? publicEntry?.title ?? change.label, {
		action: undo
			? {
					label: "Undo",
					onClick: () => {
						undo(change.groupId);
					},
				}
			: undefined,
		description,
		duration: undoToastDurationInMilliseconds,
	});
}

/**
 * The toast for a player's own Start or Set crews: those entries are the DM's alone (construction
 * design K17), so the player's copy of the change can't say who sees it.
 */
function toastPlayerBuildingChange(change: ColonyChange) {
	markAnnounced(change.entries.map((entry) => entry.id));
	toast.success(change.label, { description: "Only the DM sees it on the timeline." });
}

/**
 * A building added of a hidden blueprint writes a PUBLIC entry that players see only once the
 * blueprint is revealed, so its toast can't say everyone sees it.
 */
function hiddenBlueprintDescription(
	queryClient: QueryClient,
	campaignId: string,
	blueprintId: string,
): string | undefined {
	const page = queryClient.getQueryData<Schemas["ConstructionPage"]>(
		constructionQuery(campaignId).queryKey,
	);
	const blueprint = page?.blueprints.find((candidate) => candidate.id === blueprintId);
	return blueprint?.dm?.revealed === false
		? "Players see it on the timeline once its blueprint is visible."
		: undefined;
}

/** "40 gold added to the colony.", from the deposit's own entry. */
function depositToastTitle(change: ColonyChange): string | undefined {
	const deposited = change.entries.find((entry) => entry.kind === "COIN_DEPOSITED");
	const gold = deposited?.deltas.resources?.[goldResourceKey];
	return gold === undefined ? undefined : `${formatNumber(gold)} gold added to the colony.`;
}

/** True for the 409 the write path answers when the colony moved on (spec §5.1). */
export function isConflict(error: unknown): boolean {
	return problemStatus(error) === 409;
}

/**
 * Every colony write the DM makes from the board, the timeline, /people, an NPC's page, a quest, a
 * patron, the approval sheet and the Foundry inbox. The callbacks live on the
 * mutations themselves, so a toast's Undo still works after the screen that made the change closes.
 * Each screen adds its own callbacks when it calls mutate (closing a sheet, moving focus).
 */
export function useColonyChanges() {
	const queryClient = useQueryClient();
	const campaignId = useCampaignId();
	const inCampaign = { params: { path: { campaignId } } };

	const undo = api.useMutation("post", "/api/campaigns/{campaignId}/colony/undo", {
		onError: (error) => {
			refreshAfter(queryClient, campaignId, undefined);
			toast.error(problemMessage(error, "That change couldn't be undone."));
		},
		onSuccess: (change) => {
			refreshAfter(queryClient, campaignId, change);
			markAnnounced(change.entries.map((entry) => entry.id));
			toast.success(change.label);
		},
	});

	function undoGroup(groupId: string) {
		undo.mutate({ body: { groupId }, ...inCampaign });
	}

	const changeOptions = {
		onError: (error: unknown) => {
			if (isConflict(error)) {
				refreshAfter(queryClient, campaignId, undefined);
			}
		},
		// Returning the refresh keeps the mutation pending until it lands, so a screen's own onSuccess
		// (closing its sheet) runs only once the next sheet would open on the new version.
		onSuccess: (change: ColonyChange) => {
			const me = cachedMe(queryClient, campaignId);
			toastChange(change, me && canUseDmTools(me) ? undoGroup : null);
			return refreshAfter(queryClient, campaignId, change);
		},
	};

	const buildingChangeOptions = {
		...changeOptions,
		onSuccess: (change: ColonyChange) => {
			const me = cachedMe(queryClient, campaignId);
			if (me && canUseDmTools(me)) {
				toastChange(change, undoGroup);
			} else {
				toastPlayerBuildingChange(change);
			}
			return refreshAfter(queryClient, campaignId, change);
		},
	};

	return {
		acceptBridgeDeposit: api.useMutation(
			"post",
			"/api/campaigns/{campaignId}/bridge/inbox/{eventId}/accept-deposit",
			{
				...changeOptions,
				onSuccess: (change: ColonyChange) => {
					const me = cachedMe(queryClient, campaignId);
					toastChange(
						change,
						me && canUseDmTools(me) ? undoGroup : null,
						depositToastTitle(change),
					);
					return refreshAfter(queryClient, campaignId, change);
				},
			},
		),
		activateBuilding: api.useMutation(
			"post",
			"/api/campaigns/{campaignId}/construction/buildings/{buildingId}/activate",
			changeOptions,
		),
		addBuilding: api.useMutation(
			"post",
			"/api/campaigns/{campaignId}/construction/blueprints/{blueprintId}/buildings",
			{
				...changeOptions,
				onSuccess: (
					change: ColonyChange,
					variables: { params: { path: { blueprintId: string } } },
				) => {
					toastChange(
						change,
						undoGroup,
						undefined,
						hiddenBlueprintDescription(queryClient, campaignId, variables.params.path.blueprintId),
					);
					return refreshAfter(queryClient, campaignId, change);
				},
			},
		),
		adjust: api.useMutation("post", "/api/campaigns/{campaignId}/colony/adjust", changeOptions),
		adjustFavor: api.useMutation(
			"post",
			"/api/campaigns/{campaignId}/patrons/{patronId}/adjust-favor",
			changeOptions,
		),
		advance: api.useMutation("post", "/api/campaigns/{campaignId}/colony/advance", changeOptions),
		approveRequest: api.useMutation(
			"post",
			"/api/campaigns/{campaignId}/requests/{requestId}/approve",
			changeOptions,
		),
		cancelBuilding: api.useMutation(
			"post",
			"/api/campaigns/{campaignId}/construction/buildings/{buildingId}/cancel",
			changeOptions,
		),
		changeColonyRole: api.useMutation(
			"post",
			"/api/campaigns/{campaignId}/npcs/{npcId}/change-role",
			changeOptions,
		),
		clearShipments: api.useMutation(
			"post",
			"/api/campaigns/{campaignId}/colony/shipments/clear",
			changeOptions,
		),
		completeQuest: api.useMutation(
			"post",
			"/api/campaigns/{campaignId}/quests/{questId}/complete",
			changeOptions,
		),
		correctDay: api.useMutation(
			"post",
			"/api/campaigns/{campaignId}/colony/correct-day",
			changeOptions,
		),
		demolishBuilding: api.useMutation(
			"post",
			"/api/campaigns/{campaignId}/construction/buildings/{buildingId}/demolish",
			changeOptions,
		),
		failQuest: api.useMutation(
			"post",
			"/api/campaigns/{campaignId}/quests/{questId}/fail",
			changeOptions,
		),
		idleBuilding: api.useMutation(
			"post",
			"/api/campaigns/{campaignId}/construction/buildings/{buildingId}/idle",
			changeOptions,
		),
		/** The path parameters every change sends: the campaign on screen. */
		inCampaign,
		joinColony: api.useMutation(
			"post",
			"/api/campaigns/{campaignId}/npcs/{npcId}/join-colony",
			changeOptions,
		),
		leaveColony: api.useMutation(
			"post",
			"/api/campaigns/{campaignId}/npcs/{npcId}/leave-colony",
			changeOptions,
		),
		payMarketOrder: api.useMutation(
			"post",
			"/api/campaigns/{campaignId}/market/orders/{orderId}/pay",
			changeOptions,
		),
		reopenQuest: api.useMutation(
			"post",
			"/api/campaigns/{campaignId}/quests/{questId}/reopen",
			changeOptions,
		),
		setBuildingCrews: api.useMutation(
			"put",
			"/api/campaigns/{campaignId}/construction/buildings/{buildingId}/crews",
			buildingChangeOptions,
		),
		setRoleCount: api.useMutation(
			"post",
			"/api/campaigns/{campaignId}/people/set-count",
			changeOptions,
		),
		setShipments: api.useMutation(
			"put",
			"/api/campaigns/{campaignId}/colony/shipments",
			changeOptions,
		),
		startBuilding: api.useMutation(
			"post",
			"/api/campaigns/{campaignId}/construction/buildings",
			buildingChangeOptions,
		),
		transferColonists: api.useMutation(
			"post",
			"/api/campaigns/{campaignId}/people/transfer",
			changeOptions,
		),
		undo,
		undoGroup,
		useActionNow: api.useMutation(
			"post",
			"/api/campaigns/{campaignId}/requests/use-action",
			changeOptions,
		),
	};
}

export type ColonyChanges = ReturnType<typeof useColonyChanges>;

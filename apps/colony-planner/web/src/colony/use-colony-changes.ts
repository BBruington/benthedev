import { type QueryClient, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { problemMessage, problemStatus } from "@/api/problem";
import {
	adjustPreviewQueryKeyPrefix,
	advancePreviewQueryKeyPrefix,
	approveRequestPreviewQueryKeyPrefix,
	colonyBoardQuery,
	npcQueryKeyPrefix,
	npcsQueryKeyPrefix,
	patronActionsQuery,
	patronQueryKeyPrefix,
	patronsQuery,
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
import { isPublicEntry } from "@/lib/colony";
import { markAnnounced } from "./live-toasts";

type ColonyChange = Schemas["ColonyChange"];

const undoToastDurationInMilliseconds = 12_000;

/**
 * Brings this tab up to date after the DM's own change, without waiting for the live stream: the
 * board comes back with the change, and everything it touched is read again. It settles once the
 * workforce and the roles are fresh, the version the next workforce sheet starts from.
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
		queryClient.invalidateQueries({ queryKey: rolesQuery(campaignId).queryKey }),
	]);
}

/** The DM's toast for their own change: its public text, and Undo (spec §6.3). */
function toastChange(change: ColonyChange, undo: (groupId: string) => void) {
	markAnnounced(change.entries.map((entry) => entry.id));
	const publicEntry = change.entries.find(isPublicEntry);
	let description = "Only you can see it on the timeline.";
	if (publicEntry) {
		description = publicEntry.body || "Everyone sees it on the timeline.";
	}
	toast.success(publicEntry?.title ?? change.label, {
		action: {
			label: "Undo",
			onClick: () => {
				undo(change.groupId);
			},
		},
		description,
		duration: undoToastDurationInMilliseconds,
	});
}

/** True for the 409 the write path answers when the colony moved on (spec §5.1). */
export function isConflict(error: unknown): boolean {
	return problemStatus(error) === 409;
}

/**
 * Every colony write the DM makes from the board, the timeline, /people, an NPC's page, a quest, a
 * patron and the approval sheet. The callbacks live on the
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
			toastChange(change, undoGroup);
			return refreshAfter(queryClient, campaignId, change);
		},
	};

	return {
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
		failQuest: api.useMutation(
			"post",
			"/api/campaigns/{campaignId}/quests/{questId}/fail",
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
		reopenQuest: api.useMutation(
			"post",
			"/api/campaigns/{campaignId}/quests/{questId}/reopen",
			changeOptions,
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

import { type QueryClient, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { problemMessage, problemStatus } from "@/api/problem";
import {
	adjustPreviewQueryKeyPrefix,
	advancePreviewQueryKeyPrefix,
	colonyBoardQuery,
	recentTimelineQuery,
	resourcesQuery,
	timelineQueryKeyPrefix,
} from "@/api/queries";
import { isPublicEntry } from "@/lib/colony";
import { markAnnounced } from "./live-toasts";

type ColonyChange = Schemas["ColonyChange"];

const undoToastDurationInMilliseconds = 12_000;

/**
 * Brings this tab up to date after the DM's own change, without waiting for the live stream: the
 * board comes back with the change, and everything it touched is read again.
 */
function refreshAfter(queryClient: QueryClient, change: ColonyChange | undefined) {
	if (change) {
		queryClient.setQueryData(colonyBoardQuery.queryKey, change.board);
	} else {
		queryClient.invalidateQueries({ queryKey: colonyBoardQuery.queryKey });
	}
	queryClient.invalidateQueries({ queryKey: advancePreviewQueryKeyPrefix });
	queryClient.invalidateQueries({ queryKey: adjustPreviewQueryKeyPrefix });
	queryClient.invalidateQueries({ queryKey: timelineQueryKeyPrefix });
	queryClient.invalidateQueries({ queryKey: recentTimelineQuery.queryKey });
	queryClient.invalidateQueries({ queryKey: resourcesQuery.queryKey });
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
 * Every colony write the DM makes from the board and the timeline. The callbacks live on the
 * mutations themselves, so a toast's Undo still works after the screen that made the change closes.
 * Each screen adds its own callbacks when it calls mutate (closing a sheet, moving focus).
 */
export function useColonyChanges() {
	const queryClient = useQueryClient();

	const undo = api.useMutation("post", "/api/colony/undo", {
		onError: (error) => {
			refreshAfter(queryClient, undefined);
			toast.error(problemMessage(error, "That change couldn't be undone."));
		},
		onSuccess: (change) => {
			refreshAfter(queryClient, change);
			markAnnounced(change.entries.map((entry) => entry.id));
			toast.success(change.label);
		},
	});

	function undoGroup(groupId: string) {
		undo.mutate({ body: { groupId } });
	}

	const changeOptions = {
		onError: (error: unknown) => {
			if (isConflict(error)) {
				refreshAfter(queryClient, undefined);
			}
		},
		onSuccess: (change: ColonyChange) => {
			refreshAfter(queryClient, change);
			toastChange(change, undoGroup);
		},
	};

	return {
		adjust: api.useMutation("post", "/api/colony/adjust", changeOptions),
		advance: api.useMutation("post", "/api/colony/advance", changeOptions),
		clearShipments: api.useMutation("post", "/api/colony/shipments/clear", changeOptions),
		correctDay: api.useMutation("post", "/api/colony/correct-day", changeOptions),
		setShipments: api.useMutation("put", "/api/colony/shipments", changeOptions),
		undo,
		undoGroup,
	};
}

export type ColonyChanges = ReturnType<typeof useColonyChanges>;

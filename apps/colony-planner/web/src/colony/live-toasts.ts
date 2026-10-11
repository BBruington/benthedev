import { type QueryClient, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { toast } from "sonner";
import type { Schemas } from "@/api/client";
import { recentTimelineQuery } from "@/api/queries";
import { entriesToToast, newestWrittenAt } from "@/lib/colony";

type TimelineEntry = Schemas["TimelineEntry"];

// Entries this tab has already toasted itself: the DM's own change toasts with Undo the moment it
// lands, so the live refetch that follows must not toast the same entries again.
const announcedEntryIds = new Set<string>();

export function markAnnounced(entryIds: Iterable<string>) {
	for (const entryId of entryIds) {
		announcedEntryIds.add(entryId);
	}
}

const mostToastsAtOnce = 3;
const ownChangeRecheckInMilliseconds = 250;
const toastDurationInMilliseconds = 8000;

/** A colony write from this tab that hasn't answered yet: its own toast is on the way. */
function isOwnColonyChangeInFlight(queryClient: QueryClient): boolean {
	return (
		queryClient.isMutating({
			predicate: (mutation) => {
				const path = mutation.options.mutationKey?.[1];
				return typeof path === "string" && path.startsWith("/api/colony");
			},
		}) > 0
	);
}

function showToasts(entries: readonly TimelineEntry[]) {
	const fresh = entries.filter((entry) => !announcedEntryIds.has(entry.id));
	markAnnounced(fresh.map((entry) => entry.id));
	if (fresh.length === 0) {
		return;
	}
	// A long advance can write a dozen entries; the newest few are toasted and the rest summed up.
	const shown = fresh.length > mostToastsAtOnce ? fresh.slice(-(mostToastsAtOnce - 1)) : fresh;
	const hiddenCount = fresh.length - shown.length;
	if (hiddenCount > 0) {
		toast(`${hiddenCount} more entries on the timeline`, {
			duration: toastDurationInMilliseconds,
		});
	}
	for (const entry of shown) {
		toast(entry.title, {
			description: entry.body || undefined,
			duration: toastDurationInMilliseconds,
		});
	}
}

/**
 * Toasts each new PUBLIC timeline entry for everyone at the table (spec §6.3). It watches the recent
 * timeline, which the live stream refetches after every colony change, so a toast is exactly as
 * secret as its entry. Mounted once, in the signed-in layout.
 */
export function useTimelineToasts() {
	const queryClient = useQueryClient();
	const { data } = useQuery(recentTimelineQuery);
	const seenEntryIds = useRef<Set<string> | null>(null);
	const newestSeenAt = useRef<number | null>(null);

	useEffect(() => {
		if (!data) {
			return;
		}
		const newestNow = newestWrittenAt(data.entries);
		if (seenEntryIds.current === null) {
			// What was already there when the tab opened is history, not news.
			seenEntryIds.current = new Set(data.entries.map((entry) => entry.id));
			newestSeenAt.current = newestNow;
			markAnnounced(seenEntryIds.current);
			return;
		}
		const fresh = entriesToToast(data.entries, seenEntryIds.current, newestSeenAt.current);
		for (const entry of data.entries) {
			seenEntryIds.current.add(entry.id);
		}
		if (newestNow !== null && (newestSeenAt.current === null || newestNow > newestSeenAt.current)) {
			newestSeenAt.current = newestNow;
		}
		if (fresh.length === 0) {
			return;
		}
		let timer: ReturnType<typeof setTimeout> | undefined;
		const showWhenSettled = () => {
			if (isOwnColonyChangeInFlight(queryClient)) {
				timer = setTimeout(showWhenSettled, ownChangeRecheckInMilliseconds);
				return;
			}
			showToasts(fresh);
		};
		showWhenSettled();
		return () => {
			clearTimeout(timer);
		};
	}, [data, queryClient]);
}

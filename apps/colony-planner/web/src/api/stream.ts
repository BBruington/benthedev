import { type QueryClient, useQueryClient } from "@tanstack/react-query";
import { useEffect, useSyncExternalStore } from "react";
import { createLiveInvalidator } from "./live-invalidation";
import { campaignMeQuery, meQuery } from "./queries";
import { isInvalidateMessage, queryKeysForTopics } from "./topics";

/**
 * Live updates over GET /api/campaigns/{campaignId}/stream (docs/design/multi-campaign.md §7.1): one
 * stream per campaign, open while a screen inside that campaign is mounted. Messages only name topics;
 * the client turns each topic into that campaign's query keys and lets TanStack Query refetch what is
 * on screen. Pages outside a campaign have no stream.
 */
export type LiveState = "connecting" | "live" | "reconnecting";

const retryDelaysInMilliseconds = [1000, 2000, 5000, 10_000, 30_000];
// The server rotates every stream about every ten minutes and asks the browser to reconnect after
// two seconds; the dot only turns grey if reconnecting takes longer than this.
const reconnectingGraceInMilliseconds = 3500;

let liveState: LiveState = "connecting";
const liveStateListeners = new Set<() => void>();
// Kept per stream URL, outside any one stream, so a stream reopened after the tab was hidden, or after
// moving from the waiting page into the campaign, resumes from the last message it saw instead of
// refetching everything. Stream IDs count one campaign's messages, so they never mix across campaigns.
const lastEventIds = new Map<string, string>();

function setLiveState(nextState: LiveState) {
	if (nextState === liveState) {
		return;
	}
	liveState = nextState;
	for (const listener of liveStateListeners) {
		listener();
	}
}

function subscribeToLiveState(listener: () => void) {
	liveStateListeners.add(listener);
	return () => {
		liveStateListeners.delete(listener);
	};
}

function readLiveState() {
	return liveState;
}

export function useLiveState(): LiveState {
	return useSyncExternalStore(subscribeToLiveState, readLiveState, readLiveState);
}

export function streamPath(campaignId: string): string {
	return `/api/campaigns/${encodeURIComponent(campaignId)}/stream`;
}

export function streamUrl(campaignId: string, resumeFrom: string | undefined): string {
	const path = streamPath(campaignId);
	if (!resumeFrom) {
		return path;
	}
	return `${path}?${new URLSearchParams({ last: resumeFrom }).toString()}`;
}

/**
 * Opens one campaign's stream and keeps it open while the tab is visible; returns a function that
 * stops it.
 */
export function startLiveStream(queryClient: QueryClient, campaignId: string): () => void {
	let source: EventSource | undefined;
	let retryTimer: ReturnType<typeof setTimeout> | undefined;
	let graceTimer: ReturnType<typeof setTimeout> | undefined;
	let failureCount = 0;
	let stopped = false;
	const invalidator = createLiveInvalidator(queryClient);

	function clearTimers() {
		clearTimeout(retryTimer);
		clearTimeout(graceTimer);
		retryTimer = undefined;
		graceTimer = undefined;
	}

	function showReconnectingSoon() {
		if (graceTimer !== undefined) {
			return;
		}
		graceTimer = setTimeout(() => {
			graceTimer = undefined;
			setLiveState("reconnecting");
		}, reconnectingGraceInMilliseconds);
	}

	function open() {
		clearTimeout(retryTimer);
		retryTimer = undefined;
		if (stopped || source || document.hidden) {
			return;
		}
		const eventSource = new EventSource(
			streamUrl(campaignId, lastEventIds.get(streamPath(campaignId))),
		);
		source = eventSource;
		eventSource.addEventListener("open", () => {
			failureCount = 0;
			clearTimeout(graceTimer);
			graceTimer = undefined;
			setLiveState("live");
		});
		eventSource.addEventListener("invalidate", (event) => {
			handleInvalidate(event);
		});
		eventSource.addEventListener("error", () => {
			handleError(eventSource);
		});
	}

	function handleInvalidate(event: MessageEvent<unknown>) {
		if (event.lastEventId) {
			lastEventIds.set(streamPath(campaignId), event.lastEventId);
		}
		let message: unknown;
		try {
			message = JSON.parse(String(event.data));
		} catch {
			return;
		}
		if (!isInvalidateMessage(message)) {
			return;
		}
		const queryKeys = queryKeysForTopics(campaignId, message.topics);
		if (queryKeys === "everything") {
			queryClient.invalidateQueries();
			return;
		}
		for (const queryKey of queryKeys) {
			invalidator.invalidate(queryKey);
		}
	}

	function handleError(eventSource: EventSource) {
		if (eventSource !== source) {
			return;
		}
		if (eventSource.readyState === EventSource.CONNECTING) {
			// The browser is already retrying, with Last-Event-ID.
			showReconnectingSoon();
			return;
		}
		// Closed for good: the server refused the stream (401 once a session ends, or the server is
		// down), and EventSource never retries a refused stream on its own.
		eventSource.close();
		source = undefined;
		failureCount += 1;
		setLiveState("reconnecting");
		if (failureCount === 1) {
			// The common refusals are an ended session (signed out elsewhere) and a campaign that is gone
			// (deleted, 404). Refetching /api/me and this campaign's .../me finds out which; a 401 there
			// signs this tab out.
			queryClient.invalidateQueries({ queryKey: meQuery.queryKey });
			queryClient.invalidateQueries({ queryKey: campaignMeQuery(campaignId).queryKey });
		}
		const delayIndex = Math.min(failureCount, retryDelaysInMilliseconds.length) - 1;
		retryTimer = setTimeout(open, retryDelaysInMilliseconds[delayIndex]);
	}

	function close() {
		source?.close();
		source = undefined;
		clearTimers();
	}

	function handleVisibilityChange() {
		if (document.hidden) {
			close();
			return;
		}
		failureCount = 0;
		open();
	}

	function handleOnline() {
		if (!source) {
			failureCount = 0;
			open();
		}
	}

	document.addEventListener("visibilitychange", handleVisibilityChange);
	window.addEventListener("online", handleOnline);
	open();

	return () => {
		stopped = true;
		close();
		invalidator.stop();
		document.removeEventListener("visibilitychange", handleVisibilityChange);
		window.removeEventListener("online", handleOnline);
		setLiveState("connecting");
	};
}

/** Keeps the campaign's live stream open for as long as the calling screen is mounted. */
export function useLiveUpdates(campaignId: string) {
	const queryClient = useQueryClient();
	useEffect(() => startLiveStream(queryClient, campaignId), [queryClient, campaignId]);
}

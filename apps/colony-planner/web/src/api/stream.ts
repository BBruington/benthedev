import { type QueryClient, useQueryClient } from "@tanstack/react-query";
import { useEffect, useSyncExternalStore } from "react";
import { meQuery } from "./queries";
import { isInvalidateMessage, queryKeysForTopics } from "./topics";

/**
 * Live updates over GET /api/stream (spec §8.8). Messages only name topics; the client turns each
 * topic into query keys and lets TanStack Query refetch what is on screen.
 */
export type LiveState = "connecting" | "live" | "reconnecting";

const streamPath = "/api/stream";
const retryDelaysInMilliseconds = [1000, 2000, 5000, 10_000, 30_000];
// The server rotates every stream about every ten minutes and asks the browser to reconnect after
// two seconds; the dot only turns grey if reconnecting takes longer than this.
const reconnectingGraceInMilliseconds = 3500;

let liveState: LiveState = "connecting";
const liveStateListeners = new Set<() => void>();
// Kept outside any one stream so a stream reopened after the tab was hidden, or after moving from
// /pending into the app, resumes from the last message it saw instead of refetching everything.
let lastEventId: string | undefined;

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

export function streamUrl(resumeFrom: string | undefined): string {
	if (!resumeFrom) {
		return streamPath;
	}
	return `${streamPath}?${new URLSearchParams({ last: resumeFrom }).toString()}`;
}

/** Opens the stream and keeps it open while the tab is visible; returns a function that stops it. */
export function startLiveStream(queryClient: QueryClient): () => void {
	let source: EventSource | undefined;
	let retryTimer: ReturnType<typeof setTimeout> | undefined;
	let graceTimer: ReturnType<typeof setTimeout> | undefined;
	let failureCount = 0;
	let stopped = false;

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
		const eventSource = new EventSource(streamUrl(lastEventId));
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
			lastEventId = event.lastEventId;
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
		const queryKeys = queryKeysForTopics(message.topics);
		if (queryKeys === "everything") {
			queryClient.invalidateQueries();
			return;
		}
		for (const queryKey of queryKeys) {
			queryClient.invalidateQueries({ queryKey });
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
			// The most common refusal is an ended session (removed, or signed out elsewhere). Refetching
			// /api/me finds out; a 401 there signs this tab out.
			queryClient.invalidateQueries({ queryKey: meQuery.queryKey });
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
		document.removeEventListener("visibilitychange", handleVisibilityChange);
		window.removeEventListener("online", handleOnline);
		setLiveState("connecting");
	};
}

/** Keeps the live stream open for as long as the calling screen is mounted. */
export function useLiveUpdates() {
	const queryClient = useQueryClient();
	useEffect(() => startLiveStream(queryClient), [queryClient]);
}

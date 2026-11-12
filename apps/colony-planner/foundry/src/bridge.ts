import { isActiveGm } from "./active-gm";
import type { BridgeAdapter } from "./adapters/adapter";
import { createBridgeClient, type Fetch, UnauthorizedError } from "./bridge-client";
import { createCoalescingTask } from "./coalescing-task";
import { createEventQueue, type EventQueue } from "./event-queue";
import type { FoundryGame, FoundryNotifications } from "./foundry-types";
import { syncOutbox } from "./outbox";
import type { Schedule } from "./schedule";
import { type BridgeSettings, moduleId } from "./settings";
import {
	type ConnectionState,
	type StreamConnection,
	startStreamConnection,
} from "./stream-connection";
import { createTimeTracker, type TimeTracker } from "./time-tracker";
import { maximumStatusFieldLength, type StatusPayload } from "./wire";

/** Why the bridge is or isn't running in this browser, for the settings menu. */
export type BridgeCondition =
	| "running"
	| "turned-off"
	| "no-colony-address"
	| "not-linked"
	| "needs-new-code"
	| "another-gm-is-active";

export interface BridgeStatus {
	campaignName: string;
	condition: BridgeCondition;
	connection: ConnectionState | undefined;
	lastProblem: string | undefined;
	linked: boolean;
}

export interface Bridge {
	/**
	 * Stops the bridge while a pairing code is traded: Colony Planner revokes the old link as it
	 * accepts the code, so the old session's requests must not answer that with "link ended". Call the
	 * returned function when pairing fails; saveLink ends it on success.
	 */
	beginPairing(): () => void;
	/** Starts or stops the bridge to match the settings and who the active GM is. */
	evaluate(): void;
	/** Forgets this browser's token; the link on Colony Planner stays until the DM unlinks it there. */
	forgetLink(): Promise<void>;
	observeWorldTime(worldTime: number, options: unknown): void;
	/** Stores a new link from pairing and connects with it. */
	saveLink(link: { campaignName: string; token: string }): Promise<void>;
	status(): BridgeStatus;
	subscribe(listener: () => void): () => void;
}

interface Session {
	abortController: AbortController;
	connection: StreamConnection;
	queue: EventQueue;
	tracker: TimeTracker;
}

export function createBridge(options: {
	adapter: BridgeAdapter;
	colonyOrigin: string | undefined;
	fetchFunction?: Fetch;
	foundryGame: Pick<
		FoundryGame,
		"modules" | "system" | "time" | "user" | "users" | "version" | "world"
	>;
	notifications: FoundryNotifications;
	schedule: Schedule;
	settings: BridgeSettings;
}): Bridge {
	const { foundryGame, settings, notifications } = options;
	const listeners = new Set<() => void>();
	let session: Session | undefined;
	let connection: ConnectionState | undefined;
	let lastProblem: string | undefined;
	let needsNewCode = false;
	let pairing = false;
	// Every session's outbox syncs run one after another, so a sync still finishing for a stopped
	// session never applies the same rows beside the next session's (two "Colony" entries, a clock
	// moved twice).
	let outboxSyncs: Promise<void> = Promise.resolve();

	function notifyListeners() {
		for (const listener of listeners) {
			listener();
		}
	}

	function recordProblem(error: unknown) {
		lastProblem = error instanceof Error ? error.message : String(error);
		notifyListeners();
	}

	function condition(): BridgeCondition {
		if (!settings.bridgeEnabled()) {
			return "turned-off";
		}
		if (!options.colonyOrigin) {
			return "no-colony-address";
		}
		if (needsNewCode) {
			return "needs-new-code";
		}
		if (!settings.token()) {
			return "not-linked";
		}
		if (!isActiveGm(foundryGame)) {
			return "another-gm-is-active";
		}
		return "running";
	}

	function handleUnauthorized() {
		if (needsNewCode) {
			return;
		}
		needsNewCode = true;
		stopSession();
		notifications.error(
			"Colony Bridge: Colony Planner ended this world's link. Make a new code on Colony Planner's Settings and connect again.",
		);
		settings.saveLink({ campaignName: "", token: "" }).catch(recordProblem);
		notifyListeners();
	}

	function statusPayload(): StatusPayload {
		const clip = (text: string | undefined) => (text ?? "").slice(0, maximumStatusFieldLength);
		return {
			foundryVersion: clip(foundryGame.version),
			moduleVersion: clip(foundryGame.modules.get(moduleId)?.version),
			systemId: clip(foundryGame.system.id),
			systemVersion: clip(foundryGame.system.version),
			worldTitle: clip(foundryGame.world.title),
		};
	}

	function startSession(colonyOrigin: string): Session {
		const abortController = new AbortController();
		const { signal } = abortController;
		const client = createBridgeClient({
			fetchFunction: options.fetchFunction,
			onUnauthorized: handleUnauthorized,
			origin: colonyOrigin,
			signal,
			token: settings.token(),
		});
		const queue = createEventQueue({
			onDropped: (reason) =>
				notifications.warn(
					`Colony Bridge: Colony Planner refused an update from Foundry. ${reason}`,
				),
			onError: recordProblem,
			schedule: options.schedule,
			send: client.sendEvents,
			storage: { load: settings.pendingEvents, save: settings.savePendingEvents },
		});
		const tracker = createTimeTracker({
			load: settings.lastReportedDay,
			onError: recordProblem,
			report: (days) => queue.enqueue("TIME_ADVANCED", { days }),
			save: settings.saveLastReportedDay,
		});
		const requestOutboxSync = createCoalescingTask(
			() => {
				const sync = outboxSyncs.then(() =>
					syncOutbox({
						appliers: options.adapter.appliers,
						client,
						cursor: { load: settings.acknowledgedThrough, save: settings.saveAcknowledgedThrough },
						signal,
					}),
				);
				outboxSyncs = sync.catch(() => undefined);
				return sync;
			},
			(error) => {
				if (signal.aborted || error instanceof UnauthorizedError) {
					return;
				}
				recordProblem(error);
				// A fresh connection syncs again from the cursor, after the reconnect backoff.
				streamConnection.reconnect();
			},
		);
		const streamConnection = startStreamConnection({
			onConnected: () => {
				lastProblem = undefined;
				queue.enqueue("STATUS", statusPayload());
				queue.retryNow();
				// Catches up on clock moves made while this browser wasn't running the bridge.
				tracker.observe(foundryGame.time.worldTime, false);
				requestOutboxSync();
			},
			onOutbox: requestOutboxSync,
			onStateChange: (state) => {
				if (signal.aborted) {
					return;
				}
				connection = state;
				notifyListeners();
			},
			open: client.stream,
			schedule: options.schedule,
		});
		return { abortController, connection: streamConnection, queue, tracker };
	}

	function stopSession() {
		if (!session) {
			return;
		}
		session.abortController.abort();
		session.connection.stop();
		session.queue.stop();
		session = undefined;
		connection = undefined;
	}

	function evaluate() {
		const running = condition() === "running" && !pairing;
		if (running && !session && options.colonyOrigin) {
			session = startSession(options.colonyOrigin);
		} else if (!running && session) {
			stopSession();
		}
		notifyListeners();
	}

	return {
		beginPairing() {
			pairing = true;
			stopSession();
			notifyListeners();
			return () => {
				pairing = false;
				evaluate();
			};
		},
		evaluate,
		async forgetLink() {
			stopSession();
			needsNewCode = false;
			await settings.saveLink({ campaignName: "", token: "" });
			evaluate();
		},
		observeWorldTime(worldTime, updateOptions) {
			const madeByBridge =
				typeof updateOptions === "object" &&
				updateOptions !== null &&
				"colonyBridge" in updateOptions &&
				updateOptions.colonyBridge === true;
			session?.tracker.observe(worldTime, madeByBridge);
		},
		async saveLink(link) {
			stopSession();
			pairing = false;
			needsNewCode = false;
			lastProblem = undefined;
			await settings.saveLink(link);
			evaluate();
		},
		status: () => ({
			campaignName: settings.campaignName(),
			condition: condition(),
			connection,
			lastProblem,
			linked: settings.token() !== "",
		}),
		subscribe(listener) {
			listeners.add(listener);
			return () => listeners.delete(listener);
		},
	};
}

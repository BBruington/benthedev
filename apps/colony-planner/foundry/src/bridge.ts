import { isActiveGm } from "./active-gm";
import type { BridgeAdapter } from "./adapters/adapter";
import { createBridgeClient, type Fetch, UnauthorizedError } from "./bridge-client";
import { createCoalescingTask } from "./coalescing-task";
import type { DepositEvents } from "./deposits";
import { createEventQueue, type EventQueue } from "./event-queue";
import type { FoundryGame, FoundryNotifications, FoundryUsers } from "./foundry-types";
import { syncOutbox, WithdrawFailedError } from "./outbox";
import type { Schedule } from "./schedule";
import { type BridgeSettings, moduleId } from "./settings";
import {
	type ConnectionState,
	type StreamConnection,
	startStreamConnection,
} from "./stream-connection";
import { createTimeTracker, type TimeTracker } from "./time-tracker";
import { type InboundEvent, maximumStatusFieldLength, type StatusPayload } from "./wire";

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
	/** This world's link's random ID, which deposits record; "" while unlinked. */
	linkId(): string;
	/** Queues an event made elsewhere (an import, a deposit); false when the bridge isn't running here. */
	queueEvent(event: InboundEvent): boolean;
	/**
	 * Stores a new link from pairing and connects with it. Its first sync first withdraws the journal
	 * entries earlier links made (design §13.5: the backfill shows the revealed ones again).
	 */
	saveLink(link: { campaignName: string; token: string }): Promise<void>;
	status(): BridgeStatus;
	subscribe(listener: () => void): () => void;
}

/** Web Locks (`navigator.locks`), shared by every tab of one browser. */
export interface OutboxLocks {
	request(name: string, task: () => Promise<void>): Promise<void>;
}

const outboxLockName = "colony-bridge-outbox";
const noOutboxLocks =
	"This browser has no Web Locks, so Colony Bridge can't keep two of its tabs from applying the same Colony Planner change twice, and it reads nothing from Colony Planner. Use a current browser or Foundry's desktop app.";

interface Session {
	abortController: AbortController;
	connection: StreamConnection;
	queue: EventQueue;
	tracker: TimeTracker;
}

export function createBridge(options: {
	adapter: BridgeAdapter;
	colonyOrigin: string | undefined;
	/** The escrow, told what becomes of deposit events; PF2e worlds only make them. */
	deposits?: DepositEvents;
	fetchFunction?: Fetch;
	foundryGame: Pick<FoundryGame, "modules" | "system" | "time" | "user" | "version" | "world"> & {
		users: Pick<FoundryUsers, "activeGM">;
	};
	notifications: FoundryNotifications;
	/**
	 * Only this browser holds the link's token (a new pairing revokes the old link), so its tabs are the
	 * only sessions that can sync at once, and they share the token and the cursor (design §14.8).
	 */
	outboxLocks: OutboxLocks | undefined;
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
	let withdrawFailureShown = false;
	let withdrawRowFailureShown = false;
	let noOutboxLocksShown = false;
	// Every session's outbox syncs run one after another, so a sync still finishing for a stopped
	// session never applies the same rows beside the next session's (two "Colony" entries, a clock
	// moved twice).
	let outboxSyncs: Promise<void> = Promise.resolve();
	// The ended link's clean-up; a link saved meanwhile waits for it, so its token isn't forgotten.
	let forgettingEndedLink: Promise<void> = Promise.resolve();

	function notifyListeners() {
		for (const listener of listeners) {
			listener();
		}
	}

	function recordProblem(error: unknown) {
		lastProblem = error instanceof Error ? error.message : String(error);
		notifyListeners();
	}

	/**
	 * This world's token, or "" when the stored link belongs to another world on this Foundry server
	 * (client settings are shared by every world a browser opens there) or names no world (0.1.1).
	 */
	function linkToken(): string {
		return settings.linkedWorldId() === foundryGame.world.id ? settings.token() : "";
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
		if (!linkToken()) {
			return "not-linked";
		}
		if (!isActiveGm(foundryGame)) {
			return "another-gm-is-active";
		}
		return "running";
	}

	/**
	 * Once the link is gone no later withdraw row can reach this world, so every entry the bridge made
	 * is hidden now, best effort, after any sync still in flight and before the token is forgotten. It
	 * makes no request, and needsNewCode ignores every later refusal, so it runs once.
	 */
	function handleUnauthorized() {
		if (needsNewCode) {
			return;
		}
		needsNewCode = true;
		stopSession();
		notifications.error(
			"Colony Bridge: Colony Planner ended this world's link. Make a new code on Colony Planner's Settings and connect again.",
		);
		forgettingEndedLink = outboxSyncs
			.then(() => options.adapter.withdrawBridgedEntries())
			.catch((error: unknown) => {
				const reason = error instanceof Error ? error.message : String(error);
				notifications.error(
					`Colony Bridge: Foundry couldn't hide the journal entries this link made. Hide them by hand, or connect again to hide them. ${reason}`,
				);
			})
			.then(async () => {
				await settings.saveLink({ campaignName: "", token: "", worldId: "" });
			})
			.catch(recordProblem);
		outboxSyncs = forgettingEndedLink;
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

	/**
	 * Owed until it succeeds, so a failure or a reload holds back the new link's rows: syncing them
	 * first would leave entries hidden while unlinked in players' view for good.
	 */
	async function withdrawOwedEntries(signal: AbortSignal) {
		if (!settings.withdrawOwed()) {
			return;
		}
		try {
			await options.adapter.withdrawBridgedEntries();
		} catch (error) {
			const reason = error instanceof Error ? error.message : String(error);
			const problem = `Foundry couldn't hide the journal entries an earlier link made, so the bridge waits to sync until it can. ${reason}`;
			// Once per failing run, so a GM who never opens the menu still learns why nothing arrives.
			if (!withdrawFailureShown) {
				withdrawFailureShown = true;
				notifications.error(`Colony Bridge: ${problem}`);
			}
			throw new Error(problem);
		}
		withdrawFailureShown = false;
		// A link saved meanwhile owes its own withdrawal.
		signal.throwIfAborted();
		await settings.saveWithdrawOwed(false);
	}

	function startSession(colonyOrigin: string): Session {
		const abortController = new AbortController();
		const { signal } = abortController;
		const client = createBridgeClient({
			fetchFunction: options.fetchFunction,
			onUnauthorized: handleUnauthorized,
			origin: colonyOrigin,
			signal,
			token: linkToken(),
		});
		const linkId = settings.linkId();
		const queue = createEventQueue({
			maySend: (event) => options.deposits?.maySend(event, linkId) ?? true,
			onDropped: (reason, events, status) => {
				// A refused deposit gets its own warning once its coins are back.
				if (events.some((event) => event.kind !== "COIN_DEPOSIT")) {
					notifications.warn(
						`Colony Bridge: Colony Planner refused an update from Foundry. ${reason}`,
					);
				}
				options.deposits?.returnRefused(events, reason, status).catch(recordProblem);
			},
			onError: recordProblem,
			onSent: (events) => {
				options.deposits?.markSent(events).catch(recordProblem);
			},
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
				const locks = options.outboxLocks;
				if (!locks) {
					if (!noOutboxLocksShown) {
						noOutboxLocksShown = true;
						notifications.error(`Colony Bridge: ${noOutboxLocks}`);
					}
					recordProblem(noOutboxLocks);
					return Promise.resolve();
				}
				// Another tab's sync may hold the lock; the cursor is read only once it ends, so this tab
				// never fetches a row that tab applied.
				const sync = outboxSyncs.then(() =>
					locks.request(outboxLockName, async () => {
						await withdrawOwedEntries(signal);
						await syncOutbox({
							appliers: options.adapter.appliers,
							client,
							cursor: {
								load: settings.acknowledgedThrough,
								save: settings.saveAcknowledgedThrough,
							},
							signal,
						});
						withdrawRowFailureShown = false;
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
				// Once per failing run, as the connect-time withdrawal does.
				if (error instanceof WithdrawFailedError && !withdrawRowFailureShown) {
					withdrawRowFailureShown = true;
					notifications.error(`Colony Bridge: ${error.message}`);
				}
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
		options.deposits?.resume(queue, linkId).catch(recordProblem);
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
			await forgettingEndedLink;
			await settings.saveLink({ campaignName: "", token: "", worldId: "" });
			evaluate();
		},
		linkId: () => (linkToken() ? settings.linkId() : ""),
		observeWorldTime(worldTime, updateOptions) {
			const madeByBridge =
				typeof updateOptions === "object" &&
				updateOptions !== null &&
				"colonyBridge" in updateOptions &&
				updateOptions.colonyBridge === true;
			session?.tracker.observe(worldTime, madeByBridge);
		},
		queueEvent(event) {
			if (!session) {
				return false;
			}
			session.queue.add(event);
			return true;
		},
		async saveLink(link) {
			stopSession();
			pairing = false;
			needsNewCode = false;
			lastProblem = undefined;
			await forgettingEndedLink;
			await settings.saveLink({ ...link, worldId: foundryGame.world.id });
			evaluate();
		},
		status: () => ({
			campaignName: linkToken() ? settings.campaignName() : "",
			condition: condition(),
			connection,
			lastProblem,
			linked: linkToken() !== "",
		}),
		subscribe(listener) {
			listeners.add(listener);
			return () => listeners.delete(listener);
		},
	};
}

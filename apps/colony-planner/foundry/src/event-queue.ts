import { backoffDelay } from "./backoff";
import { BridgeHttpError, UnauthorizedError } from "./bridge-client";
import type { Schedule } from "./schedule";
import {
	type InboundEvent,
	type InboundEventKind,
	maximumEventsPerBatch,
	type StatusPayload,
	type TimeAdvancedPayload,
} from "./wire";

export interface EventQueue {
	enqueue(kind: "STATUS", payload: StatusPayload): void;
	enqueue(kind: "TIME_ADVANCED", payload: TimeAdvancedPayload): void;
	flush(): void;
	/** Sends now, cancelling a waiting retry: for when the connection has just come back. */
	retryNow(): void;
	/** Ends this queue for good; anything it still holds stays in storage for the next one. */
	stop(): void;
}

export interface EventQueueOptions {
	createId?: () => string;
	now?: () => Date;
	/** A batch Colony Planner refused for good (a 4xx other than 401, 408 or 429), dropped from the queue. */
	onDropped: (reason: string) => void;
	onError: (error: unknown) => void;
	schedule: Schedule;
	send: (events: InboundEvent[]) => Promise<void>;
	storage: { load(): InboundEvent[]; save(events: InboundEvent[]): Promise<unknown> };
}

/**
 * Inbound events (design §5), kept in this browser's storage until Colony Planner accepts them, so a
 * reload or an outage loses none. Each carries its own UUID, so a batch sent twice (an answer lost on
 * the way back) is counted as duplicates, not applied twice.
 */
export function createEventQueue(options: EventQueueOptions): EventQueue {
	const createId = options.createId ?? (() => crypto.randomUUID());
	const now = options.now ?? (() => new Date());
	let events = options.storage.load();
	let sending = false;
	let stopped = false;
	let failedAttempts = 0;
	let cancelRetry: (() => void) | undefined;

	function persist() {
		if (stopped) {
			return;
		}
		options.storage.save(events).catch(options.onError);
	}

	function removeSent(batch: InboundEvent[]) {
		const sentIds = new Set(batch.map((event) => event.id));
		events = events.filter((event) => !sentIds.has(event.id));
		persist();
	}

	/** Sends the oldest batch; true when it left the queue, so the next one may go. */
	async function sendNextBatch(): Promise<boolean> {
		const batch = events.slice(0, maximumEventsPerBatch);
		try {
			await options.send(batch);
			failedAttempts = 0;
			removeSent(batch);
			return true;
		} catch (error) {
			if (error instanceof UnauthorizedError) {
				stopped = true;
				return false;
			}
			if (!isPermanentRefusal(error)) {
				scheduleRetry();
				return false;
			}
			removeSent(batch);
			options.onDropped(error.message);
			return true;
		}
	}

	function scheduleRetry() {
		if (stopped) {
			return;
		}
		const delay = backoffDelay(failedAttempts);
		failedAttempts += 1;
		cancelRetry = options.schedule(() => {
			cancelRetry = undefined;
			flush();
		}, delay);
	}

	function flush() {
		if (stopped || sending || cancelRetry || events.length === 0) {
			return;
		}
		sending = true;
		sendNextBatch().then(
			(sendMore) => {
				sending = false;
				if (sendMore) {
					flush();
				}
			},
			(error: unknown) => {
				sending = false;
				options.onError(error);
			},
		);
	}

	return {
		enqueue(kind: InboundEventKind, payload: StatusPayload | TimeAdvancedPayload) {
			events = [...events, { id: createId(), kind, occurredAt: now().toISOString(), payload }];
			persist();
			flush();
		},
		flush,
		retryNow() {
			cancelRetry?.();
			cancelRetry = undefined;
			failedAttempts = 0;
			flush();
		},
		stop() {
			stopped = true;
			cancelRetry?.();
			cancelRetry = undefined;
		},
	};
}

function isPermanentRefusal(error: unknown): error is BridgeHttpError {
	return (
		error instanceof BridgeHttpError &&
		error.status >= 400 &&
		error.status < 500 &&
		error.status !== 408 &&
		error.status !== 429
	);
}

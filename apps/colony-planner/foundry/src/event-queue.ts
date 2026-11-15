import { backoffDelay } from "./backoff";
import { BridgeHttpError, UnauthorizedError } from "./bridge-client";
import type { Schedule } from "./schedule";
import {
	type InboundEvent,
	type InboundEventKind,
	type InboundPayloads,
	maximumEventsPerBatch,
} from "./wire";

export interface EventQueue {
	/** Queues an event made elsewhere (a deposit's, kept in its ledger), unless the queue holds it. */
	add(event: InboundEvent): void;
	enqueue<Kind extends InboundEventKind>(kind: Kind, payload: InboundPayloads[Kind]): void;
	flush(): void;
	holds(eventId: string): boolean;
	/** Sends now, cancelling a waiting retry: for when the connection has just come back. */
	retryNow(): void;
	/** Ends this queue for good; anything it still holds stays in storage for the next one. */
	stop(): void;
}

export interface EventQueueOptions {
	createId?: () => string;
	now?: () => Date;
	/** False for an event that may no longer leave (a deposit its ledger stopped holding): dropped unsent. */
	maySend?: (event: InboundEvent) => boolean;
	/** A batch Colony Planner refused for good (a 4xx other than 401, 408 or 429), dropped from the queue. */
	onDropped: (reason: string, events: InboundEvent[], status: number) => void;
	onError: (error: unknown) => void;
	/** A batch Colony Planner answered with a 2xx: recorded, or already recorded. */
	onSent?: (events: InboundEvent[]) => void;
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

	function remove(batch: InboundEvent[]) {
		const removedIds = new Set(batch.map((event) => event.id));
		events = events.filter((event) => !removedIds.has(event.id));
		persist();
	}

	/** Sends the oldest batch; true when it left the queue, so the next one may go. */
	async function sendNextBatch(): Promise<boolean> {
		const batch = nextBatch(events);
		const unsendable = batch.filter((event) => options.maySend?.(event) === false);
		if (unsendable.length > 0) {
			remove(unsendable);
			return true;
		}
		try {
			await options.send(batch);
			failedAttempts = 0;
			remove(batch);
			options.onSent?.(batch);
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
			remove(batch);
			options.onDropped(error.message, batch, error.status);
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

	function add(event: InboundEvent) {
		if (events.some((queued) => queued.id === event.id)) {
			return;
		}
		events = [...events, event];
		persist();
		flush();
	}

	return {
		add,
		enqueue(kind, payload) {
			add({ id: createId(), kind, occurredAt: now().toISOString(), payload });
		},
		flush,
		holds: (eventId) => events.some((event) => event.id === eventId),
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

const loneKinds: ReadonlySet<InboundEventKind> = new Set([
	"COIN_DEPOSIT",
	"NPC_IMPORT",
	"ITEM_IMPORT",
]);

/**
 * A deposit or an import goes alone, so a refusal never takes another event with it, and a refused
 * deposit is known never to have been stored (design §14.11). Other events go up to 100 at a time.
 */
function nextBatch(events: InboundEvent[]): InboundEvent[] {
	const [first] = events;
	if (first && loneKinds.has(first.kind)) {
		return [first];
	}
	const loneIndex = events.findIndex((event) => loneKinds.has(event.kind));
	const end = loneIndex === -1 ? events.length : loneIndex;
	return events.slice(0, Math.min(end, maximumEventsPerBatch));
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

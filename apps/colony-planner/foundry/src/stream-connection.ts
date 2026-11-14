import { backoffDelay } from "./backoff";
import { UnauthorizedError } from "./bridge-client";
import type { Schedule } from "./schedule";
import { createSseParser } from "./sse";

export type ConnectionState = "connecting" | "live" | "reconnecting" | "stopped";

export interface StreamConnection {
	/** Drops the current stream and connects again after the usual backoff. */
	reconnect(): void;
	stop(): void;
}

// The server sends `: ping` every 20 seconds, so three missed pings mean the connection died quietly
// (a laptop waking from sleep often leaves a fetch waiting forever, before or after its headers).
const defaultIdleTimeoutInMilliseconds = 60_000;
// A stream that ends sooner than this counts as a failure, so a server that answers and hangs up at
// once is retried with backoff instead of every second.
const healthyStreamInMilliseconds = 30_000;

/**
 * Holds GET /api/bridge/stream open (design §4.2), read with fetch because EventSource can't send the
 * Authorization header. Reconnects with backoff from 1 s to 60 s; a stream the server ended cleanly
 * after a healthy while (it rotates them about every 10 minutes) resets the backoff. A refused token
 * stops it for good.
 */
export function startStreamConnection(options: {
	idleTimeoutInMilliseconds?: number;
	now?: () => number;
	onConnected: () => void;
	onOutbox: () => void;
	onStateChange: (state: ConnectionState) => void;
	open: (signal: AbortSignal) => Promise<ReadableStream<Uint8Array>>;
	schedule: Schedule;
}): StreamConnection {
	const idleTimeout = options.idleTimeoutInMilliseconds ?? defaultIdleTimeoutInMilliseconds;
	const now = options.now ?? Date.now;
	let stopped = false;
	let failedAttempts = 0;
	let currentConnection: AbortController | undefined;
	let cancelReconnect: (() => void) | undefined;

	/** Resolves true when the stream ended cleanly after a healthy while. */
	async function readStream(connection: AbortController): Promise<boolean> {
		let cancelIdleTimer = options.schedule(() => connection.abort(), idleTimeout);
		try {
			const body = await options.open(connection.signal);
			// A stop or reconnect while the answer was on its way already ended this connection.
			if (stopped || connection.signal.aborted) {
				body.cancel().catch(() => undefined);
				return false;
			}
			const openedAt = now();
			options.onStateChange("live");
			options.onConnected();
			const parser = createSseParser((event) => {
				if (event.type === "outbox") {
					options.onOutbox();
				}
			});
			const decoder = new TextDecoder();
			await body.pipeTo(
				new WritableStream({
					write(chunk) {
						cancelIdleTimer();
						cancelIdleTimer = options.schedule(() => connection.abort(), idleTimeout);
						parser.push(decoder.decode(chunk, { stream: true }));
					},
				}),
				{ signal: connection.signal },
			);
			return now() - openedAt >= healthyStreamInMilliseconds;
		} finally {
			cancelIdleTimer();
		}
	}

	function connect() {
		const connection = new AbortController();
		currentConnection = connection;
		readStream(connection).then(
			(healthy) => afterDisconnect(healthy ? "ended" : "failed"),
			(error: unknown) =>
				afterDisconnect(error instanceof UnauthorizedError ? "refused" : "failed"),
		);
	}

	function afterDisconnect(outcome: "ended" | "failed" | "refused") {
		currentConnection = undefined;
		if (stopped) {
			return;
		}
		if (outcome === "refused") {
			stopped = true;
			options.onStateChange("stopped");
			return;
		}
		failedAttempts = outcome === "ended" ? 0 : failedAttempts + 1;
		options.onStateChange("reconnecting");
		cancelReconnect = options.schedule(
			() => {
				cancelReconnect = undefined;
				connect();
			},
			backoffDelay(Math.max(0, failedAttempts - 1)),
		);
	}

	options.onStateChange("connecting");
	connect();

	return {
		reconnect() {
			currentConnection?.abort();
		},
		stop() {
			if (stopped) {
				return;
			}
			stopped = true;
			cancelReconnect?.();
			currentConnection?.abort();
			options.onStateChange("stopped");
		},
	};
}
